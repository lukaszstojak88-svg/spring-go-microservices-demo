// Package product is the HTTP client of the Product Service.
package product

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"bff-service/internal/client"
	"bff-service/internal/model"
)

const (
	serviceName   = "product-service"
	productsPath  = "/api/products"
	lookupPath    = "/api/products/lookup"
	readinessPath = "/actuator/health/readiness"
)

// Client calls the Product Service REST API. Every call is a single attempt bounded by the
// configured timeout and by the caller's context.
type Client struct {
	http *client.HTTP
}

// New creates a client for the Product Service at baseURL (without a trailing slash).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: client.New(serviceName, baseURL, timeout)}
}

// List returns a page of products; query is forwarded as is.
func (c *Client) List(ctx context.Context, query url.Values) (model.ServiceProductPage, error) {
	var page model.ServiceProductPage
	if err := c.http.Do(ctx, http.MethodGet, productsPath, query, nil, &page); err != nil {
		return model.ServiceProductPage{}, err
	}
	if page.Content == nil {
		return model.ServiceProductPage{}, c.http.Invalid("product page without content")
	}
	for _, p := range page.Content {
		if !p.IsComplete() {
			return model.ServiceProductPage{}, c.http.Invalid("incomplete product in page")
		}
	}
	return page, nil
}

// Get returns one product.
func (c *Client) Get(ctx context.Context, id string) (model.ServiceProduct, error) {
	return c.product(ctx, http.MethodGet, itemPath(id), nil)
}

// Create creates a product (POST, never retried).
func (c *Client) Create(ctx context.Context, body model.ServiceProductWrite) (model.ServiceProduct, error) {
	return c.product(ctx, http.MethodPost, productsPath, body)
}

// Update replaces the editable fields of a product.
func (c *Client) Update(ctx context.Context, id string, body model.ServiceProductWrite) (model.ServiceProduct, error) {
	return c.product(ctx, http.MethodPut, itemPath(id), body)
}

// Delete removes a product.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.http.Do(ctx, http.MethodDelete, itemPath(id), nil, nil, nil)
}

// Lookup fetches several products in ONE batch call. Missing products are not an error: they
// are listed in MissingIDs.
func (c *Client) Lookup(ctx context.Context, ids []string) (model.ServiceLookupResponse, error) {
	var resp model.ServiceLookupResponse
	err := c.http.Do(ctx, http.MethodPost, lookupPath, nil, model.ServiceLookupRequest{IDs: ids}, &resp)
	if err != nil {
		return model.ServiceLookupResponse{}, err
	}
	if resp.Products == nil || resp.MissingIDs == nil {
		return model.ServiceLookupResponse{}, c.http.Invalid("incomplete lookup response")
	}
	for _, p := range resp.Products {
		if !p.IsComplete() {
			return model.ServiceLookupResponse{}, c.http.Invalid("incomplete product in lookup response")
		}
	}
	return resp, nil
}

// Ready checks the Product Service readiness probe.
func (c *Client) Ready(ctx context.Context) error {
	return c.http.Do(ctx, http.MethodGet, readinessPath, nil, nil, nil)
}

func (c *Client) product(ctx context.Context, method, path string, body any) (model.ServiceProduct, error) {
	var p model.ServiceProduct
	if err := c.http.Do(ctx, method, path, nil, body, &p); err != nil {
		return model.ServiceProduct{}, err
	}
	if !p.IsComplete() {
		return model.ServiceProduct{}, c.http.Invalid("incomplete product")
	}
	return p, nil
}

// itemPath escapes the id so it cannot change the downstream path (e.g. "../x" or "a?b").
func itemPath(id string) string {
	return productsPath + "/" + url.PathEscape(id)
}
