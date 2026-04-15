//go:build integration

package bidder_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// waitForCache polls cl.GetCampaign(id) until the predicate matches `want`
// (true = should be present, false = should be absent) or the timeout fires.
func waitForCache(t *testing.T, cl *bidder.CampaignLoader, id int64, want bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := cl.GetCampaign(id) != nil
		if got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForCache: id=%d want=%v after %v", id, want, timeout)
}

// waitForGeo polls until the cache entry for id has its first Targeting.Geo
// element equal to wantGeo, or the timeout fires.
func waitForGeo(t *testing.T, cl *bidder.CampaignLoader, id int64, wantGeo string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c := cl.GetCampaign(id)
		if c != nil && len(c.Targeting.Geo) > 0 && c.Targeting.Geo[0] == wantGeo {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForGeo: id=%d want=%s after %v", id, wantGeo, timeout)
}

// startLoader builds, starts, and registers a t.Cleanup Stop for a loader.
func startLoader(t *testing.T, h *qaharness.TestHarness) *bidder.CampaignLoader {
	t.Helper()
	cl := bidder.NewCampaignLoader(h.PG, h.RDB)
	ctx, cancel := context.WithCancel(h.Ctx)
	if err := cl.Start(ctx); err != nil {
		cancel()
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(func() {
		cl.Stop()
		cancel()
	})
	return cl
}

// Scenario 1 — startup full load: seed 3 active + 2 paused + 1 draft,
// start loader, expect exactly the 3 active qa-* campaigns in the cache.
func TestLoader_InitialFullLoad(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-full")

	active := make([]int64, 0, 3)
	for i := 0; i < 3; i++ {
		id := h.SeedCampaign(qaharness.CampaignSpec{
			AdvertiserID: advID,
			Name:         fmt.Sprintf("qa-loader-active-%d", i),
			BidCPMCents:  1000,
		})
		h.SeedCreative(id, "", "")
		active = append(active, id)
	}
	paused := []int64{
		h.SeedCampaign(qaharness.CampaignSpec{AdvertiserID: advID, Name: "qa-loader-paused-a", Status: campaign.StatusPaused, BidCPMCents: 1000}),
		h.SeedCampaign(qaharness.CampaignSpec{AdvertiserID: advID, Name: "qa-loader-paused-b", Status: campaign.StatusPaused, BidCPMCents: 1000}),
	}
	draftID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID, Name: "qa-loader-draft", Status: campaign.StatusDraft, BidCPMCents: 1000,
	})

	cl := startLoader(t, h)

	cached := cl.GetActiveCampaigns()

	// Filter to only qa-* campaigns from THIS test (loader sees ALL active campaigns).
	seen := map[int64]bool{}
	for _, c := range cached {
		seen[c.ID] = true
	}
	for _, id := range active {
		if !seen[id] {
			t.Errorf("active campaign %d missing from cache", id)
		}
	}
	for _, id := range paused {
		if seen[id] {
			t.Errorf("paused campaign %d should not be in cache", id)
		}
	}
	if seen[draftID] {
		t.Errorf("draft campaign %d should not be in cache", draftID)
	}
}

// Scenario 2 — pub/sub "activated": a paused campaign flipped to active via
// UpdateCampaignStatus + publish should land in the cache within 1s.
func TestLoader_PubSubActivated(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-activated")

	id := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-loader-activated",
		Status:       campaign.StatusPaused,
		BidCPMCents:  1000,
	})
	h.SeedCreative(id, "", "")

	cl := startLoader(t, h)

	// Paused at startup → not in cache.
	waitForCache(t, cl, id, false, 500*time.Millisecond)

	// Flip to active and publish.
	h.UpdateCampaignStatus(id, campaign.StatusActive)
	h.PublishCampaignUpdate(id, "activated")

	waitForCache(t, cl, id, true, 1*time.Second)
}

