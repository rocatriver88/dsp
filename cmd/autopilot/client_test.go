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
		AdvertiserID:     1,
		Name:             "Autopilot Test Campaign",
		BillingModel:     "cpm",
		BudgetTotalCents: 50000,
		BudgetDailyCents: 10000,
		BidCPMCents:      500,
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
			"today_spend_cents": 1500,
			"today_impressions": 300,
			"today_clicks":      15,
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
