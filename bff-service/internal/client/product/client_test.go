package product

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"bff-service/internal/client"
	"bff-service/internal/middleware"
	"bff-service/internal/model"
)

const keyboardJSON = `{"id":"p1","name":"Mechanical Keyboard","description":"Compact","price":"349.99",
"currency":"PLN","available":true,"createdAt":"2026-01-15T10:30:00Z","updatedAt":"2026-01-15T10:31:00.123Z"}`

// stub starts a fake Product Service and records the last request.
type stub struct {
	server   *httptest.Server
	calls    atomic.Int32
	method   string
	path     string
	rawQuery string
	body     string
	header   http.Header
}

func newStub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *stub {
	t.Helper()
	s := &stub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		s.method, s.path, s.rawQuery, s.body, s.header = r.Method, r.URL.EscapedPath(), r.URL.RawQuery, string(b), r.Header.Clone()
		handler(w, r)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func jsonResponse(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func ctxWithID(t *testing.T) context.Context {
	return middleware.WithRequestID(t.Context(), "req-abc")
}

func wantKind(t *testing.T, err error, kind client.Kind) *client.Error {
	t.Helper()
	var ce *client.Error
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v (%T), want *client.Error", err, err)
	}
	if ce.Kind != kind {
		t.Fatalf("kind = %v, want %v (err: %v)", ce.Kind, kind, err)
	}
	return ce
}

func ptr[T any](v T) *T { return &v }

func TestGet_ReturnsProductAndForwardsRequestID(t *testing.T) {
	s := newStub(t, jsonResponse(200, keyboardJSON))
	c := New(s.server.URL, time.Second)

	p, err := c.Get(ctxWithID(t), "p1")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID != "p1" || p.Price != "349.99" || p.Currency != "PLN" || !p.Available ||
		*p.Description != "Compact" || p.UpdatedAt != "2026-01-15T10:31:00.123Z" {
		t.Errorf("unexpected product: %+v", p)
	}
	if s.method != http.MethodGet || s.path != "/api/products/p1" {
		t.Errorf("request = %s %s", s.method, s.path)
	}
	if s.header.Get("X-Request-Id") != "req-abc" || s.header.Get("Accept") != "application/json" {
		t.Errorf("headers = %v", s.header)
	}
}

func TestGet_EscapesIDInPath(t *testing.T) {
	s := newStub(t, jsonResponse(200, keyboardJSON))

	_, _ = New(s.server.URL, time.Second).Get(t.Context(), "../admin?x=1")

	if s.path != "/api/products/..%2Fadmin%3Fx=1" || s.rawQuery != "" {
		t.Errorf("path = %q, query = %q; the id must stay one escaped segment", s.path, s.rawQuery)
	}
}

func TestList_ForwardsQueryAndDecodesPage(t *testing.T) {
	s := newStub(t, jsonResponse(200, `{"content":[`+keyboardJSON+`],"page":1,"size":5,"totalElements":6,"totalPages":2}`))

	page, err := New(s.server.URL, time.Second).List(t.Context(), url.Values{"page": {"1"}, "size": {"5"}})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Content) != 1 || page.Page != 1 || page.Size != 5 || page.TotalElements != 6 || page.TotalPages != 2 {
		t.Errorf("unexpected page: %+v", page)
	}
	if s.path != "/api/products" || s.rawQuery != "page=1&size=5" {
		t.Errorf("request = %s?%s", s.path, s.rawQuery)
	}
}

func TestCreate_SendsJSONOnceAndDecodes(t *testing.T) {
	s := newStub(t, jsonResponse(201, keyboardJSON))

	p, err := New(s.server.URL, time.Second).Create(ctxWithID(t), model.ServiceProductWrite{
		Name: ptr("Mechanical Keyboard"), Price: ptr("349.99"), Currency: ptr("PLN"), Available: ptr(true),
	})

	if err != nil || p.ID != "p1" {
		t.Fatalf("p = %+v, err = %v", p, err)
	}
	want := `{"name":"Mechanical Keyboard","description":null,"price":"349.99","currency":"PLN","available":true}`
	if s.method != http.MethodPost || s.path != "/api/products" || s.body != want {
		t.Errorf("request = %s %s %s", s.method, s.path, s.body)
	}
	if s.header.Get("Content-Type") != "application/json" || s.header.Get("X-Request-Id") != "req-abc" {
		t.Errorf("headers = %v", s.header)
	}
}

func TestUpdate_SendsPut(t *testing.T) {
	s := newStub(t, jsonResponse(200, keyboardJSON))

	_, err := New(s.server.URL, time.Second).Update(t.Context(), "p1", model.ServiceProductWrite{Name: ptr("X")})

	if err != nil || s.method != http.MethodPut || s.path != "/api/products/p1" || !strings.Contains(s.body, `"name":"X"`) {
		t.Errorf("err = %v, request = %s %s %s", err, s.method, s.path, s.body)
	}
}

func TestDelete_AcceptsNoContent(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	err := New(s.server.URL, time.Second).Delete(t.Context(), "p1")

	if err != nil || s.method != http.MethodDelete || s.path != "/api/products/p1" {
		t.Errorf("err = %v, request = %s %s", err, s.method, s.path)
	}
}

