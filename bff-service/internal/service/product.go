package service

import (
	"context"
	"log/slog"
	"net/url"

	"bff-service/internal/model"
)

// ProductAPI is the subset of the Product Service client used here (an interface for tests).
type ProductAPI interface {
	List(ctx context.Context, query url.Values) (model.ServiceProductPage, error)
	Get(ctx context.Context, id string) (model.ServiceProduct, error)
	Create(ctx context.Context, body model.ServiceProductWrite) (model.ServiceProduct, error)
	Update(ctx context.Context, id string, body model.ServiceProductWrite) (model.ServiceProduct, error)
	Delete(ctx context.Context, id string) error
}

// listParams are the only query parameters forwarded to the Product Service.
var listParams = []string{"page", "size", "sort"}

// Products forwards product operations to the Product Service and translates between the
// service contract (flat price + currency) and the client contract (price {amount, currency}).
// Values are moved, never recomputed: the meaning of the data does not change.
type Products struct {
	api    ProductAPI
	logger *slog.Logger
}

// NewProducts creates the product service.
func NewProducts(api ProductAPI, logger *slog.Logger) *Products {
	return &Products{api: api, logger: logger}
}

// List returns a page of products.
func (s *Products) List(ctx context.Context, query url.Values) (model.ProductPage, error) {
	page, err := s.api.List(ctx, pick(query, listParams))
	if err != nil {
		return model.ProductPage{}, s.mapError(ctx, err)
	}
	content := make([]model.Product, 0, len(page.Content))
	for _, p := range page.Content {
		content = append(content, ToClientProduct(p))
	}
	return model.ProductPage{
		Content:       content,
		Page:          page.Page,
		Size:          page.Size,
		TotalElements: page.TotalElements,
		TotalPages:    page.TotalPages,
	}, nil
}

// Get returns one product.
func (s *Products) Get(ctx context.Context, id string) (model.Product, error) {
	p, err := s.api.Get(ctx, id)
	if err != nil {
		return model.Product{}, s.mapError(ctx, err)
	}
	return ToClientProduct(p), nil
}

// Create creates a product.
func (s *Products) Create(ctx context.Context, body model.ProductWrite) (model.Product, error) {
	p, err := s.api.Create(ctx, ToServiceWrite(body))
	if err != nil {
		return model.Product{}, s.mapError(ctx, err)
	}
	return ToClientProduct(p), nil
}

// Update replaces a product.
func (s *Products) Update(ctx context.Context, id string, body model.ProductWrite) (model.Product, error) {
	p, err := s.api.Update(ctx, id, ToServiceWrite(body))
	if err != nil {
		return model.Product{}, s.mapError(ctx, err)
	}
	return ToClientProduct(p), nil
}

// Delete removes a product.
func (s *Products) Delete(ctx context.Context, id string) error {
	if err := s.api.Delete(ctx, id); err != nil {
		return s.mapError(ctx, err)
	}
	return nil
}

func (s *Products) mapError(ctx context.Context, err error) *Error {
	return mapDownstreamError(ctx, s.logger, err, errorPolicy{})
}

// ToClientProduct converts the Product Service representation into the client contract.
func ToClientProduct(p model.ServiceProduct) model.Product {
	return model.Product{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Currency:    p.Currency,
		Available:   p.Available,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// ToServiceWrite converts a client create/update body into the Product Service contract.
// Missing values stay null so the Product Service reports them.
func ToServiceWrite(w model.ProductWrite) model.ServiceProductWrite {
	return model.ServiceProductWrite{
		Name:        w.Name,
		Description: w.Description,
		Price:       w.Price,
		Currency:    w.Currency,
		Available:   w.Available,
	}
}

// pick keeps only the given query parameters.
func pick(query url.Values, keys []string) url.Values {
	out := url.Values{}
	for _, key := range keys {
		if values, ok := query[key]; ok {
			out[key] = values
		}
	}
	return out
}
