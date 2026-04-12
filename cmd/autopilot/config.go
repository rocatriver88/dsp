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
	APIKey      string // will be created during verify
	AdminToken  string // for admin endpoints
	FrontendURL string // http://localhost:4000

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
	ReportHour     int           // hour to generate daily report (default 9)

	// Screenshots
	ScreenshotDir string // directory to save screenshots
	ReportDir     string // directory to save HTML reports

	// Grafana (for monitoring screenshots)
	GrafanaURL string // http://localhost:3100
}

func LoadAutopilotConfig() *AutopilotConfig {
	return &AutopilotConfig{
		APIURL:          getEnv("AUTOPILOT_API_URL", "http://localhost:8181"),
		AdminToken:      getEnv("ADMIN_TOKEN", "admin-secret"),
		FrontendURL:     getEnv("AUTOPILOT_FRONTEND_URL", "http://localhost:4000"),
		ExchangeSimURL:  getEnv("AUTOPILOT_EXCHANGE_SIM_URL", "http://localhost:9090"),
		WebhookURL:      getEnv("AUTOPILOT_WEBHOOK_URL", ""),
		WebhookType:     getEnv("AUTOPILOT_WEBHOOK_TYPE", "dingtalk"),
		TrafficDuration: parseDuration("AUTOPILOT_TRAFFIC_DURATION", 5*time.Minute),
		DayStartHour:    parseInt("AUTOPILOT_DAY_START", 8),
		DayEndHour:      parseInt("AUTOPILOT_DAY_END", 22),
		DayQPS:          parseInt("AUTOPILOT_DAY_QPS", 10),
		NightQPS:        parseInt("AUTOPILOT_NIGHT_QPS", 1),
		HealthInterval:  parseDuration("AUTOPILOT_HEALTH_INTERVAL", time.Minute),
		ReportHour:      parseInt("AUTOPILOT_REPORT_HOUR", 9),
		ScreenshotDir:   getEnv("AUTOPILOT_SCREENSHOT_DIR", "autopilot-output/screenshots"),
		ReportDir:       getEnv("AUTOPILOT_REPORT_DIR", "autopilot-output/reports"),
		GrafanaURL:      getEnv("AUTOPILOT_GRAFANA_URL", "http://localhost:3100"),
	}
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

func parseDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
