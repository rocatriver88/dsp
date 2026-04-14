package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
	"github.com/heartgryphon/dsp/internal/audit"
	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/autopause"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/handler"
	"github.com/heartgryphon/dsp/internal/reconciliation"
	"github.com/heartgryphon/dsp/internal/observability"
	"github.com/heartgryphon/dsp/internal/ratelimit"
	"github.com/heartgryphon/dsp/internal/registration"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// @title DSP Platform API
// @version 1.0
// @description Demand-Side Platform — programmatic advertising API
// @host localhost:8181
// @BasePath /api/v1
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
// @securityDefinitions.apikey AdminAuth
// @in header
// @name X-Admin-Token
func main() {
	observability.InitLogger()
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	// processCtx is cancelled when the process receives SIGINT or SIGTERM.
	// It's used for one-shot startup operations (they complete long before
	// any signal can arrive) and as the trigger that unblocks the shutdown
	// sequence at the bottom of main. It is deliberately NOT passed to
	// long-lived background loops — those use workerCtx below.
	processCtx, processStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer processStop()

	// workerCtx drives every long-lived background loop (autopause,
	// reconciliation, etc.). The shutdown sequence cancels it explicitly
	// *before* draining HTTP so workers stop generating new Redis/DB
	// writes while inflight requests are still finishing. Separating this
	// from the request lifecycle is the V5 §P1 lifecycle requirement:
	// cancelling one root context would kill inflight requests mid-flight
	// and cause spurious client retries.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Connect PostgreSQL
	db, err := pgxpool.New(processCtx, cfg.DSN())
	if err != nil {
		log.Fatalf("connect to postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(processCtx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Connect Redis (optional)
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	defer rdb.Close()
	if err := rdb.Ping(processCtx).Err(); err != nil {
		log.Printf("Warning: Redis not available (%v), pub/sub notifications disabled", err)
		rdb = nil
	} else {
		log.Println("Connected to Redis")
	}

	// Initialize guardrail (optional, requires Redis)
	var guard *guardrail.Guardrail
	if rdb != nil {
		guard = guardrail.New(rdb, guardrail.Config{
			GlobalDailyBudgetCents: cfg.GlobalDailyBudgetCents,
			MaxBidCPMCents:         cfg.MaxBidCPMCents,
		})
		log.Println("Guardrail initialized")
	}

	// Initialize services
	store := campaign.NewStore(db)
	billingSvc := billing.New(db)
	regSvc := registration.New(db)
	auditLogger := audit.NewLogger(db)
	var budgetSvc *budget.Service
	if rdb != nil {
		budgetSvc = budget.New(rdb)
	}
	log.Println("Billing + Registration services initialized")

	// Connect ClickHouse (optional)
	var reportStore *reporting.Store
	rs, chErr := reporting.NewStore(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword)
	if chErr != nil {
		log.Printf("Warning: ClickHouse not available (%v), reports disabled", chErr)
	} else {
		reportStore = rs
		log.Println("Connected to ClickHouse")
	}

	// Start auto-pause background service
	autoPauseSvc := autopause.New(store, reportStore, rdb)
	go autoPauseSvc.Start(workerCtx)

	// Start hourly reconciliation
	if reportStore != nil && rdb != nil {
		reconSvc := reconciliation.New(rdb, store, reportStore, billingSvc, alert.Noop{})
		reconSvc.StartHourlySchedule(workerCtx, 1.0) // 1% threshold
		log.Println("Hourly reconciliation started")
	}

	// Handler dependencies
	h := &handler.Deps{
		Store:       store,
		ReportStore: reportStore,
		BillingSvc:  billingSvc,
		RegSvc:      regSvc,
		BudgetSvc:   budgetSvc,
		Redis:       rdb,
		Guardrail:   guard,
		AuditLog:    auditLogger,
	}

	// Public API routes
	publicMux := http.NewServeMux()
	publicMux.HandleFunc("POST /api/v1/advertisers", h.HandleCreateAdvertiser)
	publicMux.HandleFunc("GET /api/v1/advertisers/{id}", h.HandleGetAdvertiser)
	publicMux.HandleFunc("POST /api/v1/campaigns", h.HandleCreateCampaign)
	publicMux.HandleFunc("GET /api/v1/campaigns", h.HandleListCampaigns)
	publicMux.HandleFunc("GET /api/v1/campaigns/{id}", h.HandleGetCampaign)
	publicMux.HandleFunc("PUT /api/v1/campaigns/{id}", h.HandleUpdateCampaign)
	publicMux.HandleFunc("POST /api/v1/campaigns/{id}/start", h.HandleStartCampaign)
	publicMux.HandleFunc("POST /api/v1/campaigns/{id}/pause", h.HandlePauseCampaign)
	publicMux.HandleFunc("GET /api/v1/campaigns/{id}/creatives", h.HandleListCreatives)
	publicMux.HandleFunc("POST /api/v1/creatives", h.HandleCreateCreative)
	publicMux.HandleFunc("PUT /api/v1/creatives/{id}", h.HandleUpdateCreative)
	publicMux.HandleFunc("DELETE /api/v1/creatives/{id}", h.HandleDeleteCreative)
	publicMux.HandleFunc("GET /api/v1/ad-types", h.HandleAdTypes)
	publicMux.HandleFunc("GET /api/v1/billing-models", h.HandleBillingModels)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/stats", h.HandleCampaignStats)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/hourly", h.HandleHourlyStats)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/geo", h.HandleGeoBreakdown)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/bids", h.HandleBidTransparency)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/attribution", h.HandleAttribution)
	publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/simulate", h.HandleBidSimulate)
	publicMux.HandleFunc("GET /api/v1/reports/overview", h.HandleOverviewStats)
	publicMux.HandleFunc("GET /api/v1/export/campaign/{id}/stats", h.HandleExportCampaignCSV)
	publicMux.HandleFunc("GET /api/v1/export/campaign/{id}/bids", h.HandleExportBidsCSV)
	publicMux.HandleFunc("GET /api/v1/audit-log", h.HandleMyAuditLog)
	publicMux.HandleFunc("GET /api/v1/analytics/stream", h.HandleAnalyticsStream)
	publicMux.HandleFunc("GET /api/v1/analytics/snapshot", h.HandleAnalyticsSnapshot)
	publicMux.HandleFunc("POST /api/v1/billing/topup", h.HandleTopUp)
	publicMux.HandleFunc("GET /api/v1/billing/transactions", h.HandleTransactions)
	publicMux.HandleFunc("GET /api/v1/billing/balance", h.HandleBalance)
	// Legacy path kept for backward compatibility. The handler resolves the
	// advertiser from auth context regardless of the path id, so a client
	// that still sends GET /billing/balance/{id} silently gets its own
	// balance — the path id is ignored. Safe for reads per the V5 three-
	// code rule (silent routing is only rejected on writes like topup).
	publicMux.HandleFunc("GET /api/v1/billing/balance/{id}", h.HandleBalance)
	publicMux.HandleFunc("POST /api/v1/upload", h.HandleUpload)
	publicMux.Handle("/uploads/", http.StripPrefix("/uploads/", handler.UploadFileServer()))
	publicMux.HandleFunc("POST /api/v1/register", h.HandleRegister)
	publicMux.HandleFunc("GET /api/v1/docs", h.HandleAPIDocs)
	publicMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	// Internal routes (separate port, admin auth required)
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /internal/active-campaigns", h.HandleActiveCampaigns)
	adminMux.HandleFunc("GET /api/v1/admin/registrations", h.HandleListRegistrations)
	adminMux.HandleFunc("POST /api/v1/admin/registrations/{id}/approve", h.HandleApproveRegistration)
	adminMux.HandleFunc("POST /api/v1/admin/registrations/{id}/reject", h.HandleRejectRegistration)
	adminMux.HandleFunc("GET /api/v1/admin/health", h.HandleSystemHealth)
	adminMux.HandleFunc("GET /api/v1/admin/creatives", h.HandleListCreativesForReview)
	adminMux.HandleFunc("POST /api/v1/admin/creatives/{id}/approve", h.HandleApproveCreative)
	adminMux.HandleFunc("POST /api/v1/admin/creatives/{id}/reject", h.HandleRejectCreative)
	adminMux.HandleFunc("POST /api/v1/admin/circuit-break", h.HandleCircuitBreak)
	adminMux.HandleFunc("POST /api/v1/admin/circuit-reset", h.HandleCircuitReset)
	adminMux.HandleFunc("GET /api/v1/admin/circuit-status", h.HandleCircuitStatus)
	adminMux.HandleFunc("GET /api/v1/admin/advertisers", h.HandleListAdvertisers)
	adminMux.HandleFunc("POST /api/v1/admin/topup", h.HandleAdminTopUp)
	adminMux.HandleFunc("POST /api/v1/admin/invite-codes", h.HandleCreateInviteCode)
	adminMux.HandleFunc("GET /api/v1/admin/invite-codes", h.HandleListInviteCodes)
	adminMux.HandleFunc("GET /api/v1/admin/audit-log", h.HandleAuditLog)

	internalMux := http.NewServeMux()
	internalMux.Handle("GET /metrics", promhttp.Handler())
	internalMux.Handle("/internal/", handler.AdminAuthMiddleware(adminMux))
	internalMux.Handle("/api/v1/admin/", handler.AdminAuthMiddleware(adminMux))
	internalMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","port":"internal","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})

	// Middleware chain
	apiKeyLookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}
	limiter := ratelimit.New(rdb)
	authedHandler := auth.APIKeyMiddleware(apiKeyLookup)(publicMux)
	rateLimited := ratelimit.Middleware(limiter, ratelimit.APIKeyFunc, 100, time.Minute)(authedHandler)
	publicHandler := handler.WithAuthExemption(rateLimited, publicMux)
	publicSrv := &http.Server{Addr: ":" + cfg.APIPort, Handler: handler.WithCORS(cfg, observability.RequestIDMiddleware(observability.LoggingMiddleware(publicHandler)))}
	internalSrv := &http.Server{Addr: ":" + cfg.InternalPort, Handler: handler.WithCORS(cfg, observability.LoggingMiddleware(internalMux))}

	// Start servers
	go func() {
		log.Printf("DSP API Server (public) listening on :%s", cfg.APIPort)
		if err := publicSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("public server: %v", err)
		}
	}()
	go func() {
		log.Printf("DSP API Server (internal) listening on :%s", cfg.InternalPort)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("internal server: %v", err)
		}
	}()

	// Graceful shutdown: five invariants from V5 §P1 lifecycle.
	//
	//   1. new requests stop entering
	//   2. workers exit
	//   3. inflight requests can drain
	//   4. producer flushes after last business write  (no producer in api)
	//   5. storage connections close last              (via deferred closers)
	//
	// Ordering rationale (codex V4):
	//   - Cancel workerCtx BEFORE HTTP drain so autopause / reconciliation
	//     stop producing new DB/Redis writes during drain. If we drained
	//     HTTP first, workers could still race inflight handlers.
	//   - http.Server.Shutdown atomically handles (1) stop Accept and
	//     (3) drain inflight; the call blocks until drain completes or
	//     shutdownCtx times out.
	//   - Storage close is via the top-of-main defers, which fire after
	//     main() returns — i.e. after every worker and every HTTP handler
	//     is done, satisfying invariant 5.
	<-processCtx.Done()
	log.Println("Shutting down API server...")

	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = publicSrv.Shutdown(shutdownCtx)
	_ = internalSrv.Shutdown(shutdownCtx)
	log.Println("API server stopped")
}
