//go:build e2e
// +build e2e

package handler_test

import (
	"net/http"
	"strconv"
	"testing"
)

// TestCreative_ListByCampaign exercises GET /api/v1/campaigns/{id}/creatives
// via the mux (so PathValue is populated by the ServeMux) and asserts the
// creative seeded via newCreative appears in the response.
//
// After the P2.5b IDOR fix, HandleListCreatives reads the advertiser from
// context and scopes the campaign lookup to that advertiser, so this test
// must drive the request through execAuthed (the real API-key middleware)
// with a valid key for the owning advertiser.
func TestCreative_ListByCampaign(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	req := authedReq(t, http.MethodGet,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignID, 10)+"/creatives", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /campaigns/%d/creatives: expected 200, got %d: %s",
			campaignID, w.Code, w.Body.String())
	}

	// HandleListCreatives returns []*campaign.Creative as a JSON array.
	var list []struct {
		ID         int64  `json:"id"`
		CampaignID int64  `json:"campaign_id"`
		Name       string `json:"name"`
	}
	decodeJSON(t, w, &list)

	found := false
	for _, c := range list {
		if c.ID == creativeID {
			if c.CampaignID != campaignID {
				t.Fatalf("creative %d: campaign_id mismatch: want %d, got %d",
					creativeID, campaignID, c.CampaignID)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /campaigns/%d/creatives: seeded creative %d not in list (body=%s)",
			campaignID, creativeID, w.Body.String())
	}
}

// TestCreative_Create_BadAdType_400 verifies HandleCreateCreative rejects an
// ad_type that is not in campaign.AdTypeConfig. The handler returns 400 with
// "invalid ad_type" — see campaign.go:478.
//
// HandleCreateCreative reads campaign_id from the body (not from context),
// so we use execPublic to avoid the auth dance.
func TestCreative_Create_BadAdType_400(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)

	body := map[string]any{
		"campaign_id":     campaignID,
		"name":            "bad-" + safeName(t.Name()),
		"ad_type":         "NOT_A_REAL_TYPE",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<a href="https://example.com">ad</a>`,
		"destination_url": "https://example.com",
	}
	// V5 hardened creative handlers to authenticate before running
	// ad_type validation. Use execAuthed + a real api key so the auth
	// context is populated and the request reaches the 400 branch.
	req := authedReq(t, http.MethodPost, "/api/v1/creatives", body, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("POST /creatives (bad ad_type): expected 400, got %d: %s",
			w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "ad_type") {
		t.Fatalf("POST /creatives (bad ad_type): expected body to mention ad_type, got %s",
			w.Body.String())
	}
}

// TestCreative_Update_NotFound_404 verifies PUT /api/v1/creatives/{id} with
// a non-existent id returns 404. The handler calls GetCreativeByID first
// (added in P1 commit 3350437) and returns 404 on lookup failure. V5
// hardened this path to authenticate before the lookup — the test now
// passes a real api key so the auth context is populated.
func TestCreative_Update_NotFound_404(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	body := map[string]any{
		"name":            "ghost-" + safeName(t.Name()),
		"ad_type":         "banner",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<a>ghost</a>`,
		"destination_url": "https://example.com",
	}
	req := authedReq(t, http.MethodPut, "/api/v1/creatives/999999999", body, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("PUT /creatives/999999999: expected 404, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCreative_Delete_NotFound_404 verifies DELETE /api/v1/creatives/{id}
// with a non-existent id returns 404 via the same GetCreativeByID guard.
// Needs a real api key per the V5 auth-first ordering (see above).
func TestCreative_Delete_NotFound_404(t *testing.T) {
	d := mustDeps(t)
	_, apiKey := newAdvertiser(t, d)

	req := authedReq(t, http.MethodDelete, "/api/v1/creatives/999999999", nil, apiKey)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /creatives/999999999: expected 404, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCreative_Create_CrossTenant_404 verifies that advertiser B cannot
// create a creative attached to advertiser A's campaign by supplying
// advertiser A's campaign_id in the request body. The handler must scope
// the campaign lookup to the caller's advertiser id and return 404 on
// mismatch (avoids leaking existence compared to 403).
//
// This is a regression guard for the creative CRUD IDOR closed in P2.5b.
func TestCreative_Create_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	campaignA := newCampaign(t, d, advA)
	_, keyB := newAdvertiser(t, d)

	body := map[string]any{
		"campaign_id":     campaignA,
		"name":            "xtenant-" + safeName(t.Name()),
		"ad_type":         "banner",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<a href="https://example.com">ad</a>`,
		"destination_url": "https://example.com",
	}
	req := authedReq(t, http.MethodPost, "/api/v1/creatives", body, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /creatives (cross-tenant campaign_id): expected 404, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestCreative_Update_CrossTenant_404 verifies that advertiser B cannot
// PUT /api/v1/creatives/{id} where {id} belongs to advertiser A's campaign.
// The handler must verify the creative's owning campaign belongs to the
// caller's advertiser id and return 404 otherwise.
func TestCreative_Update_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	campaignA := newCampaign(t, d, advA)
	creativeA := newCreative(t, d, campaignA)
	_, keyB := newAdvertiser(t, d)

	body := map[string]any{
		"name":            "hijack-" + safeName(t.Name()),
		"ad_type":         "banner",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<a>hijacked</a>`,
		"destination_url": "https://evil.example.com",
	}
	req := authedReq(t, http.MethodPut,
		"/api/v1/creatives/"+strconv.FormatInt(creativeA, 10), body, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("PUT /creatives/%d (cross-tenant): expected 404, got %d: %s",
			creativeA, w.Code, w.Body.String())
	}
}

// TestCreative_Delete_CrossTenant_404 verifies that advertiser B cannot
// DELETE a creative belonging to advertiser A's campaign via the id path.
func TestCreative_Delete_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	campaignA := newCampaign(t, d, advA)
	creativeA := newCreative(t, d, campaignA)
	_, keyB := newAdvertiser(t, d)

	req := authedReq(t, http.MethodDelete,
		"/api/v1/creatives/"+strconv.FormatInt(creativeA, 10), nil, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("DELETE /creatives/%d (cross-tenant): expected 404, got %d: %s",
			creativeA, w.Code, w.Body.String())
	}
}

// TestCreative_List_CrossTenant_404 verifies that advertiser B cannot list
// creatives under advertiser A's campaign via GET /campaigns/{id}/creatives.
// The handler must load the campaign scoped to the caller's advertiser id
// and return 404 when the campaign is not owned by the caller.
func TestCreative_List_CrossTenant_404(t *testing.T) {
	d := mustDeps(t)
	advA, _ := newAdvertiser(t, d)
	campaignA := newCampaign(t, d, advA)
	_ = newCreative(t, d, campaignA)
	_, keyB := newAdvertiser(t, d)

	req := authedReq(t, http.MethodGet,
		"/api/v1/campaigns/"+strconv.FormatInt(campaignA, 10)+"/creatives", nil, keyB)
	w := execAuthed(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /campaigns/%d/creatives (cross-tenant): expected 404, got %d: %s",
			campaignA, w.Code, w.Body.String())
	}
}
