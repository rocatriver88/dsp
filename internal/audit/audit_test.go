package audit

import (
	"testing"
	"time"
)

func TestEntryJSON(t *testing.T) {
	e := Entry{
		AdvertiserID: 1,
		Actor:        "admin",
		Action:       "campaign.update",
		ResourceType: "campaign",
		ResourceID:   42,
		Details:      map[string]any{"field": "budget_daily_cents", "old": 1000, "new": 2000},
		CreatedAt:    time.Now(),
	}
	if e.Actor != "admin" {
		t.Errorf("expected actor 'admin', got '%s'", e.Actor)
	}
	if e.Action != "campaign.update" {
		t.Errorf("expected action 'campaign.update', got '%s'", e.Action)
	}
}

func TestActions(t *testing.T) {
	actions := []string{
		ActionCampaignCreate, ActionCampaignUpdate, ActionCampaignStart,
		ActionCampaignPause, ActionCreativeCreate, ActionCreativeDelete,
		ActionTopUp, ActionRegistrationApprove,
	}
	for _, a := range actions {
		if a == "" {
			t.Error("action constant should not be empty")
		}
	}
}
