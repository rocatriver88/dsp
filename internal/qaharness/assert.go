package qaharness

import (
	"time"
)

// AssertKafkaEqCH checks that the count of Kafka messages matches CH row count
// for a (campaign, eventType) tuple. Uses CountMessages + WaitForBidLogRows.
func (h *TestHarness) AssertKafkaEqCH(topic, reqPrefix string, campaignID int64, eventType string, want int) {
	h.TestT.Helper()
	kafkaN := h.CountMessages(topic, reqPrefix, 3*time.Second)
	if kafkaN != want {
		h.TestT.Fatalf("Kafka count mismatch: topic=%s prefix=%s want=%d got=%d",
			topic, reqPrefix, want, kafkaN)
	}
	h.WaitForBidLogRows(campaignID, eventType, want, 5*time.Second)
}

// AssertBudgetDelta checks that (before - after) equals an expected delta in
// cents, with ±1 cent tolerance for float rounding.
func (h *TestHarness) AssertBudgetDelta(campaignID int64, before int64, wantDeltaCents int64) {
	h.TestT.Helper()
	after := h.GetBudgetRemaining(campaignID)
	got := before - after
	diff := got - wantDeltaCents
	if diff < -1 || diff > 1 {
		h.TestT.Fatalf("Budget delta mismatch: before=%d after=%d want=%d got=%d",
			before, after, wantDeltaCents, got)
	}
}

// AssertSpendConsistency checks Redis budget delta == CH sum(charge_cents) for
// win events on a campaign.
func (h *TestHarness) AssertSpendConsistency(campaignID int64, before int64) {
	h.TestT.Helper()
	after := h.GetBudgetRemaining(campaignID)
	redisDelta := uint64(before - after)
	chSpend := h.QueryCampaignSpend(campaignID, "win")
	if redisDelta != chSpend {
		h.TestT.Fatalf("Spend inconsistency: redisDelta=%d CH(win)=%d", redisDelta, chSpend)
	}
}
