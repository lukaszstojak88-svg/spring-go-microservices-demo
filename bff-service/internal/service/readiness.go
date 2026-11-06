package service

import (
	"context"
	"log/slog"
	"sync"

	"bff-service/internal/model"
)

// Status values used by the readiness response.
const (
	StatusUp   = "UP"
	StatusDown = "DOWN"
)

// ReadinessCheck is a named dependency check (e.g. a downstream readiness probe).
type ReadinessCheck struct {
	Name  string
	Check func(ctx context.Context) error
}

// Readiness runs all checks concurrently; the BFF is ready only when every dependency is.
type Readiness struct {
	checks []ReadinessCheck
	logger *slog.Logger
}

// NewReadiness creates the readiness service.
func NewReadiness(logger *slog.Logger, checks ...ReadinessCheck) *Readiness {
	return &Readiness{checks: checks, logger: logger}
}

// Check returns the aggregated status and the status of every dependency.
func (r *Readiness) Check(ctx context.Context) model.ReadinessResponse {
	results := make([]string, len(r.checks))
	var wg sync.WaitGroup
	for i, c := range r.checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Check(ctx); err != nil {
				r.logger.WarnContext(ctx, "readiness check failed", "dependency", c.Name, "error", err)
				results[i] = StatusDown
				return
			}
			results[i] = StatusUp
		}()
	}
	wg.Wait()

	resp := model.ReadinessResponse{Status: StatusUp, Checks: make(map[string]string, len(r.checks))}
	for i, c := range r.checks {
		resp.Checks[c.Name] = results[i]
		if results[i] != StatusUp {
			resp.Status = StatusDown
		}
	}
	return resp
}
