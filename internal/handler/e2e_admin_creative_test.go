//go:build e2e
// +build e2e

package handler_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"
)

// Admin creative review handlers exercised here
// (internal/handler/admin.go):
//
//   GET  /api/v1/admin/creatives?status=pending
//     Success: 200 []*campaign.Creative filtered by status
//
//   POST /api/v1/admin/creatives/{id}/approve
//     Success: 200 {status: "approved"}
//     Underlying: store.UpdateCreativeStatus(id, "approved")
//
//   POST /api/v1/admin/creatives/{id}/reject
//     Request body: {reason string} (currently ignored by handler — see
//       concerns: HandleRejectCreative does not pass reason through)
//     Success: 200 {status: "rejected"}
//
// All cases use execAdmin (bare admin mux). The newCreative fixture
// auto-approves via UpdateCreativeStatus so tests that need pending
// creatives re-set the status to "pending" before the assertion.

// TestAdmin_Creatives_ListPending creates a creative, forces it to
// pending, and verifies GET /admin/creatives?status=pending returns it.
func TestAdmin_Creatives_ListPending(t *testing.T) {
	d := mustDeps(t)
	ctx := context.Background()

	advID, _ := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	if err := d.Store.UpdateCreativeStatus(ctx, creativeID, "pending"); err != nil {
		t.Fatalf("force creative %d pending: %v", creativeID, err)
	}

	req := adminReq(t, http.MethodGet, "/api/v1/admin/creatives?status=pending", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /admin/creatives?status=pending: expected 200, got %d: %s",
			w.Code, w.Body.String())
	}

	var listed []struct {
		ID int64 `json:"id"`
	}
	decodeJSON(t, w, &listed)

	found := false
	for _, c := range listed {
		if c.ID == creativeID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("GET /admin/creatives?status=pending: expected list to contain creative %d, got %d entries (body=%s)",
			creativeID, len(listed), w.Body.String())
	}
}

// TestAdmin_Creatives_Approve flips a pending creative to approved and
// verifies the DB row reflects the new status.
func TestAdmin_Creatives_Approve(t *testing.T) {
	d := mustDeps(t)
	ctx := context.Background()

	advID, _ := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	if err := d.Store.UpdateCreativeStatus(ctx, creativeID, "pending"); err != nil {
		t.Fatalf("force creative %d pending: %v", creativeID, err)
	}

	path := "/api/v1/admin/creatives/" + strconv.FormatInt(creativeID, 10) + "/approve"
	req := adminReq(t, http.MethodPost, path, nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /admin/creatives/%d/approve: expected 200, got %d: %s",
			creativeID, w.Code, w.Body.String())
	}

	after, err := d.Store.GetCreativeByID(ctx, creativeID)
	if err != nil {
		t.Fatalf("GetCreativeByID(%d): %v", creativeID, err)
	}
	if after.Status != "approved" {
		t.Fatalf("creative %d status: want %q, got %q",
			creativeID, "approved", after.Status)
	}
}

// TestAdmin_Creatives_Reject flips a pending creative to rejected using
// the admin reject endpoint and verifies the DB row reflects it.
func TestAdmin_Creatives_Reject(t *testing.T) {
	d := mustDeps(t)
	ctx := context.Background()

	advID, _ := newAdvertiser(t, d)
	campaignID := newCampaign(t, d, advID)
	creativeID := newCreative(t, d, campaignID)

	if err := d.Store.UpdateCreativeStatus(ctx, creativeID, "pending"); err != nil {
		t.Fatalf("force creative %d pending: %v", creativeID, err)
	}

	path := "/api/v1/admin/creatives/" + strconv.FormatInt(creativeID, 10) + "/reject"
	body := map[string]any{"reason": "qa test reject"}
	req := adminReq(t, http.MethodPost, path, body)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /admin/creatives/%d/reject: expected 200, got %d: %s",
			creativeID, w.Code, w.Body.String())
	}

	after, err := d.Store.GetCreativeByID(ctx, creativeID)
	if err != nil {
		t.Fatalf("GetCreativeByID(%d): %v", creativeID, err)
	}
	if after.Status != "rejected" {
		t.Fatalf("creative %d status: want %q, got %q",
			creativeID, "rejected", after.Status)
	}
}

// TestAdmin_Creatives_Approve_NotFound_404 verifies approving a
// non-existent creative returns 404. Closed in P2.7b by adding a
// GetCreativeByID precheck to HandleApproveCreative.
func TestAdmin_Creatives_Approve_NotFound_404(t *testing.T) {
	d := mustDeps(t)

	req := adminReq(t, http.MethodPost, "/api/v1/admin/creatives/999999999/approve", nil)
	w := execAdmin(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /admin/creatives/999999999/approve: expected 404, got %d: %s",
			w.Code, w.Body.String())
	}
}

// TestAdmin_Creatives_Reject_NotFound_404 is the symmetric negative test
// for HandleRejectCreative — same existence-check precheck as approve.
func TestAdmin_Creatives_Reject_NotFound_404(t *testing.T) {
	d := mustDeps(t)

	req := adminReq(t, http.MethodPost, "/api/v1/admin/creatives/999999999/reject",
		map[string]any{"reason": "does not exist"})
	w := execAdmin(t, d, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /admin/creatives/999999999/reject: expected 404, got %d: %s",
			w.Code, w.Body.String())
	}
}
