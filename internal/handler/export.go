package handler

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"

	_ "github.com/heartgryphon/dsp/internal/audit" // swaggo: audit.Entry
	"github.com/heartgryphon/dsp/internal/auth"
)

// HandleExportCampaignCSV godoc
// @Summary Export campaign stats as CSV
// @Tags export
// @Security ApiKeyAuth
// @Produce text/csv
// @Param id path int true "Campaign ID"
// @Param from query string false "Start date"
// @Param to query string false "End date"
// @Success 200 {file} file
// @Router /export/campaign/{id}/stats [get]
func (d *Deps) HandleExportCampaignCSV(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "reports not available")
		return
	}

	campaignID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID); err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}

	from, to := ParseDateRange(r)
	stats, err := d.ReportStore.GetCampaignStats(r.Context(), uint64(campaignID), from, to)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch stats")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="campaign_%d_%s_%s.csv"`,
			campaignID, from.Format("20060102"), to.Format("20060102")))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	writer.Write([]string{
		"Campaign ID", "Period Start", "Period End",
		"Impressions", "Clicks", "Conversions", "Wins", "Bids",
		"Spend (cents)", "ADX Cost (cents)", "CTR (%)", "Win Rate (%)", "CVR (%)", "CPA (cents)",
	})

	writer.Write([]string{
		fmt.Sprintf("%d", campaignID),
		from.Format("2006-01-02"),
		to.Format("2006-01-02"),
		fmt.Sprintf("%d", stats.Impressions),
		fmt.Sprintf("%d", stats.Clicks),
		fmt.Sprintf("%d", stats.Conversions),
		fmt.Sprintf("%d", stats.Wins),
		fmt.Sprintf("%d", stats.Bids),
		fmt.Sprintf("%d", stats.SpendCents),
		fmt.Sprintf("%d", stats.AdxCostCents),
		fmt.Sprintf("%.2f", stats.CTR),
		fmt.Sprintf("%.2f", stats.WinRate),
		fmt.Sprintf("%.2f", stats.CVR),
		fmt.Sprintf("%.2f", stats.CPA),
	})
}

// HandleExportBidsCSV godoc
// @Summary Export bid details as CSV
// @Tags export
// @Security ApiKeyAuth
// @Produce text/csv
// @Param id path int true "Campaign ID"
// @Success 200 {file} file
// @Router /export/campaign/{id}/bids [get]
func (d *Deps) HandleExportBidsCSV(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "reports not available")
		return
	}

	campaignID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	if _, err := d.Store.GetCampaignForAdvertiser(r.Context(), campaignID, advID); err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}

	from, to := ParseDateRange(r)
	exportLimit := 10000
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &exportLimit)
	}
	if exportLimit > 50000 {
		exportLimit = 50000
	}

	bids, err := d.ReportStore.GetBidTransparency(r.Context(), uint64(campaignID), from, to, exportLimit, 0)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to fetch bids")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="bids_%d_%s_%s.csv"`,
			campaignID, from.Format("20060102"), to.Format("20060102")))

	writer := csv.NewWriter(w)
	defer writer.Flush()

	writer.Write([]string{
		"Time", "Event Type", "Geo", "Device OS",
		"Bid Price (cents)", "Clear Price (cents)",
	})

	for _, b := range bids {
		writer.Write([]string{
			b.Time.Format("2006-01-02 15:04:05"),
			b.EventType,
			b.GeoCountry,
			b.DeviceOS,
			fmt.Sprintf("%d", b.BidPriceCents),
			fmt.Sprintf("%d", b.ClearPriceCents),
		})
	}
}

// HandleMyAuditLog godoc
// @Summary Get my audit log
// @Tags audit
// @Security ApiKeyAuth
// @Produce json
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {array} audit.Entry
// @Router /audit-log [get]
func (d *Deps) HandleMyAuditLog(w http.ResponseWriter, r *http.Request) {
	if d.AuditLog == nil {
		WriteError(w, http.StatusServiceUnavailable, "audit log not available")
		return
	}

	advID := auth.AdvertiserIDFromContext(r.Context())
	limit := 50
	offset := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}

	entries, err := d.AuditLog.Query(r.Context(), advID, limit, offset)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, entries)
}
