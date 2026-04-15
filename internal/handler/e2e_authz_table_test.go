//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/handler"
)

// TestAuthz_PublicRoutes_401WithoutAPIKey asserts that every non-exempt
// public route returns 401 when no X-API-Key header is set. It builds the
// full production public handler chain (routes + auth + exemption) minus
// the rate-limit middleware (skipped for e2e per the P2 design).
//
// The exempt list in handler.WithAuthExemption (internal/handler/middleware.go)
// is: /health, /api/v1/docs, /uploads/* (prefix), and POST /api/v1/register.
// Anything else must be gated by the API-key middleware, so any route in the
// table below that fails to 401 is either a wrongly-exempt path or a missing
// middleware wiring in BuildPublicHandler.
func TestAuthz_PublicRoutes_401WithoutAPIKey(t *testing.T) {
	d := mustDeps(t)

	mux := handler.BuildPublicMux(d)
	lookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := d.Store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}
	withAuth := authMiddlewareImpl(lookup, mux)
	chain := handler.WithAuthExemption(withAuth, mux)

	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/campaigns"},
		{"POST", "/api/v1/campaigns"},
		{"GET", "/api/v1/campaigns/1"},
		{"PUT", "/api/v1/campaigns/1"},
		{"POST", "/api/v1/campaigns/1/start"},
		{"POST", "/api/v1/campaigns/1/pause"},
		{"GET", "/api/v1/campaigns/1/creatives"},
		{"POST", "/api/v1/creatives"},
		{"PUT", "/api/v1/creatives/1"},
		{"DELETE", "/api/v1/creatives/1"},
		{"GET", "/api/v1/reports/campaign/1/stats"},
		{"GET", "/api/v1/reports/campaign/1/hourly"},
		{"GET", "/api/v1/reports/campaign/1/geo"},
		{"GET", "/api/v1/reports/campaign/1/bids"},
		{"GET", "/api/v1/reports/campaign/1/attribution"},
		{"GET", "/api/v1/reports/campaign/1/simulate"},
		{"GET", "/api/v1/reports/overview"},
		{"GET", "/api/v1/export/campaign/1/stats"},
		{"GET", "/api/v1/export/campaign/1/bids"},
		{"GET", "/api/v1/audit-log"},
		// /api/v1/analytics/stream and /snapshot are no longer on
		// BuildPublicMux. They live on BuildAnalyticsSSEMux behind
		// SSETokenMiddleware and are verified separately by
		// TestAuthz_AnalyticsSSE_401WithoutToken below (V5.1 P1-1).
		{"POST", "/api/v1/analytics/token"},
		{"POST", "/api/v1/billing/topup"},
		{"GET", "/api/v1/billing/transactions"},
		{"GET", "/api/v1/billing/balance/1"},
		{"POST", "/api/v1/upload"},
		{"POST", "/api/v1/advertisers"},
		{"GET", "/api/v1/advertisers/1"},
		{"GET", "/api/v1/ad-types"},
		{"GET", "/api/v1/billing-models"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			// Intentionally no X-API-Key header.
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: want 401, got %d body=%s",
					c.method, c.path, w.Code, w.Body.String())
			}
		})
	}
}

// TestAuthz_AdminRoutes_401WithoutAdminToken asserts that every admin route
// returns 401 when no X-Admin-Token header (and no admin_token query param)
// is provided. It wraps the admin mux in handler.AdminAuthMiddleware just
// like BuildInternalHandler does in production.
func TestAuthz_AdminRoutes_401WithoutAdminToken(t *testing.T) {
	d := mustDeps(t)

	chain := handler.AdminAuthMiddleware(handler.BuildAdminMux(d))

	cases := []struct {
		method string
		path   string
	}{
		{"GET", "/internal/active-campaigns"},
		{"GET", "/api/v1/admin/registrations"},
		{"POST", "/api/v1/admin/registrations/1/approve"},
		{"POST", "/api/v1/admin/registrations/1/reject"},
		{"GET", "/api/v1/admin/health"},
		{"GET", "/api/v1/admin/creatives"},
		{"POST", "/api/v1/admin/creatives/1/approve"},
		{"POST", "/api/v1/admin/creatives/1/reject"},
		{"POST", "/api/v1/admin/circuit-break"},
		{"POST", "/api/v1/admin/circuit-reset"},
		{"GET", "/api/v1/admin/circuit-status"},
		{"GET", "/api/v1/admin/advertisers"},
		{"POST", "/api/v1/admin/topup"},
		{"POST", "/api/v1/admin/invite-codes"},
		{"GET", "/api/v1/admin/invite-codes"},
		{"GET", "/api/v1/admin/audit-log"},
	}

	for _, c := range cases {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			// Intentionally no X-Admin-Token header / admin_token query param.
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: want 401, got %d body=%s",
					c.method, c.path, w.Code, w.Body.String())
			}
		})
	}
}

// TestAuthz_AnalyticsSSE_401WithoutToken asserts that the SSE endpoints
// (/api/v1/analytics/stream, /api/v1/analytics/snapshot) return 401 when
// no ?token= query parameter is present. V5.1 P1-1 regression guard: the
// old middleware accepted ?api_key= here and promoted it to X-API-Key,
// which leaked credentials into URL logs. The new path must reject both
// missing tokens and bare ?api_key= queries.
func TestAuthz_AnalyticsSSE_401WithoutToken(t *testing.T) {
	d := mustDeps(t)

	sseMux := handler.BuildAnalyticsSSEMux(d)
	chain := auth.SSETokenMiddleware(d.SSETokenSecret)(sseMux)

	cases := []struct {
		method, path string
	}{
		{"GET", "/api/v1/analytics/stream"},
		{"GET", "/api/v1/analytics/snapshot"},
		// Regression guard: the P1-1 leak was sending X-API-Key via
		// query. Make sure SSETokenMiddleware refuses this path even
		// though it "looks" authenticated.
		{"GET", "/api/v1/analytics/stream?api_key=dsp_abc123"},
		{"GET", "/api/v1/analytics/snapshot?api_key=dsp_abc123"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			req := httptest.NewRequest(c.method, c.path, nil)
			w := httptest.NewRecorder()
			chain.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s: want 401, got %d body=%s",
					c.method, c.path, w.Code, w.Body.String())
			}
		})
	}
}
