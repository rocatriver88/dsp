package config

import (
	"strings"
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

func TestValidate_DevMode_NoError(t *testing.T) {
	// Development mode must never error, even when every production-only
	// secret is at its baked-in default.
	t.Setenv("ENV", "development")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("dev mode should not return an error, got: %v", err)
	}
}

func TestValidate_ProductionWithAllSecrets_NoError(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("production with all secrets set should not error, got: %v", err)
	}
}

func TestValidate_ProductionDefaultHMAC_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	// Intentionally do NOT set BIDDER_HMAC_SECRET so Load picks the dev default.
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when BIDDER_HMAC_SECRET is the dev default in production")
	}
	if !strings.Contains(err.Error(), "BIDDER_HMAC_SECRET") {
		t.Errorf("error should mention BIDDER_HMAC_SECRET, got: %v", err)
	}
}

func TestValidate_ProductionMissingAdminToken_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when ADMIN_TOKEN is unset in production")
	}
	if !strings.Contains(err.Error(), "ADMIN_TOKEN") {
		t.Errorf("error should mention ADMIN_TOKEN, got: %v", err)
	}
}

func TestValidate_ProductionDefaultCORS_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	// Intentionally do NOT set CORS_ALLOWED_ORIGINS so Load picks the dev default.
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when CORS_ALLOWED_ORIGINS is the dev default in production")
	}
	if !strings.Contains(err.Error(), "CORS_ALLOWED_ORIGINS") {
		t.Errorf("error should mention CORS_ALLOWED_ORIGINS, got: %v", err)
	}
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