// Scenario 3 — pub/sub remove actions (paused, completed, deleted):
// each subtest seeds an active campaign, verifies it's in cache, publishes
// the corresponding remove-action, and verifies removal.
func TestLoader_PubSubRemoveActions(t *testing.T) {
	actions := []string{"paused", "completed", "deleted"}
	for _, action := range actions {
		action := action
		t.Run(action, func(t *testing.T) {
			h := qaharness.New(t)
			advID := h.SeedAdvertiser("loader-remove-" + action)
			id := h.SeedCampaign(qaharness.CampaignSpec{
				AdvertiserID: advID,
				Name:         "qa-loader-remove-" + action,
				BidCPMCents:  1000,
			})
			h.SeedCreative(id, "", "")

			cl := startLoader(t, h)

			waitForCache(t, cl, id, true, 1*time.Second)

			h.PublishCampaignUpdate(id, action)

			waitForCache(t, cl, id, false, 1*time.Second)
		})
	}
}

// Scenario 4 — pub/sub "updated": changing the targeting JSON in Postgres
// and publishing should propagate to the cache within 1s.
func TestLoader_PubSubUpdatedTargeting(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-updated")

	id := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-loader-updated",
		BidCPMCents:  1000,
		TargetingGeo: []string{"US"},
		TargetingOS:  []string{"iOS"},
	})
	h.SeedCreative(id, "", "")

	cl := startLoader(t, h)

	// Initial: US.
	waitForCache(t, cl, id, true, 1*time.Second)
	waitForGeo(t, cl, id, "US", 500*time.Millisecond)

	// Update DB directly; the loader should not know until we publish.
	_, err := h.PG.Exec(h.Ctx, `UPDATE campaigns SET targeting = $1 WHERE id = $2`,
		[]byte(`{"geo":["CN"],"os":["iOS"]}`), id)
	if err != nil {
		t.Fatalf("update targeting: %v", err)
	}

	h.PublishCampaignUpdate(id, "updated")

	waitForGeo(t, cl, id, "CN", 1*time.Second)
}

// Scenario 5 — periodic fallback reload: seed a campaign AFTER the loader
// starts without publishing pub/sub. The periodic refresh should pick it up.
// This scenario uses a 200ms refresh interval via WithRefreshInterval so the
// test completes in <1s instead of exercising the production 30s cadence.
func TestLoader_FallbackReload(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-fallback")

	cl := bidder.NewCampaignLoader(h.PG, h.RDB, bidder.WithRefreshInterval(200*time.Millisecond))
	ctx, cancel := context.WithCancel(h.Ctx)
	if err := cl.Start(ctx); err != nil {
		cancel()
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(func() {
		cl.Stop()
		cancel()
	})

	// Seed directly in Postgres; no pub/sub.
	id := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-loader-fallback",
		BidCPMCents:  1000,
	})
	h.SeedCreative(id, "", "")

	// Should not yet be in cache.
	if cl.GetCampaign(id) != nil {
		t.Fatalf("campaign %d unexpectedly already in cache before periodic refresh", id)
	}

	t.Log("negative wait: ensuring two ticker intervals elapse for the fallback reload")
	time.Sleep(600 * time.Millisecond)

	if cl.GetCampaign(id) == nil {
		t.Fatalf("campaign %d missing from cache after fallback refresh", id)
	}
}

// Scenario 6 — malformed pub/sub message must not crash the loader; a
// subsequent valid message should still be processed.
func TestLoader_MalformedPubSub(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-malformed")

	cl := startLoader(t, h)

	// Send garbage — the loader should log-and-continue.
	h.PublishRaw("campaign:updates", "{not json")

	// Now do a valid flow on a fresh campaign.
	id := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-loader-malformed-recover",
		Status:       campaign.StatusPaused,
		BidCPMCents:  1000,
	})
	h.SeedCreative(id, "", "")
	h.UpdateCampaignStatus(id, campaign.StatusActive)
	h.PublishCampaignUpdate(id, "activated")

	waitForCache(t, cl, id, true, 1*time.Second)
}

// Scenario 7 — unknown action must be ignored; the cache must be unchanged.
func TestLoader_UnknownAction(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader-unknown")

	id := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-loader-unknown",
		BidCPMCents:  1000,
	})
	h.SeedCreative(id, "", "")

	cl := startLoader(t, h)

	waitForCache(t, cl, id, true, 1*time.Second)

	h.PublishCampaignUpdate(id, "weird")
	// Negative assertion: wait long enough that a mis-routed event would have
	// arrived and removed the campaign. 500ms is ~10x the normal delivery latency.
	time.Sleep(500 * time.Millisecond)

	if cl.GetCampaign(id) == nil {
		t.Fatalf("campaign %d removed by unknown action; should have been ignored", id)
	}
}
