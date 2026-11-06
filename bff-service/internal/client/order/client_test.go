package order

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bff-service/internal/client"
	"bff-service/internal/middleware"
)

func TestReady_UpForwardsRequestID(t *testing.T) {
	var path, requestID string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, requestID = r.URL.Path, r.Header.Get("X-Request-Id")
		_, _ = w.Write([]byte(`{"status":"UP"}`))
	}))
	defer s.Close()

	err := New(s.URL, time.Second).Ready(middleware.WithRequestID(t.Context(), "req-1"))

	if err != nil || path != "/actuator/health/readiness" || requestID != "req-1" {
		t.Errorf("err = %v, path = %q, requestId = %q", err, path, requestID)
	}
}

func TestReady_DownAndUnreachable(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()

	for name, url := range map[string]string{"down": down.URL, "unreachable": closedURL} {
		t.Run(name, func(t *testing.T) {
			err := New(url, time.Second).Ready(t.Context())

			var ce *client.Error
			if !errors.As(err, &ce) || ce.Kind != client.KindUnavailable || ce.Service != "order-service" {
				t.Errorf("err = %v, want order-service unavailable", err)
			}
		})
	}
}
