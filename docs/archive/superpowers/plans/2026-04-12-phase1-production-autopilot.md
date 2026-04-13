# Phase 1: Production Hardening + Autopilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deploy the DSP to production-ready state and build an automated simulation tool (autopilot) that validates the entire system end-to-end with UI screenshots and fault injection.

**Architecture:** New `cmd/autopilot/` service with two modes: `verify` (one-shot E2E validation producing HTML report) and `run` (continuous 24/7 simulation). Uses chromedp for browser screenshots, Docker API for fault injection, and HTTP client for DSP API calls. New `internal/alert/` package for DingTalk/Feishu webhook notifications, reused by autopilot and future services.

**Tech Stack:** Go 1.26, chromedp (headless Chrome), Docker Engine API, Go html/template, existing DSP API + exchange-sim

---

## File Structure

```
internal/alert/
├── alert.go                  # Webhook interface + DingTalk/Feishu implementations
└── alert_test.go             # Tests with HTTP mock

cmd/autopilot/
├── main.go                   # CLI entry: verify / run subcommands
├── config.go                 # Autopilot-specific config
├── client.go                 # DSP API HTTP client
├── client_test.go            # Client tests with httptest
├── browser.go                # chromedp screenshot helper
├── report.go                 # HTML report generator
├── report_test.go            # Report generation tests
├── report.html               # HTML template (embedded)
├── scenarios.go              # Verify mode: ordered scenario runner
├── scenarios_test.go         # Scenario logic tests (mocked client)
├── fault.go                  # Docker API fault injection
├── continuous.go             # Run mode: traffic simulation + health checks
└── continuous_test.go        # Traffic curve + scheduling tests
```

**Modifications:**
- `docker-compose.yml` — add restart policies, app service healthchecks
- `go.mod` — add `github.com/chromedp/chromedp`, `github.com/docker/docker`

---

### Task 1: Alert Webhook Package

**Files:**
- Create: `internal/alert/alert.go`
- Create: `internal/alert/alert_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/alert/alert_test.go
package alert

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDingTalkSend(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Write([]byte(`{"errcode":0}`))
	}))
	defer srv.Close()

	d := NewDingTalk(srv.URL)
	err := d.Send("test title", "test content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, _ := received["msgtype"].(string)
	if msgType != "markdown" {
		t.Errorf("expected markdown, got %s", msgType)
	}
	md, _ := received["markdown"].(map[string]any)
	title, _ := md["title"].(string)
	if title != "test title" {
		t.Errorf("expected 'test title', got %s", title)
	}
}

func TestFeishuSend(t *testing.T) {
	var received map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	f := NewFeishu(srv.URL)
	err := f.Send("test title", "test content")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgType, _ := received["msg_type"].(string)
	if msgType != "interactive" {
		t.Errorf("expected interactive, got %s", msgType)
	}
}

func TestNoop(t *testing.T) {
	n := Noop{}
	if err := n.Send("a", "b"); err != nil {
		t.Errorf("noop should not error: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/alert/ -v`
Expected: FAIL — package does not exist

- [ ] **Step 3: Write implementation**

```go
// internal/alert/alert.go
package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Sender sends alert notifications.
type Sender interface {
	Send(title, content string) error
}

// DingTalk sends alerts via DingTalk webhook.
type DingTalk struct {
	WebhookURL string
	client     *http.Client
}

func NewDingTalk(webhookURL string) *DingTalk {
	return &DingTalk{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DingTalk) Send(title, content string) error {
	payload := map[string]any{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("## %s\n\n%s", title, content),
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := d.client.Post(d.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("dingtalk send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dingtalk: status %d", resp.StatusCode)
	}
	return nil
}

// Feishu sends alerts via Feishu (Lark) webhook.
type Feishu struct {
	WebhookURL string
	client     *http.Client
}

func NewFeishu(webhookURL string) *Feishu {
	return &Feishu{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (f *Feishu) Send(title, content string) error {
	payload := map[string]any{
		"msg_type": "interactive",
		"card": map[string]any{
			"header": map[string]any{
				"title": map[string]string{"tag": "plain_text", "content": title},
			},
			"elements": []map[string]any{
				{"tag": "markdown", "content": content},
			},
		},
	}
	body, _ := json.Marshal(payload)
	resp, err := f.client.Post(f.WebhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("feishu send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("feishu: status %d", resp.StatusCode)
	}
	return nil
}

// Noop discards alerts silently. Used when no webhook is configured.
type Noop struct{}

func (Noop) Send(string, string) error { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./internal/alert/ -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Commit**

```bash
git add internal/alert/
git commit -m "feat: add alert webhook package (DingTalk + Feishu)"
```

---

### Task 2: Autopilot Config

**Files:**
- Create: `cmd/autopilot/config.go`

- [ ] **Step 1: Write config struct**

```go
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
	DayStartHour int // 8
	DayEndHour   int // 22
	DayQPS       int // target QPS during daytime
	NightQPS     int // target QPS during nighttime
	HealthInterval time.Duration // how often to check service health (default 1m)
	ReportHour   int // hour to generate daily report (default 9)

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
```

- [ ] **Step 2: Commit**

```bash
git add cmd/autopilot/config.go
git commit -m "feat(autopilot): add configuration"
```

---

### Task 3: DSP API Client

**Files:**
- Create: `cmd/autopilot/client.go`
- Create: `cmd/autopilot/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/autopilot/client_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientCreateAdvertiser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/advertisers" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["company_name"] != "Test Co" {
			t.Errorf("expected 'Test Co', got %v", body["company_name"])
		}
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": 1, "api_key": "dsp_test123"})
	}))
	defer srv.Close()

	c := NewDSPClient(srv.URL, "")
	adv, err := c.CreateAdvertiser("Test Co", "test@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adv.ID != 1 {
		t.Errorf("expected id=1, got %d", adv.ID)
	}
	if adv.APIKey != "dsp_test123" {
		t.Errorf("expected api_key=dsp_test123, got %s", adv.APIKey)
	}
}

func TestClientTopUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v1/billing/topup" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		apiKey := r.Header.Get("X-API-Key")
		if apiKey != "dsp_test123" {
			t.Errorf("expected X-API-Key dsp_test123, got %s", apiKey)
		}
		json.NewEncoder(w).Encode(map[string]any{"balance_cents": 100000})
	}))
	defer srv.Close()

	c := NewDSPClient(srv.URL, "dsp_test123")
	balance, err := c.TopUp(1, 100000, "autopilot test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if balance != 100000 {
		t.Errorf("expected balance 100000, got %d", balance)
	}
}