func TestClientError_ParsesDownstreamErrorBody(t *testing.T) {
	s := newStub(t, jsonResponse(400, `{"timestamp":"2026-01-15T10:30:00Z","status":400,"code":"VALIDATION_ERROR",
"message":"Request validation failed","path":"/api/products","requestId":"x",
"details":[{"field":"price","message":"must not be null"}]}`))

	_, err := New(s.server.URL, time.Second).Create(t.Context(), model.ServiceProductWrite{})

	ce := wantKind(t, err, client.KindStatus)
	if ce.Status != 400 || ce.Code != "VALIDATION_ERROR" || len(ce.Details) != 1 || ce.Details[0].Field != "price" {
		t.Errorf("unexpected error: %+v", ce)
	}
}

func TestClientError_NotFound(t *testing.T) {
	s := newStub(t, jsonResponse(404, `{"status":404,"code":"NOT_FOUND","message":"Product p9 not found"}`))

	_, err := New(s.server.URL, time.Second).Get(t.Context(), "p9")

	ce := wantKind(t, err, client.KindStatus)
	if ce.Status != 404 || ce.Code != "NOT_FOUND" || ce.Message != "Product p9 not found" {
		t.Errorf("unexpected error: %+v", ce)
	}
}

func TestClientError_WithoutJSONBody(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", http.StatusConflict) })

	_, err := New(s.server.URL, time.Second).Get(t.Context(), "p1")

	ce := wantKind(t, err, client.KindStatus)
	if ce.Status != 409 || ce.Code != "" {
		t.Errorf("unexpected error: %+v", ce)
	}
}

func TestServerError_IsUnavailableAndPostIsNotRetried(t *testing.T) {
	for _, status := range []int{500, 502, 503} {
		s := newStub(t, jsonResponse(status, `{"code":"INTERNAL_ERROR"}`))

		_, err := New(s.server.URL, time.Second).Create(t.Context(), model.ServiceProductWrite{Name: ptr("A")})

		ce := wantKind(t, err, client.KindUnavailable)
		if ce.Status != status {
			t.Errorf("status = %d, want %d", ce.Status, status)
		}
		if n := s.calls.Load(); n != 1 {
			t.Errorf("status %d: Product Service called %d times, want exactly 1 (no retry)", status, n)
		}
	}
}

func TestTimeout_IsClassifiedAsTimeout(t *testing.T) {
	release := make(chan struct{})
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	defer close(release)

	start := time.Now()
	_, err := New(s.server.URL, 100*time.Millisecond).Get(t.Context(), "p1")

	wantKind(t, err, client.KindTimeout)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("call took %v, want it bounded by the 100ms timeout", elapsed)
	}
}

func TestTimeout_WhileReadingBody(t *testing.T) {
	s := newStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"p1",`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})

	_, err := New(s.server.URL, 150*time.Millisecond).Get(t.Context(), "p1")

	wantKind(t, err, client.KindTimeout)
}

func TestConnectionRefused_IsUnavailable(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	baseURL := s.URL
	s.Close()

	_, err := New(baseURL, time.Second).Get(t.Context(), "p1")

	wantKind(t, err, client.KindUnavailable)
}

func TestContextCanceled_IsClassifiedAsCanceled(t *testing.T) {
	s := newStub(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(50*time.Millisecond, cancel)

	_, err := New(s.server.URL, 5*time.Second).Get(ctx, "p1")

	wantKind(t, err, client.KindCanceled)
}

func TestContextDeadline_IsClassifiedAsTimeout(t *testing.T) {
	s := newStub(t, func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := New(s.server.URL, 5*time.Second).Get(ctx, "p1")

	wantKind(t, err, client.KindTimeout)
}

func TestInvalidResponses(t *testing.T) {
	tests := map[string]func(http.ResponseWriter, *http.Request){
		"malformed json":  jsonResponse(200, `{not json`),
		"empty body":      func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) },
		"incomplete":      jsonResponse(200, `{"id":"p1","name":"A","currency":"PLN","available":true}`),
		"price as number": jsonResponse(200, `{"id":"p1","name":"A","price":1.5,"currency":"PLN","available":true}`),
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://evil.example/", http.StatusFound)
		},
	}
	for name, handler := range tests {
		t.Run(name, func(t *testing.T) {
			s := newStub(t, handler)

			_, err := New(s.server.URL, time.Second).Get(t.Context(), "p1")

			wantKind(t, err, client.KindInvalidResponse)
		})
	}
}

func TestList_RejectsPageWithoutContentOrIncompleteItems(t *testing.T) {
	for name, body := range map[string]string{
		"no content":      `{"page":0,"size":20,"totalElements":0,"totalPages":0}`,
		"incomplete item": `{"content":[{"id":"p1"}],"page":0,"size":20,"totalElements":1,"totalPages":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := newStub(t, jsonResponse(200, body))

			_, err := New(s.server.URL, time.Second).List(t.Context(), nil)

			wantKind(t, err, client.KindInvalidResponse)
		})
	}
}

func TestReady(t *testing.T) {
	up := newStub(t, jsonResponse(200, `{"status":"UP"}`))
	if err := New(up.server.URL, time.Second).Ready(t.Context()); err != nil || up.path != "/actuator/health/readiness" {
		t.Errorf("ready: err = %v, path = %s", err, up.path)
	}
	down := newStub(t, jsonResponse(503, `{"status":"DOWN"}`))
	wantKind(t, New(down.server.URL, time.Second).Ready(t.Context()), client.KindUnavailable)
}

func TestErrorText_IsForLogsOnly(t *testing.T) {
	s := newStub(t, jsonResponse(500, `{}`))

	_, err := New(s.server.URL, time.Second).Get(t.Context(), "p1")

	if !strings.Contains(err.Error(), "product-service") || !strings.Contains(err.Error(), "500") {
		t.Errorf("log text should name the service and status: %q", err.Error())
	}
}
