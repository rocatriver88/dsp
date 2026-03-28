package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/ratelimit"
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

	rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
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
	rs, chErr := reporting.NewStore(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword)
	if chErr != nil {
		log.Printf("Warning: ClickHouse not available (%v), reports disabled", chErr)
	} else {
		reportStore = rs
		log.Println("Connected to ClickHouse")
	}

	// API Key lookup function for auth middleware
	apiKeyLookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}

	// Rate limiter (fail-open if Redis unavailable)
	limiter := ratelimit.New(rdb)

	// --- Public API routes ---
	publicMux := http.NewServeMux()

	// Advertiser endpoints
	publicMux.HandleFunc("POST /api/v1/advertisers", handleCreateAdvertiser)
	publicMux.HandleFunc("GET /api/v1/advertisers/{id}", handleGetAdvertiser)

	// Campaign endpoints
	publicMux.HandleFunc("POST /api/v1/campaigns", handleCreateCampaign)
	publicMux.HandleFunc("GET /api/v1/campaigns", handleListCampaigns)
	publicMux.HandleFunc("GET /api/v1/campaigns/{id}", handleGetCampaign)
	publicMux.HandleFunc("PUT /api/v1/campaigns/{id}", handleUpdateCampaign)
	publicMux.HandleFunc("POST /api/v1/campaigns/{id}/start", handleStartCampaign)
	publicMux.HandleFunc("POST /api/v1/campaigns/{id}/pause", handlePauseCampaign)

	// Creative endpoints
	publicMux.HandleFunc("POST /api/v1/creatives", handleCreateCreative)
	publicMux.HandleFunc("GET /api/v1/ad-types", handleAdTypes)
	publicMux.HandleFunc("GET /api/v1/billing-models", handleBillingModels)

	// Report endpoints (Phase 2)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/stats", handleCampaignStats)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/hourly", handleHourlyStats)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/geo", handleGeoBreakdown)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/bids", handleBidTransparency)
	publicMux.HandleFunc("GET /api/v1/reports/overview", handleOverviewStats)

	// Billing endpoints (Phase 4)
	publicMux.HandleFunc("POST /api/v1/billing/topup", handleTopUp)
	publicMux.HandleFunc("GET /api/v1/billing/transactions", handleTransactions)
	publicMux.HandleFunc("GET /api/v1/billing/balance/{id}", handleBalance)

	// Registration (public: self-register only)
	publicMux.HandleFunc("POST /api/v1/register", handleRegister)

	// Health
	publicMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	// --- Internal routes (separate port, not exposed externally) ---
	internalMux := http.NewServeMux()
	internalMux.HandleFunc("GET /internal/active-campaigns", handleActiveCampaigns)
	internalMux.HandleFunc("GET /api/v1/admin/registrations", handleListRegistrations)
	internalMux.HandleFunc("POST /api/v1/admin/registrations/{id}/approve", handleApproveRegistration)
	internalMux.HandleFunc("POST /api/v1/admin/registrations/{id}/reject", handleRejectRegistration)
	internalMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","port":"internal","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	// Start both servers
	publicAddr := ":" + cfg.APIPort
	internalAddr := ":" + cfg.InternalPort

	// Middleware chain: CORS → Rate Limit → API Key Auth → Logging → Routes
	// Public endpoints that skip auth: /health, POST /api/v1/register
	authedHandler := auth.APIKeyMiddleware(apiKeyLookup)(publicMux)
	rateLimited := ratelimit.Middleware(limiter, ratelimit.APIKeyFunc, 100, time.Minute)(authedHandler)
	// Wrap with auth-exempt routes
	publicHandler := withAuthExemption(rateLimited, publicMux)
	publicSrv := &http.Server{Addr: publicAddr, Handler: withCORS(cfg, withLogging(publicHandler))}
	internalSrv := &http.Server{Addr: internalAddr, Handler: withLogging(internalMux)}

	go func() {
		log.Printf("DSP API Server (public) listening on %s", publicAddr)
		if err := publicSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("public server: %v", err)
		}
	}()
	go func() {
		log.Printf("DSP API Server (internal) listening on %s", internalAddr)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("internal server: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	log.Println("Shutting down API server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	publicSrv.Shutdown(shutdownCtx)
	internalSrv.Shutdown(shutdownCtx)
	log.Println("API server stopped")
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
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Use authenticated advertiser_id, ignore request body advertiser_id
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID != 0 {
		req.AdvertiserID = advID
	}
	if req.Name == "" || req.AdvertiserID == 0 {
		writeError(w, http.StatusBadRequest, "name and advertiser_id required")
		return
	}
	if req.BillingModel == "" {
		req.BillingModel = "cpm"
	}
	if _, ok := campaign.BillingModelConfig[req.BillingModel]; !ok {
		writeError(w, http.StatusBadRequest, "invalid billing_model: must be cpm, cpc, or ocpm")
		return
	}
	if req.BudgetTotalCents <= 0 || req.BudgetDailyCents <= 0 {
		writeError(w, http.StatusBadRequest, "budget must be positive")
		return
	}
	switch req.BillingModel {
	case "cpm":
		if req.BidCPMCents <= 0 {
			writeError(w, http.StatusBadRequest, "bid_cpm_cents required for CPM billing")
			return
		}
	case "cpc":
		if req.BidCPCCents <= 0 {
			writeError(w, http.StatusBadRequest, "bid_cpc_cents required for CPC billing")
			return
		}
	case "ocpm":
		if req.OCPMTargetCPACents <= 0 {
			writeError(w, http.StatusBadRequest, "ocpm_target_cpa_cents required for oCPM billing")
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
	}

	id, err := store.CreateCampaign(r.Context(), c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": "draft"})
}

func handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	advID := auth.AdvertiserIDFromContext(r.Context())
	if advID == 0 {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	campaigns, err := store.ListCampaigns(r.Context(), advID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Enrich with ClickHouse spend data
	if reportStore != nil {
		today := time.Now().UTC().Truncate(24 * time.Hour)
		tomorrow := today.Add(24 * time.Hour)
		for _, c := range campaigns {
			stats, err := reportStore.GetCampaignStats(r.Context(), uint64(c.ID), today, tomorrow)
			if err == nil && stats != nil {
				c.SpentCents = int64(stats.SpendCents)
			}
		}
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
	advID := auth.AdvertiserIDFromContext(r.Context())
	c, err := store.GetCampaignForAdvertiser(r.Context(), id, advID)
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
	advID := auth.AdvertiserIDFromContext(r.Context())
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
	if err := store.UpdateCampaign(r.Context(), id, advID, req.Name, req.BidCPMCents, req.BudgetDailyCents, req.Targeting); err != nil {
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
	advID := auth.AdvertiserIDFromContext(r.Context())
	if err := store.TransitionStatus(r.Context(), id, advID, campaign.StatusActive); err != nil {
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
	advID := auth.AdvertiserIDFromContext(r.Context())
	if err := store.TransitionStatus(r.Context(), id, advID, campaign.StatusPaused); err != nil {
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

func handleBillingModels(w http.ResponseWriter, r *http.Request) {
	models := make([]map[string]any, 0)
	for key, cfg := range campaign.BillingModelConfig {
		models = append(models, map[string]any{
			"model":       key,
			"label":       cfg.Label,
			"charge_on":   cfg.ChargeOn,
			"description": cfg.Description,
		})
	}
	writeJSON(w, http.StatusOK, models)
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

func handleOverviewStats(w http.ResponseWriter, r *http.Request) {
	if reportStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"today_spend_cents": 0})
		return
	}
	advID := auth.AdvertiserIDFromContext(r.Context())

	// Get all campaigns for this advertiser to sum their spend
	campaigns, err := store.ListCampaigns(r.Context(), advID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"today_spend_cents": 0})
		return
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)
	var totalSpend uint64
	var totalImpressions, totalClicks uint64
	for _, c := range campaigns {
		stats, err := reportStore.GetCampaignStats(r.Context(), uint64(c.ID), today, tomorrow)
		if err != nil || stats == nil {
			continue
		}
		totalSpend += stats.SpendCents
		totalImpressions += stats.Impressions
		totalClicks += stats.Clicks
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"today_spend_cents":  totalSpend,
		"today_impressions":  totalImpressions,
		"today_clicks":       totalClicks,
	})
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

// withAuthExemption routes unauthenticated paths directly to the mux, bypassing auth middleware.
func withAuthExemption(authed http.Handler, publicMux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for health check and self-registration
		if r.URL.Path == "/health" || (r.Method == "POST" && r.URL.Path == "/api/v1/register") {
			publicMux.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
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

func withCORS(cfg *config.Config, next http.Handler) http.Handler {
	allowed := make(map[string]bool)
	for _, origin := range strings.Split(cfg.CORSAllowedOrigins, ",") {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			allowed[origin] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if allowed[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