func TestClientCreateCampaign(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]any{"id": 10})
	}))
	defer srv.Close()

	c := NewDSPClient(srv.URL, "dsp_key")
	id, err := c.CreateCampaign(CampaignRequest{
		AdvertiserID:   1,
		Name:           "Autopilot Test Campaign",
		BillingModel:   "cpm",
		BudgetTotalCents: 50000,
		BudgetDailyCents: 10000,
		BidCPMCents:     500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 10 {
		t.Errorf("expected id=10, got %d", id)
	}
}

func TestClientGetOverviewStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/reports/overview" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"today_spend_cents":  1500,
			"today_impressions":  300,
			"today_clicks":       15,
		})
	}))
	defer srv.Close()

	c := NewDSPClient(srv.URL, "dsp_key")
	stats, err := c.GetOverviewStats()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.TodaySpendCents != 1500 {
		t.Errorf("expected spend 1500, got %d", stats.TodaySpendCents)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run TestClient -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Write implementation**

```go
// cmd/autopilot/client.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DSPClient wraps HTTP calls to the DSP API.
type DSPClient struct {
	BaseURL    string
	APIKey     string
	AdminToken string
	client     *http.Client
}

func NewDSPClient(baseURL, apiKey string) *DSPClient {
	return &DSPClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *DSPClient) do(method, path string, body any) ([]byte, int, error) {
	var reqBody io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	if c.AdminToken != "" {
		req.Header.Set("X-Admin-Token", c.AdminToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return data, resp.StatusCode, nil
}

// --- Response types ---

type AdvertiserResponse struct {
	ID     int64  `json:"id"`
	APIKey string `json:"api_key"`
}

type CampaignRequest struct {
	AdvertiserID     int64  `json:"advertiser_id"`
	Name             string `json:"name"`
	BillingModel     string `json:"billing_model"`
	BudgetTotalCents int64  `json:"budget_total_cents"`
	BudgetDailyCents int64  `json:"budget_daily_cents"`
	BidCPMCents      int    `json:"bid_cpm_cents"`
}

type CampaignResponse struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Name   string `json:"name"`
}

type OverviewStats struct {
	TodaySpendCents  int64   `json:"today_spend_cents"`
	TodayImpressions int64   `json:"today_impressions"`
	TodayClicks      int64   `json:"today_clicks"`
	CTR              float64 `json:"ctr"`
	BalanceCents     int64   `json:"balance_cents"`
}

type CampaignStats struct {
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	SpendCents  int64   `json:"spend_cents"`
	WinRate     float64 `json:"win_rate"`
	CTR         float64 `json:"ctr"`
}

type CreativeRequest struct {
	CampaignID     int64  `json:"campaign_id"`
	Name           string `json:"name"`
	AdType         string `json:"ad_type"`
	Format         string `json:"format"`
	Size           string `json:"size"`
	AdMarkup       string `json:"ad_markup"`
	DestinationURL string `json:"destination_url"`
}

// --- API methods ---

func (c *DSPClient) CreateAdvertiser(companyName, email string) (*AdvertiserResponse, error) {
	data, status, err := c.do("POST", "/api/v1/advertisers", map[string]string{
		"company_name":  companyName,
		"contact_email": email,
	})
	if err != nil {
		return nil, err
	}
	if status != 201 {
		return nil, fmt.Errorf("create advertiser: status %d, body: %s", status, data)
	}
	var resp AdvertiserResponse
	json.Unmarshal(data, &resp)
	return &resp, nil
}

func (c *DSPClient) TopUp(advertiserID int64, amountCents int64, description string) (int64, error) {
	data, status, err := c.do("POST", "/api/v1/billing/topup", map[string]any{
		"advertiser_id": advertiserID,
		"amount_cents":  amountCents,
		"description":   description,
	})
	if err != nil {
		return 0, err
	}
	if status != 200 {
		return 0, fmt.Errorf("topup: status %d, body: %s", status, data)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	balance, _ := resp["balance_cents"].(float64)
	return int64(balance), nil
}

func (c *DSPClient) CreateCampaign(req CampaignRequest) (int64, error) {
	data, status, err := c.do("POST", "/api/v1/campaigns", req)
	if err != nil {
		return 0, err
	}
	if status != 201 {
		return 0, fmt.Errorf("create campaign: status %d, body: %s", status, data)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	id, _ := resp["id"].(float64)
	return int64(id), nil
}

func (c *DSPClient) CreateCreative(req CreativeRequest) (int64, error) {
	data, status, err := c.do("POST", "/api/v1/creatives", req)
	if err != nil {
		return 0, err
	}
	if status != 201 {
		return 0, fmt.Errorf("create creative: status %d, body: %s", status, data)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	id, _ := resp["id"].(float64)
	return int64(id), nil
}

func (c *DSPClient) StartCampaign(campaignID int64) error {
	_, status, err := c.do("POST", fmt.Sprintf("/api/v1/campaigns/%d/start", campaignID), nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("start campaign: status %d", status)
	}
	return nil
}

func (c *DSPClient) PauseCampaign(campaignID int64) error {
	_, status, err := c.do("POST", fmt.Sprintf("/api/v1/campaigns/%d/pause", campaignID), nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("pause campaign: status %d", status)
	}
	return nil
}

func (c *DSPClient) GetCampaign(campaignID int64) (*CampaignResponse, error) {
	data, status, err := c.do("GET", fmt.Sprintf("/api/v1/campaigns/%d", campaignID), nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("get campaign: status %d", status)
	}
	var resp CampaignResponse
	json.Unmarshal(data, &resp)
	return &resp, nil
}

func (c *DSPClient) ListCampaigns() ([]CampaignResponse, error) {
	data, status, err := c.do("GET", "/api/v1/campaigns", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("list campaigns: status %d", status)
	}
	var resp []CampaignResponse
	json.Unmarshal(data, &resp)
	return resp, nil
}

func (c *DSPClient) GetOverviewStats() (*OverviewStats, error) {
	data, status, err := c.do("GET", "/api/v1/reports/overview", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("overview stats: status %d", status)
	}
	var resp OverviewStats
	json.Unmarshal(data, &resp)
	return &resp, nil
}

func (c *DSPClient) GetCampaignStats(campaignID int64) (*CampaignStats, error) {
	data, status, err := c.do("GET", fmt.Sprintf("/api/v1/reports/campaign/%d/stats", campaignID), nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("campaign stats: status %d", status)
	}
	var resp CampaignStats
	json.Unmarshal(data, &resp)
	return &resp, nil
}

func (c *DSPClient) UpdateCampaign(campaignID int64, updates map[string]any) error {
	_, status, err := c.do("PUT", fmt.Sprintf("/api/v1/campaigns/%d", campaignID), updates)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("update campaign: status %d", status)
	}
	return nil
}

func (c *DSPClient) GetBalance(advertiserID int64) (int64, error) {
	data, status, err := c.do("GET", fmt.Sprintf("/api/v1/billing/balance/%d", advertiserID), nil)
	if err != nil {
		return 0, err
	}
	if status != 200 {
		return 0, fmt.Errorf("balance: status %d", status)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	balance, _ := resp["balance_cents"].(float64)
	return int64(balance), nil
}

// TriggerExchangeSim sends traffic via the exchange simulator.
// mode: "single", "burst", or "load"
func (c *DSPClient) TriggerExchangeSim(simURL, mode string, params map[string]string) (map[string]any, error) {
	url := simURL + "/" + mode
	if len(params) > 0 {
		url += "?"
		for k, v := range params {
			url += k + "=" + v + "&"
		}
	}
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("exchange-sim %s: %w", mode, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

// HealthCheck checks if a service is responding.
func (c *DSPClient) HealthCheck(url string) error {
	resp, err := c.client.Get(url + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("health check %s: status %d", url, resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run TestClient -v`
Expected: PASS (4 tests)

- [ ] **Step 5: Commit**

```bash
git add cmd/autopilot/config.go cmd/autopilot/client.go cmd/autopilot/client_test.go
git commit -m "feat(autopilot): add config and DSP API client"
```

---

### Task 4: Browser Screenshot Module

**Files:**
- Create: `cmd/autopilot/browser.go`
- Modify: `go.mod` — add chromedp dependency

- [ ] **Step 1: Add chromedp dependency**

Run: `cd /c/Users/Roc/github/dsp && go get github.com/chromedp/chromedp@latest`

- [ ] **Step 2: Write browser module**

```go
// cmd/autopilot/browser.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser wraps chromedp for taking screenshots of the DSP frontend.
type Browser struct {
	frontendURL   string
	apiKey        string
	screenshotDir string
	allocCtx      context.Context
	allocCancel   context.CancelFunc
}

func NewBrowser(frontendURL, apiKey, screenshotDir string) *Browser {
	return &Browser{
		frontendURL:   frontendURL,
		apiKey:        apiKey,
		screenshotDir: screenshotDir,
	}
}

func (b *Browser) Start() error {
	os.MkdirAll(b.screenshotDir, 0o755)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1440, 900),
	)
	b.allocCtx, b.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	return nil
}

func (b *Browser) Stop() {
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// Screenshot navigates to a page, injects the API key into localStorage,
// waits for the page to load, and saves a full-page screenshot.
func (b *Browser) Screenshot(name, path string) (string, error) {
	ctx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filename := filepath.Join(b.screenshotDir, name+".png")
	var buf []byte

	url := b.frontendURL + path

	err := chromedp.Run(ctx,
		// First navigate to set localStorage
		chromedp.Navigate(b.frontendURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, exp, err := chromedp.Evaluate(
				fmt.Sprintf(`localStorage.setItem("dsp_api_key", "%s")`, b.apiKey),
				nil,
			).Do(ctx)
			if exp != nil {
				return fmt.Errorf("js exception: %v", exp)
			}
			return err
		}),
		// Navigate to target page
		chromedp.Navigate(url),
		chromedp.Sleep(2*time.Second),
		// Wait for content to render (page no longer in loading state)
		chromedp.WaitReady("body"),
		chromedp.Sleep(1*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return "", fmt.Errorf("screenshot %s: %w", name, err)
	}

	if err := os.WriteFile(filename, buf, 0o644); err != nil {
		return "", fmt.Errorf("save screenshot %s: %w", name, err)
	}

	log.Printf("[SCREENSHOT] %s -> %s (%d bytes)", name, filename, len(buf))
	return filename, nil
}

// ScreenshotGrafana takes a screenshot of a Grafana dashboard.
func (b *Browser) ScreenshotGrafana(name, grafanaURL, dashboardPath string) (string, error) {
	ctx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filename := filepath.Join(b.screenshotDir, name+".png")
	var buf []byte

	err := chromedp.Run(ctx,
		chromedp.Navigate(grafanaURL+dashboardPath),
		chromedp.Sleep(3*time.Second),
		chromedp.WaitReady("body"),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return "", fmt.Errorf("grafana screenshot %s: %w", name, err)
	}

	os.WriteFile(filename, buf, 0o644)
	log.Printf("[SCREENSHOT] %s -> %s (%d bytes)", name, filename, len(buf))
	return filename, nil
}
```

- [ ] **Step 3: Commit**

```bash
git add cmd/autopilot/browser.go go.mod go.sum
git commit -m "feat(autopilot): add chromedp browser screenshot module"
```

---

### Task 5: HTML Report Generator

**Files:**
- Create: `cmd/autopilot/report.go`
- Create: `cmd/autopilot/report.html`
- Create: `cmd/autopilot/report_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/autopilot/report_test.go
package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
	steps := []StepResult{
		{
			Name:       "Create Advertiser",
			Passed:     true,
			Duration:   150 * time.Millisecond,
			Detail:     "Created advertiser id=1",
			Screenshot: "",
		},
		{
			Name:       "Top Up Balance",
			Passed:     true,
			Duration:   80 * time.Millisecond,
			Detail:     "Balance: 100000 cents",
			Screenshot: "topup.png",
		},
		{
			Name:     "Budget Exhaustion",
			Passed:   false,
			Duration: 2 * time.Second,
			Detail:   "Campaign did not auto-pause within timeout",
			Error:    "expected status paused, got active",
		},
	}

	report := &VerifyReport{
		StartTime: time.Now().Add(-5 * time.Minute),
		EndTime:   time.Now(),
		Steps:     steps,
	}

	tmpFile := os.TempDir() + "/test-report.html"
	defer os.Remove(tmpFile)

	err := GenerateHTMLReport(report, tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	html := string(data)

	if !strings.Contains(html, "Create Advertiser") {
		t.Error("report should contain step name")
	}
	if !strings.Contains(html, "PASS") {
		t.Error("report should contain PASS")
	}
	if !strings.Contains(html, "FAIL") {
		t.Error("report should contain FAIL for failed step")
	}
	if !strings.Contains(html, "topup.png") {
		t.Error("report should reference screenshot")
	}
}

func TestReportSummary(t *testing.T) {
	report := &VerifyReport{
		Steps: []StepResult{
			{Passed: true},
			{Passed: true},
			{Passed: false},
		},
	}
	passed, failed := report.Summary()
	if passed != 2 {
		t.Errorf("expected 2 passed, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run TestGenerate -v`
Expected: FAIL — types not defined

- [ ] **Step 3: Write HTML template**

```html
<!-- cmd/autopilot/report.html -->
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<title>DSP Autopilot Verify Report</title>
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { font-family: -apple-system, "Segoe UI", sans-serif; background: #f3f4f6; color: #111827; padding: 24px; }
  .container { max-width: 960px; margin: 0 auto; }
  h1 { font-size: 24px; font-weight: 700; margin-bottom: 8px; }
  .meta { color: #6b7280; font-size: 13px; margin-bottom: 24px; }
  .summary { display: flex; gap: 16px; margin-bottom: 24px; }
  .summary-card { background: #fff; border-radius: 8px; padding: 16px 24px; flex: 1; }
  .summary-card .label { font-size: 12px; color: #6b7280; text-transform: uppercase; }
  .summary-card .value { font-size: 28px; font-weight: 700; margin-top: 4px; }
  .value.pass { color: #059669; }
  .value.fail { color: #dc2626; }
  .step { background: #fff; border-radius: 8px; padding: 20px; margin-bottom: 12px; }
  .step-header { display: flex; align-items: center; gap: 12px; margin-bottom: 8px; }
  .badge { padding: 2px 10px; border-radius: 4px; font-size: 12px; font-weight: 600; }
  .badge.pass { background: #d1fae5; color: #065f46; }
  .badge.fail { background: #fee2e2; color: #991b1b; }
  .step-name { font-size: 16px; font-weight: 600; }
  .step-duration { color: #6b7280; font-size: 13px; margin-left: auto; }
  .step-detail { font-size: 14px; color: #374151; margin-bottom: 8px; }
  .step-error { font-size: 13px; color: #dc2626; background: #fef2f2; padding: 8px 12px; border-radius: 4px; margin-bottom: 8px; }
  .screenshot { margin-top: 12px; }
  .screenshot img { max-width: 100%; border: 1px solid #e5e7eb; border-radius: 4px; }
</style>
</head>
<body>
<div class="container">
  <h1>DSP Autopilot Verify Report</h1>
  <div class="meta">
    {{.StartTime.Format "2006-01-02 15:04:05"}} — {{.EndTime.Format "2006-01-02 15:04:05"}}
    ({{.DurationStr}})
  </div>

  <div class="summary">
    <div class="summary-card">
      <div class="label">Total Steps</div>
      <div class="value">{{.TotalSteps}}</div>
    </div>
    <div class="summary-card">
      <div class="label">Passed</div>
      <div class="value pass">{{.PassedCount}}</div>
    </div>
    <div class="summary-card">
      <div class="label">Failed</div>
      <div class="value fail">{{.FailedCount}}</div>
    </div>
  </div>

  {{range .Steps}}
  <div class="step">
    <div class="step-header">
      {{if .Passed}}<span class="badge pass">PASS</span>{{else}}<span class="badge fail">FAIL</span>{{end}}
      <span class="step-name">{{.Name}}</span>
      <span class="step-duration">{{.Duration}}</span>
    </div>
    {{if .Detail}}<div class="step-detail">{{.Detail}}</div>{{end}}
    {{if .Error}}<div class="step-error">{{.Error}}</div>{{end}}
    {{if .Screenshot}}
    <div class="screenshot">
      <img src="{{.Screenshot}}" alt="{{.Name}}">
    </div>
    {{end}}
  </div>
  {{end}}
</div>
</body>
</html>
```

- [ ] **Step 4: Write report generator**

```go
// cmd/autopilot/report.go
package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

//go:embed report.html
var reportTemplateFS embed.FS

type StepResult struct {
	Name       string
	Passed     bool
	Duration   time.Duration
	Detail     string
	Error      string
	Screenshot string // relative path to screenshot file
}

type VerifyReport struct {
	StartTime time.Time
	EndTime   time.Time
	Steps     []StepResult
}

func (r *VerifyReport) Summary() (passed, failed int) {
	for _, s := range r.Steps {
		if s.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

// Template data with computed fields
type reportData struct {
	StartTime   time.Time
	EndTime     time.Time
	DurationStr string
	TotalSteps  int
	PassedCount int
	FailedCount int
	Steps       []StepResult
}

func GenerateHTMLReport(report *VerifyReport, outputPath string) error {
	tmplData, _ := reportTemplateFS.ReadFile("report.html")
	tmpl, err := template.New("report").Parse(string(tmplData))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	passed, failed := report.Summary()
	data := reportData{
		StartTime:   report.StartTime,
		EndTime:     report.EndTime,
		DurationStr: report.EndTime.Sub(report.StartTime).Round(time.Second).String(),
		TotalSteps:  len(report.Steps),
		PassedCount: passed,
		FailedCount: failed,
		Steps:       report.Steps,
	}

	os.MkdirAll(filepath.Dir(outputPath), 0o755)
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run "TestGenerate|TestReport" -v`
Expected: PASS (2 tests)

- [ ] **Step 6: Commit**

```bash
git add cmd/autopilot/report.go cmd/autopilot/report.html cmd/autopilot/report_test.go
git commit -m "feat(autopilot): add HTML report generator with embedded template"
```

---

### Task 6: Verify Mode — Normal Flow Scenarios

**Files:**
- Create: `cmd/autopilot/scenarios.go`
- Create: `cmd/autopilot/scenarios_test.go`

- [ ] **Step 1: Write the failing test**

```go
// cmd/autopilot/scenarios_test.go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunNormalScenarios(t *testing.T) {
	var callCount atomic.Int32

	// Mock DSP API
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v1/advertisers":
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"id": 1, "api_key": "dsp_auto"})
		case r.Method == "POST" && r.URL.Path == "/api/v1/billing/topup":
			json.NewEncoder(w).Encode(map[string]any{"balance_cents": 100000})
		case r.Method == "POST" && r.URL.Path == "/api/v1/campaigns":
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"id": 10})
		case r.Method == "POST" && r.URL.Path == "/api/v1/creatives":
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(map[string]any{"id": 20})
		case r.Method == "POST" && r.URL.Path == "/api/v1/campaigns/10/start":
			json.NewEncoder(w).Encode(map[string]any{"status": "active"})
		case r.Method == "POST" && r.URL.Path == "/api/v1/campaigns/10/pause":
			json.NewEncoder(w).Encode(map[string]any{"status": "paused"})
		case r.Method == "GET" && r.URL.Path == "/api/v1/campaigns/10":
			json.NewEncoder(w).Encode(map[string]any{"id": 10, "status": "active"})
		case r.Method == "GET" && r.URL.Path == "/api/v1/reports/overview":
			json.NewEncoder(w).Encode(map[string]any{"today_impressions": 50, "today_clicks": 5, "today_spend_cents": 500})
		case r.Method == "GET" && r.URL.Path == "/api/v1/reports/campaign/10/stats":
			json.NewEncoder(w).Encode(map[string]any{"impressions": 50, "clicks": 5, "spend_cents": 500})
		case r.URL.Path == "/health":
			w.Write([]byte(`{"status":"ok"}`))
		default:
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	defer apiSrv.Close()

	// Mock exchange-sim
	simSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"total": 100, "bids": 80})
	}))
	defer simSrv.Close()

	runner := &ScenarioRunner{
		client:         NewDSPClient(apiSrv.URL, ""),
		exchangeSimURL: simSrv.URL,
		browser:        nil, // no browser in unit test
		trafficWait:    1 * time.Second,
	}

	steps := runner.RunNormalFlow()

	if len(steps) < 5 {
		t.Fatalf("expected at least 5 steps, got %d", len(steps))
	}

	// First step should be create advertiser
	if steps[0].Name != "Create Advertiser" {
		t.Errorf("first step should be 'Create Advertiser', got %s", steps[0].Name)
	}
	if !steps[0].Passed {
		t.Errorf("create advertiser should pass: %s", steps[0].Error)
	}

	if callCount.Load() < 5 {
		t.Errorf("expected at least 5 API calls, got %d", callCount.Load())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run TestRunNormal -v`
Expected: FAIL — ScenarioRunner not defined

- [ ] **Step 3: Write scenario runner**

```go
// cmd/autopilot/scenarios.go
package main

import (
	"fmt"
	"log"
	"time"
)

// ScenarioRunner executes verify mode scenarios.
type ScenarioRunner struct {
	client         *DSPClient
	exchangeSimURL string
	browser        *Browser
	grafanaURL     string
	trafficWait    time.Duration

	// State populated during run
	advertiserID int64
	apiKey       string
	campaignID   int64
	creativeID   int64
}

func (s *ScenarioRunner) runStep(name string, fn func() (string, error)) StepResult {
	log.Printf("[STEP] %s ...", name)
	start := time.Now()
	detail, err := fn()
	duration := time.Since(start)

	result := StepResult{
		Name:     name,
		Duration: duration,
		Detail:   detail,
		Passed:   err == nil,
	}
	if err != nil {
		result.Error = err.Error()
		log.Printf("[FAIL] %s: %v (%.1fs)", name, err, duration.Seconds())
	} else {
		log.Printf("[PASS] %s (%.1fs)", name, duration.Seconds())
	}
	return result
}

func (s *ScenarioRunner) screenshot(name, path string) string {
	if s.browser == nil {
		return ""
	}
	file, err := s.browser.Screenshot(name, path)
	if err != nil {
		log.Printf("[WARN] screenshot failed for %s: %v", name, err)
		return ""
	}
	return file
}

func (s *ScenarioRunner) screenshotGrafana(name, dashPath string) string {
	if s.browser == nil || s.grafanaURL == "" {
		return ""
	}
	file, err := s.browser.ScreenshotGrafana(name, s.grafanaURL, dashPath)
	if err != nil {
		log.Printf("[WARN] grafana screenshot failed for %s: %v", name, err)
		return ""
	}
	return file
}

// RunNormalFlow executes steps 1-6: create advertiser, top up, create campaign, start, traffic, pause.
func (s *ScenarioRunner) RunNormalFlow() []StepResult {
	var steps []StepResult

	// Step 1: Create Advertiser
	step := s.runStep("Create Advertiser", func() (string, error) {
		adv, err := s.client.CreateAdvertiser(
			fmt.Sprintf("Autopilot Test %s", time.Now().Format("0102-1504")),
			"autopilot@test.local",
		)
		if err != nil {
			return "", err
		}
		s.advertiserID = adv.ID
		s.apiKey = adv.APIKey
		s.client.APIKey = adv.APIKey
		return fmt.Sprintf("Advertiser id=%d, api_key=%s", adv.ID, adv.APIKey), nil
	})
	step.Screenshot = s.screenshot("01-dashboard-empty", "/")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 2: Top Up Balance
	step = s.runStep("Top Up Balance", func() (string, error) {
		balance, err := s.client.TopUp(s.advertiserID, 10000000, "autopilot verify top-up")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Balance: %d cents (%.2f CNY)", balance, float64(balance)/100), nil
	})
	step.Screenshot = s.screenshot("02-billing-topup", "/billing")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 3: Create Campaign + Creative
	step = s.runStep("Create Campaign + Creative", func() (string, error) {
		cid, err := s.client.CreateCampaign(CampaignRequest{
			AdvertiserID:     s.advertiserID,
			Name:             "Autopilot Verify Campaign",
			BillingModel:     "cpm",
			BudgetTotalCents: 5000000, // 50,000 CNY
			BudgetDailyCents: 1000000, // 10,000 CNY
			BidCPMCents:      500,     // 5 CNY CPM
		})
		if err != nil {
			return "", fmt.Errorf("create campaign: %w", err)
		}
		s.campaignID = cid

		crID, err := s.client.CreateCreative(CreativeRequest{
			CampaignID:     cid,
			Name:           "Autopilot Test Banner",
			AdType:         "banner",
			Format:         "image",
			Size:           "300x250",
			AdMarkup:       `<div style="width:300px;height:250px;background:#2563eb;color:#fff;display:flex;align-items:center;justify-content:center;font-size:18px">Autopilot Test Ad</div>`,
			DestinationURL: "https://example.com/landing",
		})
		if err != nil {
			return "", fmt.Errorf("create creative: %w", err)
		}
		s.creativeID = crID

		return fmt.Sprintf("Campaign id=%d, Creative id=%d", cid, crID), nil
	})
	step.Screenshot = s.screenshot("03-campaign-detail", fmt.Sprintf("/campaigns/%d", s.campaignID))
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 4: Start Campaign + Send Traffic
	step = s.runStep("Start Campaign + Send Traffic", func() (string, error) {
		if err := s.client.StartCampaign(s.campaignID); err != nil {
			return "", fmt.Errorf("start campaign: %w", err)
		}

		// Send burst traffic via exchange-sim
		result, err := s.client.TriggerExchangeSim(s.exchangeSimURL, "burst", nil)
		if err != nil {
			return "", fmt.Errorf("exchange-sim burst: %w", err)
		}

		bids, _ := result["bids"].(float64)
		total, _ := result["total"].(float64)
		return fmt.Sprintf("Campaign started. Burst: %d/%d bids", int(bids), int(total)), nil
	})
	step.Screenshot = s.screenshot("04-analytics-live", "/analytics")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 5: Wait for Data + Check Reports
	step = s.runStep("Wait for Data Accumulation", func() (string, error) {
		log.Printf("[INFO] Waiting %s for data pipeline...", s.trafficWait)
		time.Sleep(s.trafficWait)

		stats, err := s.client.GetCampaignStats(s.campaignID)
		if err != nil {
			return "", fmt.Errorf("get stats: %w", err)
		}

		overview, _ := s.client.GetOverviewStats()

		detail := fmt.Sprintf("Campaign stats: impressions=%d clicks=%d spend=%d cents",
			stats.Impressions, stats.Clicks, stats.SpendCents)
		if overview != nil {
			detail += fmt.Sprintf("\nOverview: impressions=%d clicks=%d spend=%d cents",
				overview.TodayImpressions, overview.TodayClicks, overview.TodaySpendCents)
		}
		return detail, nil
	})
	step.Screenshot = s.screenshot("05-reports", fmt.Sprintf("/reports"))
	steps = append(steps, step)

	// Step 6: Pause Campaign
	step = s.runStep("Pause Campaign", func() (string, error) {
		if err := s.client.PauseCampaign(s.campaignID); err != nil {
			return "", err
		}
		camp, err := s.client.GetCampaign(s.campaignID)
		if err != nil {
			return "", err
		}
		if camp.Status != "paused" {
			return "", fmt.Errorf("expected status paused, got %s", camp.Status)
		}
		return fmt.Sprintf("Campaign %d paused successfully", s.campaignID), nil
	})
	step.Screenshot = s.screenshot("06-campaign-paused", "/campaigns")
	steps = append(steps, step)

	return steps
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run TestRunNormal -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add cmd/autopilot/scenarios.go cmd/autopilot/scenarios_test.go
git commit -m "feat(autopilot): implement verify normal flow scenarios (steps 1-6)"
```

---

### Task 7: Docker Fault Injection

**Files:**
- Create: `cmd/autopilot/fault.go`
- Modify: `go.mod` — add Docker client dependency

- [ ] **Step 1: Add Docker dependency**

Run: `cd /c/Users/Roc/github/dsp && go get github.com/docker/docker/client@latest && go get github.com/docker/docker/api/types@latest && go get github.com/docker/docker/api/types/container@latest`

- [ ] **Step 2: Write fault injection module**

```go
// cmd/autopilot/fault.go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// FaultInjector controls Docker containers for chaos testing.
type FaultInjector struct {
	docker *client.Client
}

func NewFaultInjector() (*FaultInjector, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}
	return &FaultInjector{docker: cli}, nil
}

func (f *FaultInjector) Close() {
	if f.docker != nil {
		f.docker.Close()
	}
}

// findContainer finds a container by name substring (e.g., "bidder", "consumer", "kafka").
func (f *FaultInjector) findContainer(ctx context.Context, nameSubstr string) (string, error) {
	containers, err := f.docker.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return "", fmt.Errorf("list containers: %w", err)
	}
	for _, c := range containers {
		for _, name := range c.Names {
			if strings.Contains(name, nameSubstr) {
				return c.ID, nil
			}
		}
	}
	return "", fmt.Errorf("container with name containing %q not found", nameSubstr)
}

// RestartContainer restarts a container and waits for it to come back.
func (f *FaultInjector) RestartContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Restarting container %s (%s)...", nameSubstr, id[:12])
	timeout := 10
	return f.docker.ContainerRestart(ctx, id, container.StopOptions{Timeout: &timeout})
}

// PauseContainer pauses a container (freezes all processes).
func (f *FaultInjector) PauseContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Pausing container %s (%s)...", nameSubstr, id[:12])
	return f.docker.ContainerPause(ctx, id)
}

// UnpauseContainer resumes a paused container.
func (f *FaultInjector) UnpauseContainer(ctx context.Context, nameSubstr string) error {
	id, err := f.findContainer(ctx, nameSubstr)
	if err != nil {
		return err
	}
	log.Printf("[FAULT] Unpausing container %s (%s)...", nameSubstr, id[:12])
	return f.docker.ContainerUnpause(ctx, id)
}

// WaitForHealthy polls a health URL until it responds 200 or timeout.
func WaitForHealthy(url string, timeout time.Duration) (time.Duration, error) {
	start := time.Now()
	deadline := time.After(timeout)
	c := &DSPClient{client: defaultHTTPClient()}
	for {
		select {
		case <-deadline:
			return time.Since(start), fmt.Errorf("service at %s not healthy after %s", url, timeout)
		default:
			if err := c.HealthCheck(url); err == nil {
				return time.Since(start), nil
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Second}
}
```

Note: add `import "net/http"` to the imports.

- [ ] **Step 3: Commit**

```bash
git add cmd/autopilot/fault.go go.mod go.sum
git commit -m "feat(autopilot): add Docker fault injection module"
```

---

### Task 8: Verify Mode — Fault Injection Scenarios

**Files:**
- Modify: `cmd/autopilot/scenarios.go` — add RunFaultScenarios method

- [ ] **Step 1: Add fault scenarios to ScenarioRunner**

Append to `cmd/autopilot/scenarios.go`:

```go
// RunFaultScenarios executes steps 7-9: budget exhaustion, service restart, Kafka delay.
// Requires a running Docker environment and an active campaign from RunNormalFlow.
func (s *ScenarioRunner) RunFaultScenarios(faultInjector *FaultInjector) []StepResult {
	var steps []StepResult
	ctx := context.Background()

	// Step 7: Budget Exhaustion
	step := s.runStep("Fault: Budget Exhaustion", func() (string, error) {
		// Set campaign budget to very low value
		err := s.client.UpdateCampaign(s.campaignID, map[string]any{
			"budget_daily_cents": 100, // 1 CNY daily budget
		})
		if err != nil {
			return "", fmt.Errorf("set low budget: %w", err)
		}

		// Restart campaign
		s.client.StartCampaign(s.campaignID)

		// Send heavy traffic to exhaust budget
		_, err = s.client.TriggerExchangeSim(s.exchangeSimURL, "load", map[string]string{
			"qps": "100", "duration": "5",
		})
		if err != nil {
			return "", fmt.Errorf("trigger load: %w", err)
		}

		// Wait for auto-pause to kick in
		time.Sleep(10 * time.Second)

		camp, err := s.client.GetCampaign(s.campaignID)
		if err != nil {
			return "", fmt.Errorf("get campaign: %w", err)
		}

		balance, _ := s.client.GetBalance(s.advertiserID)
		detail := fmt.Sprintf("Campaign status: %s, Remaining balance: %d cents", camp.Status, balance)

		if camp.Status != "paused" {
			return detail, fmt.Errorf("expected campaign to auto-pause, got status=%s", camp.Status)
		}
		return detail, nil
	})
	step.Screenshot = s.screenshot("07-budget-exhausted", "/campaigns")
	steps = append(steps, step)

	// Step 8: Service Restart (bidder)
	step = s.runStep("Fault: Bidder Service Restart", func() (string, error) {
		if faultInjector == nil {
			return "skipped (no Docker access)", nil
		}

		// Take pre-restart screenshot of Grafana
		s.screenshotGrafana("08a-grafana-before", "/d/dsp-overview")

		if err := faultInjector.RestartContainer(ctx, "bidder"); err != nil {
			return "", fmt.Errorf("restart bidder: %w", err)
		}

		// Wait for recovery
		recoveryTime, err := WaitForHealthy("http://localhost:8180", 60*time.Second)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Bidder restarted. Recovery time: %s", recoveryTime.Round(time.Millisecond)), nil
	})
	step.Screenshot = s.screenshotGrafana("08b-grafana-after", "/d/dsp-overview")
	steps = append(steps, step)

	// Step 9: Kafka Delay (pause consumer)
	step = s.runStep("Fault: Kafka Consumer Delay", func() (string, error) {
		if faultInjector == nil {
			return "skipped (no Docker access)", nil
		}

		// Pause consumer to simulate Kafka lag
		if err := faultInjector.PauseContainer(ctx, "consumer"); err != nil {
			return "", fmt.Errorf("pause consumer: %w", err)
		}

		// Restart campaign with fresh budget for traffic
		s.client.UpdateCampaign(s.campaignID, map[string]any{"budget_daily_cents": 1000000})
		s.client.StartCampaign(s.campaignID)

		// Send traffic while consumer is paused
		s.client.TriggerExchangeSim(s.exchangeSimURL, "burst", nil)
		log.Printf("[INFO] Consumer paused — messages accumulating in Kafka...")
		time.Sleep(15 * time.Second)

		// Resume consumer
		if err := faultInjector.UnpauseContainer(ctx, "consumer"); err != nil {
			return "", fmt.Errorf("unpause consumer: %w", err)
		}

		// Wait for data to catch up
		log.Printf("[INFO] Consumer resumed — waiting for catch-up...")
		time.Sleep(10 * time.Second)

		stats, err := s.client.GetCampaignStats(s.campaignID)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Consumer caught up. Impressions: %d, Clicks: %d", stats.Impressions, stats.Clicks), nil
	})
	step.Screenshot = s.screenshot("09-analytics-catchup", "/analytics")
	steps = append(steps, step)

	return steps
}
```

Add `"context"` and `"time"` to the imports in `scenarios.go` if not already present.

- [ ] **Step 2: Commit**

```bash
git add cmd/autopilot/scenarios.go
git commit -m "feat(autopilot): add fault injection scenarios (budget, restart, kafka)"
```

---

### Task 9: CLI Entry Point + Verify Command

**Files:**
- Create: `cmd/autopilot/main.go`

- [ ] **Step 1: Write main.go**

```go
// cmd/autopilot/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: autopilot <verify|run>")
		fmt.Println("")
		fmt.Println("  verify  — Run full E2E verification, produce HTML report")
		fmt.Println("  run     — Start continuous simulation mode (24/7)")
		os.Exit(1)
	}

	cfg := LoadAutopilotConfig()

	switch os.Args[1] {
	case "verify":
		runVerify(cfg)
	case "run":
		runContinuous(cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func newAlertSender(cfg *AutopilotConfig) alert.Sender {
	if cfg.WebhookURL == "" {
		return alert.Noop{}
	}
	switch cfg.WebhookType {
	case "feishu":
		return alert.NewFeishu(cfg.WebhookURL)
	default:
		return alert.NewDingTalk(cfg.WebhookURL)
	}
}

func runVerify(cfg *AutopilotConfig) {
	log.Println("=== DSP Autopilot: Verify Mode ===")
	log.Println("")

	alerter := newAlertSender(cfg)
	client := NewDSPClient(cfg.APIURL, "")
	client.AdminToken = cfg.AdminToken

	// Pre-flight: check services are running
	log.Println("[PRE-FLIGHT] Checking services...")
	services := map[string]string{
		"API":          cfg.APIURL,
		"Exchange-Sim": cfg.ExchangeSimURL,
	}
	for name, url := range services {
		if err := client.HealthCheck(url); err != nil {
			log.Fatalf("[PRE-FLIGHT] %s at %s is not reachable: %v", name, url, err)
		}
		log.Printf("[PRE-FLIGHT] %s OK", name)
	}

	// Start browser
	var browser *Browser
	browser = NewBrowser(cfg.FrontendURL, "", cfg.ScreenshotDir)
	if err := browser.Start(); err != nil {
		log.Printf("[WARN] Browser not available, screenshots disabled: %v", err)
		browser = nil
	} else {
		defer browser.Stop()
	}

	// Build scenario runner
	runner := &ScenarioRunner{
		client:         client,
		exchangeSimURL: cfg.ExchangeSimURL,
		browser:        browser,
		grafanaURL:     cfg.GrafanaURL,
		trafficWait:    cfg.TrafficDuration,
	}

	report := &VerifyReport{
		StartTime: time.Now(),
	}

	// Run normal flow (steps 1-6)
	log.Println("")
	log.Println("=== Normal Flow ===")
	normalSteps := runner.RunNormalFlow()
	report.Steps = append(report.Steps, normalSteps...)

	// Update browser with the new API key
	if browser != nil && runner.apiKey != "" {
		browser.apiKey = runner.apiKey
	}

	// Run fault scenarios (steps 7-9)
	log.Println("")
	log.Println("=== Fault Injection ===")
	faultInjector, err := NewFaultInjector()
	if err != nil {
		log.Printf("[WARN] Docker not available, fault injection will be skipped: %v", err)
	} else {
		defer faultInjector.Close()
	}
	faultSteps := runner.RunFaultScenarios(faultInjector)
	report.Steps = append(report.Steps, faultSteps...)

	// Generate report
	report.EndTime = time.Now()
	reportFile := filepath.Join(cfg.ReportDir,
		fmt.Sprintf("verify-%s.html", time.Now().Format("2006-01-02-150405")))

	if err := GenerateHTMLReport(report, reportFile); err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	passed, failed := report.Summary()
	log.Println("")
	log.Println("=== Verify Complete ===")
	log.Printf("Results: %d passed, %d failed", passed, failed)
	log.Printf("Report: %s", reportFile)

	// Send alert summary
	alerter.Send("Autopilot Verify Complete",
		fmt.Sprintf("Passed: %d / Failed: %d\nReport: %s", passed, failed, reportFile))

	if failed > 0 {
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build to verify compilation**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/autopilot/`
Expected: Build succeeds (the `runContinuous` function will be a stub until Task 10)

- [ ] **Step 3: Add stub for runContinuous** (will be implemented in Task 10)

Append to `cmd/autopilot/main.go` (temporary, replaced in Task 10):

```go
// runContinuous is implemented in continuous.go
```

Note: Actually, `runContinuous` will be defined in `continuous.go` in Task 10. For now, just make sure it compiles by having the function signature exist. Add a placeholder in main.go only if `continuous.go` doesn't exist yet:

```go
// Add to main.go temporarily if continuous.go doesn't exist yet:
func runContinuous(cfg *AutopilotConfig) {
	log.Fatal("continuous mode not yet implemented")
}
```

- [ ] **Step 4: Commit**

```bash
git add cmd/autopilot/main.go
git commit -m "feat(autopilot): add CLI entry point with verify command"
```

---

### Task 10: Continuous Mode

**Files:**
- Create: `cmd/autopilot/continuous.go`
- Create: `cmd/autopilot/continuous_test.go`
- Modify: `cmd/autopilot/main.go` — remove `runContinuous` stub if present

- [ ] **Step 1: Write the failing test**

```go
// cmd/autopilot/continuous_test.go
package main

import (
	"testing"
	"time"
)

func TestTrafficCurve(t *testing.T) {
	sim := &ContinuousSimulator{
		dayStartHour: 8,
		dayEndHour:   22,
		dayQPS:       100,
		nightQPS:     5,
	}

	// 14:00 should be daytime
	dayTime := time.Date(2026, 4, 12, 14, 0, 0, 0, time.Local)
	qps := sim.currentQPS(dayTime)
	if qps != 100 {
		t.Errorf("14:00 should be day QPS=100, got %d", qps)
	}

	// 03:00 should be nighttime
	nightTime := time.Date(2026, 4, 12, 3, 0, 0, 0, time.Local)
	qps = sim.currentQPS(nightTime)
	if qps != 5 {
		t.Errorf("03:00 should be night QPS=5, got %d", qps)
	}

	// 08:00 boundary should be daytime
	boundaryTime := time.Date(2026, 4, 12, 8, 0, 0, 0, time.Local)
	qps = sim.currentQPS(boundaryTime)
	if qps != 100 {
		t.Errorf("08:00 should be day QPS=100, got %d", qps)
	}
}

func TestShouldGenerateReport(t *testing.T) {
	sim := &ContinuousSimulator{reportHour: 9}

	// 09:00 on the dot -> should report
	at := time.Date(2026, 4, 12, 9, 0, 30, 0, time.Local)
	lastReport := time.Date(2026, 4, 11, 9, 0, 0, 0, time.Local)

	if !sim.shouldGenerateReport(at, lastReport) {
		t.Error("should generate report at 09:00 if last report was yesterday")
	}

	// Same day -> should not report again
	lastReport = time.Date(2026, 4, 12, 9, 0, 0, 0, time.Local)
	if sim.shouldGenerateReport(at, lastReport) {
		t.Error("should not generate report twice on same day")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run "TestTraffic|TestShould" -v`
Expected: FAIL — ContinuousSimulator not defined

- [ ] **Step 3: Write continuous mode implementation**

```go
// cmd/autopilot/continuous.go
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
)

type ContinuousSimulator struct {
	client         *DSPClient
	exchangeSimURL string
	browser        *Browser
	grafanaURL     string
	alerter        alert.Sender

	dayStartHour   int
	dayEndHour     int
	dayQPS         int
	nightQPS       int
	healthInterval time.Duration
	reportHour     int
	reportDir      string

	// State
	advertiserID int64
	apiKey       string
	campaignIDs  []int64
}

func (s *ContinuousSimulator) currentQPS(now time.Time) int {
	hour := now.Hour()
	if hour >= s.dayStartHour && hour < s.dayEndHour {
		return s.dayQPS
	}
	return s.nightQPS
}

func (s *ContinuousSimulator) shouldGenerateReport(now time.Time, lastReport time.Time) bool {
	if now.Hour() != s.reportHour {
		return false
	}
	return now.Day() != lastReport.Day() || now.Month() != lastReport.Month()
}

func (s *ContinuousSimulator) Run(ctx context.Context) {
	log.Println("[CONTINUOUS] Starting continuous simulation...")

	// Setup: create advertiser and initial campaign
	if err := s.setup(); err != nil {
		log.Fatalf("[CONTINUOUS] Setup failed: %v", err)
	}

	trafficTicker := time.NewTicker(10 * time.Second)
	healthTicker := time.NewTicker(s.healthInterval)
	operationTicker := time.NewTicker(5 * time.Minute)
	reportCheck := time.NewTicker(1 * time.Minute)
	defer trafficTicker.Stop()
	defer healthTicker.Stop()
	defer operationTicker.Stop()
	defer reportCheck.Stop()

	lastReport := time.Time{}

	for {
		select {
		case <-ctx.Done():
			log.Println("[CONTINUOUS] Shutting down...")
			return

		case <-trafficTicker.C:
			qps := s.currentQPS(time.Now())
			if qps > 0 {
				go s.sendTraffic(qps)
			}

		case <-healthTicker.C:
			go s.checkHealth()

		case <-operationTicker.C:
			go s.randomOperation()

		case <-reportCheck.C:
			now := time.Now()
			if s.shouldGenerateReport(now, lastReport) {
				s.generateDailyReport()
				lastReport = now
			}
		}
	}
}

func (s *ContinuousSimulator) setup() error {
	adv, err := s.client.CreateAdvertiser(
		fmt.Sprintf("Autopilot Continuous %s", time.Now().Format("0102")),
		"autopilot-continuous@test.local",
	)
	if err != nil {
		return fmt.Errorf("create advertiser: %w", err)
	}
	s.advertiserID = adv.ID
	s.apiKey = adv.APIKey
	s.client.APIKey = adv.APIKey

	if s.browser != nil {
		s.browser.apiKey = adv.APIKey
	}

	// Top up
	_, err = s.client.TopUp(adv.ID, 100000000, "autopilot continuous initial")
	if err != nil {
		return fmt.Errorf("topup: %w", err)
	}

	// Create initial campaign
	cid, err := s.client.CreateCampaign(CampaignRequest{
		AdvertiserID:     adv.ID,
		Name:             "Continuous Campaign 1",
		BillingModel:     "cpm",
		BudgetTotalCents: 10000000,
		BudgetDailyCents: 1000000,
		BidCPMCents:      500,
	})
	if err != nil {
		return fmt.Errorf("create campaign: %w", err)
	}
	s.campaignIDs = append(s.campaignIDs, cid)

	// Create creative
	s.client.CreateCreative(CreativeRequest{
		CampaignID:     cid,
		Name:           "Continuous Banner",
		AdType:         "banner",
		Format:         "image",
		Size:           "300x250",
		AdMarkup:       `<div style="width:300px;height:250px;background:#2563eb;color:#fff;display:flex;align-items:center;justify-content:center">Continuous Test</div>`,
		DestinationURL: "https://example.com",
	})

	// Start campaign
	s.client.StartCampaign(cid)
	log.Printf("[CONTINUOUS] Setup complete: advertiser=%d, campaign=%d", adv.ID, cid)
	return nil
}

func (s *ContinuousSimulator) sendTraffic(qps int) {
	_, err := s.client.TriggerExchangeSim(s.exchangeSimURL, "load", map[string]string{
		"qps":      fmt.Sprintf("%d", qps),
		"duration": "9",
	})
	if err != nil {
		log.Printf("[TRAFFIC] Error: %v", err)
	}
}

func (s *ContinuousSimulator) checkHealth() {
	services := map[string]string{
		"API":     "http://localhost:8181",
		"Bidder":  "http://localhost:8180",
	}
	for name, url := range services {
		if err := s.client.HealthCheck(url); err != nil {
			msg := fmt.Sprintf("Service %s is DOWN: %v", name, err)
			log.Printf("[HEALTH] ALERT: %s", msg)
			s.alerter.Send("DSP Service Down", msg)
		}
	}
}

func (s *ContinuousSimulator) randomOperation() {
	ops := []string{"create_campaign", "pause_campaign", "adjust_budget"}
	op := ops[rand.Intn(len(ops))]

	switch op {
	case "create_campaign":
		cid, err := s.client.CreateCampaign(CampaignRequest{
			AdvertiserID:     s.advertiserID,
			Name:             fmt.Sprintf("Auto Campaign %s", time.Now().Format("150405")),
			BillingModel:     "cpm",
			BudgetTotalCents: 5000000,
			BudgetDailyCents: 500000,
			BidCPMCents:      300 + rand.Intn(500),
		})
		if err == nil {
			s.campaignIDs = append(s.campaignIDs, cid)
			s.client.CreateCreative(CreativeRequest{
				CampaignID: cid, Name: "Auto Creative", AdType: "banner",
				Format: "image", Size: "300x250",
				AdMarkup:       `<div style="width:300px;height:250px;background:#059669;color:#fff;display:flex;align-items:center;justify-content:center">Auto</div>`,
				DestinationURL: "https://example.com",
			})
			s.client.StartCampaign(cid)
			log.Printf("[OP] Created and started campaign %d", cid)
		}

	case "pause_campaign":
		if len(s.campaignIDs) > 1 {
			idx := rand.Intn(len(s.campaignIDs))
			cid := s.campaignIDs[idx]
			s.client.PauseCampaign(cid)
			log.Printf("[OP] Paused campaign %d", cid)
		}

	case "adjust_budget":
		if len(s.campaignIDs) > 0 {
			cid := s.campaignIDs[rand.Intn(len(s.campaignIDs))]
			newBudget := 200000 + rand.Intn(1000000)
			s.client.UpdateCampaign(cid, map[string]any{"budget_daily_cents": newBudget})
			log.Printf("[OP] Adjusted campaign %d daily budget to %d", cid, newBudget)
		}
	}
}

func (s *ContinuousSimulator) generateDailyReport() {
	log.Println("[REPORT] Generating daily report...")

	overview, _ := s.client.GetOverviewStats()

	var steps []StepResult
	steps = append(steps, StepResult{
		Name:   "Daily Overview",
		Passed: true,
		Detail: fmt.Sprintf("Impressions: %d, Clicks: %d, Spend: %d cents",
			overview.TodayImpressions, overview.TodayClicks, overview.TodaySpendCents),
	})

	// Screenshot dashboard
	if s.browser != nil {
		ss, _ := s.browser.Screenshot("daily-dashboard", "/")
		steps[0].Screenshot = ss
	}

	// Campaign stats
	for _, cid := range s.campaignIDs {
		stats, err := s.client.GetCampaignStats(cid)
		if err != nil {
			continue
		}
		steps = append(steps, StepResult{
			Name:   fmt.Sprintf("Campaign %d Stats", cid),
			Passed: true,
			Detail: fmt.Sprintf("Impressions: %d, Clicks: %d, Spend: %d cents, WinRate: %.2f%%",
				stats.Impressions, stats.Clicks, stats.SpendCents, stats.WinRate*100),
		})
	}

	report := &VerifyReport{
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Steps:     steps,
	}

	reportFile := filepath.Join(s.reportDir,
		fmt.Sprintf("daily-%s.html", time.Now().Format("2006-01-02")))
	GenerateHTMLReport(report, reportFile)

	passed, _ := report.Summary()
	s.alerter.Send("DSP Daily Report",
		fmt.Sprintf("Date: %s\nCampaigns: %d\nReport: %s",
			time.Now().Format("2006-01-02"), len(s.campaignIDs), reportFile))

	log.Printf("[REPORT] Daily report: %d steps, all passed=%v, file=%s", len(steps), passed == len(steps), reportFile)
}

// runContinuous is called from main.go
func runContinuous(cfg *AutopilotConfig) {
	log.Println("=== DSP Autopilot: Continuous Mode ===")

	alerter := newAlertSender(cfg)
	client := NewDSPClient(cfg.APIURL, "")
	client.AdminToken = cfg.AdminToken

	var browser *Browser
	browser = NewBrowser(cfg.FrontendURL, "", cfg.ScreenshotDir)
	if err := browser.Start(); err != nil {
		log.Printf("[WARN] Browser not available: %v", err)
		browser = nil
	} else {
		defer browser.Stop()
	}

	sim := &ContinuousSimulator{
		client:         client,
		exchangeSimURL: cfg.ExchangeSimURL,
		browser:        browser,
		grafanaURL:     cfg.GrafanaURL,
		alerter:        alerter,
		dayStartHour:   cfg.DayStartHour,
		dayEndHour:     cfg.DayEndHour,
		dayQPS:         cfg.DayQPS,
		nightQPS:       cfg.NightQPS,
		healthInterval: cfg.HealthInterval,
		reportHour:     cfg.ReportHour,
		reportDir:      cfg.ReportDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go sim.Run(ctx)

	alerter.Send("Autopilot Started", "Continuous simulation mode activated")
	log.Println("[CONTINUOUS] Running. Press Ctrl+C to stop.")

	<-quit
	cancel()
	alerter.Send("Autopilot Stopped", "Continuous simulation mode deactivated")
	log.Println("[CONTINUOUS] Stopped.")
}
```

- [ ] **Step 4: Run tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -run "TestTraffic|TestShould" -v`
Expected: PASS (2 tests)

- [ ] **Step 5: Build to verify full compilation**

Run: `cd /c/Users/Roc/github/dsp && go build ./cmd/autopilot/`
Expected: Build succeeds. Remove the `runContinuous` stub from `main.go` if it exists (now defined in `continuous.go`).

- [ ] **Step 6: Commit**

```bash
git add cmd/autopilot/continuous.go cmd/autopilot/continuous_test.go
git commit -m "feat(autopilot): implement continuous simulation mode (traffic curves, health checks, daily reports)"
```

---

### Task 11: Docker Compose Production Hardening

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add restart policies and healthcheck improvements**

Update `docker-compose.yml` to add `restart: unless-stopped` to all services:

```yaml
services:
  postgres:
    image: postgres:16-alpine
    restart: unless-stopped
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: dsp
      POSTGRES_USER: dsp
      POSTGRES_PASSWORD: dsp_dev_password
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U dsp"]
      interval: 5s
      timeout: 3s
      retries: 5

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    ports:
      - "6380:6379"
    command: ["redis-server", "--requirepass", "${REDIS_PASSWORD:-dsp_dev_password}", "--appendonly", "yes"]
    volumes:
      - redisdata:/data
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD:-dsp_dev_password}", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  clickhouse:
    image: clickhouse/clickhouse-server:24-alpine
    restart: unless-stopped
    ports:
      - "8124:8123"
      - "9001:9000"
    environment:
      CLICKHOUSE_USER: ${CLICKHOUSE_USER:-default}
      CLICKHOUSE_PASSWORD: ${CLICKHOUSE_PASSWORD:-dsp_dev_password}
      CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT: 1
    volumes:
      - chdata:/var/lib/clickhouse
    healthcheck:
      test: ["CMD", "clickhouse-client", "--password", "${CLICKHOUSE_PASSWORD:-dsp_dev_password}", "--query", "SELECT 1"]
      interval: 5s
      timeout: 3s
      retries: 5

  kafka:
    image: apache/kafka:latest
    restart: unless-stopped
    ports:
      - "9094:9094"
    environment:
      KAFKA_NODE_ID: 1
      KAFKA_PROCESS_ROLES: broker,controller
      KAFKA_CONTROLLER_QUORUM_VOTERS: 1@kafka:9193
      KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9094,CONTROLLER://0.0.0.0:9193
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://localhost:9094
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: CONTROLLER:PLAINTEXT,PLAINTEXT:PLAINTEXT
      KAFKA_CONTROLLER_LISTENER_NAMES: CONTROLLER
      CLUSTER_ID: dsp-kafka-cluster-001
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_REPLICATION_FACTOR: 1
      KAFKA_TRANSACTION_STATE_LOG_MIN_ISR: 1
    volumes:
      - kafkadata:/var/kafka/data

  prometheus:
    image: prom/prometheus:latest
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
    extra_hosts:
      - "host.docker.internal:host-gateway"

  grafana:
    image: grafana/grafana:latest
    restart: unless-stopped
    ports:
      - "3100:3000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: admin
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Viewer
    volumes:
      - ./monitoring/provisioning:/etc/grafana/provisioning
      - ./monitoring/dashboards:/var/lib/grafana/dashboards
      - grafanadata:/var/lib/grafana
    depends_on:
      - prometheus

