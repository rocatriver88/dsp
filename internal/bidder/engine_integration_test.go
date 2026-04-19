//go:build integration

package bidder_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// engineFixture bundles a TestHarness with a fully-wired Engine + CampaignLoader.
// Seed advertisers/campaigns/creatives BEFORE calling Start so the loader's
// initial full-load picks them up — the 30s periodic refresh is too slow for
// unit-style tests.
type engineFixture struct {
	*qaharness.TestHarness
	engine *bidder.Engine
	loader *bidder.CampaignLoader
	guard  *guardrail.Guardrail
}

// newEngineFixtureWithGuard allows overriding guardrail config for scenarios
// 18/19. The loader is constructed but NOT started; call f.Start(t) after
// seeding rows.
func newEngineFixtureWithGuard(t *testing.T, guardCfg guardrail.Config) *engineFixture {
	t.Helper()
	h := qaharness.New(t)
	loader := bidder.NewCampaignLoader(h.PG, h.RDB)
	budgetSvc := budget.New(h.RDB)
	strategySvc := bidder.NewBidStrategy(h.RDB)
	statsCache := bidder.NewStatsCache(h.RDB, nil, loader.GetActiveCampaigns)
	fraudFilter := antifraud.NewFilter(h.RDB)
	guard := guardrail.New(h.RDB, guardCfg)
	producer := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	t.Cleanup(producer.Close)

	eng := bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)

	return &engineFixture{
		TestHarness: h,
		engine:      eng,
		loader:      loader,
		guard:       guard,
	}
}

// Start begins the loader with a context that is cancelled on test cleanup.
// Call after all SeedCampaign/SeedCreative calls so the loader's synchronous
// initial fullLoad sees them.
func (f *engineFixture) Start(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithCancel(f.Ctx)
	if err := f.loader.Start(ctx); err != nil {
		cancel()
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(func() {
		f.loader.Stop()
		cancel()
	})
}

func newEngineFixture(t *testing.T) *engineFixture {
	return newEngineFixtureWithGuard(t, guardrail.Config{})
}

// TestEngine_BidHappyPath (Scenario 12) — a seeded CPM campaign receives one
// bid, the engine returns a priced response, and Kafka sees the bid event.
func TestEngine_BidHappyPath(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("bid-happy")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-happy",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	reqID := fmt.Sprintf("qa-happy-%d", time.Now().UnixNano())
	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{ID: reqID})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: %v", err)
	}
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		t.Fatal("expected a bid response")
	}
	bid := resp.SeatBid[0].Bid[0]
	if bid.Price <= 0 {
		t.Errorf("bid.Price should be > 0, got %v", bid.Price)
	}
	if bid.CID != fmt.Sprintf("%d", campID) {
		t.Errorf("bid.CID: want %d, got %s", campID, bid.CID)
	}

	// Kafka should see exactly 1 dsp.bids event for this request_id.
	// The producer is Async, so the write is batched — give it ample time
	// for the initial connection handshake and batch flush.
	kCount := f.CountMessages("dsp.bids", reqID, 15*time.Second)
	if kCount != 1 {
		t.Errorf("dsp.bids count: want 1 got %d", kCount)
	}
}

// TestEngine_MultiCandidateHighestBid (Scenario 13) — with three candidates
// on the same advertiser, the highest effective CPM wins the auction.
func TestEngine_MultiCandidateHighestBid(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("multi-candidate")

	lowID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID, Name: "qa-multi-low", BidCPMCents: 500,
	})
	f.SeedCreative(lowID, "", "")

	midID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID, Name: "qa-multi-mid", BidCPMCents: 1000,
	})
	f.SeedCreative(midID, "", "")

	highID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID, Name: "qa-multi-high", BidCPMCents: 2000,
	})
	f.SeedCreative(highID, "", "")

	f.Start(t)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID: fmt.Sprintf("qa-multi-%d", time.Now().UnixNano()),
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: %v", err)
	}
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		t.Fatal("expected a bid response")
	}
	gotCID := resp.SeatBid[0].Bid[0].CID
	wantCID := fmt.Sprintf("%d", highID)
	if gotCID != wantCID {
		t.Errorf("highest-bid campaign: want CID=%s (2000 cents), got %s", wantCID, gotCID)
	}
}

