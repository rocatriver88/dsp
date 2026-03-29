package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

// HandleAnalyticsStream provides real-time bidding analytics via Server-Sent Events (SSE).
// The client connects once and receives updates every 5 seconds with fresh ClickHouse data.
func (d *Deps) HandleAnalyticsStream(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ctx := r.Context()

	// Send initial data immediately
	d.sendAnalyticsEvent(ctx, w, flusher, advID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.sendAnalyticsEvent(ctx, w, flusher, advID)
		}
	}
}

// HandleAnalyticsSnapshot returns a single snapshot of real-time analytics (non-streaming).
func (d *Deps) HandleAnalyticsSnapshot(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	data := d.buildAnalyticsData(r.Context(), advID)
	WriteJSON(w, http.StatusOK, data)
}

type campaignLive struct {
	CampaignID  int64   `json:"campaign_id"`
	Name        string  `json:"name"`
	Impressions uint64  `json:"impressions"`
	Clicks      uint64  `json:"clicks"`
	Wins        uint64  `json:"wins"`
	Bids        uint64  `json:"bids"`
	WinRate     float64 `json:"win_rate"`
	CTR         float64 `json:"ctr"`
	SpendCents  uint64  `json:"spend_cents"`
	ProfitCents int64   `json:"profit_cents"`
}

func (d *Deps) buildAnalyticsData(ctx context.Context, advID int64) map[string]any {
	campaigns, err := d.Store.ListCampaigns(ctx, advID)
	if err != nil {
		return map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339), "campaigns": []any{}}
	}

	now := time.Now().UTC()
	today := now.Truncate(24 * time.Hour)

	var live []campaignLive
	for _, c := range campaigns {
		if c.Status != "active" {
			continue
		}
		stats, err := d.ReportStore.GetCampaignStats(ctx, uint64(c.ID), today, now)
		if err != nil || stats == nil {
			continue
		}
		live = append(live, campaignLive{
			CampaignID:  c.ID,
			Name:        c.Name,
			Impressions: stats.Impressions,
			Clicks:      stats.Clicks,
			Wins:        stats.Wins,
			Bids:        stats.Bids,
			WinRate:     stats.WinRate,
			CTR:         stats.CTR,
			SpendCents:  stats.SpendCents,
			ProfitCents: stats.ProfitCents,
		})
	}

	if live == nil {
		live = []campaignLive{}
	}

	return map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"campaigns": live,
	}
}

func (d *Deps) sendAnalyticsEvent(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, advID int64) {
	data, _ := json.Marshal(d.buildAnalyticsData(ctx, advID))
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
