//go:build integration

package integration

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"
)

// TestTenantIsolation_AccessesOwnResources_OK is the happy-path control: with
// advertiser A's key, A's own resources are reachable. If this fails, the
// 404-on-cross-tenant assertions below become meaningless (we'd be asserting
// that everything 404s, which is trivially true if auth is broken).
func TestTenantIsolation_AccessesOwnResources_OK(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	aID, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")
	aCamp := shared.createCampaign(t, aID, "Alice Campaign 1")

	cases := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"own advertiser", http.MethodGet, "/api/v1/advertisers/" + itoa(aID), http.StatusOK},
		{"own balance no-id", http.MethodGet, "/api/v1/billing/balance", http.StatusOK},
		{"own balance legacy", http.MethodGet, "/api/v1/billing/balance/" + itoa(aID), http.StatusOK},
		{"own campaign", http.MethodGet, "/api/v1/campaigns/" + itoa(aCamp), http.StatusOK},
		{"own campaign creatives", http.MethodGet, "/api/v1/campaigns/" + itoa(aCamp) + "/creatives", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doReq(t, tc.method, tc.path, nil, aKey)
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("want %d, got %d (body: %s)", tc.wantStatus, resp.StatusCode, string(body))
			}
		})
	}
}

// TestTenantIsolation_CrossTenantReadsAre404 is the load-bearing P2 test: A
// must not be able to read any of B's resources — every attempt gets 404
// (not 403), per the V5 three-code rule's tenant-hiding policy.
func TestTenantIsolation_CrossTenantReadsAre404(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	_, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")
	bID, _ := shared.createAdvertiser(t, "Bob Co", "bob@test.local")
	bCamp := shared.createCampaign(t, bID, "Bob Campaign 1")

	cases := []struct {
		name string
		path string
	}{
		{"foreign advertiser", "/api/v1/advertisers/" + itoa(bID)},
		{"foreign balance legacy", "/api/v1/billing/balance/" + itoa(bID)},
		{"foreign campaign", "/api/v1/campaigns/" + itoa(bCamp)},
		{"foreign campaign creatives", "/api/v1/campaigns/" + itoa(bCamp) + "/creatives"},
		{"foreign stats", "/api/v1/reports/campaign/" + itoa(bCamp) + "/stats"},
		{"foreign hourly", "/api/v1/reports/campaign/" + itoa(bCamp) + "/hourly"},
		{"foreign geo", "/api/v1/reports/campaign/" + itoa(bCamp) + "/geo"},
		{"foreign bids", "/api/v1/reports/campaign/" + itoa(bCamp) + "/bids"},
		{"foreign attribution", "/api/v1/reports/campaign/" + itoa(bCamp) + "/attribution"},
		// Round 1 review I1: simulate + export paths were previously
		// uncovered by the regression suite even though V5 §P0 §3 lists
		// them as "已有检查 保持一致". Now they're explicitly guarded.
		{"foreign simulate", "/api/v1/reports/campaign/" + itoa(bCamp) + "/simulate?bid_cpm_cents=100"},
		{"foreign export stats", "/api/v1/export/campaign/" + itoa(bCamp) + "/stats"},
		{"foreign export bids", "/api/v1/export/campaign/" + itoa(bCamp) + "/bids"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doReq(t, http.MethodGet, tc.path, nil, aKey)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNotFound {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("want 404 for cross-tenant read, got %d (body: %s)", resp.StatusCode, string(body))
			}
		})
	}
}

