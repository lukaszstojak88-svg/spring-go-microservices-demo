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
		"PORT":                    "9090",
		"PRODUCT_SERVICE_URL":     "http://p:1",
		"ORDER_SERVICE_URL":       "https://o:2",
		"PRODUCT_SERVICE_TIMEOUT": "750ms",
		"ORDER_SERVICE_TIMEOUT":   "2s",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != 9090 || cfg.ProductTimeout != 750*time.Millisecond || cfg.OrderTimeout != 2*time.Second {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadFrom_InvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		values  map[string]string
		wantErr string
	}{
		{"missing urls", map[string]string{}, "PRODUCT_SERVICE_URL: required"},
		{"bad url", map[string]string{"PRODUCT_SERVICE_URL": "product-service", "ORDER_SERVICE_URL": "http://o:2"}, "PRODUCT_SERVICE_URL: invalid URL"},
		{"bad port", map[string]string{"PORT": "abc", "PRODUCT_SERVICE_URL": "http://p:1", "ORDER_SERVICE_URL": "http://o:2"}, "PORT"},
		{"bad timeout", map[string]string{"PRODUCT_SERVICE_URL": "http://p:1", "ORDER_SERVICE_URL": "http://o:2", "ORDER_SERVICE_TIMEOUT": "-1s"}, "ORDER_SERVICE_TIMEOUT"},
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
