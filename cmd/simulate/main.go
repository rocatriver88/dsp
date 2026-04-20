package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
)

const (
	bidderURL   = "http://localhost:8180"
	totalBids   = 10000
	targetQPS   = 200
	workers     = 10
	clickRate   = 0.10 // 10% CTR (of impressions, not wins)
	convertRate = 0.30 // 30% of clicks convert
	fillRate    = 0.50 // 50% of wins actually render (fill rate)

	// F6 (#27): the bidder's dev-default HMAC secret — matches
	// defaultBidderHMACSecret in internal/config/config.go. Simulate
	// needs the secret to mint per-handler tokens for /click and /convert
	// (the NURL it receives back only carries the /win token, and
	// post-F6 /win tokens do NOT validate on other endpoints).
	// Override via BIDDER_HMAC_SECRET env var when hitting a non-dev
	// bidder.
	defaultSimHMACSecret = "dev-hmac-secret-change-in-production"
)

// hmacSecret is resolved at package init so simulate can mint tokens that
// match the bidder it targets.
var hmacSecret = func() string {
	if v := os.Getenv("BIDDER_HMAC_SECRET"); v != "" {
		return v
	}
	return defaultSimHMACSecret
}()

var (
	geos    = []string{"CN", "CN", "CN", "US", "JP", "KR"} // weighted toward CN
	oses    = []string{"iOS", "Android", "iOS", "Android", "Windows", "macOS"}
	devices = []string{"mobile", "mobile", "mobile", "desktop", "tablet"}

	bidCount     int64
	winCount     int64
	impCount     int64
	clickCount   int64
	convertCount int64
	noBidCount   int64
	errCount     int64
)

type BidResponse struct {
	ID      string    `json:"id"`
	SeatBid []struct {
		Bid []struct {
			ID    string  `json:"id"`
			ImpID string  `json:"impid"`
			Price float64 `json:"price"`
			AdM   string  `json:"adm"`
			CID   string  `json:"cid"`
			NURL  string  `json:"nurl"`
		} `json:"bid"`
	} `json:"seatbid"`
}

