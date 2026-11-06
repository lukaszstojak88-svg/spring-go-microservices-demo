// Package model contains the BFF's client-facing models and its own copies of the downstream
// service contracts (no code is shared with the Java services).
package model

import "time"

// Error codes used by the BFF (same vocabulary as the Java services).
const (
	CodeValidationError    = "VALIDATION_ERROR"
	CodeNotFound           = "NOT_FOUND"
	CodeMethodNotAllowed   = "METHOD_NOT_ALLOWED"
	CodeServiceUnavailable = "SERVICE_UNAVAILABLE"
	CodeInternalError      = "INTERNAL_ERROR"
)

// FieldDetail describes one invalid field of a request.
type FieldDetail struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ErrorResponse is the stable error body returned by every service.
type ErrorResponse struct {
	Timestamp time.Time     `json:"timestamp"`
	Status    int           `json:"status"`
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Path      string        `json:"path"`
	RequestID string        `json:"requestId"`
	Details   []FieldDetail `json:"details"`
}