volumes:
  pgdata:
  chdata:
  redisdata:
  kafkadata:
  grafanadata:
```

Key changes from current `docker-compose.yml`:
1. `restart: unless-stopped` on all services
2. Redis: `--appendonly yes` for AOF persistence + `redisdata` volume
3. Kafka: `kafkadata` volume for data persistence
4. Grafana: `grafanadata` volume for dashboard persistence
5. New volumes: `redisdata`, `kafkadata`, `grafanadata`

- [ ] **Step 2: Verify compose file is valid**

Run: `cd /c/Users/Roc/github/dsp && docker compose config --quiet`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add docker-compose.yml
git commit -m "ops: harden docker-compose for production (restart policies, AOF, persistent volumes)"
```

---

### Task 12: Integration Smoke Test

**Files:**
- None new — this task runs existing code

- [ ] **Step 1: Run all unit tests**

Run: `cd /c/Users/Roc/github/dsp && go test ./cmd/autopilot/ -v`
Expected: All tests pass (client, report, scenarios, continuous)

- [ ] **Step 2: Run full project test suite**

Run: `cd /c/Users/Roc/github/dsp && go test ./... -short -count=1`
Expected: All tests pass. `-short` skips integration tests that need live databases.

- [ ] **Step 3: Build autopilot binary**

