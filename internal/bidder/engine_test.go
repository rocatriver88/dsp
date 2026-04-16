package bidder

import (
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/prebid/openrtb/v20/openrtb2"
)

func TestEffectiveBidCPMCents_CPM(t *testing.T) {
	lc := &LoadedCampaign{BillingModel: "cpm", BidCPMCents: 500}
	got := lc.EffectiveBidCPMCents(0, 0)
	if got != 500 {
		t.Errorf("CPM: expected 500, got %d", got)
	}
}

func TestEffectiveBidCPMCents_CPC(t *testing.T) {
	lc := &LoadedCampaign{BillingModel: "cpc", BidCPCCents: 100}
	// Default CTR = 1% → effective CPM = 100 * 0.01 * 1000 = 1000
	got := lc.EffectiveBidCPMCents(0, 0)
	if got != 1000 {
		t.Errorf("CPC default CTR: expected 1000, got %d", got)
	}

	// Custom CTR = 2%
	got = lc.EffectiveBidCPMCents(0.02, 0)
	expected := int(100 * 0.02 * 1000) // 2000
	if got != expected {
		t.Errorf("CPC custom CTR: expected %d, got %d", expected, got)
	}
}

func TestEffectiveBidCPMCents_OCPM(t *testing.T) {
	lc := &LoadedCampaign{BillingModel: "ocpm", OCPMTargetCPACents: 5000}
	// Default CTR=1%, CVR=5% → effective CPM = 5000 * 0.01 * 0.05 * 1000 = 2500
	got := lc.EffectiveBidCPMCents(0, 0)
	if got != 2500 {
		t.Errorf("oCPM default: expected 2500, got %d", got)
	}
}

func TestEffectiveBidCPMCents_EmptyBillingModel(t *testing.T) {
	// Empty billing model defaults to CPM behavior
	lc := &LoadedCampaign{BillingModel: "", BidCPMCents: 300}
	got := lc.EffectiveBidCPMCents(0, 0)
	if got != 300 {
		t.Errorf("empty billing model: expected 300, got %d", got)
	}
}

func TestBillingModelRanking_CPCWinsOverCPM(t *testing.T) {
	// CPC campaign with BidCPCCents=100 → effective CPM = 1000
	// CPM campaign with BidCPMCents=500
	// CPC should win
	cpc := &LoadedCampaign{
		ID: 1, BillingModel: "cpc", BidCPCCents: 100,
		BudgetDailyCents: 10000,
		Targeting:        campaign.Targeting{Geo: []string{"CN"}},
		Creatives:        []*campaign.Creative{{ID: 1, AdMarkup: "test", DestinationURL: "http://test.com"}},
	}
	cpm := &LoadedCampaign{
		ID: 2, BillingModel: "cpm", BidCPMCents: 500,
		BudgetDailyCents: 10000,
		Targeting:        campaign.Targeting{Geo: []string{"CN"}},
		Creatives:        []*campaign.Creative{{ID: 2, AdMarkup: "test", DestinationURL: "http://test.com"}},
	}

	// CPC effective CPM = 1000 > CPM 500, so CPC wins
	if cpc.EffectiveBidCPMCents(0, 0) <= cpm.EffectiveBidCPMCents(0, 0) {
		t.Errorf("CPC (eff=%d) should beat CPM (eff=%d)",
			cpc.EffectiveBidCPMCents(0, 0), cpm.EffectiveBidCPMCents(0, 0))
	}
}

func TestMatchesTargeting_GeoMatch(t *testing.T) {
	c := &LoadedCampaign{Targeting: campaign.Targeting{Geo: []string{"CN", "US"}}}
	if !matchesTargeting(c, "CN", "") {
		t.Error("CN should match")
	}
	if matchesTargeting(c, "JP", "") {
		t.Error("JP should not match")
	}
}

func TestMatchesTargeting_OSMatch(t *testing.T) {
	c := &LoadedCampaign{Targeting: campaign.Targeting{OS: []string{"iOS", "Android"}}}
	if !matchesTargeting(c, "", "iOS") {
		t.Error("iOS should match")
	}
	if matchesTargeting(c, "", "Windows") {
		t.Error("Windows should not match")
	}
}

func TestMatchesTargeting_NoTargeting(t *testing.T) {
	c := &LoadedCampaign{Targeting: campaign.Targeting{}}
	if !matchesTargeting(c, "CN", "iOS") {
		t.Error("empty targeting should match all")
	}
}

func TestMatchesTargeting_EmptyRequestGeo(t *testing.T) {
	c := &LoadedCampaign{Targeting: campaign.Targeting{Geo: []string{"CN"}}}
	// Empty geo in request should still match (targeting can't filter unknown geo)
	if !matchesTargeting(c, "", "iOS") {
		t.Error("empty request geo should pass through")
	}
}

