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
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	bidderURL  = "http://localhost:8180"
	totalBids  = 10000
	targetQPS  = 200
	workers    = 10
	clickRate  = 0.03 // 3% CTR
)

var (
	geos    = []string{"CN", "CN", "CN", "US", "JP", "KR"} // weighted toward CN
	oses    = []string{"iOS", "Android", "iOS", "Android", "Windows", "macOS"}
	devices = []string{"mobile", "mobile", "mobile", "desktop", "tablet"}

	bidCount   int64
	winCount   int64
	clickCount int64
	noBidCount int64
	errCount   int64
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
				log.Printf("Progress: %d/%d bids sent, %d wins, %d clicks",
					i+1, totalBids, atomic.LoadInt64(&winCount), atomic.LoadInt64(&clickCount))
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
	log.Printf("Bids sent:  %d", atomic.LoadInt64(&bidCount))
	log.Printf("No-bids:    %d", atomic.LoadInt64(&noBidCount))
	log.Printf("Wins:       %d", atomic.LoadInt64(&winCount))
	log.Printf("Clicks:     %d", atomic.LoadInt64(&clickCount))
	log.Printf("Errors:     %d", atomic.LoadInt64(&errCount))
	log.Printf("Win rate:   %.1f%%", float64(atomic.LoadInt64(&winCount))/float64(atomic.LoadInt64(&bidCount))*100)
	log.Printf("CTR:        %.2f%%", float64(atomic.LoadInt64(&clickCount))/float64(atomic.LoadInt64(&winCount))*100)
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
	}

	// Simulate click (3% CTR)
	if rand.Float64() < clickRate {
		// Build click URL from win URL params
		u, _ := url.Parse(winURL)
		q := u.Query()
		clickURL := fmt.Sprintf("%s/click?campaign_id=%s&request_id=%s&token=%s&geo=%s&os=%s&dest=%s",
			bidderURL, q.Get("campaign_id"), q.Get("request_id"), q.Get("token"),
			q.Get("geo"), q.Get("os"), url.QueryEscape("https://example.com"))
		clickResp, err := http.Get(clickURL)
		if err == nil {
			clickResp.Body.Close()
			if clickResp.StatusCode == 200 || clickResp.StatusCode == 302 {
				atomic.AddInt64(&clickCount, 1)
			}
		}
	}
}
