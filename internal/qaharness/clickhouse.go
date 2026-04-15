package qaharness

import (
	"time"
)

// WaitForBidLogRows polls bid_log until it sees `want` rows matching the given
// (campaignID, eventType) pair, or fails the test on timeout.
func (h *TestHarness) WaitForBidLogRows(campaignID int64, eventType string, want int, timeout time.Duration) {
	h.TestT.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var n uint64
		row := h.CH.QueryRow(h.Ctx, `
			SELECT count() FROM bid_log
			WHERE campaign_id = ? AND event_type = ?
		`, uint64(campaignID), eventType)
		if err := row.Scan(&n); err != nil {
			h.TestT.Fatalf("WaitForBidLogRows: query: %v", err)
		}
		if int(n) >= want {
			return
		}
		if time.Now().After(deadline) {
			h.TestT.Fatalf("WaitForBidLogRows: campaign=%d type=%s want=%d got=%d (timeout)",
				campaignID, eventType, want, n)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// QueryCampaignSpend returns sum(charge_cents) for a campaign. If eventType is
// "", aggregates across all event_types; otherwise restricts to that type.
func (h *TestHarness) QueryCampaignSpend(campaignID int64, eventType string) uint64 {
	h.TestT.Helper()
	var q string
	var args []any
	if eventType == "" {
		q = `SELECT sum(charge_cents) FROM bid_log WHERE campaign_id = ?`
		args = []any{uint64(campaignID)}
	} else {
		q = `SELECT sum(charge_cents) FROM bid_log WHERE campaign_id = ? AND event_type = ?`
		args = []any{uint64(campaignID), eventType}
	}
	var sum uint64
	if err := h.CH.QueryRow(h.Ctx, q, args...).Scan(&sum); err != nil {
		h.TestT.Fatalf("QueryCampaignSpend: %v", err)
	}
	return sum
}

// InsertBidLogRow inserts a synthetic row directly, bypassing the bidder/consumer
// pipeline. Used by tests (reconciliation, attribution, stats) that need to seed
// ClickHouse with a specific event without spinning up the whole write path.
func (h *TestHarness) InsertBidLogRow(
	campaignID, advertiserID, creativeID int64,
	eventType, requestID, deviceID string,
	bidPriceCents, clearPriceCents, chargeCents uint32,
	when time.Time,
) {
	h.TestT.Helper()
	err := h.CH.Exec(h.Ctx, `
		INSERT INTO bid_log (
			event_date, event_time, campaign_id, creative_id, advertiser_id,
			exchange_id, request_id, geo_country, device_os, device_id,
			bid_price_cents, clear_price_cents, charge_cents, event_type, loss_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, when, when, uint64(campaignID), uint64(creativeID), uint64(advertiserID),
		"qa-exchange", requestID, "CN", "iOS", deviceID,
		bidPriceCents, clearPriceCents, chargeCents, eventType, "")
	if err != nil {
		h.TestT.Fatalf("InsertBidLogRow: %v", err)
	}
}
