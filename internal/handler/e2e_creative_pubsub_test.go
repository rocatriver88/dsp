//go:build e2e
// +build e2e

package handler_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// After the P2.5b creative-CRUD tenant-scope fix, Handle*Creative now
// reads the advertiser id from context and rejects calls without it with
// 404. These pubsub tests therefore drive requests through execAuthed
// (the real API-key middleware) with a valid key for the owning advertiser
// — the same path production takes, which is a strictly better integration
// surface than the prior direct-handler calls.
func TestCreativePubSub_Create(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)

	wait := subscribeUpdates(t, d.Redis, campaignID)

	body := map[string]any{
		"campaign_id":     campaignID,
		"name":            "pubsub create",
		"ad_type":         "banner",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<img src="https://example.com/a.png">`,
		"destination_url": "https://example.com/landing",
	}
	req := authedReq(t, http.MethodPost, "/api/v1/creatives", body, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /creatives: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates for campaign_id=%d within 2s", campaignID)
	}
}

func TestCreativePubSub_Update(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	wait := subscribeUpdates(t, d.Redis, campaignID)

	body := map[string]any{
		"name":            "pubsub update",
		"ad_type":         "banner",
		"format":          "banner",
		"size":            "300x250",
		"ad_markup":       `<img src="https://example.com/b.png">`,
		"destination_url": "https://example.com/landing2",
	}
	req := authedReq(t, http.MethodPut, fmt.Sprintf("/api/v1/creatives/%d", creativeID), body, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /creatives/%d: expected 200, got %d: %s", creativeID, w.Code, w.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates for campaign_id=%d within 2s", campaignID)
	}
}

func TestCreativePubSub_Delete(t *testing.T) {
	d := mustDeps(t)
	advID, apiKey := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	wait := subscribeUpdates(t, d.Redis, campaignID)

	req := authedReq(t, http.MethodDelete, fmt.Sprintf("/api/v1/creatives/%d", creativeID), nil, apiKey)
	w := execAuthed(t, d, req)

	if w.Code != http.StatusOK {
		t.Fatalf("DELETE /creatives/%d: expected 200, got %d: %s", creativeID, w.Code, w.Body.String())
	}
	if !wait(2 * time.Second) {
		t.Fatalf("did not receive campaign:updates for campaign_id=%d within 2s", campaignID)
	}
}
