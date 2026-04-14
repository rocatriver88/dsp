package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCreativeHandlers_UnauthReturns401 guards the 401 path on all four
// creative handlers. Because ensureCampaignOwner / ensureCreativeOwner both
// call requireAuth first, a nil Store is safe on this path.
//
// Cross-tenant coverage (authenticated but other-owner) requires a real
// Store and is deferred to the Batch 6 integration test suite per V5 §P2.
func TestCreativeHandlers_UnauthReturns401(t *testing.T) {
	d := &Deps{} // Store, Redis intentionally nil

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		pathID string
		fn     func(http.ResponseWriter, *http.Request)
	}{
		{
			name:   "list",
			method: http.MethodGet,
			path:   "/campaigns/99/creatives",
			pathID: "99",
			fn:     d.HandleListCreatives,
		},
		{
			name:   "create",
			method: http.MethodPost,
			path:   "/creatives",
			body:   `{"campaign_id": 99, "name": "x", "ad_type": "banner"}`,
			fn:     d.HandleCreateCreative,
		},
		{
			name:   "update",
			method: http.MethodPut,
			path:   "/creatives/99",
			body:   `{"name": "x"}`,
			pathID: "99",
			fn:     d.HandleUpdateCreative,
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			path:   "/creatives/99",
			pathID: "99",
			fn:     d.HandleDeleteCreative,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var body *strings.Reader
			if tc.body != "" {
				body = strings.NewReader(tc.body)
			}
			var req *http.Request
			if body != nil {
				req = httptest.NewRequest(tc.method, tc.path, body)
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			if tc.pathID != "" {
				req.SetPathValue("id", tc.pathID)
			}
			req = req.WithContext(context.Background())
			w := httptest.NewRecorder()

			tc.fn(w, req)

			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401 for unauthenticated call, got %d (body: %s)",
					tc.name, w.Code, w.Body.String())
			}
		})
	}
}

// TestHandleCreateCreative_InvalidAdTypeReturns400 verifies the pre-existing
// input validation still fires before the scope check, so malformed input
// gets a clean 400 response without leaking any owner information.
func TestHandleCreateCreative_InvalidAdTypeReturns400(t *testing.T) {
	d := &Deps{}

	body := `{"campaign_id": 99, "name": "x", "ad_type": "totally_made_up"}`
	req := httptest.NewRequest(http.MethodPost, "/creatives", strings.NewReader(body))
	req = req.WithContext(ctxWithAdvertiser(req.Context(), 42))
	w := httptest.NewRecorder()

	d.HandleCreateCreative(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ad_type, got %d (body: %s)", w.Code, w.Body.String())
	}
}
