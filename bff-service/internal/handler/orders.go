package handler

import (
	"log/slog"
	"net/http"
	"net/url"

	"bff-service/internal/model"
	"bff-service/internal/service"
)

type orderHandler struct {
	orders *service.Orders
	logger *slog.Logger
}

func (h *orderHandler) create(w http.ResponseWriter, r *http.Request) {
	var body model.OrderCreate
	if err := decodeJSONObject(w, r, &body); err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	o, err := h.orders.Create(r.Context(), body)
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	w.Header().Set("Location", "/api/orders/"+url.PathEscape(o.ID))
	writeJSON(w, http.StatusCreated, o)
}

func (h *orderHandler) list(w http.ResponseWriter, r *http.Request) {
	page, err := h.orders.List(r.Context(), r.URL.Query())
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *orderHandler) get(w http.ResponseWriter, r *http.Request) {
	o, err := h.orders.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (h *orderHandler) cancel(w http.ResponseWriter, r *http.Request) {
	o, err := h.orders.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}
