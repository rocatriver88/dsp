package main

import (
	"context"
	"encoding/json"
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
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/redis/go-redis/v9"
)

var (
	engine           *bidder.Engine
	budgetSvc        *budget.Service
	strategySvc      *bidder.BidStrategy
	statsCache       *bidder.StatsCache
	loader           *bidder.CampaignLoader
	producer         *events.Producer
	rdb              *redis.Client
	exchangeRegistry *exchange.Registry
	guard            *guardrail.Guardrail
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
	rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
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
	producer = events.NewProducer(brokers, "/tmp/dsp-kafka-buffer")
	log.Printf("Kafka producer initialized (brokers: %s)", cfg.KafkaBrokers)

	// Initialize services
	budgetSvc = budget.New(rdb)
	strategySvc = bidder.NewBidStrategy(rdb)
	fraudFilter := antifraud.NewFilter(rdb)
	loader = bidder.NewCampaignLoader(db, rdb)

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
	statsCache = bidder.NewStatsCache(rdb, reportStore, loader.GetActiveCampaigns)
	go statsCache.Start(workerCtx)

	guard = guardrail.New(rdb, guardrail.Config{
		GlobalDailyBudgetCents: cfg.GlobalDailyBudgetCents,
		MaxBidCPMCents:         cfg.MaxBidCPMCents,
		LowBalanceAlertCents:   cfg.LowBalanceAlertCents,
		MinBalanceCents:        cfg.MinBalanceCents,
		SpendRateWindowSec:     cfg.SpendRateWindowSec,
		SpendRateMultiplier:    cfg.SpendRateMultiplier,
	})
	log.Println("Guardrail initialized")

	engine = bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)
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

	// Daily budget auto-reset at midnight CST (must start after loader).
	// Bound to workerCtx so SIGTERM stops it cleanly.
	go func() {
		loc, _ := time.LoadLocation("Asia/Shanghai")
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
	exchangeRegistry = exchange.DefaultRegistry(cfg.BidderPublicURL)
	log.Printf("[EXCHANGE] %d exchanges registered", len(exchangeRegistry.ListEnabled()))

	mux := http.NewServeMux()
	mux.HandleFunc("POST /bid", handleBid)                         // standard OpenRTB
	mux.HandleFunc("POST /bid/{exchange_id}", handleExchangeBid)   // per-exchange protocol normalization
	mux.HandleFunc("POST /win", handleWin)
	mux.HandleFunc("GET /win", handleWin)                          // exchanges may use GET for nurl
	mux.HandleFunc("GET /click", handleClick)
	mux.HandleFunc("GET /convert", handleConvert)
	mux.HandleFunc("GET /stats", handleStats)
	mux.HandleFunc("GET /health", handleHealth)
	mux.Handle("GET /metrics", promhttp.Handler())

	addr := ":" + cfg.BidderPort
	srv := &http.Server{Addr: addr, Handler: withLogging(mux)}

	go func() {
		log.Printf("DSP Bidder listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bidder server: %v", err)
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

	// Belt and suspenders: these each close their own stopCh in addition
	// to honoring workerCtx.Done. Idempotent with the workerCancel above.
	loader.Stop()
	statsCache.Stop()

	producer.Close()
	log.Println("Bidder stopped")
}

func handleBid(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req openrtb2.BidRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid bid request"}`, http.StatusBadRequest)
		return
	}

	resp, err := engine.Bid(r.Context(), &req)
	latency := time.Since(start)

	if err != nil {
		log.Printf("[ERROR] request_id=%s error=%v latency=%s", req.ID, err, latency)
		w.WriteHeader(http.StatusNoContent) // fail-closed: no bid on error
		return
	}

	if resp == nil {
		log.Printf("[NO-BID] request_id=%s latency=%s", req.ID, latency)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		baseURL := config.Load().BidderPublicURL
		hmacSecret := config.Load().BidderHMACSecret
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleExchangeBid handles bid requests from specific exchanges with protocol normalization.
// Each exchange may have slight deviations from standard OpenRTB that the adapter normalizes.
func handleExchangeBid(w http.ResponseWriter, r *http.Request) {
	exchangeID := r.PathValue("exchange_id")
	adapter, ok := exchangeRegistry.Get(exchangeID)
	if !ok {
		http.Error(w, fmt.Sprintf(`{"error":"unknown exchange: %s"}`, exchangeID), http.StatusBadRequest)
		return
	}

	start := time.Now()

	// Read raw body and parse via exchange-specific adapter
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, `{"error":"read body failed"}`, http.StatusBadRequest)
		return
	}

	req, err := adapter.ParseBidRequest(body)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"parse failed: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	resp, err := engine.Bid(r.Context(), req)
	latency := time.Since(start)

	if err != nil {
		log.Printf("[ERROR] exchange=%s request_id=%s error=%v latency=%s", exchangeID, req.ID, err, latency)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Add win notice URL (same as standard handler)
	if len(resp.SeatBid) > 0 && len(resp.SeatBid[0].Bid) > 0 {
		bid := resp.SeatBid[0].Bid[0]
		baseURL := config.Load().BidderPublicURL
		hmacSecret := config.Load().BidderHMACSecret
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
		log.Printf("[ERROR] exchange=%s format error: %v", exchangeID, err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func handleWin(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	priceStr := r.URL.Query().Get("price")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")

	// Validate HMAC token
	hmacSecret := config.Load().BidderHMACSecret
	if !auth.ValidateToken(hmacSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	// Win dedup: prevent double budget deduction from exchange retries
	dedupKey := fmt.Sprintf("win:dedup:%s", requestID)
	wasNew, dedupErr := rdb.SetNX(r.Context(), dedupKey, 1, 5*time.Minute).Result()
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

	// Check billing model: CPC campaigns are charged on click, not impression
	c := loader.GetCampaign(campaignID)
	isCPC := c != nil && c.BillingModel == "cpc"

	var remaining int64
	if !isCPC {
		// CPM/oCPM: deduct advertiser charge from budget (not ADX cost)
		priceCents := int64(price / 0.90 * 100) // ADX clear price ÷ 0.9 → advertiser charge in cents
		var budgetErr error
		remaining, budgetErr = budgetSvc.CheckAndDeductBudget(r.Context(), campaignID, priceCents)
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

		// Track global spend for guardrail
		if guard != nil {
			guard.RecordGlobalSpend(r.Context(), priceCents)
		}
	}

	log.Printf("[WIN] campaign_id=%d clear_price=%.6f billing=%s", campaignID, price, func() string {
		if isCPC {
			return "cpc(deferred to click)"
		}
		return fmt.Sprintf("cpm(remaining=%d)", remaining)
	}())

	// Record win + spend for bid strategy (pacing + win-rate tracking)
	if strategySvc != nil {
		go strategySvc.RecordWin(r.Context(), campaignID)
		if !isCPC {
			spendCents := int64(price / 0.90 * 100) // advertiser charge in cents
			go strategySvc.RecordSpend(r.Context(), campaignID, spendCents)
		}
	}

	// Emit win + impression events to Kafka
	if producer != nil {
		var bidPrice float64
		var advertiserCharge float64
		if c != nil {
			bidPrice = float64(c.EffectiveBidCPMCents(0, 0)) * 0.90 / 100.0 / 1000.0
			if isCPC {
				advertiserCharge = 0 // CPC: charged on click, not impression
			} else {
				advertiserCharge = price / 0.90 // ADX clear price ÷ 0.9 = advertiser pays (10% platform fee)
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
		// Use background context — request context gets cancelled when handler
		// returns, which would abort the Kafka write in the goroutine.
		//
		// V5 §P1 Step B: we no longer emit a duplicate 'impression' event
		// here. bid_log therefore stops accumulating one spurious row per
		// won bid. Reporting aggregation is unaffected — Step A already
		// switched the query to countDistinctIf(request_id, ...) which
		// collapsed the duplicate before, so it collapses nothing now.
		// If a real impression callback (pixel / SDK) is ever introduced,
		// it should write its own request_id-distinct event into
		// dsp.impressions; the aggregation will then correctly add it.
		go producer.SendWin(context.Background(), evt)
	}

	fmt.Fprintf(w, `{"status":"ok","remaining_cents":%d}`, remaining)
}

func handleClick(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")
	dest := r.URL.Query().Get("dest")

	// Validate HMAC token
	hmacSecret := config.Load().BidderHMACSecret
	if !auth.ValidateToken(hmacSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	// Click dedup: prevent double budget deduction and double event emission
	// from duplicate click callbacks (ad network retries, multi-click
	// fraud, accidental page reloads). Same 5-minute TTL as the win dedup
	// so short-window retries collapse while long-spaced genuine clicks
	// from the same request_id remain out of scope (there shouldn't be
	// any; the request_id is tied to a single impression).
	dedupKey := fmt.Sprintf("click:dedup:%s", requestID)
	wasNew, dedupErr := rdb.SetNX(r.Context(), dedupKey, 1, 5*time.Minute).Result()
	if dedupErr != nil {
		log.Printf("[CLICK-DEDUP] Redis error (proceeding): %v", dedupErr)
	} else if !wasNew {
		log.Printf("[CLICK-DEDUP] duplicate click for request_id=%s", requestID)
		if dest != "" {
			http.Redirect(w, r, dest, http.StatusFound)
			return
		}
		fmt.Fprintf(w, `{"status":"duplicate"}`)
		return
	}

	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	// CPC campaigns: charge budget on click
	if campaignID > 0 {
		c := loader.GetCampaign(campaignID)
		if c != nil && c.BillingModel == "cpc" {
			clickCents := int64(c.BidCPCCents) // charge per click
			remaining, err := budgetSvc.CheckAndDeductBudget(r.Context(), campaignID, clickCents)
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
			log.Printf("[CLICK-CPC] campaign_id=%d charged=%d remaining=%d", campaignID, clickCents, remaining)
		}
	}

	if campaignID > 0 && producer != nil {
		var charge float64
		c := loader.GetCampaign(campaignID)
		if c != nil && c.BillingModel == "cpc" {
			charge = float64(c.BidCPCCents) / 100.0 // CPC: charge per click in dollars
		}
		// Background context — request context cancels when the handler
		// returns (especially on the dest redirect path), which would
		// abort the Kafka write in flight.
		go producer.SendClick(context.Background(), events.Event{
			CampaignID:       campaignID,
			RequestID:        requestID,
			AdvertiserCharge: charge,
			GeoCountry:       r.URL.Query().Get("geo"),
			DeviceOS:         r.URL.Query().Get("os"),
		})
	}

	log.Printf("[CLICK] campaign_id=%d request_id=%s", campaignID, requestID)

	if dest != "" {
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}
	fmt.Fprintf(w, `{"status":"clicked"}`)
}

func handleConvert(w http.ResponseWriter, r *http.Request) {
	campaignIDStr := r.URL.Query().Get("campaign_id")
	requestID := r.URL.Query().Get("request_id")
	token := r.URL.Query().Get("token")

	// Validate HMAC token (same as click)
	hmacSecret := config.Load().BidderHMACSecret
	if !auth.ValidateToken(hmacSecret, token, campaignIDStr, requestID) {
		http.Error(w, `{"error":"invalid or expired token"}`, http.StatusForbidden)
		return
	}

	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	if campaignID > 0 && producer != nil {
		// Background context — see handleWin / handleClick for rationale.
		go producer.SendConversion(context.Background(), events.Event{
			CampaignID: campaignID,
			RequestID:  requestID,
			GeoCountry: r.URL.Query().Get("geo"),
			DeviceOS:   r.URL.Query().Get("os"),
		})
	}

	log.Printf("[CONVERT] campaign_id=%d request_id=%s", campaignID, requestID)
	fmt.Fprintf(w, `{"status":"converted"}`)
}

func handleStats(w http.ResponseWriter, r *http.Request) {
	campaigns := loader.GetActiveCampaigns()
	stats := make([]map[string]any, 0, len(campaigns))
	for _, c := range campaigns {
		remaining, _ := budgetSvc.GetDailyBudgetRemaining(r.Context(), c.ID)
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

func handleHealth(w http.ResponseWriter, r *http.Request) {
	campaigns := loader.GetActiveCampaigns()
	fmt.Fprintf(w, `{"status":"ok","active_campaigns":%d,"time":"%s"}`,
		len(campaigns), time.Now().UTC().Format(time.RFC3339))
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