// TestEngine_NoTargetingMatch (Scenario 14) — a campaign targeting CN does
// not match a US bid request; engine returns nil, nil.
func TestEngine_NoTargetingMatch(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("no-target")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-no-target",
		BidCPMCents:  2000,
		TargetingGeo: []string{"CN"},
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID:  fmt.Sprintf("qa-no-target-%d", time.Now().UnixNano()),
		Geo: "US",
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid (nil response), got %+v", resp)
	}
}

// TestEngine_NoDevice (Scenario 15) — a bid request with Device=nil is
// treated as a likely bot and gets no-bid.
func TestEngine_NoDevice(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("nodevice")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-nodevice",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	w, h := int64(320), int64(50)
	req := &openrtb2.BidRequest{
		ID: fmt.Sprintf("qa-nodevice-%d", time.Now().UnixNano()),
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: nil,
	}
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid on Device=nil, got %+v", resp)
	}
}

// TestEngine_NoFormat (Scenario 16) — an imp with no Banner/Video/Native
// format gets no-bid.
func TestEngine_NoFormat(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("noformat")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-noformat",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	req := &openrtb2.BidRequest{
		ID:  fmt.Sprintf("qa-noformat-%d", time.Now().UnixNano()),
		Imp: []openrtb2.Imp{{ID: "imp-1"}},
		Device: &openrtb2.Device{
			OS:  "iOS",
			Geo: &openrtb2.Geo{Country: "CN"},
		},
	}
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid on missing format, got %+v", resp)
	}
}

// TestEngine_BidFloorFilter (Scenario 17) — a cheap CPM campaign is filtered
// out when the imp's bidfloor exceeds the per-impression bid price.
func TestEngine_BidFloorFilter(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("bidfloor")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-bidfloor",
		BidCPMCents:  100, // bidPrice ≈ 0.0009, far below floor
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID:       fmt.Sprintf("qa-bidfloor-%d", time.Now().UnixNano()),
		BidFloor: 999.0,
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid below floor, got %+v", resp)
	}
}

// TestEngine_GuardrailPreCheckDenies (Scenario 18) — when the circuit breaker
// is tripped, PreCheck denies and the engine returns nil without iterating
// any candidates.
//
// REGRESSION SENTINEL: P0-3 engine-layer honoring of guardrail denial
// (docs/testing-strategy-bidder.md §3 P0-3). The guardrail unit test
// TestCircuitBreaker_FailClosed_OnRedisError guards the fail-closed return
// value; this integration test guards the WIRING — that the engine's Bid()
// actually respects !PreCheck.Allowed by short-circuiting. The test uses
// CB.Trip() as a proxy trigger; the Redis-down scenario produces the same
// downstream path (Allowed=false), so covering one covers the wiring for
// both.
func TestEngine_GuardrailPreCheckDenies(t *testing.T) {
	f := newEngineFixtureWithGuard(t, guardrail.Config{})
	advID := f.SeedAdvertiser("precheck")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-precheck",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	// Trip the circuit breaker: PreCheck() denies if CB is NOT open.
	f.guard.CB.Trip(f.Ctx, "qa-test-trip")
	t.Cleanup(func() { f.guard.CB.Reset(f.Ctx) })

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID: fmt.Sprintf("qa-precheck-%d", time.Now().UnixNano()),
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid when CB tripped, got %+v", resp)
	}
}

