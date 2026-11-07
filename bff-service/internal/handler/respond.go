package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"bff-service/internal/middleware"
	"bff-service/internal/model"
	"bff-service/internal/service"
)

// maxRequestBytes limits the size of a request body.
const maxRequestBytes = 1 << 20

// arrayIndex matches the ".0" index segments of encoding/json field paths.
var arrayIndex = regexp.MustCompile(`\.(\d+)`)

// fieldPath turns "items.0.quantity" into "items[0].quantity", the notation used by the
// downstream services, so clients see one naming style in validation details.
func fieldPath(jsonPath string) string {
	return arrayIndex.ReplaceAllString(jsonPath, "[$1]")
}

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
				Field:   fieldPath(typeErr.Field),
				Message: "must be " + jsonTypeName(typeErr.Type.Kind().String()),
			})
		}
		return validationError("Malformed JSON request body")
	}
	return nil
}

// jsonTypeName describes the expected JSON type of a Go field kind.
func jsonTypeName(goKind string) string {
	switch goKind {
	case "string":
		return "a JSON string"
	case "bool":
		return "a JSON boolean"
	case "struct", "map":
		return "a JSON object"
	case "slice", "array":
		return "a JSON array"
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "an integer"
	default:
		return "a JSON number"
	}
}
