package handler

import (
	"log/slog"
	"net/http"

	"bff-service/internal/middleware"
	"bff-service/internal/model"
	"bff-service/internal/service"
)

// Deps are the services the HTTP layer needs.
type Deps struct {
	Products  *service.Products
	Readiness *service.Readiness
	Logger    *slog.Logger
}

// NewRouter registers all routes on a method-aware ServeMux and wraps it in the middleware
// chain recover → request ID → access log.
func NewRouter(d Deps) http.Handler {
	products := &productHandler{products: d.Products, logger: d.Logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", Health)
	mux.HandleFunc("GET /ready", ready(d.Readiness))
	mux.HandleFunc("GET /api/products", products.list)
	mux.HandleFunc("POST /api/products", products.create)
	mux.HandleFunc("GET /api/products/{id}", products.get)
	mux.HandleFunc("PUT /api/products/{id}", products.update)
	mux.HandleFunc("DELETE /api/products/{id}", products.delete)

	return middleware.Chain(jsonFallback(mux, d.Logger),
		middleware.Recover(d.Logger),
		middleware.RequestID,
		middleware.AccessLog(d.Logger),
	)
}

// jsonFallback keeps ServeMux routing but replaces its plain-text 404/405 responses with the
// standard JSON error body (the Allow header of a 405 is preserved).
func jsonFallback(mux *http.ServeMux, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		probe := &statusProbe{header: http.Header{}}
		mux.ServeHTTP(probe, r)
		switch probe.status {
		case http.StatusNotFound:
			writeError(w, r, logger, &service.Error{Status: http.StatusNotFound,
				Code: model.CodeNotFound, Message: "Resource not found"})
		case http.StatusMethodNotAllowed:
			w.Header().Set("Allow", probe.header.Get("Allow"))
			writeError(w, r, logger, &service.Error{Status: http.StatusMethodNotAllowed,
				Code: model.CodeMethodNotAllowed, Message: "Method " + r.Method + " is not supported for this resource"})
		default:
			// e.g. a redirect to the cleaned path: replay the mux response as is.
			for k, v := range probe.header {
				w.Header()[k] = v
			}
			w.WriteHeader(probe.status)
		}
	})
}

// statusProbe captures the status and headers of a ServeMux fallback response.
type statusProbe struct {
	header http.Header
	status int
}

func (p *statusProbe) Header() http.Header { return p.header }

func (p *statusProbe) Write(b []byte) (int, error) {
	if p.status == 0 {
		p.status = http.StatusOK
	}
	return len(b), nil
}

func (p *statusProbe) WriteHeader(code int) {
	if p.status == 0 {
		p.status = code
	}
}
