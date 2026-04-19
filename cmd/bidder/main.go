package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/observability"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("config validation failed: %v", err)
	}

	// processCtx cancels on SIGINT/SIGTERM and is the single source of
	// truth for "should the process be winding down". Startup operations
	// use it too (they complete long before any signal), so a signal
	// arriving mid-startup still unwinds cleanly.
	processCtx, processStop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer processStop()

	// workerCtx drives long-lived background loops (stats cache refresh,
	// campaign loader periodic pull + pub/sub subscribe, daily budget
	// reset). The shutdown sequence cancels it explicitly before HTTP
	// drain so workers stop generating new writes while inflight bid /
	// win / click handlers are still finishing. Keeping worker lifetime
	// separate from request lifetime is the V5 §P1 lifecycle rule — one
	// shared root ctx would cancel inflight HTTP handlers mid-flight.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Connect PostgreSQL
	db, err := pgxpool.New(processCtx, cfg.DSN())
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(processCtx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Connect Redis
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	defer rdb.Close()
	if err := rdb.Ping(processCtx).Err(); err != nil {
		log.Fatalf("connect redis: %v (bidder requires Redis for budget/freq control)", err)
	}
	log.Println("Connected to Redis")

	// Initialize Kafka producer. Producer.Close is called explicitly in
	// the shutdown sequence below (after HTTP drain, before storage
	// close) so its ordering is load-bearing — relying on defer LIFO
	// would work today but is opaque to readers.
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer := events.NewProducer(brokers, "/tmp/dsp-kafka-buffer")
	log.Printf("Kafka producer initialized (brokers: %s)", cfg.KafkaBrokers)

	// Initialize services
	budgetSvc := budget.New(rdb)
	strategySvc := bidder.NewBidStrategy(rdb)
	fraudFilter := antifraud.NewFilter(rdb)
	loader := bidder.NewCampaignLoader(db, rdb, bidder.WithBudgetService(budgetSvc))

	// Connect ClickHouse for stats cache (optional — falls back to defaults)
	var reportStore *reporting.Store
	rs, chErr := reporting.NewStore(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword)
	if chErr != nil {
		log.Printf("Warning: ClickHouse not available (%v), using default CTR/CVR", chErr)
	} else {
		reportStore = rs
		log.Println("Connected to ClickHouse (stats cache)")
	}

	// Stats cache: 24h rolling CTR/CVR from ClickHouse → Redis, refreshed every 5min
	statsCache := bidder.NewStatsCache(rdb, reportStore, loader.GetActiveCampaigns)
	go statsCache.Start(workerCtx)

	guard := guardrail.New(rdb, guardrail.Config{
		GlobalDailyBudgetCents: cfg.GlobalDailyBudgetCents,
		MaxBidCPMCents:         cfg.MaxBidCPMCents,
		LowBalanceAlertCents:   cfg.LowBalanceAlertCents,
		MinBalanceCents:        cfg.MinBalanceCents,
		SpendRateWindowSec:     cfg.SpendRateWindowSec,
		SpendRateMultiplier:    cfg.SpendRateMultiplier,
		FailClosed:             cfg.IsProduction(),
	})
	log.Println("Guardrail initialized")

	engine := bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)
	log.Printf("Anti-fraud filter initialized (%v)", fraudFilter.Stats())
	log.Println("BidStrategy + StatsCache initialized (dynamic bidding active)")

	// Replay buffered Kafka events from prior outages
	if err := producer.ReplayBuffer(processCtx); err != nil {
		log.Printf("Kafka replay: %v (continuing)", err)
	}

	// Load campaigns from DB. loader.Start does a one-shot fullLoad and
	// then spawns its periodicRefresh + pub/sub subscriber goroutines
	// bound to workerCtx, so cancelling workerCtx cleanly unwinds them.
	// loader.Stop() is still called in the shutdown sequence as belt
	// and suspenders (it closes an internal stopCh honored in the same
	// for-select).
	if err := loader.Start(workerCtx); err != nil {
		log.Fatalf("campaign loader: %v", err)
	}
	observability.CampaignActiveTotal.Set(float64(len(loader.GetActiveCampaigns())))

	// Daily budget auto-reset at midnight CST (must start after loader).
	// Bound to workerCtx so SIGTERM stops it cleanly.
	go func() {
		loc := config.CSTLocation
		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 5, 0, loc)
			timer := time.NewTimer(next.Sub(now))
			select {
			case <-timer.C:
				campaigns := loader.GetActiveCampaigns()
				for _, c := range campaigns {
					if err := budgetSvc.InitDailyBudget(workerCtx, c.ID, c.BudgetDailyCents); err != nil {
						log.Printf("[BUDGET-RESET] campaign=%d error=%v", c.ID, err)
					}
				}
				log.Printf("[BUDGET-RESET] Reset %d campaigns at %s", len(campaigns), time.Now().In(loc).Format(time.RFC3339))
			case <-workerCtx.Done():
				timer.Stop()
				return
			}
		}
	}()

	// Exchange registry: register self-owned + any configured external exchanges
	exchangeRegistry := exchange.DefaultRegistry(cfg.BidderPublicURL)
	log.Printf("[EXCHANGE] %d exchanges registered", len(exchangeRegistry.ListEnabled()))

	deps := &Deps{
		Engine:           engine,
		BudgetSvc:        budgetSvc,
		StrategySvc:      strategySvc,
		Loader:           loader,
		Producer:         producer,
		RDB:              rdb,
		ExchangeRegistry: exchangeRegistry,
		Guard:            guard,
		HMACSecret:       cfg.BidderHMACSecret,
		PublicURL:        cfg.BidderPublicURL,
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)

	internalMux := http.NewServeMux()
	RegisterInternalRoutes(internalMux, deps)

	addr := ":" + cfg.BidderPort
	srv := &http.Server{
		Addr:              addr,
		Handler:           withLogging(mux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	internalAddr := ":" + cfg.BidderInternalPort
	internalSrv := &http.Server{
		Addr:              internalAddr,
		Handler:           withLogging(internalMux),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("DSP Bidder listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bidder server: %v", err)
		}
	}()
	go func() {
		log.Printf("DSP Bidder (internal) listening on %s", internalAddr)
		if err := internalSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bidder internal server: %v", err)
		}
	}()

	// Graceful shutdown: five invariants from V5 §P1 lifecycle.
	//
	//   1. new requests stop entering
	//   2. workers exit (stats cache, loader goroutines, daily budget reset)
	//   3. inflight bid / win / click handlers can drain
	//   4. Kafka producer flushes its buffer after the last business write
	//   5. storage connections (redis, db) close last
	//
	// Ordering rationale (codex V4):
	//   - Cancel workerCtx BEFORE HTTP drain so the periodic refresh and
	//     pub/sub listener stop writing to the loader cache during drain.
	//   - http.Server.Shutdown atomically handles (1) stop Accept and
	//     (3) drain inflight; it blocks until drain completes or the
	//     shutdown context times out.
	//   - producer.Close happens AFTER Shutdown so any inflight click /
	//     convert / win handler that enqueued an event finishes before
	//     the Kafka writer flushes. Going in the opposite order would
	//     drop the tail of the in-flight event stream.
	//   - rdb.Close / db.Close happen via top-of-main defers after
	//     main() returns, so every preceding step has already finished
	//     before storage goes away.
	<-processCtx.Done()
	log.Println("Shutting down bidder...")

	workerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	_ = internalSrv.Shutdown(shutdownCtx)

	// Belt and suspenders: these each close their own stopCh in addition
	// to honoring workerCtx.Done. Idempotent with the workerCancel above.
	loader.Stop()
	statsCache.Stop()

	// Invariant 4 (strict): wait for every producer.Go goroutine the
	// handlers spawned — click / convert / win notices — to finish
	// writing to Kafka (or to the disk buffer fallback) BEFORE we
	// close the Kafka writers. 5-second cap: any goroutine still
	// writing after that loses its message to the at-least-once
	// disk-buffer guarantee documented in docs/runtime.md.
	flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := producer.WaitInflight(flushCtx); err != nil {
		log.Printf("[SHUTDOWN] producer inflight flush timed out: %v (residual events fall back to disk buffer)", err)
	}
	flushCancel()

	producer.Close()
	log.Println("Bidder stopped")
}

