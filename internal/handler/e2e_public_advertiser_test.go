//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
)

// TestAdvertiser_CreateAndGet exercises the happy path for
// POST /api/v1/advertisers followed by GET /api/v1/advertisers/{id}.
//
// Handler shapes (from internal/handler/campaign.go):
//   POST request:  {company_name, contact_email, balance_cents}
//   POST response (201): {id, api_key, message}
//   GET  response (200): full campaign.Advertiser struct
func TestAdvertiser_CreateAndGet(t *testing.T) {
	d := mustDeps(t)

	body := map[string]any{
		"company_name":  "acme-" + safeName(t.Name()),
		"contact_email": fmt.Sprintf("create-%d@test.local", nowNano()),
	}
	req := authedReq(t, http.MethodPost, "/api/v1/advertisers", body, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("POST /advertisers: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created struct {
		ID     int64  `json:"id"`
		APIKey string `json:"api_key"`
	}
	decodeJSON(t, w, &created)
	if created.ID == 0 {
		t.Fatalf("POST /advertisers: expected non-zero id, got 0 (body=%s)", w.Body.String())
	}
	if created.APIKey == "" {
		t.Fatalf("POST /advertisers: expected non-empty api_key (body=%s)", w.Body.String())
	}

	// After P2.8b, HandleGetAdvertiser scopes the lookup to the authenticated
	// caller (one may only fetch their own record). Route through execAuthed
	// so the APIKey middleware populates advID in the request context.
	getReq := authedReq(t, http.MethodGet,
		"/api/v1/advertisers/"+strconv.FormatInt(created.ID, 10), nil, created.APIKey)
	getW := execAuthed(t, d, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GET /advertisers/{id}: expected 200, got %d: %s", getW.Code, getW.Body.String())
	}

	var fetched struct {
		ID           int64  `json:"id"`
		CompanyName  string `json:"company_name"`
		ContactEmail string `json:"contact_email"`
	}
	decodeJSON(t, getW, &fetched)
	if fetched.ID != created.ID {
		t.Fatalf("GET /advertisers/{id}: id mismatch: want %d, got %d", created.ID, fetched.ID)
	}
}

// TestAdvertiser_Create_MissingFields_400 verifies the handler rejects a
// body with no company_name / contact_email.
func TestAdvertiser_Create_MissingFields_400(t *testing.T) {
	d := mustDeps(t)
	req := authedReq(t, http.MethodPost, "/api/v1/advertisers", map[string]any{}, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /advertisers (empty): expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdvertiser_Get_NotFound_404 verifies a GET for a non-existent id
// returns 404. Now also exercises the P2.8b scope check: even with a
// valid api key, requesting an id that isn't yours returns 404.
func TestAdvertiser_Get_NotFound_404(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)
	req := authedReq(t, http.MethodGet, "/api/v1/advertisers/99999999", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /advertisers/99999999: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

// TestAdvertiser_Get_CrossTenant_404 is the regression guard for the
// cross-tenant credential-leak bug closed in P2.8b. Advertiser A's key
// must not be readable via GET /api/v1/advertisers/{advA.id} when the
// request is authenticated as advertiser B. Pre-fix the endpoint
// returned A's full record (including api_key) to any authenticated
// caller. Post-fix it returns 404.
func TestAdvertiser_Get_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	_, keyB := newAdvertiser(t, d)

	req := authedReq(t, http.MethodGet,
		"/api/v1/advertisers/"+strconv.FormatInt(advA, 10), nil, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /advertisers/{advA}: expected 404 as advB, got %d body=%s",
			w.Code, w.Body.String())
	}
	// Extra guard: the response must not contain the api_key field. Even
	// if a future handler change replaces 404 with something more permissive,
	// it must never return another tenant's key.
	if contains(w.Body.String(), "api_key") {
		t.Fatalf("cross-tenant response leaked api_key: %s", w.Body.String())
	}
}

// TestBilling_TopUp_UpdatesBalance confirms that POST /billing/topup
// atomically increases balance by the requested amount.
//
// After P4.2b, all billing handlers scope by auth context and ignore
// client-supplied advertiser_id. The body no longer carries advertiser_id;
// the balance path id is validated against the caller and 404s on mismatch.
// Tests run through execAuthed to populate the context advID.
func TestBilling_TopUp_UpdatesBalance(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)

	beforeReq := authedReq(t, http.MethodGet,
		"/api/v1/billing/balance/"+strconv.FormatInt(advID, 10), nil, apiKey)
	beforeW := execAuthed(t, d, beforeReq)
	if beforeW.Code != http.StatusOK {
		t.Fatalf("GET /billing/balance (before): expected 200, got %d: %s",
			beforeW.Code, beforeW.Body.String())
	}
	var before struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	decodeJSON(t, beforeW, &before)

	topupReq := authedReq(t, http.MethodPost, "/api/v1/billing/topup", map[string]any{
		"amount_cents": 500,
	}, apiKey)
	topupW := execAuthed(t, d, topupReq)
	if topupW.Code != http.StatusOK {
		t.Fatalf("POST /billing/topup: expected 200, got %d: %s",
			topupW.Code, topupW.Body.String())
	}

	afterReq := authedReq(t, http.MethodGet,
		"/api/v1/billing/balance/"+strconv.FormatInt(advID, 10), nil, apiKey)
	afterW := execAuthed(t, d, afterReq)
	if afterW.Code != http.StatusOK {
		t.Fatalf("GET /billing/balance (after): expected 200, got %d: %s",
			afterW.Code, afterW.Body.String())
	}
	var after struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	decodeJSON(t, afterW, &after)

	if after.BalanceCents < before.BalanceCents+500 {
		t.Fatalf("topup did not raise balance: before=%d after=%d (want >= before+500)",
			before.BalanceCents, after.BalanceCents)
	}
}

// TestBilling_TopUp_NegativeAmount_400 verifies the handler rejects a
// negative amount_cents.
func TestBilling_TopUp_NegativeAmount_400(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	req := authedReq(t, http.MethodPost, "/api/v1/billing/topup", map[string]any{
		"amount_cents": -100,
	}, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /billing/topup (negative): expected 400, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestBilling_Transactions_ListsTopUp confirms GET /billing/transactions
// returns the most recent topup for the caller. After P4.2b the query
// param is ignored; the handler always scopes by auth context.
func TestBilling_Transactions_ListsTopUp(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	topupReq := authedReq(t, http.MethodPost, "/api/v1/billing/topup", map[string]any{
		"amount_cents": 777,
	}, apiKey)
	topupW := execAuthed(t, d, topupReq)
	if topupW.Code != http.StatusOK {
		t.Fatalf("POST /billing/topup: expected 200, got %d: %s",
			topupW.Code, topupW.Body.String())
	}

	listReq := authedReq(t, http.MethodGet, "/api/v1/billing/transactions", nil, apiKey)
	listW := execAuthed(t, d, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("GET /billing/transactions: expected 200, got %d: %s",
			listW.Code, listW.Body.String())
	}
	// Decode into a typed view and look for a transaction with amount_cents=777
	// rather than substring-matching the raw body — 777 could appear in ids,
	// cents fields, or timestamps.
	var txns []struct {
		AmountCents int64  `json:"amount_cents"`
		Type        string `json:"type"`
	}
	decodeJSON(t, listW, &txns)
	found := false
	for _, tx := range txns {
		if tx.AmountCents == 777 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /billing/transactions: no transaction with amount_cents=777 in body: %s",
			listW.Body.String())
	}
}

// TestBilling_Balance_CrossTenant_404 is the regression guard for the
// P4.2b billing IDOR: GET /api/v1/billing/balance/{advA} as advertiser B
// must return 404, not advertiser A's balance. Pre-fix the handler
// trusted the path id and returned A's balance to any authenticated
// caller. Post-fix the handler validates pathID == context advID.
func TestBilling_Balance_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	_, keyB := newAdvertiser(t, d)

	req := authedReq(t, http.MethodGet,
		"/api/v1/billing/balance/"+strconv.FormatInt(advA, 10), nil, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant GET /billing/balance/{advA}: expected 404 as advB, got %d body=%s",
			w.Code, w.Body.String())
	}
}

