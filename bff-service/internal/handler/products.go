package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"regexp"

	"bff-service/internal/model"
	"bff-service/internal/service"
)

// decimalShape is a shape check only (digits with an optional fraction). Sign, scale and ranges
// are validated by the Product Service; the BFF never parses amounts into numbers.
var decimalShape = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

type productHandler struct {
	products *service.Products
	logger   *slog.Logger
}

func (h *productHandler) list(w http.ResponseWriter, r *http.Request) {
	page, err := h.products.List(r.Context(), r.URL.Query())
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *productHandler) get(w http.ResponseWriter, r *http.Request) {
	p, err := h.products.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *productHandler) create(w http.ResponseWriter, r *http.Request) {
	body, ok := h.readBody(w, r)
	if !ok {
		return
	}
	p, err := h.products.Create(r.Context(), body)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	// The Location points to the BFF resource, never to the downstream service.
	w.Header().Set("Location", "/api/products/"+url.PathEscape(p.ID))
	writeJSON(w, http.StatusCreated, p)
}

func (h *productHandler) update(w http.ResponseWriter, r *http.Request) {
	body, ok := h.readBody(w, r)
	if !ok {
		return
	}
	p, err := h.products.Update(r.Context(), r.PathValue("id"), body)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *productHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.products.Delete(r.Context(), r.PathValue("id")); err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// readBody decodes and structurally checks a create/update body; on failure it writes the
// 400 response and returns false (the Product Service is not called).
func (h *productHandler) readBody(w http.ResponseWriter, r *http.Request) (model.ProductWrite, bool) {
	var body model.ProductWrite
	if err := decodeJSONObject(w, r, &body); err != nil {
		writeError(w, r, h.logger, err)
		return model.ProductWrite{}, false
	}
	if body.Price != nil && body.Price.Amount != nil && !decimalShape.MatchString(*body.Price.Amount) {
		writeError(w, r, h.logger, validationError("Request validation failed", model.FieldDetail{
			Field:   "price.amount",
			Message: `must be a decimal string, e.g. "349.99"`,
		}))
		return model.ProductWrite{}, false
	}
	return body, true
}
