package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/campaign"
)

func (d *Deps) HandleCreateAdvertiser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompanyName  string `json:"company_name"`
		ContactEmail string `json:"contact_email"`
		BalanceCents int64  `json:"balance_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" || req.ContactEmail == "" {
		WriteError(w, http.StatusBadRequest, "company_name and contact_email required")
		return
	}

	apiKey := GenerateAPIKey()
	adv := &campaign.Advertiser{
		CompanyName:  req.CompanyName,
		ContactEmail: req.ContactEmail,
		APIKey:       apiKey,
		BalanceCents: req.BalanceCents,
		BillingType:  "prepaid",
	}

	id, err := d.Store.CreateAdvertiser(r.Context(), adv)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"api_key": apiKey,
		"message": "advertiser created",
	})
}

func (d *Deps) HandleGetAdvertiser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	adv, err := d.Store.GetAdvertiser(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusNotFound, "advertiser not found")
		return
	}
	WriteJSON(w, http.StatusOK, adv)
}

func (d *Deps) HandleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdvertiserID       int64           `json:"advertiser_id"`
		Name               string          `json:"name"`
		BillingModel       string          `json:"billing_model"`
		BudgetTotalCents   int64           `json:"budget_total_cents"`
		BudgetDailyCents   int64           `json:"budget_daily_cents"`
		BidCPMCents        int             `json:"bid_cpm_cents"`
		BidCPCCents        int             `json:"bid_cpc_cents"`
		OCPMTargetCPACents int             `json:"ocpm_target_cpa_cents"`
		StartDate          *time.Time      `json:"start_date"`
		EndDate            *time.Time      `json:"end_date"`
		Targeting          json.RawMessage `json:"targeting"`
		Sandbox            bool            `json:"sandbox"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID != 0 {
		req.AdvertiserID = advID
	}
	if req.Name == "" || req.AdvertiserID == 0 {
		WriteError(w, http.StatusBadRequest, "name and advertiser_id required")
		return
	}
	if req.BillingModel == "" {
		req.BillingModel = "cpm"
	}
	if _, ok := campaign.BillingModelConfig[req.BillingModel]; !ok {
		WriteError(w, http.StatusBadRequest, "invalid billing_model: must be cpm, cpc, or ocpm")
		return
	}
	if req.BudgetTotalCents <= 0 || req.BudgetDailyCents <= 0 {
		WriteError(w, http.StatusBadRequest, "budget must be positive")
		return
	}
	switch req.BillingModel {
	case "cpm":
		if req.BidCPMCents <= 0 {
			WriteError(w, http.StatusBadRequest, "bid_cpm_cents required for CPM billing")
			return
		}
	case "cpc":
		if req.BidCPCCents <= 0 {
			WriteError(w, http.StatusBadRequest, "bid_cpc_cents required for CPC billing")
			return
		}
	case "ocpm":
		if req.OCPMTargetCPACents <= 0 {
			WriteError(w, http.StatusBadRequest, "ocpm_target_cpa_cents required for oCPM billing")
			return
		}
	}

	c := &campaign.Campaign{
		AdvertiserID:       req.AdvertiserID,
		Name:               req.Name,
		BillingModel:       req.BillingModel,
		BudgetTotalCents:   req.BudgetTotalCents,
		BudgetDailyCents:   req.BudgetDailyCents,
		BidCPMCents:        req.BidCPMCents,
		BidCPCCents:        req.BidCPCCents,
		OCPMTargetCPACents: req.OCPMTargetCPACents,
		StartDate:          req.StartDate,
		EndDate:            req.EndDate,
		Targeting:          req.Targeting,
		Sandbox:            req.Sandbox,
	}

	id, err := d.Store.CreateCampaign(r.Context(), c)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "draft"})
}

func (d *Deps) HandleListCampaigns(w http.ResponseWriter, r *http.Request) {
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaigns, err := d.Store.ListCampaigns(r.Context(), advID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if d.ReportStore != nil {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		tomorrow := today.Add(24 * time.Hour)
		for _, c := range campaigns {
			stats, err := d.ReportStore.GetCampaignStats(r.Context(), uint64(c.ID), today, tomorrow)
			if err == nil && stats != nil {
				c.SpentCents = int64(stats.SpendCents)
			}
		}
	}
	if campaigns == nil {
		campaigns = []*campaign.Campaign{}
	}
	WriteJSON(w, http.StatusOK, campaigns)
}

func (d *Deps) HandleGetCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())
	c, err := d.Store.GetCampaignForAdvertiser(r.Context(), id, advID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}
	WriteJSON(w, http.StatusOK, c)
}

