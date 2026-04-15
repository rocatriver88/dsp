package qaharness

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/heartgryphon/dsp/internal/config"
)

// budgetDailyKey formats the same key production uses (internal/budget/budget.go:126).
func budgetDailyKey(campaignID int64) string {
	date := time.Now().In(config.CSTLocation).Format("2006-01-02")
	return fmt.Sprintf("budget:daily:%d:%s", campaignID, date)
}

// GetBudgetRemaining reads budget:daily:{campaign_id}:{today-CST}. Returns 0
// if the key is absent (matches redis.Nil semantics).
func (h *TestHarness) GetBudgetRemaining(campaignID int64) int64 {
	h.TestT.Helper()
	v, err := h.RDB.Get(h.Ctx, budgetDailyKey(campaignID)).Int64()
	if err != nil {
		return 0
	}
	return v
}

// SetBudgetRemaining forces budget:daily to a specific value with 25h TTL
// (same TTL production uses at internal/budget/budget.go:51).
func (h *TestHarness) SetBudgetRemaining(campaignID int64, cents int64) {
	h.TestT.Helper()
	if err := h.RDB.Set(h.Ctx, budgetDailyKey(campaignID), cents, 25*time.Hour).Err(); err != nil {
		h.TestT.Fatalf("SetBudgetRemaining: %v", err)
	}
}

// GetFreqCount returns the value of freq:{campaign}:{user}, or 0 if unset.
func (h *TestHarness) GetFreqCount(campaignID int64, userID string) int64 {
	h.TestT.Helper()
	key := fmt.Sprintf("freq:%d:%s", campaignID, userID)
	v, err := h.RDB.Get(h.Ctx, key).Int64()
	if err != nil {
		return 0
	}
	return v
}

// PublishCampaignUpdate publishes to campaign:updates with the given action
// (see internal/bidder/loader.go:240-262 for valid actions).
func (h *TestHarness) PublishCampaignUpdate(campaignID int64, action string) {
	h.TestT.Helper()
	payload, _ := json.Marshal(map[string]any{
		"campaign_id": campaignID,
		"action":      action,
	})
	if err := h.RDB.Publish(h.Ctx, "campaign:updates", payload).Err(); err != nil {
		h.TestT.Fatalf("PublishCampaignUpdate: %v", err)
	}
}

// PublishRaw publishes an arbitrary raw string to a channel. Used by the
// loader boundary test that verifies malformed payloads don't panic.
func (h *TestHarness) PublishRaw(channel, raw string) {
	h.TestT.Helper()
	if err := h.RDB.Publish(h.Ctx, channel, raw).Err(); err != nil {
		h.TestT.Fatalf("PublishRaw: %v", err)
	}
}
