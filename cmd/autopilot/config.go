// cmd/autopilot/config.go
package main

import (
	"os"
	"strconv"
	"time"
)

type AutopilotConfig struct {
	// DSP API
	APIURL      string // http://localhost:8181
	AdminURL    string // http://localhost:8182 (internal port)
	APIKey      string // will be created during verify
	AdminToken  string // for admin endpoints
	FrontendURL string // http://localhost:4000

	// Bidder
	BidderURL string // http://localhost:8180

	// Exchange simulator
	ExchangeSimURL string // http://localhost:9090

	// Alert webhook
	WebhookURL  string // DingTalk or Feishu webhook URL
	WebhookType string // "dingtalk" or "feishu"

	// Verify mode
	TrafficDuration time.Duration // how long to run exchange-sim traffic (default 5m)

	// Continuous mode
	DayStartHour   int           // 8
	DayEndHour     int           // 22
	DayQPS         int           // target QPS during daytime
	NightQPS       int           // target QPS during nighttime
	HealthInterval time.Duration // how often to check service health (default 1m)
	ReportHour          int           // hour to generate daily report (default 9)
	LowBalanceThreshold int64         // balance threshold in cents for alerts (default 100000)

	// Screenshots
	ScreenshotDir string // directory to save screenshots
	ReportDir     string // directory to save HTML reports

	// Grafana (for monitoring screenshots)
	GrafanaURL string // http://localhost:3100
}

func LoadAutopilotConfig() *AutopilotConfig {
	return &AutopilotConfig{
		APIURL:          getEnv("AUTOPILOT_API_URL", "http://localhost:8181"),
		AdminURL:        getEnv("AUTOPILOT_ADMIN_URL", "http://localhost:8182"),
		AdminToken:      getEnv("ADMIN_TOKEN", "admin-secret"),
		FrontendURL:     getEnv("AUTOPILOT_FRONTEND_URL", "http://localhost:4000"),
		BidderURL:       getEnv("AUTOPILOT_BIDDER_URL", "http://localhost:8180"),
		ExchangeSimURL:  getEnv("AUTOPILOT_EXCHANGE_SIM_URL", "http://localhost:9090"),
		WebhookURL:      getEnv("AUTOPILOT_WEBHOOK_URL", ""),
		WebhookType:     getEnv("AUTOPILOT_WEBHOOK_TYPE", "dingtalk"),
		TrafficDuration: parseDuration("AUTOPILOT_TRAFFIC_DURATION", 5*time.Minute),
		DayStartHour:    parseInt("AUTOPILOT_DAY_START", 8),
		DayEndHour:      parseInt("AUTOPILOT_DAY_END", 22),
		DayQPS:          parseInt("AUTOPILOT_DAY_QPS", 10),
		NightQPS:        parseInt("AUTOPILOT_NIGHT_QPS", 1),
		HealthInterval:  parseDuration("AUTOPILOT_HEALTH_INTERVAL", time.Minute),
		ReportHour:          parseInt("AUTOPILOT_REPORT_HOUR", 9),
		LowBalanceThreshold: parseInt64("AUTOPILOT_LOW_BALANCE_CENTS", 100000),
		ScreenshotDir:   getEnv("AUTOPILOT_SCREENSHOT_DIR", "autopilot-output/screenshots"),
		ReportDir:       getEnv("AUTOPILOT_REPORT_DIR", "autopilot-output/reports"),
		GrafanaURL:      getEnvAllowEmpty("AUTOPILOT_GRAFANA_URL", "http://localhost:3100"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvAllowEmpty(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
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

func parseDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
