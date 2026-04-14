package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleGetAdvertiser_CrossTenantReturns404 verifies that an advertiser
// cannot read another advertiser's object. Because ensureSelfAccess runs
// before any Store call, Deps{Store: nil} is safe — the handler never
// dereferences it on the 404 path.
func TestHandleGetAdvertiser_CrossTenantReturns404(t *testing.T) {
	d := &Deps{} // Store intentionally nil

	req := httptest.NewRequest(http.MethodGet, "/advertisers/99", nil)
	req.SetPathValue("id", "99")
	// Authenticate as advertiser 42, trying to access advertiser 99.
	req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
	w := httptest.NewRecorder()

	d.HandleGetAdvertiser(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for cross-tenant read, got %d", w.Code)
	}
}

// TestHandleGetAdvertiser_UnauthReturns401 verifies that a request with no
// auth context is rejected before any Store lookup. Per V5 §P0 three-code
// rule, missing credentials is 401, not 404.
func TestHandleGetAdvertiser_UnauthReturns401(t *testing.T) {
	d := &Deps{} // Store intentionally nil

	req := httptest.NewRequest(http.MethodGet, "/advertisers/42", nil)
	req.SetPathValue("id", "42")
	req = req.WithContext(context.Background())
	w := httptest.NewRecorder()

	d.HandleGetAdvertiser(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unauthenticated read, got %d", w.Code)
	}
}

// TestHandleGetAdvertiser_InvalidIDReturns404 verifies malformed path id is
// rejected consistently with the 404 hiding rule (not 400, which would leak
// "this route exists").
func TestHandleGetAdvertiser_InvalidIDReturns404(t *testing.T) {
	d := &Deps{}

	req := httptest.NewRequest(http.MethodGet, "/advertisers/not-a-number", nil)
	req.SetPathValue("id", "not-a-number")
	req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
	w := httptest.NewRecorder()

	d.HandleGetAdvertiser(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for invalid id, got %d", w.Code)
	}
}
