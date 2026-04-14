package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleTopUp_RejectsForeignAdvertiser guards against the silent-redirect
// vulnerability: a caller who puts someone else's advertiser_id into the body
// must be refused with 400, not have the charge routed to their own account.
// BillingSvc is left nil because the 400 path short-circuits before any
// service call.
func TestHandleTopUp_RejectsForeignAdvertiser(t *testing.T) {
	d := &Deps{}

	body := `{"advertiser_id": 99, "amount_cents": 1000, "description": "test"}`
	req := httptest.NewRequest(http.MethodPost, "/billing/topup", strings.NewReader(body))
	req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
	w := httptest.NewRecorder()

	d.HandleTopUp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for foreign advertiser_id, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "advertiser_id") {
		t.Errorf("error body should mention advertiser_id: %s", w.Body.String())
	}
}

// TestHandleTopUp_ZeroAmountReturns400 verifies the existing amount validation
// still fires before the BillingSvc call, so nil deps remain safe on this path.
func TestHandleTopUp_ZeroAmountReturns400(t *testing.T) {
	d := &Deps{}

	body := `{"amount_cents": 0}`
	req := httptest.NewRequest(http.MethodPost, "/billing/topup", strings.NewReader(body))
	req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
	w := httptest.NewRecorder()

	d.HandleTopUp(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for zero amount, got %d", w.Code)
	}
}

// TestHandleTopUp_UnauthenticatedReturns401 is a defense-in-depth check:
// APIKeyMiddleware should have blocked unauthenticated requests upstream,
// but if the handler is somehow reached without auth, it must not proceed
// to BillingSvc on behalf of "advertiser 0".
func TestHandleTopUp_UnauthenticatedReturns401(t *testing.T) {
	d := &Deps{}

	body := `{"amount_cents": 1000}`
	req := httptest.NewRequest(http.MethodPost, "/billing/topup", strings.NewReader(body))
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	d.HandleTopUp(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated topup, got %d", w.Code)
	}
}

// TestHandleBalance_UnauthenticatedReturns401 covers the defense-in-depth
// auth check on /billing/balance. The route has no path id, so there's no
// cross-tenant scenario — only authenticated vs unauthenticated.
func TestHandleBalance_UnauthenticatedReturns401(t *testing.T) {
	d := &Deps{}

	req := httptest.NewRequest(http.MethodGet, "/billing/balance", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	d.HandleBalance(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated balance read, got %d", w.Code)
	}
}

// TestHandleTransactions_UnauthenticatedReturns401 — defense-in-depth like
// the topup case. No mock is required because the 401 branch fires first.
func TestHandleTransactions_UnauthenticatedReturns401(t *testing.T) {
	d := &Deps{}

	req := httptest.NewRequest(http.MethodGet, "/billing/transactions", nil)
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	d.HandleTransactions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated transactions, got %d", w.Code)
	}
}
