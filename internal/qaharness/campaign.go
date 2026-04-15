package qaharness

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
)

// SeedAdvertiser inserts a qa-prefixed advertiser and returns its id.
// id is constrained to the 900000+ range so ClickHouse Reset can target it.
func (h *TestHarness) SeedAdvertiser(name string) int64 {
	h.TestT.Helper()
	if name == "" {
		name = "qa-adv"
	}
	id := int64(900000 + rand.Int63n(99999))
	var returned int64
	err := h.PG.QueryRow(h.Ctx, `
		INSERT INTO advertisers (id, company_name, contact_email, api_key, balance_cents, billing_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, id, fmt.Sprintf("qa-%s-%d", name, id), fmt.Sprintf("qa-%d@test.local", id),
		fmt.Sprintf("qa-key-%d", id), int64(10_000_000), "prepaid").Scan(&returned)
	if err != nil {
		h.TestT.Fatalf("SeedAdvertiser: insert (id=%d): %v", id, err)
	}
	return returned
}

// CampaignSpec describes a campaign to seed. Zero values get safe defaults.
type CampaignSpec struct {
	AdvertiserID       int64
	Name               string
	Status             campaign.Status // default active
	BillingModel       string          // default cpm
	BidCPMCents        int             // default 1000 (10 CNY/CPM)
	BidCPCCents        int
	OCPMTargetCPACents int
	BudgetDailyCents   int64    // default 100_000 (1000 CNY/day)
	BudgetTotalCents   int64    // default BudgetDailyCents*30 (long enough for a single-test run)
	TargetingGeo       []string // default ["CN"]
	TargetingOS        []string // default ["iOS"]
	FreqCap            int
	FreqPeriodHours    int
}

// SeedCampaign inserts a qa-prefixed campaign and pre-seeds its Redis daily
// budget key using the same Asia/Shanghai-based date format production uses
// (see internal/budget/budget.go:127).
func (h *TestHarness) SeedCampaign(spec CampaignSpec) int64 {
	h.TestT.Helper()
	if spec.Status == "" {
		spec.Status = campaign.StatusActive
	}
	if spec.BillingModel == "" {
		spec.BillingModel = campaign.BillingCPM
	}
	if spec.BidCPMCents == 0 && spec.BillingModel == campaign.BillingCPM {
		spec.BidCPMCents = 1000
	}
	if spec.BudgetDailyCents == 0 {
		spec.BudgetDailyCents = 100_000
	}
	if spec.BudgetTotalCents == 0 {
		spec.BudgetTotalCents = spec.BudgetDailyCents * 30
	}
	if spec.TargetingGeo == nil {
		spec.TargetingGeo = []string{"CN"}
	}
	if spec.TargetingOS == nil {
		spec.TargetingOS = []string{"iOS"}
	}
	if spec.Name == "" {
		spec.Name = fmt.Sprintf("qa-camp-%d", time.Now().UnixNano())
	} else if !strings.HasPrefix(spec.Name, "qa-") {
		spec.Name = "qa-" + spec.Name
	}

	targeting := map[string]any{
		"geo": spec.TargetingGeo,
		"os":  spec.TargetingOS,
	}
	if spec.FreqCap > 0 {
		targeting["frequency_cap"] = map[string]any{
			"count":        spec.FreqCap,
			"period_hours": spec.FreqPeriodHours,
		}
	}
	tjson, err := json.Marshal(targeting)
	if err != nil {
		h.TestT.Fatalf("SeedCampaign: marshal targeting: %v", err)
	}

	var id int64
	err = h.PG.QueryRow(h.Ctx, `
		INSERT INTO campaigns
		  (advertiser_id, name, status, billing_model,
		   budget_total_cents, budget_daily_cents,
		   bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		   targeting, sandbox)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false)
		RETURNING id
	`, spec.AdvertiserID, spec.Name, string(spec.Status), spec.BillingModel,
		spec.BudgetTotalCents, spec.BudgetDailyCents,
		spec.BidCPMCents, spec.BidCPCCents, spec.OCPMTargetCPACents,
		tjson).Scan(&id)
	if err != nil {
		h.TestT.Fatalf("SeedCampaign: %v", err)
	}

	// Pre-seed Redis daily budget so bidder's PipelineCheck has something to deduct from.
	// Uses the same Asia/Shanghai date format as internal/budget/budget.go:127.
	date := time.Now().In(config.CSTLocation).Format("2006-01-02")
	key := fmt.Sprintf("budget:daily:%d:%s", id, date)
	if err := h.RDB.Set(h.Ctx, key, spec.BudgetDailyCents, 25*time.Hour).Err(); err != nil {
		h.TestT.Fatalf("SeedCampaign: init budget: %v", err)
	}
	return id
}

// SeedCreative adds a qa creative to a campaign.
func (h *TestHarness) SeedCreative(campaignID int64, adMarkup, destURL string) int64 {
	h.TestT.Helper()
	if adMarkup == "" {
		adMarkup = `<a href="https://qa.example.invalid/click">qa-creative</a>`
	}
	if destURL == "" {
		destURL = "https://qa.example.invalid/landing"
	}
	var id int64
	err := h.PG.QueryRow(h.Ctx, `
		INSERT INTO creatives
		  (campaign_id, name, format, size, ad_type, ad_markup, destination_url, status)
		VALUES ($1, 'qa-creative', 'banner', '320x50', 'banner', $2, $3, 'approved')
		RETURNING id
	`, campaignID, adMarkup, destURL).Scan(&id)
	if err != nil {
		h.TestT.Fatalf("SeedCreative: %v", err)
	}
	return id
}

// UpdateCampaignStatus flips a campaign to a new status via a bare UPDATE.
// It deliberately does NOT publish to the "campaign:updates" Redis channel —
// this lets tests exercise the loader's 30s periodic-refresh fallback path.
// If you need instant propagation, call h.PublishCampaignUpdate explicitly
// (added in T06) after this method.
func (h *TestHarness) UpdateCampaignStatus(id int64, status campaign.Status) {
	h.TestT.Helper()
	_, err := h.PG.Exec(h.Ctx, `UPDATE campaigns SET status=$1 WHERE id=$2`, string(status), id)
	if err != nil {
		h.TestT.Fatalf("UpdateCampaignStatus: %v", err)
	}
}
