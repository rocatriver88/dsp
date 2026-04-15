package main

import (
	"net/http"

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
	mux.HandleFunc("GET /stats", d.handleStats)
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.Handle("GET /metrics", promhttp.Handler())
}