// TestBilling_TopUp_IgnoresBodyAdvertiserID documents that after P4.2b
// the topup handler ignores any client-supplied advertiser_id and
// always credits the caller. The legacy field is still accepted in the
// body (for backward compat with callers that still send it) but does
// not influence which tenant is credited.
func TestBilling_TopUp_IgnoresBodyAdvertiserID(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	advB, keyB := newAdvertiser(t, d)

	// advB attempts to top up with advertiser_id=advA in body.
	topupReq := authedReq(t, http.MethodPost, "/api/v1/billing/topup", map[string]any{
		"advertiser_id": advA, // should be ignored
		"amount_cents":  1234,
	}, keyB)
	topupW := execAuthed(t, d, topupReq)
	if topupW.Code != http.StatusOK {
		t.Fatalf("POST /billing/topup: expected 200, got %d: %s",
			topupW.Code, topupW.Body.String())
	}

	// Verify advB's balance increased, not advA's. Both GETs use the
	// owner's key so pathID == context advID holds.
	var bBal struct {
		BalanceCents int64 `json:"balance_cents"`
	}
	reqB := authedReq(t, http.MethodGet,
		"/api/v1/billing/balance/"+strconv.FormatInt(advB, 10), nil, keyB)
	decodeJSON(t, execAuthed(t, d, reqB), &bBal)
	if bBal.BalanceCents < 1234 {
		t.Fatalf("advB balance want >=1234, got %d — handler credited wrong tenant",
			bBal.BalanceCents)
	}
}

