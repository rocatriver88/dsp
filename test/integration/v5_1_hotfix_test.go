//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestP1_2_CreateAdvertiser_BlockedOnPublicPath is the V5.1 P1-2
// regression guard: POST /api/v1/advertisers MUST NOT be accepted
// through the tenant-authenticated public mux. Before the hotfix this
// endpoint existed at the public path and let any authenticated tenant
// POST a request body with a client-chosen `balance_cents`, receive a
// new advertiser id + api_key, and start spending ADX dollars without
// going through the admin top-up audit trail.
//
// After the hotfix the route is registered only on BuildAdminMux under
// admin auth. The integration harness mux deliberately does not mount
// admin routes (admin tests exercise handlers directly — see
// admin_list_test.go). This test only asserts the public path returns
// a rejection status. Any of 404 (route not registered) or 405 (method
// not allowed) is acceptable — Go's `http.ServeMux` returns 404 for
// unregistered method+path pairs.
func TestP1_2_CreateAdvertiser_BlockedOnPublicPath(t *testing.T) {
	ctx := context.Background()
	defer shared.truncateAll(ctx)

	// Create a legitimate authenticated tenant so the request reaches
	// the handler chain with a valid X-API-Key (ruling out the 401
	// branch — we want to see what the mux does when the auth check
	// passes).
	_, apiKey := shared.createAdvertiser(t, "p1-2-tenant", "p1-2@example.com")

	body, _ := json.Marshal(map[string]any{
		"company_name":  "evilco",
		"contact_email": "evil@example.com",
		"balance_cents": 1_000_000_00, // 1,000,000 yuan self-credit attempt
	})
	req, err := http.NewRequest("POST", shared.Server.URL+"/api/v1/advertisers", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		// Expected. The route is no longer registered on the tenant mux.
	case http.StatusCreated, http.StatusOK:
		t.Fatalf("V5.1 P1-2 regression: POST /api/v1/advertisers returned %d on the public path — the privilege-escalation route is still live", resp.StatusCode)
	default:
		t.Fatalf("unexpected status %d from POST /api/v1/advertisers (want 404 or 405)", resp.StatusCode)
	}
}