Run: `cd /c/Users/Roc/github/dsp && go build -o autopilot.exe ./cmd/autopilot/`
Expected: Binary compiles successfully.

- [ ] **Step 4: Verify help output**

Run: `cd /c/Users/Roc/github/dsp && ./autopilot.exe`
Expected output:
```
Usage: autopilot <verify|run>

  verify  — Run full E2E verification, produce HTML report
  run     — Start continuous simulation mode (24/7)
```

- [ ] **Step 5: Commit final state**

```bash
git add -A
git commit -m "feat(autopilot): complete Phase 1 autopilot implementation"
```

---

## Out of Scope (Ops Tasks)

The following items from the Phase 1 spec are infrastructure/ops configuration, not Go code. They should be done manually during server setup:

- **HTTPS + Nginx**: Install Nginx, configure reverse proxy to ports 8181/4000, set up Let's Encrypt certbot
- **PostgreSQL backup**: Set up `pg_dump` cron job + cloud disk snapshots
- **Log rotation**: Configure `logrotate` for application log files
- **Config externalization**: Already works via env vars; use `.env` file or cloud secret manager on the server

---

## Dependency Summary

Install order for new Go dependencies:
```bash
go get github.com/chromedp/chromedp@latest
go get github.com/docker/docker/client@latest
go get github.com/docker/docker/api/types/container@latest
```

System requirement: Google Chrome or Chromium must be installed on the machine for chromedp to work. On a Linux server:
```bash
apt-get install -y chromium-browser  # Ubuntu/Debian
```

## Task Dependency Graph

```
Task 1 (alert) ─────────────────────────┐
Task 2 (config) ─────────────────────────┤
Task 3 (client + tests) ────────────────┤
Task 4 (browser) ───────────────────────┤
Task 5 (report + tests) ───────────────┤
                                         ├── Task 9 (CLI main.go) ── Task 12 (smoke test)
Task 6 (normal scenarios + tests) ──────┤
Task 7 (fault module) ─────────────────┤
Task 8 (fault scenarios) ──────────────┤
Task 10 (continuous mode + tests) ─────┤
Task 11 (docker-compose) ─────────────────── (independent)
```

Tasks 1-5 and 7 can be parallelized. Tasks 6 depends on 3+5. Task 8 depends on 6+7. Task 9 depends on 1+6+8+10. Task 12 depends on all.
