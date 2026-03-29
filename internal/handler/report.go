package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/reporting"
)

func (d *Deps) HandleCampaignStats(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
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

func (d *Deps) HandleHourlyStats(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
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

func (d *Deps) HandleGeoBreakdown(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
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

func (d *Deps) HandleBidTransparency(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
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

	WriteJSON(w, http.StatusOK, map[string]any{
		"today_spend_cents": totalSpend,
		"today_impressions": totalImpressions,
		"today_clicks":      totalClicks,
	})
}
