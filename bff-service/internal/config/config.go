// Package config loads the BFF configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment variable names.
const (
	EnvPort              = "BFF_PORT"
	EnvProductServiceURL = "PRODUCT_SERVICE_URL"
	EnvOrderServiceURL   = "ORDER_SERVICE_URL"
	EnvProductTimeout    = "PRODUCT_CLIENT_TIMEOUT"
	EnvOrderTimeout      = "ORDER_CLIENT_TIMEOUT"
)

// Config holds the runtime configuration of the BFF service.
type Config struct {
	Port              int
	ProductServiceURL string
	OrderServiceURL   string
	ProductTimeout    time.Duration
	OrderTimeout      time.Duration
}

const (
	// DefaultPort is used when BFF_PORT is not set.
	DefaultPort           = 8080
	defaultProductTimeout = 3 * time.Second
	defaultOrderTimeout   = 5 * time.Second
)

// Load reads the configuration using os.Getenv.
func Load() (Config, error) {
	return LoadFrom(os.Getenv)
}

// LoadFrom reads the configuration using the given lookup function, which
// makes it testable without touching the process environment.
func LoadFrom(getenv func(string) string) (Config, error) {
	var errs []error

	port, err := PortFrom(getenv)
	errs = appendIf(errs, err)

	productURL, err := requiredURL(getenv, EnvProductServiceURL)
	errs = appendIf(errs, err)
	orderURL, err := requiredURL(getenv, EnvOrderServiceURL)
	errs = appendIf(errs, err)

	productTimeout, err := durationOr(getenv, EnvProductTimeout, defaultProductTimeout)
	errs = appendIf(errs, err)
	orderTimeout, err := durationOr(getenv, EnvOrderTimeout, defaultOrderTimeout)
	errs = appendIf(errs, err)

	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return Config{
		Port:              port,
		ProductServiceURL: productURL,
		OrderServiceURL:   orderURL,
		ProductTimeout:    productTimeout,
		OrderTimeout:      orderTimeout,
	}, nil
}

// PortFrom reads only the listening port; the -healthcheck mode needs nothing else.
func PortFrom(getenv func(string) string) (int, error) {
	raw := getenv(EnvPort)
	if raw == "" {
		return DefaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s: invalid port %q", EnvPort, raw)
	}
	return port, nil
}

func appendIf(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

// requiredURL validates the downstream base URL and strips a trailing slash so that
// paths can be appended safely.
func requiredURL(getenv func(string) string, key string) (string, error) {
	raw := getenv(key)
	if raw == "" {
		return "", fmt.Errorf("%s: required", key)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s: invalid URL %q", key, raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

func durationOr(getenv func(string) string, key string, fallback time.Duration) (time.Duration, error) {
	raw := getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: invalid positive duration %q", key, raw)
	}
	return d, nil
}
