//go:build integration

package reporting_test

import (
	"math"
	"strconv"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// seedPath seeds a conversion + preceding touchpoints for one device.
// touchpoints: ordered slice of (event_type, time-offset-from-base).
// A conversion event at baseTime + convOffset is appended implicitly.
func seedPath(
	h *qaharness.TestHarness,
	campID, advID int64,
	deviceID string,
	baseTime time.Time,
	touchpoints []struct {
		typ    string
		offset time.Duration
	},
	convOffset time.Duration,
) {
	for i, tp := range touchpoints {
		reqID := "qa-attr-tp-" + tp.typ + "-" + strconv.Itoa(i)
		h.InsertBidLogRow(campID, advID, 1, tp.typ, reqID, deviceID, 0, 0, 0, baseTime.Add(tp.offset))
	}
	h.InsertBidLogRow(campID, advID, 1, "conversion", "qa-attr-conv-"+deviceID, deviceID, 0, 0, 0, baseTime.Add(convOffset))
}

func newStore(t *testing.T, h *qaharness.TestHarness) *reporting.Store {
	t.Helper()
	s, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatalf("reporting store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// Scenario 38 — last_click model credits the LAST touchpoint.
// Touchpoints: imp → click → imp → conv. Last touchpoint is imp (right before conv).
// Per current implementation, last_click gives Credit=1.0 to the last entry in
// the Credit array, regardless of type.
func TestAttribution_LastClick(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("attr-last")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-attr-last",
	})

	base := time.Now().UTC().Add(-2 * time.Hour)
	seedPath(h, campID, advID, "dA", base,
		[]struct {
			typ    string
			offset time.Duration
		}{
			{"impression", 0},
			{"click", 10 * time.Minute},
			{"impression", 20 * time.Minute},
		},
		30*time.Minute,
	)

	store := newStore(t, h)
	from := base.Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), from, to, reporting.ModelLastClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.AttributedPaths) != 1 {
		t.Fatalf("paths: want 1 got %d", len(report.AttributedPaths))
	}
	path := report.AttributedPaths[0]
	if len(path.Credit) != 3 {
		t.Fatalf("credits: want 3 got %d", len(path.Credit))
	}
	var sum float64
	for _, c := range path.Credit {
		sum += c.Credit
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("credit sum: want 1.0 got %v", sum)
	}
	if path.Credit[2].Credit != 1.0 {
		t.Errorf("last_click: last credit should be 1.0, got %v", path.Credit[2].Credit)
	}
	for i := 0; i < 2; i++ {
		if path.Credit[i].Credit != 0 {
			t.Errorf("last_click: credit[%d] should be 0, got %v", i, path.Credit[i].Credit)
		}
	}
}

// Scenario 39 — first_click model credits the FIRST touchpoint.
func TestAttribution_FirstClick(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("attr-first")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-attr-first",
	})

	base := time.Now().UTC().Add(-2 * time.Hour)
	seedPath(h, campID, advID, "dB", base,
		[]struct {
			typ    string
			offset time.Duration
		}{
			{"impression", 0},
			{"click", 10 * time.Minute},
			{"impression", 20 * time.Minute},
		},
		30*time.Minute,
	)

	store := newStore(t, h)
	from := base.Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), from, to, reporting.ModelFirstClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.AttributedPaths) != 1 {
		t.Fatalf("paths: want 1 got %d", len(report.AttributedPaths))
	}
	path := report.AttributedPaths[0]
	if len(path.Credit) != 3 {
		t.Fatalf("credits: want 3 got %d", len(path.Credit))
	}
	if path.Credit[0].Credit != 1.0 {
		t.Errorf("first_click: first credit should be 1.0, got %v", path.Credit[0].Credit)
	}
	var sum float64
	for _, c := range path.Credit {
		sum += c.Credit
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("credit sum: want 1.0 got %v", sum)
	}
	for i := 1; i < 3; i++ {
		if path.Credit[i].Credit != 0 {
			t.Errorf("first_click: credit[%d] should be 0, got %v", i, path.Credit[i].Credit)
		}
	}
}

// Scenario 40 — linear model splits credit equally.
func TestAttribution_Linear(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("attr-linear")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-attr-linear",
	})

	base := time.Now().UTC().Add(-2 * time.Hour)
	seedPath(h, campID, advID, "dC", base,
		[]struct {
			typ    string
			offset time.Duration
		}{
			{"impression", 0},
			{"impression", 10 * time.Minute},
			{"click", 20 * time.Minute},
		},
		30*time.Minute,
	)

	store := newStore(t, h)
	from := base.Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), from, to, reporting.ModelLinear, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.AttributedPaths) != 1 {
		t.Fatalf("paths: want 1 got %d", len(report.AttributedPaths))
	}
	path := report.AttributedPaths[0]
	if len(path.Credit) != 3 {
		t.Fatalf("credits: want 3 got %d", len(path.Credit))
	}
	expected := 1.0 / 3.0
	var sum float64
	for i, c := range path.Credit {
		if math.Abs(c.Credit-expected) > 1e-9 {
			t.Errorf("linear: credit[%d] = %v, want %v", i, c.Credit, expected)
		}
		sum += c.Credit
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("linear: credit sum = %v, want 1.0", sum)
	}
}

// Scenario 41 — empty touchpoints: conversion with no prior imp/click is skipped.
func TestAttribution_EmptyTouchpoints(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("attr-empty")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-attr-empty",
	})

	base := time.Now().UTC().Add(-time.Hour)
	// Insert conversion but NO preceding impression/click for this device.
	h.InsertBidLogRow(campID, advID, 1, "conversion", "qa-attr-empty-conv", "dD", 0, 0, 0, base)

	store := newStore(t, h)
	from := base.Add(-24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), from, to, reporting.ModelLastClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalConversions != 0 {
		t.Errorf("TotalConversions: want 0 (path had no touchpoints, should be skipped), got %d",
			report.TotalConversions)
	}
}
