//go:build integration

package bidder_test

import (
	"testing"

	"github.com/heartgryphon/dsp/internal/qaharness"
)

// TestLoader_TenantIsolation_NoCrossAdvertiserLeak asserts that when the
// CampaignLoader caches active campaigns from multiple advertisers, each
// cached LoadedCampaign retains its correct advertiser_id attribution.
//
// A bug where the loader accidentally swaps, shares, or defaults advertiser_id
// — for example a SQL refactor that drops the column, a row-iteration bug that
// re-uses a struct across iterations, or a cache-key collision — would cause
// bid events to be attributed to the wrong tenant and corrupt billing.
//
// REGRESSION SENTINEL: tenant attribution at bidder hot path.
// Implements CLAUDE.md TDD Evidence Rule 3 (tenant-isolation tests must hit a
// real Store — nil-store / mock-only is blocked for this class of test).
func TestLoader_TenantIsolation_NoCrossAdvertiserLeak(t *testing.T) {
	h := qaharness.New(t)

	advA := h.SeedAdvertiser("tenant-iso-a")
	advB := h.SeedAdvertiser("tenant-iso-b")
	if advA == advB {
		t.Fatalf("seed precondition: distinct advertisers collided on id=%d", advA)
	}

	campA := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advA,
		Name:         "qa-tenant-iso-a",
		BidCPMCents:  1000,
	})
	h.SeedCreative(campA, "", "")

	campB := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advB,
		Name:         "qa-tenant-iso-b",
		BidCPMCents:  1000,
	})
	h.SeedCreative(campB, "", "")

	cl := startLoader(t, h)

	loadedA := cl.GetCampaign(campA)
	if loadedA == nil {
		t.Fatalf("campaign %d (tenant A) missing from cache after initial load", campA)
	}
	loadedB := cl.GetCampaign(campB)
	if loadedB == nil {
		t.Fatalf("campaign %d (tenant B) missing from cache after initial load", campB)
	}

	if loadedA.AdvertiserID != advA {
		t.Errorf("tenant A's campaign %d loaded with wrong advertiser_id: got %d, want %d",
			campA, loadedA.AdvertiserID, advA)
	}
	if loadedB.AdvertiserID != advB {
		t.Errorf("tenant B's campaign %d loaded with wrong advertiser_id: got %d, want %d",
			campB, loadedB.AdvertiserID, advB)
	}

	// Cross-check: if the loader collapsed distinct advertisers into one, the
	// per-campaign assertions above might individually pass (both matching the
	// wrong advertiser) while attribution is still broken across tenants.
	if loadedA.AdvertiserID == loadedB.AdvertiserID {
		t.Errorf("loader collapsed distinct advertisers: both campaigns report advertiser_id=%d (expected A=%d, B=%d)",
			loadedA.AdvertiserID, advA, advB)
	}
}