// TestRegister_ValidInviteCode creates an invite code via RegSvc.CreateInviteCode
// and then POSTs /register with it, expecting a 2xx.
//
// Handler shape (internal/handler/admin.go):
//   POST body:     registration.Request (company_name, contact_email, invite_code, ...)
//   POST response: 201 {id, status, message}
func TestRegister_ValidInviteCode(t *testing.T) {
	d := mustDeps(t)
	code, err := d.RegSvc.CreateInviteCode(context.Background(), "qa-"+safeName(t.Name()), 10, nil)
	if err != nil {
		t.Fatalf("create invite code: %v", err)
	}

	body := map[string]any{
		"company_name":  "reg-" + safeName(t.Name()),
		"contact_email": fmt.Sprintf("reg-%d@test.local", nowNano()),
		"invite_code":   code,
	}
	req := authedReq(t, http.MethodPost, "/api/v1/register", body, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("POST /register: expected 200/201, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRegister_InvalidInviteCode_400 verifies the handler rejects an unknown
// invite code.
//
// Handler behaviour note: HandleRegister (internal/handler/admin.go) maps any
// RegSvc.Submit error to HTTP 409 Conflict, not 400. The invite-code path hits
// Submit → ValidateAndUseInviteCode → returns an error → handler responds 409.
// This is a real bug — invalid input (bad invite code, blocked domain, malformed
// email) should be 400; 409 is only correct for duplicate-email conflicts.
// Tracked for P2.10 triage / P5 report. Re-tighten to 400 once HandleRegister
// distinguishes user-input errors from conflicts.
func TestRegister_InvalidInviteCode_400(t *testing.T) {
	d := mustDeps(t)
	body := map[string]any{
		"company_name":  "reg-" + safeName(t.Name()),
		"contact_email": fmt.Sprintf("reg-invalid-%d@test.local", nowNano()),
		"invite_code":   "INVALID-CODE-XYZ",
	}
	req := authedReq(t, http.MethodPost, "/api/v1/register", body, "")
	w := execPublic(t, d, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusConflict {
		t.Fatalf("POST /register (invalid code): expected 400 or 409, got %d: %s",
			w.Code, w.Body.String())
	}
}
