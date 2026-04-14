package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// parseCampaignID is the shared path-id parse used by the report handlers.
// On parse failure we return 404 (not 400) to stay consistent with the
// tenant-hiding rule: a client poking at an invalid id learns nothing.
func parseCampaignID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return 0, false
	}
	return id, true
}

// HandleCampaignStats godoc
// @Summary Get campaign stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param from query string false "Start date (YYYY-MM-DD)"
// @Param to query string false "End date (YYYY-MM-DD)"
// @Success 200 {object} reporting.CampaignStats
// @Failure 401 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /reports/campaign/{id}/stats [get]
func (d *Deps) HandleCampaignStats(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCampaignID(w, r)
	if !ok {
		return
	}
	if !d.ensureCampaignOwner(w, r, id) {
		return
	}
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	from, to := ParseDateRange(r)
	stats, err := d.ReportStore.GetCampaignStats(r.Context(), uint64(id), from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, stats)
}

// HandleHourlyStats godoc
// @Summary Get hourly stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param date query string false "Date (YYYY-MM-DD)"
// @Success 200 {array} reporting.HourlyStats
// @Failure 401 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /reports/campaign/{id}/hourly [get]
func (d *Deps) HandleHourlyStats(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCampaignID(w, r)
	if !ok {
		return
	}
	if !d.ensureCampaignOwner(w, r, id) {
		return
	}
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	dateStr := r.URL.Query().Get("date")
	date := time.Now().UTC()
	if dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			date = d
		}
	}
	stats, err := d.ReportStore.GetHourlyStats(r.Context(), uint64(id), date)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		stats = []reporting.HourlyStats{}
	}
	WriteJSON(w, http.StatusOK, stats)
}

// HandleGeoBreakdown godoc
// @Summary Get geo breakdown
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {array} reporting.GeoStats
// @Failure 401 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /reports/campaign/{id}/geo [get]
func (d *Deps) HandleGeoBreakdown(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCampaignID(w, r)
	if !ok {
		return
	}
	if !d.ensureCampaignOwner(w, r, id) {
		return
	}
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	from, to := ParseDateRange(r)
	stats, err := d.ReportStore.GetGeoBreakdown(r.Context(), uint64(id), from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		stats = []reporting.GeoStats{}
	}
	WriteJSON(w, http.StatusOK, stats)
}

// HandleBidTransparency godoc
// @Summary Get bid-level details
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param limit query int false "Limit" default(100)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} reporting.BidDetail
// @Failure 401 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /reports/campaign/{id}/bids [get]
func (d *Deps) HandleBidTransparency(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCampaignID(w, r)
	if !ok {
		return
	}
	if !d.ensureCampaignOwner(w, r, id) {
		return
	}
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	from, to := ParseDateRange(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	bids, err := d.ReportStore.GetBidTransparency(r.Context(), uint64(id), from, to, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bids == nil {
		bids = []reporting.BidDetail{}
	}
	WriteJSON(w, http.StatusOK, bids)
}

// HandleAttribution godoc
// @Summary Get attribution report
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Success 200 {object} reporting.AttributionReport
// @Failure 401 {object} object{error=string}
// @Failure 404 {object} object{error=string}
// @Router /reports/campaign/{id}/attribution [get]
func (d *Deps) HandleAttribution(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCampaignID(w, r)
	if !ok {
		return
	}
	if !d.ensureCampaignOwner(w, r, id) {
		return
	}
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	from, to := ParseDateRange(r)
	model := r.URL.Query().Get("model")
	if model == "" {
		model = "last_click"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	report, err := d.ReportStore.GetAttributionReport(r.Context(), uint64(id), from, to, model, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, report)
}

// HandleBidSimulate godoc
// @Summary Simulate bid outcome
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param bid_cpm_cents query int true "Simulated CPM bid in cents"
// @Success 200 {object} reporting.BidSimulation
// @Router /reports/campaign/{id}/simulate [get]
func (d *Deps) HandleBidSimulate(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}

	bidStr := r.URL.Query().Get("bid_cpm_cents")
	bidCPM, err := strconv.Atoi(bidStr)
	if err != nil || bidCPM <= 0 {
		WriteError(w, http.StatusBadRequest, "bid_cpm_cents must be a positive integer")
		return
	}

	// Get campaign to fill current bid and verify ownership
	advID := auth.AdvertiserIDFromContext(r.Context())
	camp, err := d.Store.GetCampaignForAdvertiser(r.Context(), id, advID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "not found")
		return
	}

	sim, err := d.ReportStore.SimulateBid(r.Context(), uint64(id), bidCPM)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sim.CurrentBidCPMCents = camp.BidCPMCents

	WriteJSON(w, http.StatusOK, sim)
}

// HandleOverviewStats godoc
// @Summary Get today overview stats
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Success 200 {object} object{today_spend_cents=integer,today_impressions=integer,today_clicks=integer,ctr=number,balance_cents=integer}
// @Router /reports/overview [get]
func (d *Deps) HandleOverviewStats(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteJSON(w, http.StatusOK, map[string]any{"today_spend_cents": 0})
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())

	campaigns, err := d.Store.ListCampaigns(r.Context(), advID)
	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]any{"today_spend_cents": 0})
		return
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)
	var totalSpend uint64
	var totalImpressions, totalClicks uint64
	for _, c := range campaigns {
		stats, err := d.ReportStore.GetCampaignStats(r.Context(), uint64(c.ID), today, tomorrow)
		if err != nil || stats == nil {
			continue
		}
		totalSpend += stats.SpendCents
		totalImpressions += stats.Impressions
		totalClicks += stats.Clicks
	}

	var ctr float64
	if totalImpressions > 0 {
		ctr = float64(totalClicks) / float64(totalImpressions) * 100
	}

	// Get advertiser balance
	var balanceCents int64
	if d.BillingSvc != nil {
		bal, _, err := d.BillingSvc.GetBalance(r.Context(), advID)
		if err == nil {
			balanceCents = bal
		}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"today_spend_cents": totalSpend,
		"today_impressions": totalImpressions,
		"today_clicks":      totalClicks,
		"ctr":               ctr,
		"balance_cents":     balanceCents,
	})
}
