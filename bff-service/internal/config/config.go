// Package config loads the BFF configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
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
	defaultPort           = 8080
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

	port, err := intOr(getenv("PORT"), defaultPort)
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, fmt.Errorf("PORT: invalid port %q", getenv("PORT")))
	}

	productURL, err := requiredURL(getenv, "PRODUCT_SERVICE_URL")
	errs = appendIf(errs, err)
	orderURL, err := requiredURL(getenv, "ORDER_SERVICE_URL")
	errs = appendIf(errs, err)

	productTimeout, err := durationOr(getenv, "PRODUCT_SERVICE_TIMEOUT", defaultProductTimeout)
	errs = appendIf(errs, err)
	orderTimeout, err := durationOr(getenv, "ORDER_SERVICE_TIMEOUT", defaultOrderTimeout)
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

func appendIf(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
}

func intOr(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}

func requiredURL(getenv func(string) string, key string) (string, error) {
	raw := getenv(key)
	if raw == "" {
		return "", fmt.Errorf("%s: required", key)
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s: invalid URL %q", key, raw)
	}
	return raw, nil
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
