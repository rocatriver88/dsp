package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/registration"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

var (
	store       *campaign.Store
	rdb         *redis.Client
	reportStore *reporting.Store
	billingSvc  *billing.Service
	regSvc      *registration.Service
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	db, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()

	if err := db.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Redis not available (%v), pub/sub notifications disabled", err)
		rdb = nil
	} else {
		log.Println("Connected to Redis")
	}

	store = campaign.NewStore(db)
	billingSvc = billing.New(db)
	regSvc = registration.New(db)
	log.Println("Billing + Registration services initialized")

	// Connect ClickHouse (optional for Phase 2 reports)
	rs, chErr := reporting.NewStore(cfg.ClickHouseAddr)
	if chErr != nil {
		log.Printf("Warning: ClickHouse not available (%v), reports disabled", chErr)
	} else {
		reportStore = rs
		log.Println("Connected to ClickHouse")
	}

	mux := http.NewServeMux()

	// Advertiser endpoints
	mux.HandleFunc("POST /api/v1/advertisers", handleCreateAdvertiser)
	mux.HandleFunc("GET /api/v1/advertisers/{id}", handleGetAdvertiser)

	// Campaign endpoints
	mux.HandleFunc("POST /api/v1/campaigns", handleCreateCampaign)
	mux.HandleFunc("GET /api/v1/campaigns", handleListCampaigns)
	mux.HandleFunc("GET /api/v1/campaigns/{id}", handleGetCampaign)
	mux.HandleFunc("PUT /api/v1/campaigns/{id}", handleUpdateCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/start", handleStartCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/pause", handlePauseCampaign)

	// Creative endpoints
	mux.HandleFunc("POST /api/v1/creatives", handleCreateCreative)
	mux.HandleFunc("GET /api/v1/ad-types", handleAdTypes)

	// Report endpoints (Phase 2)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/stats", handleCampaignStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/hourly", handleHourlyStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/geo", handleGeoBreakdown)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/bids", handleBidTransparency)

	// Billing endpoints (Phase 4)
	mux.HandleFunc("POST /api/v1/billing/topup", handleTopUp)
	mux.HandleFunc("GET /api/v1/billing/transactions", handleTransactions)
	mux.HandleFunc("GET /api/v1/billing/balance/{id}", handleBalance)

	// Registration endpoints (Phase 4)
	mux.HandleFunc("POST /api/v1/register", handleRegister)
	mux.HandleFunc("GET /api/v1/admin/registrations", handleListRegistrations)
	mux.HandleFunc("POST /api/v1/admin/registrations/{id}/approve", handleApproveRegistration)
	mux.HandleFunc("POST /api/v1/admin/registrations/{id}/reject", handleRejectRegistration)

	// Internal: active campaigns for bidder
	mux.HandleFunc("GET /internal/active-campaigns", handleActiveCampaigns)

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	addr := ":" + cfg.APIPort
	log.Printf("DSP API Server listening on %s", addr)
	if err := http.ListenAndServe(addr, withCORS(withLogging(mux))); err != nil {
		log.Fatal(err)
	}
}

func handleCreateAdvertiser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		CompanyName  string `json:"company_name"`
		ContactEmail string `json:"contact_email"`
		BalanceCents int64  `json:"balance_cents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CompanyName == "" || req.ContactEmail == "" {
		writeError(w, http.StatusBadRequest, "company_name and contact_email required")
		return
	}

	apiKey := generateAPIKey()
	adv := &campaign.Advertiser{
		CompanyName:  req.CompanyName,
		ContactEmail: req.ContactEmail,
		APIKey:       apiKey,
		BalanceCents: req.BalanceCents,
		BillingType:  "prepaid",
	}

	id, err := store.CreateAdvertiser(r.Context(), adv)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"api_key": apiKey,
		"message": "advertiser created",
	})
}

func handleGetAdvertiser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	adv, err := store.GetAdvertiser(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "advertiser not found")
		return
	}
	writeJSON(w, http.StatusOK, adv)
}

func handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdvertiserID     int64           `json:"advertiser_id"`
		Name             string          `json:"name"`
		BudgetTotalCents int64           `json:"budget_total_cents"`
		BudgetDailyCents int64           `json:"budget_daily_cents"`
		BidCPMCents      int             `json:"bid_cpm_cents"`
		StartDate        *time.Time      `json:"start_date"`
		EndDate          *time.Time      `json:"end_date"`
		Targeting        json.RawMessage `json:"targeting"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.AdvertiserID == 0 {
		writeError(w, http.StatusBadRequest, "name and advertiser_id required")
		return
	}
	if req.BudgetTotalCents <= 0 || req.BudgetDailyCents <= 0 || req.BidCPMCents <= 0 {
		writeError(w, http.StatusBadRequest, "budget and bid must be positive")
		return
	}

	c := &campaign.Campaign{
		AdvertiserID:     req.AdvertiserID,
		Name:             req.Name,
		BudgetTotalCents: req.BudgetTotalCents,
		BudgetDailyCents: req.BudgetDailyCents,
		BidCPMCents:      req.BidCPMCents,
		StartDate:        req.StartDate,
		EndDate:          req.EndDate,
		Targeting:        req.Targeting,
	}

	id, err := store.CreateCampaign(r.Context(), c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "draft"})
}

func handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	advIDStr := r.URL.Query().Get("advertiser_id")
	if advIDStr == "" {
		writeError(w, http.StatusBadRequest, "advertiser_id query param required")
		return
	}
	advID, err := strconv.ParseInt(advIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid advertiser_id")
		return
	}

	campaigns, err := store.ListCampaigns(r.Context(), advID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if campaigns == nil {
		campaigns = []*campaign.Campaign{}
	}
	writeJSON(w, http.StatusOK, campaigns)
}

func handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	c, err := store.GetCampaign(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func handleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Name             string          `json:"name"`
		BidCPMCents      int             `json:"bid_cpm_cents"`
		BudgetDailyCents int64           `json:"budget_daily_cents"`
		Targeting        json.RawMessage `json:"targeting"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := store.UpdateCampaign(r.Context(), id, req.Name, req.BidCPMCents, req.BudgetDailyCents, req.Targeting); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func handleStartCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := store.TransitionStatus(r.Context(), id, campaign.StatusActive); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if rdb != nil {
		bidder.NotifyCampaignUpdate(r.Context(), rdb, id, "activated")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "active"})
}

func handlePauseCampaign(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := store.TransitionStatus(r.Context(), id, campaign.StatusPaused); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if rdb != nil {
		bidder.NotifyCampaignUpdate(r.Context(), rdb, id, "paused")
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "paused"})
}

func handleCreateCreative(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AdType == "" {
		req.AdType = "banner"
	}
	if _, ok := campaign.AdTypeConfig[req.AdType]; !ok {
		writeError(w, http.StatusBadRequest, "invalid ad_type: must be splash, interstitial, native, or banner")
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
	id, err := store.CreateCreative(r.Context(), cr)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "approved"})
}

func handleActiveCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := store.ListActiveCampaigns(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if campaigns == nil {
		campaigns = []*campaign.Campaign{}
	}
	writeJSON(w, http.StatusOK, campaigns)
}

func handleAdTypes(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, types)
}

// --- Billing handlers (Phase 4) ---

func handleTopUp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AdvertiserID int64  `json:"advertiser_id"`
		AmountCents  int64  `json:"amount_cents"`
		Description  string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.AmountCents <= 0 {
		writeError(w, http.StatusBadRequest, "amount must be positive")
		return
	}
	txn, err := billingSvc.TopUp(r.Context(), req.AdvertiserID, req.AmountCents, req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, txn)
}

func handleTransactions(w http.ResponseWriter, r *http.Request) {
	advID, _ := strconv.ParseInt(r.URL.Query().Get("advertiser_id"), 10, 64)
	if advID == 0 {
		writeError(w, http.StatusBadRequest, "advertiser_id required")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	txns, err := billingSvc.GetTransactions(r.Context(), advID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if txns == nil {
		txns = []billing.Transaction{}
	}
	writeJSON(w, http.StatusOK, txns)
}

func handleBalance(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	balance, billingType, err := billingSvc.GetBalance(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "advertiser not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"advertiser_id": id,
		"balance_cents": balance,
		"billing_type":  billingType,
	})
}

// --- Registration handlers (Phase 4) ---

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registration.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request")
		return
	}
	if req.CompanyName == "" || req.ContactEmail == "" {
		writeError(w, http.StatusBadRequest, "company_name and contact_email required")
		return
	}
	id, err := regSvc.Submit(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"status":  "pending",
		"message": "Registration submitted. We will review within 7 business days.",
	})
}

func handleListRegistrations(w http.ResponseWriter, r *http.Request) {
	reqs, err := regSvc.ListPending(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reqs == nil {
		reqs = []registration.Request{}
	}
	writeJSON(w, http.StatusOK, reqs)
}

func handleApproveRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	advID, apiKey, err := regSvc.Approve(r.Context(), id, "admin")
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"advertiser_id": advID,
		"api_key":       apiKey,
		"message":       "Registration approved. Advertiser account created.",
	})
}

func handleRejectRegistration(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if err := regSvc.Reject(r.Context(), id, "admin", req.Reason); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "rejected"})
}

// --- Report handlers (Phase 2) ---

func handleCampaignStats(w http.ResponseWriter, r *http.Request) {
	if reportStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from, to := parseDateRange(r)
	stats, err := reportStore.GetCampaignStats(r.Context(), uint64(id), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func handleHourlyStats(w http.ResponseWriter, r *http.Request) {
	if reportStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	dateStr := r.URL.Query().Get("date")
	date := time.Now().UTC()
	if dateStr != "" {
		if d, err := time.Parse("2006-01-02", dateStr); err == nil {
			date = d
		}
	}
	stats, err := reportStore.GetHourlyStats(r.Context(), uint64(id), date)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		stats = []reporting.HourlyStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func handleGeoBreakdown(w http.ResponseWriter, r *http.Request) {
	if reportStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from, to := parseDateRange(r)
	stats, err := reportStore.GetGeoBreakdown(r.Context(), uint64(id), from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if stats == nil {
		stats = []reporting.GeoStats{}
	}
	writeJSON(w, http.StatusOK, stats)
}

func handleBidTransparency(w http.ResponseWriter, r *http.Request) {
	if reportStore == nil {
		writeError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	from, to := parseDateRange(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	bids, err := reportStore.GetBidTransparency(r.Context(), uint64(id), from, to, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if bids == nil {
		bids = []reporting.BidDetail{}
	}
	writeJSON(w, http.StatusOK, bids)
}

func parseDateRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7) // default last 7 days
	to := now

	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed
		}
	}
	return from, to
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func generateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "dsp_" + hex.EncodeToString(b)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/health" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
