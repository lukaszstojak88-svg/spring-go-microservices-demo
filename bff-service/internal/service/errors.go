// Package service holds the BFF's aggregation and transformation logic. It contains no domain
// rules: validation, prices, totals, availability and statuses belong to the downstream services.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"bff-service/internal/client"
	"bff-service/internal/model"
)

// Error is a failure that the handler writes as the standard error body. Its fields are safe to
// show to API clients.
type Error struct {
	Status  int
	Code    string
	Message string
	Details []model.FieldDetail
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message)
}

// errorPolicy says which downstream errors are meaningful to the client for one service.
type errorPolicy struct {
	// conflict passes 409 CONFLICT through.
	conflict bool
	// ruleCodes are the 422 codes passed through (business rule violations).
	ruleCodes map[string]bool
	// passUnavailableCodes are 503 codes passed through with their own message.
	passUnavailableCodes map[string]string
	// timeoutMessage overrides the 504 message (e.g. to warn that a POST may have succeeded).
	timeoutMessage string
}

// mapDownstreamError translates a downstream failure into a client-facing *Error. Downstream
// bodies are never forwarded verbatim: only the status, a known code and the message and field
// details of a known client error survive. Anything unexpected is a contract error.
func mapDownstreamError(ctx context.Context, logger *slog.Logger, err error, p errorPolicy) *Error {
	var ce *client.Error
	if !errors.As(err, &ce) {
		logger.ErrorContext(ctx, "unexpected downstream error", "error", err)
		return &Error{Status: http.StatusInternalServerError, Code: model.CodeInternalError,
			Message: "An unexpected error occurred"}
	}

	switch ce.Kind {
	case client.KindStatus:
		switch {
		case ce.Status == http.StatusBadRequest:
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    model.CodeValidationError,
				Message: orDefault(ce.Message, "Request validation failed"),
				Details: copyDetails(ce.Details),
			}
		case ce.Status == http.StatusNotFound:
			return &Error{Status: http.StatusNotFound, Code: model.CodeNotFound,
				Message: orDefault(ce.Message, "Resource not found")}
		case ce.Status == http.StatusConflict && p.conflict:
			return &Error{Status: http.StatusConflict, Code: model.CodeConflict,
				Message: orDefault(ce.Message, "The request conflicts with the current state")}
		case ce.Status == http.StatusUnprocessableEntity && p.ruleCodes[ce.Code]:
			return &Error{
				Status:  http.StatusUnprocessableEntity,
				Code:    ce.Code,
				Message: orDefault(ce.Message, "The request violates a business rule"),
				Details: copyDetails(ce.Details),
			}
		default:
			// The BFF sends well-formed requests; any other 4xx is a contract mismatch.
			logger.ErrorContext(ctx, "downstream rejected the request (contract error)",
				"service", ce.Service, "status", ce.Status, "code", ce.Code)
			return unavailable(ce.Service)
		}
	case client.KindTimeout:
		logger.WarnContext(ctx, "downstream timed out", "service", ce.Service, "error", ce.Err)
		return &Error{Status: http.StatusGatewayTimeout, Code: model.CodeServiceUnavailable,
			Message: orDefault(p.timeoutMessage, serviceLabel(ce.Service)+" did not respond in time")}
	case client.KindCanceled:
		logger.InfoContext(ctx, "request canceled by the client", "service", ce.Service)
		return unavailable(ce.Service)
	case client.KindInvalidResponse:
		logger.ErrorContext(ctx, "downstream returned an invalid response (contract error)",
			"service", ce.Service, "status", ce.Status, "error", ce.Err)
		return unavailable(ce.Service)
	default:
		if msg, ok := p.passUnavailableCodes[ce.Code]; ok && ce.Status == http.StatusServiceUnavailable {
			logger.WarnContext(ctx, "downstream dependency unavailable", "service", ce.Service, "code", ce.Code)
			return &Error{Status: http.StatusServiceUnavailable, Code: ce.Code, Message: msg}
		}
		logger.WarnContext(ctx, "downstream unavailable",
			"service", ce.Service, "status", ce.Status, "error", ce.Err)
		return unavailable(ce.Service)
	}
}

func unavailable(service string) *Error {
	return &Error{Status: http.StatusServiceUnavailable, Code: model.CodeServiceUnavailable,
		Message: serviceLabel(service) + " is temporarily unavailable"}
}

// serviceLabel is a human-readable name; it never contains a hostname or port.
func serviceLabel(service string) string {
	switch service {
	case "product-service":
		return "Product Service"
	case "order-service":
		return "Order Service"
	default:
		return "A downstream service"
	}
}

// copyDetails keeps only field and message of each downstream detail (never nil). Field names
// are the same in the client contract and in the downstream services.
func copyDetails(details []model.FieldDetail) []model.FieldDetail {
	out := make([]model.FieldDetail, 0, len(details))
	for _, d := range details {
		out = append(out, model.FieldDetail{Field: d.Field, Message: d.Message})
	}
	return out
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
