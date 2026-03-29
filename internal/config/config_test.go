package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	cfg := Load()
	if cfg.APIPort != "8181" {
		t.Errorf("expected default APIPort 8181, got %s", cfg.APIPort)
	}
	if cfg.InternalPort != "8182" {
		t.Errorf("expected default InternalPort 8182, got %s", cfg.InternalPort)
	}
	if cfg.BidderPublicURL != "http://localhost:8180" {
		t.Errorf("expected default BidderPublicURL, got %s", cfg.BidderPublicURL)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("API_PORT", "9999")
	t.Setenv("BIDDER_HMAC_SECRET", "my-secret")
	cfg := Load()
	if cfg.APIPort != "9999" {
		t.Errorf("expected overridden APIPort 9999, got %s", cfg.APIPort)
	}
	if cfg.BidderHMACSecret != "my-secret" {
		t.Errorf("expected overridden HMAC secret, got %s", cfg.BidderHMACSecret)
	}
}

func TestLoad_CORSDefault(t *testing.T) {
	cfg := Load()
	if cfg.CORSAllowedOrigins == "" {
		t.Error("expected non-empty CORS default")
	}
}
