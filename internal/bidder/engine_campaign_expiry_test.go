//go:build integration

package bidder_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
)

// TestEngine_CampaignExpiry_ExcludedFromBid asserts the engine-level
// end-to-end integration of the `campaignDateActive` predicate: a
// campaign whose end_date lies in the past must not receive any bid,
// even if every other filter (targeting, budget, fraud, guardrail)
// would otherwise match.
//
// The unit-level predicates (TestCampaignDateActive_PastEndDate etc.
// in engine_test.go) guard the function's return value. This
// integration test guards the WIRING — that the engine actually
// consults the predicate inside the candidate iteration at
// engine.go:140 and short-circuits correctly.
//
// REGRESSION SENTINEL: P2-10 campaign expiry end-to-end
// (docs/testing-strategy-bidder.md §3 P2). Break-revert verified
// 2026-04-19: see commit message for the exact dance.
func TestEngine_CampaignExpiry_ExcludedFromBid(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("expiry")

	// Seed a campaign normally, then UPDATE its end_date to one hour ago.
	// The default CampaignSpec has StartDate=nil and EndDate=nil, so the
	// direct SQL update below is the only source of the past end_date.
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-expired",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")

	// campaigns.end_date is compared in Postgres via `end_date >= NOW()` during
	// loader full-load. The column is timezone-naive, so seed the past value in
	// UTC to avoid local-time offsets making a logically expired campaign appear
	// future-dated on a UTC-configured test database.
	pastEnd := time.Now().UTC().Add(-1 * time.Hour)
	_, err := f.PG.Exec(f.Ctx,
		`UPDATE campaigns SET end_date = $1 WHERE id = $2`, pastEnd, campID)
	if err != nil {
		t.Fatalf("update end_date: %v", err)
	}

	// Start the loader AFTER the end_date update so the initial full-load
	// reads the already-expired state.
	f.Start(t)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID: fmt.Sprintf("qa-expiry-%d", time.Now().UnixNano()),
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid (nil) for expired campaign, got %+v", resp)
	}
}
