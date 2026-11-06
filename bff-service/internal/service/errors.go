// Package service holds the BFF's aggregation and transformation logic. It contains no domain
// rules: validation, prices and availability belong to the downstream services.
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

// mapDownstreamError translates a downstream failure into a client-facing *Error. Downstream
// bodies are never forwarded verbatim: only the status, a known code, the message of a known
// validation/not-found error and remapped field details survive.
func mapDownstreamError(ctx context.Context, logger *slog.Logger, err error, remap func(string) string) *Error {
	var ce *client.Error
	if !errors.As(err, &ce) {
		logger.ErrorContext(ctx, "unexpected downstream error", "error", err)
		return &Error{Status: http.StatusInternalServerError, Code: model.CodeInternalError,
			Message: "An unexpected error occurred"}
	}

	switch ce.Kind {
	case client.KindStatus:
		switch ce.Status {
		case http.StatusBadRequest:
			return &Error{
				Status:  http.StatusBadRequest,
				Code:    model.CodeValidationError,
				Message: orDefault(ce.Message, "Request validation failed"),
				Details: remapDetails(ce.Details, remap),
			}
		case http.StatusNotFound:
			return &Error{Status: http.StatusNotFound, Code: model.CodeNotFound,
				Message: orDefault(ce.Message, "Resource not found")}
		default:
			// The BFF sends well-formed requests; any other 4xx is a contract mismatch.
			logger.ErrorContext(ctx, "downstream rejected the request (contract error)",
				"service", ce.Service, "status", ce.Status, "code", ce.Code)
			return unavailable(ce.Service)
		}
	case client.KindTimeout:
		logger.WarnContext(ctx, "downstream timed out", "service", ce.Service, "error", ce.Err)
		return &Error{Status: http.StatusGatewayTimeout, Code: model.CodeServiceUnavailable,
			Message: serviceLabel(ce.Service) + " did not respond in time"}
	case client.KindCanceled:
		logger.InfoContext(ctx, "request canceled by the client", "service", ce.Service)
		return unavailable(ce.Service)
	case client.KindInvalidResponse:
		logger.ErrorContext(ctx, "downstream returned an invalid response (contract error)",
			"service", ce.Service, "status", ce.Status, "error", ce.Err)
		return unavailable(ce.Service)
	default:
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

func remapDetails(details []model.FieldDetail, remap func(string) string) []model.FieldDetail {
	out := make([]model.FieldDetail, 0, len(details))
	for _, d := range details {
		field := d.Field
		if remap != nil {
			field = remap(field)
		}
		out = append(out, model.FieldDetail{Field: field, Message: d.Message})
	}
	return out
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
