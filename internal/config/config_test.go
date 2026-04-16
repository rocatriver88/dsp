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
	t.Setenv("API_HMAC_SECRET", "real-production-api-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("REDIS_ADDR", "redis.prod.internal:6379")
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/services/T00/B00/xxx")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("production with all secrets set should not error, got: %v", err)
	}
}

func TestValidate_ProductionNoAlertChannel_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("API_HMAC_SECRET", "real-production-api-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("REDIS_ADDR", "redis.prod.internal:6379")
	// No SLACK_WEBHOOK_URL or ALERT_EMAIL_SMTP_HOST
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when no alert channel is configured in production")
	}
	if !strings.Contains(err.Error(), "alert channel") {
		t.Errorf("error should mention alert channel, got: %v", err)
	}
}

func TestValidate_ProductionEmailAlertChannel_NoError(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("API_HMAC_SECRET", "real-production-api-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	t.Setenv("REDIS_ADDR", "redis.prod.internal:6379")
	t.Setenv("ALERT_EMAIL_SMTP_HOST", "smtp.example.com")
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		t.Errorf("production with email alert should not error, got: %v", err)
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
	t.Setenv("API_HMAC_SECRET", "real-production-api-secret-32chars-min")
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
	t.Setenv("API_HMAC_SECRET", "real-production-api-secret-32chars-min")
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

func TestLoad_APIHMACSecret_EnvOverride(t *testing.T) {
	t.Setenv("API_HMAC_SECRET", "test-api-hmac-secret-long-enough-12345678")
	cfg := Load()
	if cfg.APIHMACSecret != "test-api-hmac-secret-long-enough-12345678" {
		t.Errorf("expected API_HMAC_SECRET to be loaded into cfg.APIHMACSecret, got %q", cfg.APIHMACSecret)
	}
}

func TestValidate_ProductionDefaultAPIHMAC_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-secret-32chars-min")
	t.Setenv("ADMIN_TOKEN", "test-admin-token")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://app.example.com")
	// Intentionally do NOT set API_HMAC_SECRET so Load picks the dev default.
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when API_HMAC_SECRET is the dev default in production")
	}
	if !strings.Contains(err.Error(), "API_HMAC_SECRET") {
		t.Errorf("error should mention API_HMAC_SECRET, got: %v", err)
	}
}

func TestValidate_APIHMACSecretTooShort_Errors(t *testing.T) {
	t.Setenv("ENV", "production")
	t.Setenv("BIDDER_HMAC_SECRET", "real-production-bidder-secret-32chars-min")
	t.Setenv("API_HMAC_SECRET", "tooshort") // 8 bytes — fails length check
	t.Setenv("ADMIN_TOKEN", "real-production-admin-token-32chars-min")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://example.com")
	cfg := Load()
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when API_HMAC_SECRET is shorter than 32 bytes")
	}
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Errorf("expected length error, got: %v", err)
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
