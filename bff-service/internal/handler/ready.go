package handler

import (
	"net/http"

	"bff-service/internal/service"
)

// ready returns 200 when every downstream service is ready, otherwise 503 with per-dependency
// statuses (no error details are exposed).
func ready(readiness *service.Readiness) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := readiness.Check(r.Context())
		status := http.StatusOK
		if result.Status != service.StatusUp {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, result)
	}
}
