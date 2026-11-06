// Package order is the HTTP client of the Order Service. Only the readiness check exists so
// far; order operations are added together with the order endpoints.
package order

import (
	"context"
	"net/http"
	"time"

	"bff-service/internal/client"
)

const (
	serviceName   = "order-service"
	readinessPath = "/actuator/health/readiness"
)

// Client calls the Order Service REST API.
type Client struct {
	http *client.HTTP
}

// New creates a client for the Order Service at baseURL (without a trailing slash).
func New(baseURL string, timeout time.Duration) *Client {
	return &Client{http: client.New(serviceName, baseURL, timeout)}
}

// Ready checks the Order Service readiness probe.
func (c *Client) Ready(ctx context.Context) error {
	return c.http.Do(ctx, http.MethodGet, readinessPath, nil, nil, nil)
}
