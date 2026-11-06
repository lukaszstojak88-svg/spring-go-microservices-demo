package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"bff-service/internal/model"
)

// Recover turns a panic into a 500 INTERNAL_ERROR response. The panic value and stack trace are
// logged only; the client gets the standard error body without internal details.
//
// It is the outermost middleware, so the request ID is read from the response header set by
// the RequestID middleware further down the chain.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &responseRecorder{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if v == http.ErrAbortHandler { // sentinel value, compared by identity
					panic(v)
				}
				requestID := w.Header().Get(HeaderRequestID)
				logger.ErrorContext(WithRequestID(r.Context(), requestID), "panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", fmt.Sprint(v),
					"stack", string(debug.Stack()))
				if rec.wroteHeader {
					return // headers already sent; nothing sensible can be written
				}
				body := model.ErrorResponse{
					Timestamp: time.Now().UTC().Truncate(time.Millisecond),
					Status:    http.StatusInternalServerError,
					Code:      model.CodeInternalError,
					Message:   "An unexpected error occurred",
					Path:      r.URL.Path,
					RequestID: requestID,
					Details:   []model.FieldDetail{},
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(body)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
