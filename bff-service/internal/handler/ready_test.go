package handler

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"bff-service/internal/client/order"
	"bff-service/internal/client/product"
	"bff-service/internal/service"
)

func probe(status int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/actuator/health/readiness" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, `{"status":"UP"}`)
	}))
}

func readyRouter(productURL, orderURL string) http.Handler {
	logger := slog.New(slog.DiscardHandler)
	p := product.New(productURL, time.Second)
	o := order.New(orderURL, time.Second)
	return NewRouter(Deps{
		Products: service.NewProducts(p, logger),
		Readiness: service.NewReadiness(logger,
			service.ReadinessCheck{Name: "productService", Check: p.Ready},
			service.ReadinessCheck{Name: "orderService", Check: o.Ready}),
		Logger: logger,
	})
}

func TestReady(t *testing.T) {
	up := probe(http.StatusOK)
	defer up.Close()
	down := probe(http.StatusServiceUnavailable)
	defer down.Close()

	tests := []struct {
		name              string
		productURL        string
		orderURL          string
		wantStatus        int
		wantProd, wantOrd string
		wantOverall       string
	}{
		{"both ready", up.URL, up.URL, 200, "UP", "UP", "UP"},
		{"order not ready", up.URL, down.URL, 503, "UP", "DOWN", "DOWN"},
		{"product unreachable", closedURL(t), up.URL, 503, "DOWN", "UP", "DOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()

			readyRouter(tt.productURL, tt.orderURL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			body := decode(t, rec)
			checks := body["checks"].(map[string]any)
			if body["status"] != tt.wantOverall || checks["productService"] != tt.wantProd || checks["orderService"] != tt.wantOrd {
				t.Errorf("body = %v", body)
			}
		})
	}
}

func TestHealth_DoesNotCallDownstream(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer s.Close()
	rec := httptest.NewRecorder()

	readyRouter(s.URL, s.URL).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK || calls != 0 || rec.Header().Get("X-Request-Id") == "" {
		t.Errorf("status = %d, downstream calls = %d, request id = %q", rec.Code, calls, rec.Header().Get("X-Request-Id"))
	}
}