func (d *Deps) HandleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())
	var req struct {
		Name             string          `json:"name"`
		BidCPMCents      int             `json:"bid_cpm_cents"`
		BudgetDailyCents int64           `json:"budget_daily_cents"`
		Targeting        json.RawMessage `json:"targeting"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := d.Store.UpdateCampaign(r.Context(), id, advID, req.Name, req.BidCPMCents, req.BudgetDailyCents, req.Targeting); err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (d *Deps) HandleStartCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())

	c, err := d.Store.GetCampaignForAdvertiser(r.Context(), id, advID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}
	creatives, _ := d.Store.GetCreativesByCampaign(r.Context(), id)
	if len(creatives) == 0 {
		WriteError(w, http.StatusUnprocessableEntity, "campaign has no creatives, add at least one before starting")
		return
	}
	if c.EndDate != nil && c.EndDate.Before(time.Now()) {
		WriteError(w, http.StatusUnprocessableEntity, "campaign end_date is in the past")
		return
	}
	if c.BudgetTotalCents < c.BudgetDailyCents {
		WriteError(w, http.StatusUnprocessableEntity, "budget_total must be >= budget_daily")
		return
	}
	// Check advertiser balance before starting (skip for sandbox campaigns)
	if !c.Sandbox && d.BillingSvc != nil {
		balance, _, err := d.BillingSvc.GetBalance(r.Context(), advID)
		if err == nil && balance < c.BudgetDailyCents {
			WriteError(w, http.StatusUnprocessableEntity, "insufficient balance: please top up before starting campaign")
			return
		}
	}

	if err := d.Store.TransitionStatus(r.Context(), id, advID, campaign.StatusActive); err != nil {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}

	if d.BudgetSvc != nil {
		d.BudgetSvc.InitDailyBudget(r.Context(), id, c.BudgetDailyCents)
	}

	if d.Redis != nil {
		bidder.NotifyCampaignUpdate(r.Context(), d.Redis, id, "activated")
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func (d *Deps) HandlePauseCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())
	if err := d.Store.TransitionStatus(r.Context(), id, advID, campaign.StatusPaused); err != nil {
		WriteError(w, http.StatusConflict, err.Error())
		return
	}
	if d.Redis != nil {
		bidder.NotifyCampaignUpdate(r.Context(), d.Redis, id, "paused")
	}
	WriteJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func (d *Deps) HandleListCreatives(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	creatives, err := d.Store.GetAllCreativesByCampaign(r.Context(), id)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if creatives == nil {
		creatives = []*campaign.Creative{}
	}
	WriteJSON(w, http.StatusOK, creatives)
}

func (d *Deps) HandleCreateCreative(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CampaignID     int64  `json:"campaign_id"`
		Name           string `json:"name"`
		AdType         string `json:"ad_type"`
		Format         string `json:"format"`
		Size           string `json:"size"`
		AdMarkup       string `json:"ad_markup"`
		DestinationURL string `json:"destination_url"`
		NativeTitle    string `json:"native_title"`
		NativeDesc     string `json:"native_desc"`
		NativeIconURL  string `json:"native_icon_url"`
		NativeImageURL string `json:"native_image_url"`
		NativeCTA      string `json:"native_cta"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AdType == "" {
		req.AdType = "banner"
	}
	if _, ok := campaign.AdTypeConfig[req.AdType]; !ok {
		WriteError(w, http.StatusBadRequest, "invalid ad_type: must be splash, interstitial, native, or banner")
		return
	}
	cr := &campaign.Creative{
		CampaignID:     req.CampaignID,
		Name:           req.Name,
		AdType:         req.AdType,
		Format:         req.Format,
		Size:           req.Size,
		AdMarkup:       req.AdMarkup,
		DestinationURL: req.DestinationURL,
		NativeTitle:    req.NativeTitle,
		NativeDesc:     req.NativeDesc,
		NativeIconURL:  req.NativeIconURL,
		NativeImageURL: req.NativeImageURL,
		NativeCTA:      req.NativeCTA,
	}
	id, err := d.Store.CreateCreative(r.Context(), cr)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Auto-approve creatives in development for faster iteration.
	// In production, creatives stay "pending" until admin reviews.
	status := "pending"
	if os.Getenv("ENV") != "production" {
		_ = d.Store.UpdateCreativeStatus(r.Context(), id, "approved")
		status = "approved"
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
}

func (d *Deps) HandleAdTypes(w http.ResponseWriter, r *http.Request) {
	types := make([]map[string]any, 0)
	for key, cfg := range campaign.AdTypeConfig {
		types = append(types, map[string]any{
			"type":        key,
			"label":       cfg.Label,
			"sizes":       cfg.Sizes,
			"full_screen": cfg.FullScreen,
			"has_native":  cfg.HasNative,
		})
	}
	WriteJSON(w, http.StatusOK, types)
}

func (d *Deps) HandleBillingModels(w http.ResponseWriter, r *http.Request) {
	models := make([]map[string]any, 0)
	for key, cfg := range campaign.BillingModelConfig {
		models = append(models, map[string]any{
			"model":       key,
			"label":       cfg.Label,
			"charge_on":   cfg.ChargeOn,
			"description": cfg.Description,
		})
	}
	WriteJSON(w, http.StatusOK, models)
}
