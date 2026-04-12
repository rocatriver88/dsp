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
			json.NewEncoder(w).Encode(map[string]any{"id": 10, "status": "paused"})
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

	simSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"total": 100, "bids": 80})
	}))
	defer simSrv.Close()

	runner := &ScenarioRunner{
		client:         NewDSPClient(apiSrv.URL, ""),
		exchangeSimURL: simSrv.URL,
		browser:        nil,
		trafficWait:    1 * time.Second,
	}

	steps := runner.RunNormalFlow()

	if len(steps) < 5 {
		t.Fatalf("expected at least 5 steps, got %d", len(steps))
	}

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