func main() {
	log.Printf("Starting simulation: %d bid requests, %d workers, %.0f%% CTR", totalBids, workers, clickRate*100)
	start := time.Now()

	var wg sync.WaitGroup
	ch := make(chan int, workers*2)

	// Rate limiter: targetQPS requests per second
	ticker := time.NewTicker(time.Second / time.Duration(targetQPS))
	defer ticker.Stop()

	// Producer: feeds work at target QPS
	go func() {
		for i := 0; i < totalBids; i++ {
			<-ticker.C
			ch <- i
			if (i+1)%1000 == 0 {
				log.Printf("Progress: %d/%d bids | wins=%d imps=%d clicks=%d converts=%d",
					i+1, totalBids, atomic.LoadInt64(&winCount), atomic.LoadInt64(&impCount),
					atomic.LoadInt64(&clickCount), atomic.LoadInt64(&convertCount))
			}
		}
		close(ch)
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				simulateBid(i)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	log.Printf("=== SIMULATION COMPLETE ===")
	log.Printf("Duration: %s", elapsed)
	wins := atomic.LoadInt64(&winCount)
	imps := atomic.LoadInt64(&impCount)
	clicks := atomic.LoadInt64(&clickCount)
	converts := atomic.LoadInt64(&convertCount)
	bids := atomic.LoadInt64(&bidCount)

	log.Printf("Bids sent:   %d", bids)
	log.Printf("No-bids:     %d", atomic.LoadInt64(&noBidCount))
	log.Printf("Wins:        %d", wins)
	log.Printf("Impressions: %d", imps)
	log.Printf("Clicks:      %d", clicks)
	log.Printf("Conversions: %d", converts)
	log.Printf("Errors:      %d", atomic.LoadInt64(&errCount))
	log.Printf("Win rate:    %.1f%%", float64(wins)/float64(bids)*100)
	log.Printf("Fill rate:   %.1f%%", float64(imps)/float64(wins)*100)
	log.Printf("CTR:         %.2f%%", float64(clicks)/float64(imps)*100)
	log.Printf("CVR:         %.1f%%", float64(converts)/float64(clicks)*100)
	log.Printf("QPS:        %.0f", float64(totalBids)/elapsed.Seconds())
}

func simulateBid(i int) {
	geo := geos[rand.Intn(len(geos))]
	os := oses[rand.Intn(len(oses))]
	userID := fmt.Sprintf("user-%d-%d", i, rand.Intn(100000))
	reqID := fmt.Sprintf("req-%d-%d", i, time.Now().UnixNano())

	body := fmt.Sprintf(`{
		"id": "%s",
		"imp": [{"id": "imp-1", "banner": {"w": 300, "h": 250}}],
		"device": {
			"os": "%s",
			"ifa": "%s",
			"ip": "1.2.3.%d",
			"ua": "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X)",
			"geo": {"country": "%s"}
		}
	}`, reqID, os, userID, rand.Intn(255), geo)

	resp, err := http.Post(bidderURL+"/bid", "application/json", bytes.NewBufferString(body))
	if err != nil {
		atomic.AddInt64(&errCount, 1)
		return
	}
	defer resp.Body.Close()
	atomic.AddInt64(&bidCount, 1)

	if resp.StatusCode == 204 {
		atomic.AddInt64(&noBidCount, 1)
		return
	}

	data, _ := io.ReadAll(resp.Body)
	var bidResp BidResponse
	if err := json.Unmarshal(data, &bidResp); err != nil {
		atomic.AddInt64(&errCount, 1)
		return
	}

	if len(bidResp.SeatBid) == 0 || len(bidResp.SeatBid[0].Bid) == 0 {
		atomic.AddInt64(&noBidCount, 1)
		return
	}

	bid := bidResp.SeatBid[0].Bid[0]

	// Fill rate: only 50% of bid responses actually get rendered.
	// In real RTB, the exchange sends win notice only when the ad renders.
	// Unrendered wins = no win notice, no impression, no budget deduction.
	filled := rand.Float64() < fillRate
	if !filled {
		return // bid won auction but ad was not rendered (no win notice)
	}

	// Simulate win notice (replace AUCTION_PRICE macro)
	winURL := strings.Replace(bid.NURL, "${AUCTION_PRICE}", fmt.Sprintf("%.6f", bid.Price*0.8), 1)
	winResp, err := http.Post(winURL, "", nil)
	if err != nil {
		atomic.AddInt64(&errCount, 1)
		return
	}
	winResp.Body.Close()

	if winResp.StatusCode == 200 {
		atomic.AddInt64(&winCount, 1)
		atomic.AddInt64(&impCount, 1) // win + render = impression
	}

	// Simulate click (10% CTR of impressions)
	if rand.Float64() < clickRate {
		// Build click URL from win URL params. F6 (#27): post-handler-
		// binding the token extracted from NURL (signed for "win") does
		// NOT validate on /click — mint a fresh "click"-scoped token
		// using the same bid-time params that were signed into the /win
		// token (campID, reqID, creativeID, bidPriceCents). The
		// decorator's NURL carries bid_price_cents but the existing
		// simulate code did not parse it — read it back now so the
		// click token's HMAC payload lines up with what handleClick
		// reads from its URL.
		u, _ := url.Parse(winURL)
		q := u.Query()
		campaignIDParam := q.Get("campaign_id")
		requestIDParam := q.Get("request_id")
		creativeIDParam := q.Get("creative_id")         // may be "" for simulate's lightweight flow
		bidPriceCentsParam := q.Get("bid_price_cents") // signed in NURL by decorator
		clickToken := auth.GenerateToken(hmacSecret, "click",
			campaignIDParam, requestIDParam, creativeIDParam, bidPriceCentsParam)
		clickURL := fmt.Sprintf("%s/click?campaign_id=%s&request_id=%s&creative_id=%s&bid_price_cents=%s&token=%s&geo=%s&os=%s",
			bidderURL, campaignIDParam, requestIDParam, creativeIDParam, bidPriceCentsParam, clickToken,
			q.Get("geo"), q.Get("os"))
		clickResp, err := http.Get(clickURL)
		if err == nil {
			clickResp.Body.Close()
			if clickResp.StatusCode == 200 || clickResp.StatusCode == 302 {
				atomic.AddInt64(&clickCount, 1)

				// Simulate conversion (30% of clicks).
				// Again, mint a fresh "convert"-scoped token — /win
				// and /click tokens no longer validate here post-F6.
				if rand.Float64() < convertRate {
					convertToken := auth.GenerateToken(hmacSecret, "convert",
						campaignIDParam, requestIDParam, creativeIDParam, bidPriceCentsParam)
					convertURL := fmt.Sprintf("%s/convert?campaign_id=%s&request_id=%s&creative_id=%s&bid_price_cents=%s&token=%s&geo=%s&os=%s",
						bidderURL, campaignIDParam, requestIDParam, creativeIDParam, bidPriceCentsParam, convertToken,
						q.Get("geo"), q.Get("os"))
					convResp, err := http.Get(convertURL)
					if err == nil {
						convResp.Body.Close()
						if convResp.StatusCode == 200 {
							atomic.AddInt64(&convertCount, 1)
						}
					}
				}
			}
		}
	}
}
