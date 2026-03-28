package main

import (
	"context"
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

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prebid/openrtb/v20/openrtb2"
	"github.com/redis/go-redis/v9"
)

var (
	engine    *bidder.Engine
	budgetSvc *budget.Service
	loader    *bidder.CampaignLoader
	producer  *events.Producer
	rdb       *redis.Client
)

func main() {
	cfg := config.Load()
	ctx := context.Background()

	// Connect PostgreSQL
	db, err := pgxpool.New(ctx, cfg.DSN())
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("ping postgres: %v", err)
	}
	log.Println("Connected to PostgreSQL")

	// Connect Redis
	rdb = redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("connect redis: %v (bidder requires Redis for budget/freq control)", err)
	}
	log.Println("Connected to Redis")

	// Initialize Kafka producer (optional)
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	producer = events.NewProducer(brokers, "/tmp/dsp-kafka-buffer")
	defer producer.Close()
	log.Printf("Kafka producer initialized (brokers: %s)", cfg.KafkaBrokers)

	// Initialize services
	budgetSvc = budget.New(rdb)
	fraudFilter := antifraud.NewFilter(rdb)
	loader = bidder.NewCampaignLoader(db, rdb)
	engine = bidder.NewEngine(loader, budgetSvc, producer, fraudFilter)
	log.Printf("Anti-fraud filter initialized (%v)", fraudFilter.Stats())

	// Load campaigns from DB
	if err := loader.Start(ctx); err != nil {
		log.Fatalf("campaign loader: %v", err)
	}
	defer loader.Stop()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /bid", handleBid)
	mux.HandleFunc("POST /win", handleWin)
	mux.HandleFunc("GET /click", handleClick)
	mux.HandleFunc("GET /stats", handleStats)
	mux.HandleFunc("GET /health", handleHealth)

	addr := ":" + cfg.BidderPort
	srv := &http.Server{Addr: addr, Handler: withLogging(mux)}

	go func() {
		log.Printf("DSP Bidder listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("bidder server: %v", err)
		}
	}()

	// Graceful shutdown: drain in-flight requests, flush Kafka, close connections
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit
	log.Println("Shutting down bidder...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(shutdownCtx)
	loader.Stop()
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
		resp.SeatBid[0].Bid[0] = bid

		log.Printf("[BID] request_id=%s campaign=%s bid=%.6f latency=%s",
			req.ID, bid.CID, bid.Price, latency)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
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

	// Deduct from Redis budget
	priceCents := int64(price * 100 * 1000) // dollars per impression → cents per mille
	remaining, budgetErr := budgetSvc.CheckAndDeductBudget(r.Context(), campaignID, priceCents)
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

	log.Printf("[WIN] campaign_id=%d clear_price=%.6f remaining_cents=%d", campaignID, price, remaining)

	// Emit win + impression events to Kafka
	if producer != nil {
		// Get original bid price from campaign config
		var bidPrice float64
		if c := loader.GetCampaign(campaignID); c != nil {
			bidPrice = float64(c.BidCPMCents) / 100.0 / 1000.0 // CPM cents → dollars/impression
		}
		evt := events.Event{
			CampaignID: campaignID,
			RequestID:  r.URL.Query().Get("request_id"),
			BidPrice:   bidPrice,
			ClearPrice: price,
			GeoCountry: r.URL.Query().Get("geo"),
			DeviceOS:   r.URL.Query().Get("os"),
		}
		go producer.SendWin(r.Context(), evt)
		go producer.SendImpression(r.Context(), evt)
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

	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	if campaignID > 0 && producer != nil {
		go producer.SendClick(r.Context(), events.Event{
			CampaignID: campaignID,
			RequestID:  requestID,
			GeoCountry: r.URL.Query().Get("geo"),
			DeviceOS:   r.URL.Query().Get("os"),
		})
	}

	log.Printf("[CLICK] campaign_id=%d request_id=%s", campaignID, requestID)

	if dest != "" {
		http.Redirect(w, r, dest, http.StatusFound)
		return
	}
	fmt.Fprintf(w, `{"status":"clicked"}`)
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

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/health" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
		}
	})
}
