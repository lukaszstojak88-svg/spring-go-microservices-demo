// Package order is the HTTP client of the Order Service.
package order

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"bff-service/internal/client"
	"bff-service/internal/model"
)

const (
	serviceName   = "order-service"
	ordersPath    = "/api/orders"
	readinessPath = "/actuator/health/readiness"
)

// Client calls the Order Service REST API. Every call is a single attempt bounded by the
// configured timeout and by the caller's context. POST /api/orders is not idempotent, so it is
// never retried.
type Client struct {
	http *client.HTTP
}

// New creates a client for the Order Service at baseURL (without a trailing slash).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: client.New(serviceName, baseURL, timeout)}
}

// Create creates an order; the Order Service verifies products and computes totals.
func (c *Client) Create(ctx context.Context, body model.OrderCreate) (model.ServiceOrder, error) {
	return c.order(ctx, http.MethodPost, ordersPath, body)
}

// Get returns one order (its stored snapshot).
func (c *Client) Get(ctx context.Context, id string) (model.ServiceOrder, error) {
	return c.order(ctx, http.MethodGet, itemPath(id), nil)
}

// List returns a page of orders; query is forwarded as is.
func (c *Client) List(ctx context.Context, query url.Values) (model.ServiceOrderPage, error) {
	var page model.ServiceOrderPage
	if err := c.http.Do(ctx, http.MethodGet, ordersPath, query, nil, &page); err != nil {
		return model.ServiceOrderPage{}, err
	}
	if page.Content == nil {
		return model.ServiceOrderPage{}, c.http.Invalid("order page without content")
	}
	for _, o := range page.Content {
		if !o.IsComplete() {
			return model.ServiceOrderPage{}, c.http.Invalid("incomplete order in page")
		}
	}
	return page, nil
}

// Cancel cancels an order (the Order Service owns the status transition rules).
func (c *Client) Cancel(ctx context.Context, id string) (model.ServiceOrder, error) {
	return c.order(ctx, http.MethodPost, itemPath(id)+"/cancel", nil)
}

// Ready checks the Order Service readiness probe.
func (c *Client) Ready(ctx context.Context) error {
	return c.http.Do(ctx, http.MethodGet, readinessPath, nil, nil, nil)
}

func (c *Client) order(ctx context.Context, method, path string, body any) (model.ServiceOrder, error) {
	var o model.ServiceOrder
	if err := c.http.Do(ctx, method, path, nil, body, &o); err != nil {
		return model.ServiceOrder{}, err
	}
	if !o.IsComplete() {
		return model.ServiceOrder{}, c.http.Invalid("incomplete order")
	}
	return o, nil
}

// itemPath escapes the id so it cannot change the downstream path.
func itemPath(id string) string {
	return ordersPath + "/" + url.PathEscape(id)
}
