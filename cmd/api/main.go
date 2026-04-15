package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
	"github.com/heartgryphon/dsp/internal/audit"
	"github.com/heartgryphon/dsp/internal/autopause"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/handler"
	"github.com/heartgryphon/dsp/internal/observability"
	"github.com/heartgryphon/dsp/internal/reconciliation"
	"github.com/heartgryphon/dsp/internal/registration"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
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

	publicSrv := &http.Server{
		Addr:    ":" + cfg.APIPort,
		Handler: handler.BuildPublicHandler(cfg, h),
	}
	internalSrv := &http.Server{
		Addr:    ":" + cfg.InternalPort,
		Handler: handler.BuildInternalHandler(cfg, h),
	}

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
