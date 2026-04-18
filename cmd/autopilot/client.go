package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

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
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshal request: %w", err)
		}
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

// CreateAdvertiser creates an advertiser via the admin API. Requires
// c.AdminToken to be set. adminURL is the internal API base
// (e.g., http://localhost:8182). V5.1 P1-2: the old public POST
// /api/v1/advertisers path was removed from the tenant mux because it
// allowed any authenticated tenant to create a pre-funded advertiser;
// this method now hits POST /api/v1/admin/advertisers with X-Admin-Token.
func (c *DSPClient) CreateAdvertiser(adminURL, companyName, email string) (*AdvertiserResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"company_name":  companyName,
		"contact_email": email,
	})
	req, err := http.NewRequest("POST", adminURL+"/api/v1/admin/advertisers", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create advertiser: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create advertiser: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create advertiser: status %d, body: %s", resp.StatusCode, data)
	}
	var result AdvertiserResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("create advertiser: decode: %w", err)
	}
	return &result, nil
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("topup: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("create campaign: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("create creative: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("get campaign: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("list campaigns: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("overview stats: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("campaign stats: decode: %w", err)
	}
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
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, fmt.Errorf("balance: decode: %w", err)
	}
	balance, _ := resp["balance_cents"].(float64)
	return int64(balance), nil
}

// TriggerExchangeSim sends traffic via the exchange simulator.
func (c *DSPClient) TriggerExchangeSim(simURL, mode string, params map[string]string) (map[string]any, error) {
	u := simURL + "/" + mode
	if len(params) > 0 {
		q := make(url.Values)
		for k, v := range params {
			q.Set(k, v)
		}
		u += "?" + q.Encode()
	}
	resp, err := c.client.Get(u)
	if err != nil {
		return nil, fmt.Errorf("exchange-sim %s: %w", mode, err)
	}
	defer resp.Body.Close()
	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

type CircuitStatus struct {
	Status      string `json:"circuit_breaker"`
	Reason      string `json:"reason"`
	GlobalSpend int64  `json:"global_spend_today_cents"`
}

func (c *DSPClient) GetCircuitStatus() (*CircuitStatus, error) {
	data, status, err := c.do("GET", "/api/v1/admin/circuit-status", nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("circuit status: status %d", status)
	}
	var resp CircuitStatus
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("circuit status: decode: %w", err)
	}
	return &resp, nil
}

// --- Admin API methods (use AdminToken, hit internal port) ---

// AdminCreateInviteCode creates an invite code via admin API.
// adminURL is the internal API base (e.g., http://localhost:8182).
func (c *DSPClient) AdminCreateInviteCode(adminURL string, maxUses int) (string, error) {
	body, _ := json.Marshal(map[string]any{"max_uses": maxUses})
	req, _ := http.NewRequest("POST", adminURL+"/api/v1/admin/invite-codes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("create invite code: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return "", fmt.Errorf("create invite code: status %d, body: %s", resp.StatusCode, data)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	code, _ := result["code"].(string)
	return code, nil
}

// Register submits a registration request (auth-exempt endpoint).
func (c *DSPClient) Register(companyName, email, inviteCode string) (int64, error) {
	data, status, err := c.do("POST", "/api/v1/register", map[string]string{
		"company_name":  companyName,
		"contact_email": email,
		"invite_code":   inviteCode,
	})
	if err != nil {
		return 0, err
	}
	if status != 200 && status != 201 {
		return 0, fmt.Errorf("register: status %d, body: %s", status, data)
	}
	var resp map[string]any
	json.Unmarshal(data, &resp)
	id, _ := resp["id"].(float64)
	return int64(id), nil
}

// AdminApproveRegistration approves a registration and returns advertiser ID + API key.
// The approve response also carries user_email + temp_password for the auto-seeded
// advertiser user; autopilot does not use those fields today (it continues with
// API-key auth), so they are intentionally dropped here.
func (c *DSPClient) AdminApproveRegistration(adminURL string, registrationID int64) (*AdvertiserResponse, error) {
	req, _ := http.NewRequest("POST",
		fmt.Sprintf("%s/api/v1/admin/registrations/%d/approve", adminURL, registrationID), nil)
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("approve registration: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("approve registration: status %d, body: %s", resp.StatusCode, data)
	}
	var result map[string]any
	json.Unmarshal(data, &result)
	advID, _ := result["advertiser_id"].(float64)
	apiKey, _ := result["api_key"].(string)
	return &AdvertiserResponse{ID: int64(advID), APIKey: apiKey}, nil
}

// AdminListAdvertisers lists all advertisers via admin API.
type AdminAdvertiser struct {
	ID              int64  `json:"id"`
	CompanyName     string `json:"company_name"`
	BalanceCents    int64  `json:"balance_cents"`
	ActiveCampaigns int    `json:"active_campaigns"`
	TotalSpentCents int64  `json:"total_spent_cents"`
}

func (c *DSPClient) AdminListAdvertisers(adminURL string) ([]AdminAdvertiser, error) {
	req, _ := http.NewRequest("GET", adminURL+"/api/v1/admin/advertisers?limit=500", nil)
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("admin list advertisers: status %d", resp.StatusCode)
	}
	var result []AdminAdvertiser
	json.Unmarshal(data, &result)
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
