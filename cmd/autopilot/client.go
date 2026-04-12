package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
