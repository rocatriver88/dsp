package bidder

import (
	"testing"

	"github.com/prebid/openrtb/v20/openrtb2"
)

func TestProcessRequest_MatchesCampaign(t *testing.T) {
	b := New()
	w, h := int64(300), int64(250)

	req := &openrtb2.BidRequest{
		ID: "test-1",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "CN"},
		},
	}

	result := b.ProcessRequest(req)
	if result == nil {
		t.Fatal("expected a bid, got nil")
	}
	// Should pick highest CPM campaign that matches CN + macOS
	// Campaign 2 (cloud, CPM 8.0, CN) or Campaign 4 (SaaS, CPM 15.0, US/GB/DE only)
	// Campaign 2 should win for CN
	if result.Campaign.ID != 2 {
		t.Errorf("expected campaign 2 (highest CPM for CN+macOS), got %d (%s)",
			result.Campaign.ID, result.Campaign.Name)
	}
	if result.BidPrice <= 0 {
		t.Errorf("expected positive bid price, got %f", result.BidPrice)
	}
}

func TestProcessRequest_NoBidForUnknownGeo(t *testing.T) {
	b := New()
	w, h := int64(300), int64(250)

	req := &openrtb2.BidRequest{
		ID: "test-2",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "ZZ"}, // non-existent country
		},
	}

	result := b.ProcessRequest(req)
	if result != nil {
		t.Errorf("expected no bid for country ZZ, got campaign %d", result.Campaign.ID)
	}
}

func TestProcessRequest_NilRequest(t *testing.T) {
	b := New()
	result := b.ProcessRequest(nil)
	if result != nil {
		t.Error("expected nil for nil request")
	}
}

func TestProcessRequest_EmptyImps(t *testing.T) {
	b := New()
	req := &openrtb2.BidRequest{ID: "test-3", Imp: []openrtb2.Imp{}}
	result := b.ProcessRequest(req)
	if result != nil {
		t.Error("expected nil for empty impressions")
	}
}

func TestProcessRequest_NoFormatMatch(t *testing.T) {
	b := New()
	req := &openrtb2.BidRequest{
		ID: "test-4",
		Imp: []openrtb2.Imp{{
			ID: "imp-1",
			// No banner, video, or native — should not match
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "CN"},
		},
	}

	result := b.ProcessRequest(req)
	if result != nil {
		t.Error("expected no bid when no ad format specified")
	}
}

func TestRecordWin_DeductsBudget(t *testing.T) {
	b := New()
	ok := b.RecordWin(1, 0.005)
	if !ok {
		t.Error("expected win to succeed")
	}

	stats := b.Stats()
	for _, s := range stats {
		if s["id"].(int) == 1 {
			spent := s["spent"].(float64)
			if spent != 0.005 {
				t.Errorf("expected spent=0.005, got %f", spent)
			}
		}
	}
}

func TestRecordWin_BudgetExhausted(t *testing.T) {
	b := New()
	// Campaign 1 has DailyBudget 1000
	// Exhaust it
	b.RecordWin(1, 1000)

	ok := b.RecordWin(1, 0.005)
	if ok {
		t.Error("expected win to fail after budget exhaustion")
	}
}

func TestRecordWin_InvalidCampaign(t *testing.T) {
	b := New()
	ok := b.RecordWin(999, 0.005)
	if ok {
		t.Error("expected win to fail for non-existent campaign")
	}
}

func TestProcessRequest_SkipsBudgetExhausted(t *testing.T) {
	b := New()
	// Exhaust all campaigns that match CN
	for _, c := range b.campaigns {
		c.mu.Lock()
		c.Spent = c.DailyBudget // exhaust
		c.mu.Unlock()
	}

	w, h := int64(300), int64(250)
	req := &openrtb2.BidRequest{
		ID: "test-budget",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "CN"},
		},
	}

	result := b.ProcessRequest(req)
	if result != nil {
		t.Error("expected no bid when all budgets exhausted")
	}
}

func TestProcessRequest_PicksHighestCPM(t *testing.T) {
	b := New()
	w, h := int64(300), int64(250)

	// US + macOS should match Campaign 1 (CPM 5.0) and Campaign 4 (CPM 15.0)
	req := &openrtb2.BidRequest{
		ID: "test-cpm",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "US"},
		},
	}

	result := b.ProcessRequest(req)
	if result == nil {
		t.Fatal("expected a bid")
	}
	if result.Campaign.ID != 4 {
		t.Errorf("expected campaign 4 (highest CPM for US+macOS at 15.0), got %d (CPM %.1f)",
			result.Campaign.ID, result.Campaign.BidCPM)
	}
}
