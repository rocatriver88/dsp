//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestListAdvertisers_OmitsAPIKey is the regression guard for Round 1
// review finding C1: HandleListAdvertisers previously returned
// []*campaign.Advertiser directly, leaking every advertiser's plaintext
// api_key on a single admin GET. The fix routes through
// NewAdvertiserResponseList; this test asserts the api_key is gone.
//
// The handler is invoked directly (not via httptest.NewServer) because
// it lives behind AdminAuthMiddleware in production, not the API key
// middleware the shared harness installs. Bypassing middleware is
// acceptable here: we are testing the DTO shape of the response, not
// the admin auth surface. Admin auth is covered by the unit tests in
// internal/handler/admin_auth_test.go.
func TestListAdvertisers_OmitsAPIKey(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	shared.createAdvertiser(t, "Alice Co", "alice@test.local")
	shared.createAdvertiser(t, "Bob Co", "bob@test.local")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/advertisers", nil)
	w := httptest.NewRecorder()
	shared.Deps.HandleListAdvertisers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	body := w.Body.Bytes()

	// String-level scan.
	if strings.Contains(string(body), `"api_key"`) {
		t.Errorf("admin list leaks api_key field: %s", string(body))
	}

	// Structural scan: decode and verify each entry looks like an
	// AdvertiserResponse and has no api_key field.
	var decoded []map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, string(body))
	}
	if len(decoded) != 2 {
		t.Errorf("expected 2 advertisers, got %d", len(decoded))
	}
	for i, adv := range decoded {
		if _, ok := adv["api_key"]; ok {
			t.Errorf("entry %d contains api_key field: %+v", i, adv)
		}
		// Required AdvertiserResponse fields must still be present.
		for _, key := range []string{"id", "company_name", "contact_email", "balance_cents", "billing_type"} {
			if _, ok := adv[key]; !ok {
				t.Errorf("entry %d missing expected field %q: %+v", i, key, adv)
			}
		}
	}
}

// TestListAdvertisers_EmptyReturnsArray asserts that an empty
// advertisers table returns `[]` not `null`, matching the existing
// convention on other list handlers.
func TestListAdvertisers_EmptyReturnsArray(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/advertisers", nil)
	w := httptest.NewRecorder()
	shared.Deps.HandleListAdvertisers(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Errorf("want `[]`, got %q", got)
	}
}
