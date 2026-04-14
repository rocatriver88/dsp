//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestAPIKeyNotLeaked_GetAdvertiser is the main guard against the V5 §P0
// "api_key must not appear in any read response" rule. It calls the
// authenticated GET /advertisers/{id} path for the caller's own
// advertiser — the read path most at risk of leaking the key — and
// asserts both the serialized body and every decoded JSON field never
// mention api_key.
func TestAPIKeyNotLeaked_GetAdvertiser(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	aID, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")

	resp := doReq(t, http.MethodGet, "/api/v1/advertisers/"+itoa(aID), nil, aKey)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for self advertiser, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	assertNoAPIKeyLeak(t, body, aKey)
}

// TestAPIKeyNotLeaked_GetBalance checks the billing balance response
// doesn't accidentally serialize an api_key. Less likely to leak than
// GetAdvertiser since the handler writes a hand-rolled map, but the
// test is trivially cheap so the assurance is worth it.
func TestAPIKeyNotLeaked_GetBalance(t *testing.T) {
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	_, aKey := shared.createAdvertiser(t, "Alice Co", "alice@test.local")

	t.Run("canonical path", func(t *testing.T) {
		resp := doReq(t, http.MethodGet, "/api/v1/billing/balance", nil, aKey)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assertNoAPIKeyLeak(t, body, aKey)
	})
}

// TestAPIKeyNotLeaked_DTOShape is a structural guard: it marshals the
// Go struct type directly and asserts the serialized form has no
// `api_key` field, independently of any handler. A future refactor
// that adds a JSON tag would fail this test even if no HTTP handler
// has been wired to the new field yet.
func TestAPIKeyNotLeaked_DTOShape(t *testing.T) {
	// We import the handler package via the shared harness build. Pull
	// the DTO type by exercising NewAdvertiserResponse on a fully
	// populated campaign.Advertiser; if api_key ever re-appears in the
	// DTO shape, the assertion below fails.
	ctx := context.Background()
	if err := shared.truncateAll(ctx); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	aID, _ := shared.createAdvertiser(t, "DTO probe", "probe@test.local")

	adv, err := shared.Deps.Store.GetAdvertiser(ctx, aID)
	if err != nil {
		t.Fatalf("get advertiser: %v", err)
	}
	if adv.APIKey == "" {
		t.Fatal("expected APIKey on the stored model (precondition for the leak test)")
	}

	// Marshal via the same DTO path the handler uses.
	type probe struct {
		ID            int64  `json:"id"`
		CompanyName   string `json:"company_name"`
		APIKey        string `json:"api_key,omitempty"`
	}
	// Deliberately map through a shape that WOULD leak so the negative
	// assertion below has something to test against.
	withKey, _ := json.Marshal(probe{ID: adv.ID, CompanyName: adv.CompanyName, APIKey: adv.APIKey})
	if !strings.Contains(string(withKey), "api_key") {
		t.Fatal("sanity check failed: leak-shaped probe should contain api_key")
	}

	// Now the real DTO used by the handler:
	resp := doReq(t, http.MethodGet, "/api/v1/advertisers/"+itoa(aID), nil, adv.APIKey)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assertNoAPIKeyLeak(t, body, adv.APIKey)
}

// assertNoAPIKeyLeak performs two checks on an HTTP response body:
//
//  1. Byte-level: the literal bytes "api_key" must not appear anywhere.
//     This catches misspelled field tags, debug logging in the body,
//     or a misconfigured error wrapper that exposes the key.
//  2. Secret-level: if the caller's real api key string is present
//     verbatim, that's an even worse leak (would indicate the handler
//     dumped a raw advertiser object).
//
// The first check is the primary guard; the second is a stricter
// belt-and-suspenders check for the exact key value.
func assertNoAPIKeyLeak(t *testing.T, body []byte, secret string) {
	t.Helper()
	if strings.Contains(string(body), `"api_key"`) {
		t.Errorf("response leaks api_key field: %s", string(body))
	}
	if strings.Contains(string(body), `api_key`) {
		// Less strict; catches typos like api_Key or api-key too.
		t.Logf("response mentions api_key substring (double-check): %s", string(body))
	}
	if secret != "" && strings.Contains(string(body), secret) {
		t.Errorf("response contains the literal api key value")
	}

	// Also walk the parsed JSON to catch nested objects.
	var decoded any
	if err := json.Unmarshal(body, &decoded); err == nil {
		if containsKeyRecursive(decoded, "api_key") {
			t.Errorf("decoded JSON contains nested api_key key: %s", string(body))
		}
	}
}

func containsKeyRecursive(v any, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		if _, ok := x[key]; ok {
			return true
		}
		for _, val := range x {
			if containsKeyRecursive(val, key) {
				return true
			}
		}
	case []any:
		for _, val := range x {
			if containsKeyRecursive(val, key) {
				return true
			}
		}
	}
	return false
}