func (d *Deps) handleBid(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		observability.BidLatency.WithLabelValues("direct").Observe(time.Since(start).Seconds())
	}()

	// Enforce 1MB body cap to match exchange path and protect against
	// OOM on public /bid endpoint. MaxBytesReader returns *http.MaxBytesError
	// when exceeded; the handler maps that to 413 and everything else to 400.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req openrtb2.BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		observability.BidRequestsTotal.WithLabelValues("direct", "rejected").Inc()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":"invalid bid request"}`, http.StatusBadRequest)
		return
	}

	resp, err := d.Engine.Bid(r.Context(), &req)
	latency := time.Since(start)

	if err != nil {
		observability.BidRequestsTotal.WithLabelValues("direct", "rejected").Inc()
		log.Printf("[ERROR] request_id=%s error=%v latency=%s", req.ID, err, latency)
		w.WriteHeader(http.StatusNoContent) // fail-closed: no bid on error
		return
	}

	if resp == nil {
		observability.BidRequestsTotal.WithLabelValues("direct", "passed").Inc()
		log.Printf("[NO-BID] request_id=%s latency=%s", req.ID, latency)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		baseURL := d.PublicURL
		hmacSecret := d.HMACSecret
		// Extract geo/os for tracking URLs
		var geo, os string
		if req.Device != nil {
			os = req.Device.OS
			if req.Device.Geo != nil {
				geo = req.Device.Geo.Country
			}
		}
		// Generate HMAC token for win/click URL authentication
		token := auth.GenerateToken(hmacSecret, bid.CID, req.ID)
		// Add win notice URL with HMAC token
		bid.NURL = fmt.Sprintf("%s/win?campaign_id=%s&price=${AUCTION_PRICE}&request_id=%s&geo=%s&os=%s&token=%s",
			baseURL, bid.CID, req.ID, geo, os, token)
		// Inject click tracking URL for CPC billing
		clickURL := fmt.Sprintf("%s/click?campaign_id=%s&request_id=%s&token=%s",
			baseURL, bid.CID, req.ID, token)
		bid.AdM = injectClickTracker(bid.AdM, clickURL)
		resp.SeatBid[0].Bid[0] = bid

		log.Printf("[BID] request_id=%s campaign=%s bid=%.6f latency=%s",
			req.ID, bid.CID, bid.Price, latency)
	}

	observability.BidRequestsTotal.WithLabelValues("direct", "won").Inc()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleExchangeBid handles bid requests from specific exchanges with protocol normalization.