// TestTenantIsolation_CrossTenantMutationsAre404 verifies that write paths
// protecting another tenant's resources also return 404. Creative write
// operations (update/delete) and campaign state transitions are the
// highest-risk surface because they can silently corrupt another tenant's
// investment state.
func TestTenantIsolation_CrossTenantMutationsAre404(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	_, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")
	bID, _ := shared.createAdvertiser(t, "Bob Co", "bob@test.local")
	bCamp := shared.createCampaign(t, bID, "Bob Campaign 1")
	bCreative := shared.createCreative(t, bCamp, "Bob Creative 1")

	t.Run("update B's campaign", func(t *testing.T) {
		body := []byte(`{"name": "Alice hijack", "bid_cpm_cents": 1}`)
		resp := doReq(t, http.MethodPut, "/api/v1/campaigns/"+itoa(bCamp), body, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("start B's campaign", func(t *testing.T) {
		resp := doReq(t, http.MethodPost, "/api/v1/campaigns/"+itoa(bCamp)+"/start", nil, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("pause B's campaign", func(t *testing.T) {
		resp := doReq(t, http.MethodPost, "/api/v1/campaigns/"+itoa(bCamp)+"/pause", nil, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("create creative under B's campaign", func(t *testing.T) {
		body := []byte(`{"campaign_id": ` + itoa(bCamp) + `, "name": "Alice malicious", "ad_type": "banner", "ad_markup": "<script>alert(1)</script>", "destination_url": "https://evil.test"}`)
		resp := doReq(t, http.MethodPost, "/api/v1/creatives", body, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("update B's creative", func(t *testing.T) {
		body := []byte(`{"name": "Alice hijacked", "ad_markup": "<script>alert(1)</script>"}`)
		resp := doReq(t, http.MethodPut, "/api/v1/creatives/"+itoa(bCreative), body, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	t.Run("delete B's creative", func(t *testing.T) {
		resp := doReq(t, http.MethodDelete, "/api/v1/creatives/"+itoa(bCreative), nil, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("want 404, got %d", resp.StatusCode)
		}
	})

	// Verify B's creative was NOT deleted despite A's attempt.
	t.Run("B's creative survives", func(t *testing.T) {
		creatives, err := shared.Deps.Store.GetAllCreativesByCampaign(context.Background(), bCamp)
		if err != nil {
			t.Fatalf("list creatives: %v", err)
		}
		if len(creatives) != 1 || creatives[0].ID != bCreative {
			t.Errorf("B's creative did not survive A's delete attempt: got %+v", creatives)
		}
	})
}

// TestTenantIsolation_BillingTopupForeignIDIs400 verifies the codex V4
// stance: a topup whose body names another advertiser must fail loud
// (400 Bad Request) rather than silently redirect the charge.
func TestTenantIsolation_BillingTopupForeignIDIs400(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	aID, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")
	bID, _ := shared.createAdvertiser(t, "Bob Co", "bob@test.local")

	body := []byte(`{"advertiser_id": ` + itoa(bID) + `, "amount_cents": 5000, "description": "Alice trying to charge Bob"}`)
	resp := doReq(t, http.MethodPost, "/api/v1/billing/topup", body, aKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		t.Errorf("want 400, got %d (body: %s)", resp.StatusCode, string(b))
	}

	// Verify neither account was charged.
	aBal, _, err := shared.Deps.BillingSvc.GetBalance(context.Background(), aID)
	if err != nil {
		t.Fatalf("get balance A: %v", err)
	}
	bBal, _, err := shared.Deps.BillingSvc.GetBalance(context.Background(), bID)
	if err != nil {
		t.Fatalf("get balance B: %v", err)
	}
	if aBal != 1_000_000 {
		t.Errorf("A's balance changed despite rejected topup: got %d", aBal)
	}
	if bBal != 1_000_000 {
		t.Errorf("B's balance changed despite rejected topup: got %d", bBal)
	}
}

// TestTenantIsolation_BillingTopupOwnIDWorks verifies the positive path:
// a topup with A's own advertiser_id in the body (or no advertiser_id at
// all) credits A's account.
func TestTenantIsolation_BillingTopupOwnIDWorks(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	aID, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")

	t.Run("matching id in body", func(t *testing.T) {
		body := []byte(`{"advertiser_id": ` + itoa(aID) + `, "amount_cents": 2500, "description": "self topup"}`)
		resp := doReq(t, http.MethodPost, "/api/v1/billing/topup", body, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("want 200, got %d (body: %s)", resp.StatusCode, string(b))
		}
	})

	t.Run("no id in body", func(t *testing.T) {
		body := []byte(`{"amount_cents": 3500, "description": "self topup without id"}`)
		resp := doReq(t, http.MethodPost, "/api/v1/billing/topup", body, aKey)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("want 200, got %d (body: %s)", resp.StatusCode, string(b))
		}
	})

	// Both topups should have credited A by the combined amount.
	aBal, _, err := shared.Deps.BillingSvc.GetBalance(context.Background(), aID)
	if err != nil {
		t.Fatalf("get balance A: %v", err)
	}
	wantBal := int64(1_000_000 + 2500 + 3500)
	if aBal != wantBal {
		t.Errorf("A's balance: want %d, got %d", wantBal, aBal)
	}
}

// TestTenantIsolation_MissingAPIKeyIs401 confirms that the APIKeyMiddleware
// rejects unauthenticated requests with 401 before the handler runs.
// Without this, every "foreign advertiser" test above would have to
// carefully distinguish "no auth" from "wrong tenant".
func TestTenantIsolation_MissingAPIKeyIs401(t *testing.T) {
	resp := doReq(t, http.MethodGet, "/api/v1/advertisers/1", nil, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for missing api key, got %d", resp.StatusCode)
	}
}

// doReq runs an authenticated request against the shared httptest server.
// Pass apiKey="" for unauthenticated calls.
func doReq(t *testing.T, method, path string, body []byte, apiKey string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, shared.Server.URL+path, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
