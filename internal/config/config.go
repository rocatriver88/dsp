package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// defaultBidderHMACSecret and defaultCORSOrigins are the dev-only defaults
// baked into Load(). Validate() rejects them in production so that a
// misconfigured deployment fails fast at startup.
const (
	defaultBidderHMACSecret = "dev-hmac-secret-change-in-production"
	defaultAPIHMACSecret    = "dev-api-hmac-secret-change-in-production"
	defaultCORSOrigins      = "http://localhost:4000"
)

type Config struct {
	DBHost             string
	DBPort             string
	DBUser             string
	DBPassword         string
	DBName             string
	RedisAddr          string
	RedisPassword      string
	KafkaBrokers       string
	ClickHouseAddr     string
	ClickHouseUser     string
	ClickHousePassword string
	APIPort            string
	BidderPort         string
	InternalPort       string
	CORSAllowedOrigins string
	BidderPublicURL    string
	BidderHMACSecret   string
	APIHMACSecret      string

	// Guardrails
	GlobalDailyBudgetCents int64   // all campaigns combined, 0 = no limit
	MaxBidCPMCents         int     // single bid ceiling, 0 = no limit
	LowBalanceAlertCents   int64   // alert when below this, 0 = disabled
	MinBalanceCents        int64   // auto-pause all when below this, 0 = disabled
	SpendRateWindowSec     int     // window for spend rate check (default 300 = 5min)
	SpendRateMultiplier    float64 // alert if rate > expected * this (default 3.0)
}

func Load() *Config {
	return &Config{
		DBHost:             getEnv("DB_HOST", "localhost"),
		DBPort:             getEnv("DB_PORT", "5432"),
		DBUser:             getEnv("DB_USER", "dsp"),
		DBPassword:         getEnv("DB_PASSWORD", "dsp_dev_password"),
		DBName:             getEnv("DB_NAME", "dsp"),
		RedisAddr:          getEnv("REDIS_ADDR", "localhost:6380"),
		RedisPassword:      getEnv("REDIS_PASSWORD", ""),
		KafkaBrokers:       getEnv("KAFKA_BROKERS", "localhost:9094"),
		ClickHouseAddr:     getEnv("CLICKHOUSE_ADDR", "localhost:9001"),
		ClickHouseUser:     getEnv("CLICKHOUSE_USER", "default"),
		ClickHousePassword: getEnv("CLICKHOUSE_PASSWORD", ""),
		APIPort:            getEnv("API_PORT", "8181"),
		BidderPort:         getEnv("BIDDER_PORT", "8180"),
		InternalPort:       getEnv("INTERNAL_PORT", "8182"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", defaultCORSOrigins),
		BidderPublicURL:    getEnv("BIDDER_PUBLIC_URL", "http://localhost:8180"),
		BidderHMACSecret:   getEnv("BIDDER_HMAC_SECRET", defaultBidderHMACSecret),
		APIHMACSecret:      getEnv("API_HMAC_SECRET", defaultAPIHMACSecret),

		GlobalDailyBudgetCents: parseInt64("GLOBAL_DAILY_BUDGET_CENTS", 0),
		MaxBidCPMCents:         parseInt("MAX_BID_CPM_CENTS", 0),
		LowBalanceAlertCents:   parseInt64("LOW_BALANCE_ALERT_CENTS", 0),
		MinBalanceCents:        parseInt64("MIN_BALANCE_CENTS", 0),
		SpendRateWindowSec:     parseInt("SPEND_RATE_WINDOW_SEC", 300),
		SpendRateMultiplier:    parseFloat("SPEND_RATE_MULTIPLIER", 3.0),
	}
}

// Validate returns a non-nil error if the configuration would be unsafe for
// production use. Call it from main() immediately after Load() and fail the
// process (log.Fatal) on any error. Keeping Validate as a pure function
// makes it trivially testable; the side-effect of aborting the process
// belongs in the caller, not in the config package.
//
// The production-only checks are:
//   - BIDDER_HMAC_SECRET must be overridden (not the baked-in dev default)
//   - API_HMAC_SECRET must be overridden (signs analytics SSE tokens; kept
//     distinct from BIDDER_HMAC_SECRET so the two trust domains cannot
//     cross-contaminate if one signing key leaks)
//   - ADMIN_TOKEN must be set (no fallback is accepted, and admin_auth now
//     refuses empty tokens at request time as defense in depth)
//   - CORS_ALLOWED_ORIGINS must be overridden (the dev default trusts
//     http://localhost:4000 only, which has no meaning in production)
func (c *Config) Validate() error {
	env := getEnv("ENV", "development")
	if env != "production" {
		return nil
	}
	if c.BidderHMACSecret == defaultBidderHMACSecret {
		return fmt.Errorf("BIDDER_HMAC_SECRET must be set in production; refusing to start with the baked-in dev secret")
	}
	if c.APIHMACSecret == defaultAPIHMACSecret {
		return fmt.Errorf("API_HMAC_SECRET must be set in production; refusing to start with the baked-in dev secret")
	}
	if getEnv("ADMIN_TOKEN", "") == "" {
		return fmt.Errorf("ADMIN_TOKEN must be set in production; there is no default fallback")
	}
	if c.CORSAllowedOrigins == defaultCORSOrigins {
		return fmt.Errorf("CORS_ALLOWED_ORIGINS must be set in production; refusing to start with the dev default %q", defaultCORSOrigins)
	}
	return nil
}

// CSTLocation returns the Asia/Shanghai timezone, cached at package init.
var CSTLocation *time.Location

func init() {
	var err error
	CSTLocation, err = time.LoadLocation("Asia/Shanghai")
	if err != nil {
		CSTLocation = time.FixedZone("CST", 8*3600)
	}
}

func (c *Config) DSN() string {
	return "postgres://" + c.DBUser + ":" + c.DBPassword + "@" + c.DBHost + ":" + c.DBPort + "/" + c.DBName + "?sslmode=disable"
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func parseInt64(key string, fallback int64) int64 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return fallback
}

func parseFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
