//go:build integration

package bidder_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
)

// TestEngineBid_MultiTenant_AttributesCorrectAdvertiser asserts that when the
// engine's auction has candidates from multiple advertisers, the winning bid's
// advertiser attribution in the downstream Kafka event matches the winning
// campaign's advertiser — NOT the loser's, and not a zero / default value.
//
// This is P0-4 from docs/testing-strategy-bidder.md §3: the engine hot path
// must not leak or swap advertiser_id when building the bid response and the
// Kafka event. A refactor that reads from a stale loop variable, drops the
// AdvertiserID field from events.Event, or misattributes to the first
// candidate instead of the winner would be caught here.
//
// REGRESSION SENTINEL: multi-tenant attribution at engine hot path. Guards
// internal/bidder/engine.go:276 (`AdvertiserID: best.AdvertiserID`) and its
// surrounding loop-variable discipline. Complements PR #18's loader-level
// tenant isolation sentinel with engine-level attribution verification.
// Break-revert verified 2026-04-19: see commit message for exact dance.
func TestEngineBid_MultiTenant_AttributesCorrectAdvertiser(t *testing.T) {
	f := newEngineFixture(t)

	// Two advertisers, each with one CPM campaign. Tenant B bids higher so
	// it wins deterministically — we don't want pacing or win-rate dynamics
	// to flip the winner across runs.
	advA := f.SeedAdvertiser("attr-loser")
	campA := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advA,
		Name:         "qa-attr-loser",
		BidCPMCents:  1000,
	})
	f.SeedCreative(campA, "", "")

	advB := f.SeedAdvertiser("attr-winner")
	campB := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advB,
		Name:         "qa-attr-winner",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campB, "", "")

	// Sanity: seeded advertisers must be distinct; otherwise the test proves
	// nothing (advA == advB would make attribution assertions trivially pass).
	if advA == advB {
		t.Fatalf("seed precondition: distinct advertisers collided on id=%d", advA)
	}

	f.Start(t)

	reqID := fmt.Sprintf("qa-attr-%d", time.Now().UnixNano())
	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{ID: reqID})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: %v", err)
	}
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		t.Fatal("expected a bid response with one SeatBid > one Bid")
	}

	// SeatBid uses campaign ID for CID; winner is tenant B (higher CPM).
	wantCID := fmt.Sprintf("%d", campB)
	if got := resp.SeatBid[0].Bid[0].CID; got != wantCID {
		t.Errorf("bid.CID: want %s (tenant B — higher CPM), got %s", wantCID, got)
	}

	// The Kafka event is where advertiser attribution ACTUALLY travels
	// downstream (to ClickHouse, billing, reporting). Read it and verify.
	evts := f.ReadMessagesFrom("dsp.bids", reqID, 1, 15*time.Second)
	if len(evts) == 0 {
		t.Fatal("dsp.bids: expected 1 event, got 0")
	}
	got := evts[0]

	if got.AdvertiserID != advB {
		t.Errorf("Kafka event AdvertiserID: want %d (tenant B), got %d", advB, got.AdvertiserID)
	}
	if got.AdvertiserID == advA {
		t.Errorf("CRITICAL: winning bid event attributed to the LOSER's advertiser (advA=%d)", advA)
	}
	if got.CampaignID != campB {
		t.Errorf("Kafka event CampaignID: want %d, got %d", campB, got.CampaignID)
	}
	if got.AdvertiserID == 0 {
		t.Error("Kafka event AdvertiserID is zero — attribution lost (possible refactor dropped the field)")
	}
}
