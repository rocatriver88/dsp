//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreateAdvertiser_ReturnsKeyOnce is the Round 1 review I7 regression
// guard for the one-time key disclosure invariant:
//
//  1. POST /advertisers MUST return the freshly-minted api_key in its
//     response body (this is the only time the key is legitimately
//     exposed).
//  2. A subsequent GET /advertisers/{id} with that key MUST NOT return
//     the api_key (the read path is sanitized via AdvertiserResponse).
//
// Both halves of the invariant matter: if (1) regresses, the caller
// never learns its key and the advertiser is permanently locked out;
// if (2) regresses, every read call re-exposes the key, defeating the
// Batch 1 DTO work. This test exercises both in one flow.
func TestCreateAdvertiser_ReturnsKeyOnce(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	// Invoke HandleCreateAdvertiser directly to bypass the APIKey
	// middleware — the create path would otherwise need the
	// WithAuthExemption wrapper that lives in cmd/api/main.go. The
	// handler itself does not care about advertiser context on create.
	body := []byte(`{"company_name": "Key Probe Co", "contact_email": "probe@test.local", "balance_cents": 1000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/advertisers", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	shared.Deps.HandleCreateAdvertiser(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d (body: %s)", w.Code, w.Body.String())
	}

	// (1) Assert the creation response contains api_key.
	var createResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	newKey, ok := createResp["api_key"].(string)
	if !ok || newKey == "" {
		t.Fatalf("create response must expose api_key on creation path, got: %+v", createResp)
	}
	newIDFloat, ok := createResp["id"].(float64)
	if !ok {
		t.Fatalf("create response must include id, got: %+v", createResp)
	}
	newID := int64(newIDFloat)

	// (2) Use the freshly returned key to GET the same advertiser via
	// the authed harness, and assert the response body does NOT
	// contain api_key. Closes the loop: one-time disclosure works,
	// the read path stays sanitized.
	resp := doReq(t, http.MethodGet, "/api/v1/advertisers/"+itoa(newID), nil, newKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("read own: want 200, got %d", resp.StatusCode)
	}
	readBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(readBody), `"api_key"`) {
		t.Errorf("read path leaks api_key after creation: %s", string(readBody))
	}
	if strings.Contains(string(readBody), newKey) {
		t.Errorf("read path leaks the literal key value: %s", string(readBody))
	}
}

// TestCreateCampaign_RequiresAuth locks in the invariant that
// unauthenticated POST /campaigns returns 401, not 201 under some
// default-advertiser hack. The authenticated path is already exercised
// by the tenant-isolation happy-path test, so this focuses on the
// negative.
func TestCreateCampaign_RequiresAuth(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	body := []byte(`{"name":"probe","billing_model":"cpm","budget_total_cents":1000,"budget_daily_cents":100,"bid_cpm_cents":10}`)
	resp := doReq(t, http.MethodPost, "/api/v1/campaigns", body, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for unauthenticated create, got %d", resp.StatusCode)
	}
}
