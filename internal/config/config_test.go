package config

import (
	"testing"
	"time"
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

func TestValidate_DevMode_NoFatal(t *testing.T) {
	// Development mode should not fatal even with default HMAC secret
	t.Setenv("ENV", "development")
	cfg := Load()
	cfg.Validate() // should not panic
}

func TestValidate_ProductionWithCustomSecret_NoFatal(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	cfg := Load()
	cfg.Validate() // should not panic
}

func TestCSTLocation_NotNil(t *testing.T) {
	if CSTLocation == nil {
		t.Fatal("CSTLocation should be initialized at package init")
	}
	// Verify it's UTC+8
	_, offset := time.Date(2026, 1, 1, 0, 0, 0, 0, CSTLocation).Zone()
	if offset != 8*3600 {
		t.Errorf("expected UTC+8 (28800s offset), got %d", offset)
	}
}
