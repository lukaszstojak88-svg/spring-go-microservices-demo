package config

import (
	"strings"
	"testing"
	"time"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadFrom_Defaults(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"PRODUCT_SERVICE_URL": "http://product-service:8081",
		"ORDER_SERVICE_URL":   "http://order-service:8082",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.ProductTimeout != 3*time.Second || cfg.OrderTimeout != 5*time.Second {
		t.Errorf("timeouts = %v/%v, want 3s/5s", cfg.ProductTimeout, cfg.OrderTimeout)
	}
}

func TestLoadFrom_ExplicitValues(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"BFF_PORT":               "9090",
		"PRODUCT_SERVICE_URL":    "http://p:1/",
		"ORDER_SERVICE_URL":      "https://o:2",
		"PRODUCT_CLIENT_TIMEOUT": "750ms",
		"ORDER_CLIENT_TIMEOUT":   "2s",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 || cfg.ProductTimeout != 750*time.Millisecond || cfg.OrderTimeout != 2*time.Second {
		t.Errorf("unexpected config: %+v", cfg)
	}
	if cfg.ProductServiceURL != "http://p:1" {
		t.Errorf("ProductServiceURL = %q, want trailing slash removed", cfg.ProductServiceURL)
	}
}

func TestLoadFrom_IgnoresLegacyNames(t *testing.T) {
	cfg, err := LoadFrom(env(map[string]string{
		"PORT":                    "9999",
		"PRODUCT_SERVICE_TIMEOUT": "1ms",
		"PRODUCT_SERVICE_URL":     "http://p:1",
		"ORDER_SERVICE_URL":       "http://o:2",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != DefaultPort || cfg.ProductTimeout != 3*time.Second {
		t.Errorf("legacy variables must not be read: %+v", cfg)
	}
}

func TestLoadFrom_InvalidValues(t *testing.T) {
	urls := func(extra map[string]string) map[string]string {
		m := map[string]string{"PRODUCT_SERVICE_URL": "http://p:1", "ORDER_SERVICE_URL": "http://o:2"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name    string
		values  map[string]string
		wantErr string
	}{
		{"missing urls", map[string]string{}, "PRODUCT_SERVICE_URL: required"},
		{"missing order url", map[string]string{"PRODUCT_SERVICE_URL": "http://p:1"}, "ORDER_SERVICE_URL: required"},
		{"bad url", map[string]string{"PRODUCT_SERVICE_URL": "product-service", "ORDER_SERVICE_URL": "http://o:2"}, "PRODUCT_SERVICE_URL: invalid URL"},
		{"bad scheme", map[string]string{"PRODUCT_SERVICE_URL": "ftp://p:1", "ORDER_SERVICE_URL": "http://o:2"}, "PRODUCT_SERVICE_URL: invalid URL"},
		{"bad port", urls(map[string]string{"BFF_PORT": "abc"}), "BFF_PORT"},
		{"port out of range", urls(map[string]string{"BFF_PORT": "70000"}), "BFF_PORT"},
		{"bad product timeout", urls(map[string]string{"PRODUCT_CLIENT_TIMEOUT": "fast"}), "PRODUCT_CLIENT_TIMEOUT"},
		{"negative order timeout", urls(map[string]string{"ORDER_CLIENT_TIMEOUT": "-1s"}), "ORDER_CLIENT_TIMEOUT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadFrom(env(tt.values))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestPortFrom(t *testing.T) {
	if p, err := PortFrom(env(nil)); err != nil || p != 8080 {
		t.Errorf("default = %d, %v", p, err)
	}
	if p, err := PortFrom(env(map[string]string{"BFF_PORT": "8181"})); err != nil || p != 8181 {
		t.Errorf("explicit = %d, %v", p, err)
	}
}
