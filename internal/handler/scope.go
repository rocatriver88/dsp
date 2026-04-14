package handler

import (
	"net/http"

	"github.com/heartgryphon/dsp/internal/auth"
)

// requireAuth is the defense-in-depth auth check for handlers whose route
// does not have a path id to compare against. It returns the authenticated
// advertiser id, or 0 + false after writing a 401 Unauthorized response.
//
// APIKeyMiddleware should have already rejected unauthenticated requests
// upstream; this helper only fires if the middleware was somehow bypassed.
func requireAuth(w http.ResponseWriter, r *http.Request) (int64, bool) {
	authID := auth.AdvertiserIDFromContext(r.Context())
	if authID == 0 {
		WriteError(w, http.StatusUnauthorized, "unauthenticated")
		return 0, false
	}
	return authID, true
}

// ensureSelfAccess validates that the authenticated advertiser is the owner
// of the resource identified by pathID.
//
// The three-code rule from V5 §P0:
//   - missing / invalid credentials → 401 Unauthorized
//   - authenticated but cross-tenant → 404 Not Found (hide existence)
//
// Use this for any handler whose path parameter is an advertiser ID and the
// resource must only be visible to its owner.
func ensureSelfAccess(w http.ResponseWriter, r *http.Request, pathID int64) bool {
	authID, ok := requireAuth(w, r)
	if !ok {
		return false
	}
	if authID != pathID {
		WriteError(w, http.StatusNotFound, "not found")
		return false
	}
	return true
}

// ensureCampaignOwner validates that the authenticated advertiser owns the
// campaign identified by campaignID. Used by handlers whose path id is a
// campaign id (e.g. all report routes and creative mutations), not an
// advertiser id.
//
//   - missing / invalid credentials → 401 Unauthorized
//   - authenticated but campaign belongs to another advertiser or does not
//     exist → 404 Not Found
//
// Returns true only when the Store confirms ownership.
func (d *Deps) ensureCampaignOwner(w http.ResponseWriter, r *http.Request, campaignID int64) bool {
	authID, ok := requireAuth(w, r)
	if !ok {
		return false
	}
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, authID); err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return false
	}
	return true
}

// ensureCreativeOwner resolves a creative id to its owning campaign and then
// verifies that the authenticated advertiser owns that campaign. Returns the
// resolved campaign_id on success so callers can feed the
// campaign:updates pub/sub notification after a successful mutation.
//
//   - missing / invalid credentials → 401 Unauthorized
//   - creative does not exist, or the owning campaign belongs to another
//     advertiser → 404 Not Found
func (d *Deps) ensureCreativeOwner(w http.ResponseWriter, r *http.Request, creativeID int64) (int64, bool) {
	authID, ok := requireAuth(w, r)
	if !ok {
		return 0, false
	}
	campaignID, err := d.Store.GetCreativeCampaignID(r.Context(), creativeID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, authID); err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return campaignID, true
}