// TestEngine_BidCeilingCap (Scenario 19) — when the guardrail MaxBidCPMCents
// is below the campaign's CPM, the candidate is filtered out via
// CheckBidCeiling and the auction has no winner.
func TestEngine_BidCeilingCap(t *testing.T) {
	f := newEngineFixtureWithGuard(t, guardrail.Config{MaxBidCPMCents: 500})
	advID := f.SeedAdvertiser("ceiling")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-ceiling",
		BidCPMCents:  2000, // above the 500 cap
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID: fmt.Sprintf("qa-ceiling-%d", time.Now().UnixNano()),
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid when cap exceeded, got %+v", resp)
	}
}

// TestEngine_BudgetExhausted (Scenario 20) — if the daily budget is zero,
// PipelineCheck returns budgetOK=false and the engine returns nil.
func TestEngine_BudgetExhausted(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("budget-exhausted")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-engine-budget",
		BidCPMCents:      2000,
		BudgetDailyCents: 100_000,
	})
	f.SeedCreative(campID, "", "")
	f.Start(t)

	// Zero out the daily budget after the loader loaded it.
	f.SetBudgetRemaining(campID, 0)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{
		ID: fmt.Sprintf("qa-budget-%d", time.Now().UnixNano()),
	})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: unexpected err: %v", err)
	}
	if resp != nil {
		t.Errorf("expected no-bid when budget exhausted, got %+v", resp)
	}
}

// TestEngine_CPCStatsCacheConsistency (Scenario 21, CB6 probe) — a CPC
// campaign with a seeded CTR=0.05 in statscache should produce a bid whose
// BidPrice reflects the cached CTR. If it reflects the default CTR=0.01
// instead, CB6 is confirmed.
func TestEngine_CPCStatsCacheConsistency(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("cpc-stats")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-cpc-stats",
		BillingModel: campaign.BillingCPC,
		BidCPCCents:  100,
	})
	f.SeedCreative(campID, "", "")

	// Seed CTR=0.05, CVR=0.0 in Redis so StatsCache.Get returns it directly.
	if err := f.RDB.Set(f.Ctx, fmt.Sprintf("stats:ctr:%d", campID), "0.05", 10*time.Minute).Err(); err != nil {
		t.Fatalf("seed ctr: %v", err)
	}
	if err := f.RDB.Set(f.Ctx, fmt.Sprintf("stats:cvr:%d", campID), "0.0", 10*time.Minute).Err(); err != nil {
		t.Fatalf("seed cvr: %v", err)
	}

	f.Start(t)

	reqID := fmt.Sprintf("qa-cpc-stats-%d", time.Now().UnixNano())
	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{ID: reqID})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid: %v", err)
	}
	if resp == nil || len(resp.SeatBid) == 0 || len(resp.SeatBid[0].Bid) == 0 {
		t.Fatal("expected a bid response")
	}

	msgs := f.ReadMessagesFrom("dsp.bids", reqID, 1, 15*time.Second)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 dsp.bids event, got %d", len(msgs))
	}
	evt := msgs[0]

	// Expected path with CTR=0.05:
	//   EffectiveBidCPMCents (CPC) = BidCPCCents * CTR * 1000 = 100 * 0.05 * 1000 = 5000
	//   bidPrice = effectiveCPM * 0.90 / 100 / 1000 = 5000 * 0.0009 / 100 = 0.045
	// The CB6 failure mode is CTR=0.01 (the in-function default), which would
	// yield effectiveCPM=1000 and bidPrice=0.009.
	expectedBidPrice := float64(100) * 0.05 * 1000 * 0.90 / 100.0 / 1000.0
	tolerance := expectedBidPrice * 0.15 // strategy may shift ±10-15%

	if math.Abs(evt.BidPrice-expectedBidPrice) > tolerance {
		t.Errorf("CB6 probe: bid.Price=%.6f, expected ≈%.6f (±%.6f) — CTR from statsCache not propagated correctly",
			evt.BidPrice, expectedBidPrice, tolerance)
	}
}
