//go:build integration

package qaharness_test

import (
	"testing"

	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

func TestHarnessSmoke(t *testing.T) {
	h := qaharness.New(t)

	advID := h.SeedAdvertiser("smoke")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-smoke-campaign",
		BidCPMCents:  500,
	})
	h.SeedCreative(campID, "", "")

	if got := h.GetBudgetRemaining(campID); got != 100_000 {
		t.Errorf("expected 100000 (default budget), got %d", got)
	}

	h.UpdateCampaignStatus(campID, campaign.StatusPaused)
	var status string
	if err := h.PG.QueryRow(h.Ctx, `SELECT status FROM campaigns WHERE id=$1`, campID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "paused" {
		t.Errorf("expected paused, got %s", status)
	}
}
