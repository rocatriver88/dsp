package main

import (
	"net/http"
	"os"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

// Deps is the full set of collaborators every bidder handler needs.
// Production main() constructs one; integration tests construct one with
// test clients, attach it to an httptest.NewServer, and exercise the real
// handlers without starting the compose container.
//
// Extracted from main.go (V5 debt 2026-04-14-D1) so the bidder HTTP
// surface is reachable from `go test` without importing `package main`.
type Deps struct {
	Engine           *bidder.Engine
	BudgetSvc        *budget.Service
	StrategySvc      *bidder.BidStrategy
	Loader           *bidder.CampaignLoader
	Producer         *events.Producer
	RDB              *redis.Client
	ExchangeRegistry *exchange.Registry
	Guard            *guardrail.Guardrail
	HMACSecret       string
	PublicURL        string
}

// RegisterRoutes wires the bidder's HTTP handlers against a Deps bundle.
// Both production main() and integration tests call this.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("POST /bid", d.handleBid)
	mux.HandleFunc("POST /bid/{exchange_id}", d.handleExchangeBid)
	mux.HandleFunc("POST /win", d.handleWin)
	mux.HandleFunc("GET /win", d.handleWin)
	mux.HandleFunc("GET /click", d.handleClick)
	mux.HandleFunc("GET /convert", d.handleConvert)
	// V5.2C: /stats removed from public mux — it leaked all active
	// campaigns' bid/budget data to any client reaching the bidder's
	// public URL. Now registered at /internal/stats on the internal
	// port behind X-Admin-Token (see RegisterInternalRoutes).
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.Handle("GET /metrics", promhttp.Handler())
}

// RegisterInternalRoutes builds the internal mux for the bidder's admin-only
// endpoints. The mux is served on a separate port (BIDDER_INTERNAL_PORT)
// that is NOT exposed to external traffic.
//
// V5.2C: /stats moved here from the public mux. It exposes all active
// campaigns' id/name/bid_cpm_cents/budget_daily/budget_remaining/creatives_count
// — competitive intelligence that must not be publicly accessible.
func RegisterInternalRoutes(mux *http.ServeMux, d *Deps) {
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("GET /internal/stats", d.handleStats)

	mux.Handle("/internal/", bidderAdminAuth(adminMux))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","port":"bidder-internal"}`))
	})
	mux.Handle("GET /metrics", promhttp.Handler())
}

// bidderAdminAuth is the bidder-local equivalent of handler.AdminAuthMiddleware.
// It reads ADMIN_TOKEN from the environment and checks the X-Admin-Token
// header on every non-OPTIONS request. The bidder binary does not import
// internal/handler (it has its own Deps type), so we duplicate the small
// middleware here rather than creating a circular dependency.
//
// Behavior is identical to internal/handler/admin_auth.go:
//   - OPTIONS → pass through (CORS preflight)
//   - ADMIN_TOKEN unset → 401 fail-closed
//   - X-Admin-Token missing or wrong → 401
func bidderAdminAuth(next http.Handler) http.Handler {
	token := os.Getenv("ADMIN_TOKEN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			next.ServeHTTP(w, r)
			return
		}
		if token == "" {
			http.Error(w, `{"error":"admin authentication not configured"}`, http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Admin-Token") != token {
			http.Error(w, `{"error":"admin authentication required"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