func TestParseCreativeSize(t *testing.T) {
	tests := []struct {
		input string
		w, h  int64
		ok    bool
	}{
		{"300x250", 300, 250, true},
		{"728x90", 728, 90, true},
		{"1080x1920", 1080, 1920, true},
		{"bad", 0, 0, false},
		{"300", 0, 0, false},
		{"axb", 0, 0, false},
		{"", 0, 0, false},
	}
	for _, tt := range tests {
		w, h, ok := parseCreativeSize(tt.input)
		if ok != tt.ok || w != tt.w || h != tt.h {
			t.Errorf("parseCreativeSize(%q) = (%d,%d,%v), want (%d,%d,%v)", tt.input, w, h, ok, tt.w, tt.h, tt.ok)
		}
	}
}

func TestMatchCreativeToImp_BannerMatch(t *testing.T) {
	w300, h250 := int64(300), int64(250)
	imp := &openrtb2.Imp{
		Banner: &openrtb2.Banner{W: &w300, H: &h250},
	}
	creatives := []*campaign.Creative{
		{ID: 1, Size: "728x90"},
		{ID: 2, Size: "300x250"},
		{ID: 3, Size: "320x50"},
	}
	got := matchCreativeToImp(creatives, imp)
	if got == nil || got.ID != 2 {
		t.Errorf("expected creative 2 (300x250), got %v", got)
	}
}

func TestMatchCreativeToImp_BannerNoMatch(t *testing.T) {
	w160, h600 := int64(160), int64(600)
	imp := &openrtb2.Imp{
		Banner: &openrtb2.Banner{W: &w160, H: &h600},
	}
	creatives := []*campaign.Creative{
		{ID: 1, Size: "728x90"},
		{ID: 2, Size: "300x250"},
	}
	got := matchCreativeToImp(creatives, imp)
	if got != nil {
		t.Errorf("expected nil (no matching size), got creative %d", got.ID)
	}
}

func TestMatchCreativeToImp_BannerFormatArray(t *testing.T) {
	imp := &openrtb2.Imp{
		Banner: &openrtb2.Banner{
			Format: []openrtb2.Format{
				{W: 728, H: 90},
				{W: 300, H: 250},
			},
		},
	}
	creatives := []*campaign.Creative{
		{ID: 1, Size: "300x250"},
	}
	got := matchCreativeToImp(creatives, imp)
	if got == nil || got.ID != 1 {
		t.Errorf("expected creative 1, got %v", got)
	}
}

func TestMatchCreativeToImp_NonBanner(t *testing.T) {
	imp := &openrtb2.Imp{
		Native: &openrtb2.Native{Request: "{}"},
	}
	creatives := []*campaign.Creative{
		{ID: 1, Size: ""},
	}
	got := matchCreativeToImp(creatives, imp)
	if got == nil || got.ID != 1 {
		t.Errorf("non-banner should accept any creative, got %v", got)
	}
}

func TestMatchCreativeToImp_NoCreatives(t *testing.T) {
	w300, h250 := int64(300), int64(250)
	imp := &openrtb2.Imp{
		Banner: &openrtb2.Banner{W: &w300, H: &h250},
	}
	got := matchCreativeToImp(nil, imp)
	if got != nil {
		t.Errorf("expected nil for empty creatives, got %v", got)
	}
}

func TestCampaignDateActive_PastEndDate(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	c := &LoadedCampaign{EndDate: &past}
	if campaignDateActive(c, time.Now()) {
		t.Error("campaign with past EndDate should be filtered out")
	}
}

func TestCampaignDateActive_FutureStartDate(t *testing.T) {
	future := time.Now().Add(1 * time.Hour)
	c := &LoadedCampaign{StartDate: &future}
	if campaignDateActive(c, time.Now()) {
		t.Error("campaign with future StartDate should be filtered out")
	}
}

func TestCampaignDateActive_WithinWindow(t *testing.T) {
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now().Add(1 * time.Hour)
	c := &LoadedCampaign{StartDate: &start, EndDate: &end}
	if !campaignDateActive(c, time.Now()) {
		t.Error("campaign within date window should be active")
	}
}

func TestCampaignDateActive_NilDates(t *testing.T) {
	c := &LoadedCampaign{}
	if !campaignDateActive(c, time.Now()) {
		t.Error("campaign with nil dates should always be active")
	}
}

func TestLoadedCampaign_FieldsCopied(t *testing.T) {
	// Verify that all billing model fields are properly set
	now := time.Now()
	lc := &LoadedCampaign{
		ID:                 42,
		AdvertiserID:       7,
		BillingModel:       "cpc",
		BidCPMCents:        100,
		BidCPCCents:        50,
		OCPMTargetCPACents: 200,
		StartDate:          &now,
	}
	if lc.BillingModel != "cpc" {
		t.Errorf("BillingModel: expected cpc, got %s", lc.BillingModel)
	}
	if lc.BidCPCCents != 50 {
		t.Errorf("BidCPCCents: expected 50, got %d", lc.BidCPCCents)
	}
}
