//go:build integration

package reporting_test

import (
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// Scenario 42 — mixed event counts aggregate correctly.
// Inserts 10 bids, 5 wins, 5 impressions, 2 clicks; verifies counts and ratios.
// CB5 is sidestepped by setting charge_cents=0 on non-win rows, so SpendCents
// semantics happen to match the CPM-billing expectation for this test input.
func TestStats_MixedCounts(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("stats-mix")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-stats-mix",
	})

	base := time.Now().UTC().Truncate(time.Second)
	// 10 bids with charge=0
	for i := 0; i < 10; i++ {
		h.InsertBidLogRow(campID, advID, 1, "bid", "qa-stats-bid", "d", 500, 0, 0, base)
	}
	// 5 wins with clear_price=450 and charge=500 (the billed advertiser amount)
	for i := 0; i < 5; i++ {
		h.InsertBidLogRow(campID, advID, 1, "win", "qa-stats-win", "d", 500, 450, 500, base)
	}
	// 5 impressions with charge=0 (sidestep CB5 double-count for this test)
	for i := 0; i < 5; i++ {
		h.InsertBidLogRow(campID, advID, 1, "impression", "qa-stats-imp", "d", 0, 0, 0, base)
	}
	// 2 clicks with charge=0
	for i := 0; i < 2; i++ {
		h.InsertBidLogRow(campID, advID, 1, "click", "qa-stats-click", "d", 0, 0, 0, base)
	}

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// event_date is a CH Date column. GetCampaignStats filters `event_date >= from`,
	// so `from` must be day-start UTC to not cut off today's rows.
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := dayStart
	to := dayStart.Add(24 * time.Hour)
	stats, err := store.GetCampaignStats(h.Ctx, uint64(campID), from, to)
	if err != nil {
		t.Fatal(err)
	}

	if stats.Bids != 10 {
		t.Errorf("Bids: want 10, got %d", stats.Bids)
	}
	if stats.Wins != 5 {
		t.Errorf("Wins: want 5, got %d", stats.Wins)
	}
	if stats.Impressions != 5 {
		t.Errorf("Impressions: want 5, got %d", stats.Impressions)
	}
	if stats.Clicks != 2 {
		t.Errorf("Clicks: want 2, got %d", stats.Clicks)
	}
	// CTR = clicks / impressions * 100 = 2/5 * 100 = 40
	if stats.CTR != 40.0 {
		t.Errorf("CTR: want 40, got %v", stats.CTR)
	}
	// WinRate = wins / bids * 100 = 5/10 * 100 = 50
	if stats.WinRate != 50.0 {
		t.Errorf("WinRate: want 50, got %v", stats.WinRate)
	}
	// SpendCents should be 5 * 500 = 2500 in this sidestepped setup
	if stats.SpendCents != 2500 {
		t.Errorf("SpendCents: want 2500, got %d", stats.SpendCents)
	}
	// AdxCostCents = 5 * 450 = 2250
	if stats.AdxCostCents != 2250 {
		t.Errorf("AdxCostCents: want 2250, got %d", stats.AdxCostCents)
	}
}

// Scenario 43 — boundary values don't break the query.
// - clear_price_cents at UInt32 max
// - device_id="" is excluded by GetAttributionReport (AND device_id != '')
func TestStats_FieldBoundaries(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("stats-bound")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-stats-bound",
	})

	base := time.Now().UTC().Truncate(time.Second)
	// UInt32 max clear_price_cents — must not overflow/crash the query
	h.InsertBidLogRow(campID, advID, 1, "win", "qa-stats-max", "dX", 0, 0xFFFFFFFF, 500, base)
	// Empty device_id — must be excluded from GetAttributionReport
	h.InsertBidLogRow(campID, advID, 1, "conversion", "qa-stats-empty", "", 0, 0, 0, base)

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Day-aligned UTC range — see TestStats_MixedCounts for the rationale.
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	from := dayStart
	to := dayStart.Add(24 * time.Hour)

	// GetCampaignStats must not error on UInt32 max
	stats, err := store.GetCampaignStats(h.Ctx, uint64(campID), from, to)
	if err != nil {
		t.Fatalf("GetCampaignStats: %v", err)
	}
	if stats.Wins != 1 {
		t.Errorf("Wins: want 1, got %d", stats.Wins)
	}

	// GetAttributionReport must exclude the empty device_id conversion
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), from, to, reporting.ModelLastClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalConversions != 0 {
		t.Errorf("empty device_id should be excluded, got TotalConversions=%d",
			report.TotalConversions)
	}
}
