// Command server runs the BFF HTTP server.
//
// Flags:
//
//	-healthcheck  call the local /health endpoint and exit 0 (healthy) or 1;
//	              used by the container health check (distroless image has no curl).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"bff-service/internal/client/order"
	"bff-service/internal/client/product"
	"bff-service/internal/config"
	"bff-service/internal/handler"
	"bff-service/internal/middleware"
	"bff-service/internal/service"
)

const shutdownTimeout = 15 * time.Second

func main() {
	healthcheck := flag.Bool("healthcheck", false, "check the local /health endpoint and exit")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	logger := slog.New(middleware.NewContextHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("server stopped with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	productClient := product.New(cfg.ProductServiceURL, cfg.ProductTimeout)
	orderClient := order.New(cfg.OrderServiceURL, cfg.OrderTimeout)

	router := handler.NewRouter(handler.Deps{
		Products: service.NewProducts(productClient, logger),
		Readiness: service.NewReadiness(logger,
			service.ReadinessCheck{Name: "productService", Check: productClient.Ready},
			service.ReadinessCheck{Name: "orderService", Check: orderClient.Ready},
		),
		Logger: logger,
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("bff-service listening",
			"port", cfg.Port,
			"productServiceUrl", cfg.ProductServiceURL,
			"orderServiceUrl", cfg.OrderServiceURL,
			"productTimeout", cfg.ProductTimeout.String(),
			"orderTimeout", cfg.OrderTimeout.String())
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received, draining connections")
	}

	// In-flight requests finish (their downstream calls are bounded by the client timeouts);
	// new connections are refused.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

// runHealthcheck only needs the port, so it does not require the full configuration.
func runHealthcheck() int {
	port, err := config.PortFrom(os.Getenv)
	if err != nil {
		return 1
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
