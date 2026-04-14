package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestReportHandlers_UnauthReturns401 runs all five tenant-gated report
// handlers through the 401 path. Because ensureCampaignOwner calls
// requireAuth first, and requireAuth fails fast before any Store call,
// nil Deps fields are safe on this path.
//
// Cross-tenant coverage (authenticated but other-owner) depends on a real
// Store and is deferred to the Batch 6 integration test suite per V5 §P2.
func TestReportHandlers_UnauthReturns401(t *testing.T) {
	d := &Deps{} // Store, ReportStore intentionally nil

	cases := []struct {
		name string
		path string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"stats", "/reports/campaign/99/stats", d.HandleCampaignStats},
		{"hourly", "/reports/campaign/99/hourly", d.HandleHourlyStats},
		{"geo", "/reports/campaign/99/geo", d.HandleGeoBreakdown},
		{"bids", "/reports/campaign/99/bids", d.HandleBidTransparency},
		{"attribution", "/reports/campaign/99/attribution", d.HandleAttribution},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.SetPathValue("id", "99")
			req = req.WithContext(context.Background())
			w := httptest.NewRecorder()

			tc.fn(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 for unauthenticated report read, got %d", tc.name, w.Code)
			}
		})
	}
}

// TestReportHandlers_InvalidIDReturns404 validates that a malformed path id
// uniformly returns 404 (tenant-hiding rule, V5 §P0). Nil Deps is safe
// because parseCampaignID fails before any auth or store call.
func TestReportHandlers_InvalidIDReturns404(t *testing.T) {
	d := &Deps{}

	cases := []struct {
		name string
		fn   func(http.ResponseWriter, *http.Request)
	}{
		{"stats", d.HandleCampaignStats},
		{"hourly", d.HandleHourlyStats},
		{"geo", d.HandleGeoBreakdown},
		{"bids", d.HandleBidTransparency},
		{"attribution", d.HandleAttribution},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/reports/campaign/not-a-number/"+tc.name, nil)
			req.SetPathValue("id", "not-a-number")
			req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
			w := httptest.NewRecorder()

			tc.fn(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("%s: expected 404 for invalid id, got %d", tc.name, w.Code)
			}
		})
	}
}
