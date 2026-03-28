package campaign

import (
	"testing"
)

func TestValidTransitions(t *testing.T) {
	valid := []struct{ from, to Status }{
		{StatusDraft, StatusActive},
		{StatusDraft, StatusDeleted},
		{StatusActive, StatusPaused},
		{StatusActive, StatusCompleted},
		{StatusPaused, StatusActive},
		{StatusPaused, StatusCompleted},
	}
	for _, tt := range valid {
		if err := ValidateTransition(tt.from, tt.to); err != nil {
			t.Errorf("expected %s → %s to be valid, got error: %v", tt.from, tt.to, err)
		}
	}
}

func TestInvalidTransitions(t *testing.T) {
	invalid := []struct{ from, to Status }{
		{StatusDraft, StatusPaused},
		{StatusDraft, StatusCompleted},
		{StatusActive, StatusDraft},
		{StatusActive, StatusDeleted},
		{StatusPaused, StatusDraft},
		{StatusPaused, StatusDeleted},
		{StatusCompleted, StatusActive},
		{StatusCompleted, StatusDraft},
		{StatusDeleted, StatusActive},
	}
	for _, tt := range invalid {
		if err := ValidateTransition(tt.from, tt.to); err == nil {
			t.Errorf("expected %s → %s to be invalid, but got no error", tt.from, tt.to)
		}
	}
}

func TestEffectiveBidCPMCents_CPM(t *testing.T) {
	c := &Campaign{BillingModel: BillingCPM, BidCPMCents: 500}
	if got := c.EffectiveBidCPMCents(0, 0); got != 500 {
		t.Errorf("CPM: expected 500, got %d", got)
	}
}

func TestEffectiveBidCPMCents_CPC(t *testing.T) {
	c := &Campaign{BillingModel: BillingCPC, BidCPCCents: 100}
	// Default CTR 1% → 100 * 0.01 * 1000 = 1000
	if got := c.EffectiveBidCPMCents(0, 0); got != 1000 {
		t.Errorf("CPC default: expected 1000, got %d", got)
	}
	// Custom CTR 5% → 100 * 0.05 * 1000 = 5000
	if got := c.EffectiveBidCPMCents(0.05, 0); got != 5000 {
		t.Errorf("CPC 5%%: expected 5000, got %d", got)
	}
}

func TestEffectiveBidCPMCents_OCPM(t *testing.T) {
	c := &Campaign{BillingModel: BillingOCPM, OCPMTargetCPACents: 5000}
	// Default CTR=1%, CVR=5% → 5000 * 0.01 * 0.05 * 1000 = 2500
	if got := c.EffectiveBidCPMCents(0, 0); got != 2500 {
		t.Errorf("oCPM default: expected 2500, got %d", got)
	}
}

func TestChargeEvent(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{BillingCPM, "impression"},
		{BillingCPC, "click"},
		{BillingOCPM, "impression"},
		{"", "impression"}, // unknown defaults to impression
	}
	for _, tc := range cases {
		c := &Campaign{BillingModel: tc.model}
		if got := c.ChargeEvent(); got != tc.want {
			t.Errorf("ChargeEvent(%s): expected %s, got %s", tc.model, tc.want, got)
		}
	}
}
