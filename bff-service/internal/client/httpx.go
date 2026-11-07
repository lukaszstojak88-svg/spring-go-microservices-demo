// Package client contains the HTTP plumbing shared by the downstream clients
// (internal/client/product, internal/client/order): explicit timeouts, context propagation,
// X-Request-Id forwarding, body handling and error classification. It performs exactly one
// attempt per call; there are no automatic retries.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"bff-service/internal/middleware"
	"bff-service/internal/model"
)

// maxResponseBytes limits how much of a downstream body is read.
const maxResponseBytes = 4 << 20

// Kind classifies a downstream failure.
type Kind int

// Failure kinds.
const (
	// KindUnavailable: connection refused/reset, DNS failure, or a 5xx status.
	KindUnavailable Kind = iota + 1
	// KindTimeout: the client timeout or the context deadline expired.
	KindTimeout
	// KindCanceled: the incoming request was canceled (the client went away).
	KindCanceled
	// KindStatus: a 4xx status; Code/Message/Details hold the parsed downstream error body.
	KindStatus
	// KindInvalidResponse: an unexpected status (1xx/3xx) or an unreadable/incomplete body.
	KindInvalidResponse
)

func (k Kind) String() string {
	switch k {
	case KindUnavailable:
		return "unavailable"
	case KindTimeout:
		return "timeout"
	case KindCanceled:
		return "canceled"
	case KindStatus:
		return "status"
	case KindInvalidResponse:
		return "invalid response"
	default:
		return "unknown"
	}
}

// Error is a classified downstream failure. Its text is for logs only and is never returned to
// API clients.
type Error struct {
	Service string
	Kind    Kind
	Status  int
	Code    string
	Message string
	Details []model.FieldDetail
	Err     error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("%s: %s", e.Service, e.Kind)
	if e.Status != 0 {
		msg += fmt.Sprintf(" (status %d)", e.Status)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// HTTP performs JSON calls against one downstream service.
type HTTP struct {
	service string
	baseURL string
	client  *http.Client
}

// New creates an HTTP helper for a service. timeout bounds the whole call (connect, headers and
// body); the incoming request context additionally cancels the call when the client goes away.
func New(service, baseURL string, timeout time.Duration) *HTTP {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 32
	return &HTTP{
		service: service,
		baseURL: baseURL,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			// Never follow redirects: a 3xx from an internal service is a contract error.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Do sends one request and, for a 2xx response, decodes the JSON body into out (if out is not
// nil). Non-2xx statuses and transport failures are returned as *Error.
func (h *HTTP) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	target := h.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: encode request: %w", h.service, err)
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", h.service, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if id := middleware.RequestIDFrom(ctx); id != "" {
		req.Header.Set(middleware.HeaderRequestID, id)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return h.transportError(ctx, err)
	}
	defer func() {
		// Drain a bounded remainder so the connection can be reused, then close.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()
	limited := io.LimitReader(resp.Body, maxResponseBytes)

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		if out == nil {
			return nil
		}
		if err := json.NewDecoder(limited).Decode(out); err != nil {
			return h.bodyError(ctx, err)
		}
		return nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return h.statusError(KindStatus, resp.StatusCode, limited)
	case resp.StatusCode >= 500:
		// The body is parsed too: e.g. the Order Service reports 503 PRODUCT_SERVICE_UNAVAILABLE.
		return h.statusError(KindUnavailable, resp.StatusCode, limited)
	default:
		return &Error{Service: h.service, Kind: KindInvalidResponse, Status: resp.StatusCode}
	}
}

// statusError builds an error for a 4xx/5xx status and, when the body is the standard error
// structure, keeps its code, message and details (for logs and selective mapping only).
func (h *HTTP) statusError(kind Kind, status int, body io.Reader) *Error {
	e := &Error{Service: h.service, Kind: kind, Status: status}
	var apiErr model.ErrorResponse
	if json.NewDecoder(body).Decode(&apiErr) == nil {
		e.Code, e.Message, e.Details = apiErr.Code, apiErr.Message, apiErr.Details
	}
	return e
}

// Invalid builds a KindInvalidResponse error, used by clients that validate decoded bodies.
func (h *HTTP) Invalid(reason string) *Error {
	return &Error{Service: h.service, Kind: KindInvalidResponse, Err: errors.New(reason)}
}

func (h *HTTP) transportError(ctx context.Context, err error) *Error {
	return &Error{Service: h.service, Kind: classify(ctx, err, KindUnavailable), Err: err}
}

// bodyError classifies a failure while reading a 2xx body: a timeout or cancellation during the
// read keeps its kind; anything else means the body is not what the contract promises.
func (h *HTTP) bodyError(ctx context.Context, err error) *Error {
	return &Error{Service: h.service, Kind: classify(ctx, err, KindInvalidResponse), Err: err}
}

func classify(ctx context.Context, err error, fallback Kind) Kind {
	if errors.Is(ctx.Err(), context.Canceled) {
		return KindCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return KindTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return KindTimeout
	}
	return fallback
}
