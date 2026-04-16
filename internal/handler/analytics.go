package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

// HandleAnalyticsStream godoc
// @Summary Real-time analytics SSE stream
// @Description Authenticates via a short-lived HMAC token passed in the
// @Description `?token=` query parameter (NOT via X-API-Key header, and
// @Description NOT via an `?api_key=` query fallback — that was the
// @Description V5.1 P1-1 vulnerability). Clients mint the token via
// @Description POST /api/v1/analytics/token.
// @Tags analytics
// @Security SSETokenAuth
// @Produce text/event-stream
// @Success 200 {string} string "SSE stream"
// @Failure 401 {object} object{error=string}
// @Router /analytics/stream [get]
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

	// Override the server-level WriteTimeout for SSE: the global 60s timeout
	// (set in cmd/api/main.go for slowloris protection) would force-close the
	// stream after one minute. ResponseController.SetWriteDeadline(zero)
	// disables the per-connection write deadline so the stream stays open
	// until the client disconnects or the server shuts down. Go 1.20+.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
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

// HandleAnalyticsSnapshot godoc
// @Summary Get analytics snapshot
// @Description Authenticates via the same `?token=` query parameter as
// @Description /analytics/stream. See that endpoint for the full
// @Description rationale. V5.1 P1-1.
// @Tags analytics
// @Security SSETokenAuth
// @Produce json
// @Success 200 {object} object
// @Failure 401 {object} object{error=string}
// @Router /analytics/snapshot [get]
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

// AnalyticsTokenResponse is the JSON body returned by HandleAnalyticsToken.
// ExpiresAt is emitted as RFC3339 to match the rest of the handler package
// (admin.go, registration/invite.go) and to give frontend codegen a named
// type to import instead of parsing a bare `object{...}` swag spec.
type AnalyticsTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// HandleAnalyticsToken godoc
// @Summary Issue a short-lived SSE auth token for analytics endpoints
// @Description Returns a 5-minute HMAC-signed token bound to the authenticated advertiser.
// @Description Clients use this token in the ?token= query of /analytics/stream and /analytics/snapshot
// @Description to authenticate EventSource connections without exposing the long-lived X-API-Key
// @Description in URL query logs (V5.1 P1-1).
// @Tags analytics
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} handler.AnalyticsTokenResponse
// @Failure 401 {object} object{error=string}
// @Failure 500 {object} object{error=string}
// @Router /analytics/token [post]
func (d *Deps) HandleAnalyticsToken(w http.ResponseWriter, r *http.Request) {
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	// Defense-in-depth: Config.Validate() enforces len(APIHMACSecret) >= 32 at
	// startup in cmd/api/main.go, so in production this branch is unreachable.
	// It exists for test/library consumers that construct a Deps directly and
	// for tenants running the handler package outside the api binary —
	// TestHandleAnalyticsToken_MissingSecret_Returns500 exercises this path.
	if len(d.SSETokenSecret) == 0 {
		WriteError(w, http.StatusInternalServerError, "SSE token signing not configured")
		return
	}
	const ttl = 5 * time.Minute
	now := time.Now()
	token := auth.IssueSSEToken(d.SSETokenSecret, advID, ttl, now)
	WriteJSON(w, http.StatusOK, AnalyticsTokenResponse{
		Token:     token,
		ExpiresAt: now.Add(ttl),
	})
}
