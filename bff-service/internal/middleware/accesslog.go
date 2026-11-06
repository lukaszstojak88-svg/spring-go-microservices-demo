package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// AccessLog writes one structured log line per request (method, path, status, duration, size;
// the request ID is added by ContextHandler). Probe endpoints are logged at debug level.
// The line is written in a defer, so a panicking request is logged too (with status 500).
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &responseRecorder{ResponseWriter: w}
			completed := false
			defer func() {
				status := rec.status
				switch {
				case !completed:
					status = http.StatusInternalServerError // panic; Recover writes the response
				case status == 0:
					status = http.StatusOK // handler wrote nothing: implicit 200
				}
				level := slog.LevelInfo
				if r.URL.Path == "/health" || r.URL.Path == "/ready" {
					level = slog.LevelDebug
				}
				logger.Log(r.Context(), level, "http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", status,
					"durationMs", time.Since(start).Milliseconds(),
					"bytes", rec.bytes)
			}()
			next.ServeHTTP(rec, r)
			completed = true
		})
	}
}