// Each exchange may have slight deviations from standard OpenRTB that the adapter normalizes.
func (d *Deps) handleExchangeBid(w http.ResponseWriter, r *http.Request) {
	exchangeID := r.PathValue("exchange_id")
	adapter, ok := d.ExchangeRegistry.Get(exchangeID)
	if !ok {
		observability.BidRequestsTotal.WithLabelValues("unknown", "rejected").Inc()
		http.Error(w, fmt.Sprintf(`{"error":"unknown exchange: %s"}`, exchangeID), http.StatusBadRequest)
		return
	}

	start := time.Now()
	defer func() {
		observability.BidLatency.WithLabelValues(exchangeID).Observe(time.Since(start).Seconds())
	}()

	// Enforce 1MB body cap via MaxBytesReader (was io.LimitReader, which
	// silently truncated — caused partial bodies to parse-fail as 400
	// instead of the 413 clients expect).
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		observability.BidRequestsTotal.WithLabelValues(exchangeID, "rejected").Inc()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	req, err := adapter.ParseBidRequest(body)
	if err != nil {
		observability.BidRequestsTotal.WithLabelValues(exchangeID, "rejected").Inc()
		http.Error(w, fmt.Sprintf(`{"error":"parse failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	resp, err := d.Engine.Bid(r.Context(), req)
	latency := time.Since(start)

	if err != nil {
		observability.BidRequestsTotal.WithLabelValues(exchangeID, "rejected").Inc()
		log.Printf("[ERROR] exchange=%s request_id=%s error=%v latency=%s", exchangeID, req.ID, err, latency)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if resp == nil {
		observability.BidRequestsTotal.WithLabelValues(exchangeID, "passed").Inc()
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Add win notice URL (same as standard handler)
	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		baseURL := d.PublicURL
		hmacSecret := d.HMACSecret
		var geo, os string
		if req.Device != nil {
			os = req.Device.OS
			if req.Device.Geo != nil {
				geo = req.Device.Geo.Country
			}
		}
		token := auth.GenerateToken(hmacSecret, bid.CID, req.ID)
		bid.NURL = fmt.Sprintf("%s/win?campaign_id=%s&price=${AUCTION_PRICE}&request_id=%s&geo=%s&os=%s&token=%s",
			baseURL, bid.CID, req.ID, geo, os, token)
		resp.SeatBid[0].Bid[0] = bid

		log.Printf("[BID] exchange=%s request_id=%s campaign=%s bid=%.6f latency=%s",
			exchangeID, req.ID, bid.CID, bid.Price, latency)
	}

	// Format response via exchange-specific adapter
	out, err := adapter.FormatBidResponse(resp)
	if err != nil {
		observability.BidRequestsTotal.WithLabelValues(exchangeID, "rejected").Inc()
		log.Printf("[ERROR] exchange=%s format error: %v", exchangeID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	observability.BidRequestsTotal.WithLabelValues(exchangeID, "won").Inc()
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func (d *Deps) handleWin(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	priceStr := r.URL.Query().Get("price")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")

	// Validate HMAC token
	if !auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	// Win dedup: prevent double budget deduction from exchange retries
	dedupKey := fmt.Sprintf("win:dedup:%s", requestID)
	// TODO: migrate to SetArgs{Mode:"NX"}; semantic-equivalent replacement requires care (SetNX returns BoolCmd, SetArgs returns StatusCmd with different error handling)
	wasNew, dedupErr := d.RDB.SetNX(r.Context(), dedupKey, 1, 5*time.Minute).Result() //nolint:staticcheck // SA1019: SetNX deprecated, migration tracked separately
	if dedupErr != nil {
		log.Printf("[WIN-DEDUP] Redis error (proceeding): %v", dedupErr)
	} else if !wasNew {
		log.Printf("[WIN-DEDUP] duplicate win for request_id=%s", requestID)
		fmt.Fprintf(w, `{"status":"duplicate"}`)
		return
	}

	campaignID, err := strconv.ParseInt(campaignIDStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid campaign_id"}`, http.StatusBadRequest)
		return
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid price"}`, http.StatusBadRequest)
		return
	}

	// Check billing model: CPC campaigns are charged on click, not impression.
	// During loader warm-up, GetCampaign returns nil for campaigns that exist
	// but haven't loaded yet. Without this guard the nil check falls through
	// to the CPM path, silently billing the wrong model. Return 503 so the
	// exchange retries after the loader finishes its initial full-load.
	c := d.Loader.GetCampaign(campaignID)
	if c == nil && campaignID > 0 {
		log.Printf("[WIN-WARMUP] campaign_id=%d not loaded yet, returning 503", campaignID)
		http.Error(w, `{"error":"bidder warming up, retry"}`, http.StatusServiceUnavailable)
		return
	}
	isCPC := c != nil && c.BillingModel == "cpc"
	billingModel := "cpm"
	if c != nil {
		billingModel = c.BillingModel
	}

	var remaining int64
	if !isCPC {
		// CPM/oCPM: deduct advertiser charge from budget (not ADX cost)
		priceCents := advertiserChargeCents(price)
		var budgetErr error
		remaining, budgetErr = d.BudgetSvc.CheckAndDeductBudget(r.Context(), campaignID, priceCents)
		if budgetErr != nil {
			log.Printf("[WIN-ERROR] campaign_id=%d: %v", campaignID, budgetErr)
			http.Error(w, `{"error":"budget check failed"}`, http.StatusInternalServerError)
			return
		}
		if remaining < 0 {
			log.Printf("[WIN-REJECTED] campaign_id=%d (budget exhausted)", campaignID)
			http.Error(w, `{"error":"budget exhausted"}`, http.StatusConflict)
			return
		}

		observability.BudgetDeductedCentsTotal.WithLabelValues(billingModel).Add(float64(priceCents))

		// Track global spend for guardrail
		if d.Guard != nil {
			d.Guard.RecordGlobalSpend(r.Context(), priceCents)
		}
	}

	observability.WinsTotal.WithLabelValues(billingModel).Inc()

	log.Printf("[WIN] campaign_id=%d clear_price=%.6f billing=%s", campaignID, price, func() string {
		if isCPC {
			return "cpc(deferred to click)"
		}
		return fmt.Sprintf("cpm(remaining=%d)", remaining)
	}())

	// Record win + spend for bid strategy (pacing + win-rate tracking).
	// Use context.Background() — r.Context() is cancelled the moment the
	// handler returns its 200, which races the goroutine's Redis INCR and
	// can silently drop most RecordWin / RecordSpend calls. Same class of
	// bug as Round 2's internal/bidder/engine.go:210 fix for RecordBid;
	// Round 2 missed this handler-level instance. Carried from engine
	// branch's equivalent handleWin which had already fixed it.
	// M5 Round 3 regression caught by cmd/bidder/handlers_integration_test.go
	// TestHandlers_WinNormalCPM "C1 regression sentinel".
	if d.StrategySvc != nil {
		go d.StrategySvc.RecordWin(context.Background(), campaignID)
		if !isCPC {
			spendCents := advertiserChargeCents(price)
			go d.StrategySvc.RecordSpend(context.Background(), campaignID, spendCents)
		}
	}

	// Emit win event to Kafka
	if d.Producer != nil {
		var bidPrice float64
		var advertiserCharge float64
		if c != nil {
			bidPrice = float64(c.EffectiveBidCPMCents(0, 0)) * 0.90 / 100.0 / 1000.0
			if isCPC {
				advertiserCharge = 0 // CPC: charged on click, not impression
			} else {
				advertiserCharge = price / (1 - PlatformMargin)
			}
		}
		var creativeID, advertiserID int64
		if c != nil {
			advertiserID = c.AdvertiserID
			if len(c.Creatives) > 0 {
				creativeID = c.Creatives[0].ID
			}
		}
		evt := events.Event{
			CampaignID:       campaignID,
			CreativeID:       creativeID,
			AdvertiserID:     advertiserID,
			RequestID:        requestID,
			BidPrice:         bidPrice,
			ClearPrice:       price,
			AdvertiserCharge: advertiserCharge,
			GeoCountry:       r.URL.Query().Get("geo"),
			DeviceOS:         r.URL.Query().Get("os"),
		}
		// Background context — request context gets cancelled when handler
		// returns, which would abort the Kafka write in the goroutine.
		//
		// V5 §P1 Step B: we no longer emit a duplicate 'impression' event
		// here. bid_log therefore stops accumulating one spurious row per
		// won bid. Reporting aggregation is unaffected — Step A already
		// switched the query to countDistinctIf(request_id, ...).
		//
		// producer.Go (instead of a raw `go`) registers this send with
		// the producer's inflight WaitGroup so shutdown can drain it
		// before closing the Kafka writers. Round 1 review I4 Option A.
		prod := d.Producer
		prod.Go(func() { prod.SendWin(context.Background(), evt) })
	}

	fmt.Fprintf(w, `{"status":"ok","remaining_cents":%d}`, remaining)
}

// handleClick validates a click callback's HMAC token, deduplicates by
// request_id, and (for CPC campaigns) deducts the per-click budget.
//
// V5.1 P1-3: the legacy ?dest= redirect parameter was deleted. It was
// dead code in the legitimate flow — injectClickTracker at the bottom
// of this file only appends a 1x1 pixel and cmd/bidder/main.go:276
// constructs the clickURL without a `dest` parameter, so no real ad
// traffic ever carried one. It was also an open-redirect attack
// surface: because the HMAC token signed only (campaign_id, request_id)
// and NOT dest, anyone who observed a valid click URL (in ad exchange
// logs, browser history, or by replaying a token within its 5-minute
// TTL) could craft /click?campaign_id=X&request_id=Y&token=VALID&dest=
// https://phish.example and use the bidder's public domain to
// whitelist-launder phishing links past email / social-media link
// detectors. Delete the read, delete both redirect branches, leave
// the happy-path JSON response as the only outcome.
func (d *Deps) handleClick(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")

	// Validate HMAC token
	if !auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	// Click dedup: prevent double budget deduction and double event emission
	// from duplicate click callbacks (ad network retries, multi-click
	// fraud, accidental page reloads). Same 5-minute TTL as the win dedup.
	dedupKey := fmt.Sprintf("click:dedup:%s", requestID)
	// TODO: migrate to SetArgs{Mode:"NX"}; see sibling win-dedup comment
	wasNew, dedupErr := d.RDB.SetNX(r.Context(), dedupKey, 1, 5*time.Minute).Result() //nolint:staticcheck // SA1019: SetNX deprecated, migration tracked separately
	if dedupErr != nil {
		log.Printf("[CLICK-DEDUP] Redis error (proceeding): %v", dedupErr)
	} else if !wasNew {
		log.Printf("[CLICK-DEDUP] duplicate click for request_id=%s", requestID)
		fmt.Fprintf(w, `{"status":"duplicate"}`)
		return
	}

	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	// CPC campaigns: charge budget on click.
	// During loader warm-up, GetCampaign returns nil for campaigns that
	// exist but haven't loaded yet. Without this guard the nil check
	// falls through, skipping CPC budget deduction entirely — the click
	// is counted as CPM (free) instead of being charged. Return 503 so
	// the ad network retries after the loader finishes warm-up.
	clickBillingModel := "unknown"
	if campaignID > 0 {
		c := d.Loader.GetCampaign(campaignID)
		if c == nil {
			log.Printf("[CLICK-WARMUP] campaign_id=%d not loaded yet, returning 503", campaignID)
			http.Error(w, `{"error":"bidder warming up, retry"}`, http.StatusServiceUnavailable)
			return
		}
		clickBillingModel = c.BillingModel
		if c.BillingModel == "cpc" {
			clickCents := int64(c.BidCPCCents) // charge per click
			remaining, err := d.BudgetSvc.CheckAndDeductBudget(r.Context(), campaignID, clickCents)
			if err != nil {
				log.Printf("[CLICK-ERROR] campaign_id=%d: %v", campaignID, err)
				http.Error(w, `{"error":"budget check failed"}`, http.StatusInternalServerError)
				return
			}
			if remaining < 0 {
				log.Printf("[CLICK-REJECTED] campaign_id=%d (budget exhausted)", campaignID)
				http.Error(w, `{"error":"budget exhausted"}`, http.StatusConflict)
				return
			}
			observability.BudgetDeductedCentsTotal.WithLabelValues("cpc").Add(float64(clickCents))
			log.Printf("[CLICK-CPC] campaign_id=%d charged=%d remaining=%d", campaignID, clickCents, remaining)
		}
	}

	observability.ClicksTotal.WithLabelValues(clickBillingModel).Inc()

	if campaignID > 0 && d.Producer != nil {
		var charge float64
		c := d.Loader.GetCampaign(campaignID)
		if c != nil && c.BillingModel == "cpc" {
			charge = float64(c.BidCPCCents) / 100.0 // CPC: charge per click in dollars
		}
		// Background context — request context cancels when the handler
		// returns, which would abort the Kafka write in flight. Tracked
		// via producer.Go so shutdown drain can wait for it (Round 1
		// review I4).
		evt := events.Event{
			CampaignID:       campaignID,
			RequestID:        requestID,
			AdvertiserCharge: charge,
			GeoCountry:       r.URL.Query().Get("geo"),
			DeviceOS:         r.URL.Query().Get("os"),
		}
		prod := d.Producer
		prod.Go(func() { prod.SendClick(context.Background(), evt) })
	}

	log.Printf("[CLICK] campaign_id=%d request_id=%s", campaignID, requestID)
	fmt.Fprintf(w, `{"status":"clicked"}`)
}

func (d *Deps) handleConvert(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")

	// Validate HMAC token (same as click)
	if !auth.ValidateToken(d.HMACSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	if campaignID > 0 && d.Producer != nil {
		// Background context — see handleWin / handleClick for rationale.
		// Tracked via producer.Go so shutdown drain can wait.
		evt := events.Event{
			CampaignID: campaignID,
			RequestID:  requestID,
			GeoCountry: r.URL.Query().Get("geo"),
			DeviceOS:   r.URL.Query().Get("os"),
		}
		prod := d.Producer
		prod.Go(func() { prod.SendConversion(context.Background(), evt) })
	}

	log.Printf("[CONVERT] campaign_id=%d request_id=%s", campaignID, requestID)
	fmt.Fprintf(w, `{"status":"converted"}`)
}

func (d *Deps) handleStats(w http.ResponseWriter, r *http.Request) {
	campaigns := d.Loader.GetActiveCampaigns()
	stats := make([]map[string]any, 0, len(campaigns))
	for _, c := range campaigns {
		remaining, _ := d.BudgetSvc.GetDailyBudgetRemaining(r.Context(), c.ID)
		stats = append(stats, map[string]any{
			"id":               c.ID,
			"name":             c.Name,
			"bid_cpm_cents":    c.BidCPMCents,
			"budget_daily":     c.BudgetDailyCents,
			"budget_remaining": remaining,
			"creatives_count":  len(c.Creatives),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func (d *Deps) handleHealth(w http.ResponseWriter, r *http.Request) {
	campaigns := d.Loader.GetActiveCampaigns()
	observability.CampaignActiveTotal.Set(float64(len(campaigns)))
	fmt.Fprintf(w, `{"status":"ok","active_campaigns":%d,"time":"%s"}`,
		len(campaigns), time.Now().UTC().Format(time.RFC3339))
}

// PlatformMargin is the fraction of the advertiser charge retained by the
// platform. The exchange clear price / (1 - PlatformMargin) yields the
// advertiser-facing price — i.e. the ADX cost plus a 10% platform fee.
const PlatformMargin = 0.10

// advertiserChargeCents converts an exchange clear price (in dollars) to the
// advertiser charge in cents, adding the platform margin. This is the single
// source of truth for the `price / 0.90 * 100` formula that appears in the
// win and strategy paths.
func advertiserChargeCents(exchangePrice float64) int64 {
	return int64(exchangePrice / (1 - PlatformMargin) * 100)
}

// injectClickTracker wraps the ad markup's destination URL with a click tracking redirect.
// For HTML ads, it prepends a 1x1 tracking pixel. For all ads, it also sets the click-through
// URL in a way that passes through the tracker first.
func injectClickTracker(adMarkup, clickURL string) string {
	if adMarkup == "" || clickURL == "" {
		return adMarkup
	}
	// Append a 1x1 tracking pixel to HTML ad markup
	tracker := fmt.Sprintf(`<img src="%s" width="1" height="1" style="display:none"/>`, clickURL)
	return adMarkup + tracker
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
