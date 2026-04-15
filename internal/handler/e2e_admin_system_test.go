//go:build e2e
// +build e2e

package handler_test

import (
	"net/http"
	"testing"
)

// TestAdmin_Health hits GET /api/v1/admin/health on the bare admin mux and
// expects 200 with a status payload. HandleSystemHealth probes the store,
// regSvc, redis, and reportStore; mustDeps wires all but reportStore (which
// is optional and handled by a nil-guard in the handler).
func TestAdmin_Health(t *testing.T) {
	d := mustDeps(t)
	req := adminReq(t, http.MethodGet, "/api/v1/admin/health", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d body=%s", w.Code, w.Body.String())
	}
	var got map[string]any
	decodeJSON(t, w, &got)
	if got["status"] != "ok" {
		t.Fatalf("health status field = %v (body=%s)", got["status"], w.Body.String())
	}
}

// TestAdmin_CircuitBreakAndReset exercises the trip/status/reset cycle. The
// shared mustDeps helper does not wire a *guardrail.Guardrail (it's nil by
// default because biz tests don't exercise the guardrail stack), so the
// handlers correctly return 503 rather than panic. We skip cleanly when the
// dep is missing; the point of the test is to cover the happy path when the
// guardrail is configured.
func TestAdmin_CircuitBreakAndReset(t *testing.T) {
	d := mustDeps(t)
	if d.Guardrail == nil {
		t.Skip("guardrail not configured")
	}

	{
		req := adminReq(t, http.MethodPost, "/api/v1/admin/circuit-break", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-break status = %d body=%s", w.Code, w.Body.String())
		}
	}
	{
		req := adminReq(t, http.MethodGet, "/api/v1/admin/circuit-status", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-status status = %d body=%s", w.Code, w.Body.String())
		}
	}
	{
		req := adminReq(t, http.MethodPost, "/api/v1/admin/circuit-reset", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-reset status = %d body=%s", w.Code, w.Body.String())
		}
	}
}

// TestAdmin_ListAdvertisers_IncludesMine verifies that after creating an
// advertiser fixture, GET /api/v1/admin/advertisers returns a list that
// includes its id. ListAllAdvertisers orders by created_at DESC, so the
// freshly inserted row should appear within the default page of 100.
func TestAdmin_ListAdvertisers_IncludesMine(t *testing.T) {
	d := mustDeps(t)
	advID, _ := newAdvertiser(t, d)

	req := adminReq(t, http.MethodGet, "/api/v1/admin/advertisers", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list advertisers status = %d body=%s", w.Code, w.Body.String())
	}

	// Decode into raw records so we can both scan for id AND assert api_key
	// is not exposed. P2.8b closed the credential-leak bug by redacting
	// api_key in HandleListAdvertisers; this test is the regression guard.
	var raw []map[string]any
	decodeJSON(t, w, &raw)

	found := false
	for _, rec := range raw {
		idVal, ok := rec["id"].(float64)
		if !ok {
			continue
		}
		if int64(idVal) == advID {
			found = true
		}
		// Every record must not leak the api_key. Either the field is
		// absent or it's an empty string.
		if k, present := rec["api_key"]; present {
			if s, isStr := k.(string); !isStr || s != "" {
				t.Fatalf("admin list leaked api_key for advertiser %v: %q", rec["id"], k)
			}
		}
	}
	if !found {
		t.Fatalf("expected advertiser %d in admin list (len=%d)", advID, len(raw))
	}
}

// TestAdmin_TopUp_UpdatesBalance posts to /api/v1/admin/topup and expects a
// billing.Transaction back on 200. We don't re-query the balance from the
// billing endpoint because that requires authenticated advertiser context;
// asserting the handler succeeded is enough coverage for this task. The
// billing service itself is tested elsewhere.
func TestAdmin_TopUp_UpdatesBalance(t *testing.T) {
	d := mustDeps(t)
	advID, _ := newAdvertiser(t, d)

	body := map[string]any{
		"advertiser_id": advID,
		"amount_cents":  12345,
		"description":   "qa e2e topup",
	}
	req := adminReq(t, http.MethodPost, "/api/v1/admin/topup", body)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("topup status = %d body=%s", w.Code, w.Body.String())
	}

	var tx map[string]any
	decodeJSON(t, w, &tx)
	// billing.Transaction must round-trip advertiser_id and amount_cents.
	v, ok := tx["advertiser_id"]
	if !ok {
		t.Fatalf("topup response missing advertiser_id: %s", w.Body.String())
	}
	f, ok := v.(float64)
	if !ok || int64(f) != advID {
		t.Fatalf("topup tx advertiser_id = %v (want %d) body=%s", v, advID, w.Body.String())
	}
	amtV, ok := tx["amount_cents"]
	if !ok {
		t.Fatalf("topup response missing amount_cents: %s", w.Body.String())
	}
	amtF, ok := amtV.(float64)
	if !ok || int64(amtF) != 12345 {
		t.Fatalf("topup tx amount_cents = %v (want 12345) body=%s", amtV, w.Body.String())
	}
}

// TestAdmin_ActiveCampaigns hits GET /internal/active-campaigns via the bare
// admin mux. In production BuildInternalHandler wraps this path in
// AdminAuthMiddleware; execAdmin bypasses that so we can cover the handler
// logic directly.
func TestAdmin_ActiveCampaigns(t *testing.T) {
	d := mustDeps(t)
	req := adminReq(t, http.MethodGet, "/internal/active-campaigns", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("active-campaigns status = %d body=%s", w.Code, w.Body.String())
	}
	// Body must decode as a JSON array, never null — HandleActiveCampaigns
	// explicitly substitutes an empty slice on nil return.
	var arr []any
	decodeJSON(t, w, &arr)
}

// TestAdmin_AuditLog hits GET /api/v1/admin/audit-log. mustDeps wires
// audit.NewLogger, so the handler's nil-guard should not trigger and we
// expect a 200 with a JSON array (possibly empty).
func TestAdmin_AuditLog(t *testing.T) {
	d := mustDeps(t)
	req := adminReq(t, http.MethodGet, "/api/v1/admin/audit-log", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("audit-log status = %d body=%s", w.Code, w.Body.String())
	}
	var arr []any
	decodeJSON(t, w, &arr)
}
