package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"bff-service/internal/middleware"
	"bff-service/internal/model"
	"bff-service/internal/service"
)

// maxRequestBytes limits the size of a request body.
const maxRequestBytes = 1 << 20

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError writes the standard error body. A *service.Error is written as is; anything else
// is logged and reported as a generic 500 without internal details.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var se *service.Error
	if !errors.As(err, &se) {
		logger.ErrorContext(r.Context(), "unhandled error", "error", err)
		se = &service.Error{Status: http.StatusInternalServerError, Code: model.CodeInternalError,
			Message: "An unexpected error occurred"}
	}
	details := se.Details
	if details == nil {
		details = []model.FieldDetail{}
	}
	writeJSON(w, se.Status, model.ErrorResponse{
		Timestamp: time.Now().UTC().Truncate(time.Millisecond),
		Status:    se.Status,
		Code:      se.Code,
		Message:   se.Message,
		Path:      r.URL.Path,
		RequestID: middleware.RequestIDFrom(r.Context()),
		Details:   details,
	})
}

func validationError(message string, details ...model.FieldDetail) *service.Error {
	return &service.Error{Status: http.StatusBadRequest, Code: model.CodeValidationError,
		Message: message, Details: details}
}

// decodeJSONObject checks the basic structure of a request body: a single JSON object (no
// trailing data) whose fields have the expected JSON types. Business validation (required
// fields, ranges, formats) is the downstream service's job.
func decodeJSONObject(w http.ResponseWriter, r *http.Request, dst any) *service.Error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	dec := json.NewDecoder(r.Body)

	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return validationError("Request body is required")
		case errors.As(err, &tooLarge):
			return validationError(fmt.Sprintf("Request body must not exceed %d bytes", maxRequestBytes))
		default:
			return validationError("Malformed JSON request body")
		}
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return validationError("Malformed JSON request body: unexpected data after the JSON object")
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		return validationError("Request body must be a JSON object")
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return validationError("Request validation failed", model.FieldDetail{
				Field:   typeErr.Field,
				Message: "must be a JSON " + jsonTypeName(typeErr.Type.Kind().String()),
			})
		}
		return validationError("Malformed JSON request body")
	}
	return nil
}

func jsonTypeName(goKind string) string {
	switch goKind {
	case "string":
		return "string"
	case "bool":
		return "boolean"
	case "struct", "map":
		return "object"
	case "slice", "array":
		return "array"
	default:
		return "number"
	}
}
