// Package handler contains the HTTP handlers of the BFF service.
package handler

import (
	"encoding/json"
	"net/http"
)

// Health is the liveness endpoint. It never calls downstream services.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP"})
}
