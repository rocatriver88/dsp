package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// Exchange simulator — sends OpenRTB bid requests to the DSP bidder
// and simulates the full auction lifecycle.

var (
	bidderURL = "http://localhost:8180"
	client    = &http.Client{Timeout: 200 * time.Millisecond}

	totalRequests  atomic.Int64
	totalBids      atomic.Int64
	totalNoBids    atomic.Int64
	totalWins      atomic.Int64
	totalErrors    atomic.Int64
	totalLatencyUs atomic.Int64
)

func main() {
	log.Println("Exchange Simulator starting...")
	log.Printf("Bidder URL: %s", bidderURL)
	log.Println("")
	log.Println("Modes:")
	log.Println("  single  — send 1 bid request, print full response")
	log.Println("  burst   — send 100 requests, show summary")
	log.Println("  load    — sustained load test (1000 QPS for 10s)")
	log.Println("  stats   — show bidder campaign stats")
	log.Println("")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /single", handleSingle)
	mux.HandleFunc("GET /burst", handleBurst)
	mux.HandleFunc("GET /load", handleLoad)
	mux.HandleFunc("GET /stats", handleExchangeStats)
	mux.HandleFunc("GET /bidder-stats", handleBidderStats)

	addr := ":9090"
	log.Printf("Exchange sim listening on %s", addr)
	log.Println("Try: curl http://localhost:9090/single")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func handleSingle(w http.ResponseWriter, r *http.Request) {
	req := randomBidRequest()
	reqJSON, _ := json.MarshalIndent(req, "", "  ")

	log.Printf("[SEND] Single bid request: id=%s geo=%s os=%s",
		req.ID, req.Device.Geo.Country, req.Device.OS)

	start := time.Now()
	resp, bidResp, err := sendBidRequest(req)
	latency := time.Since(start)

	result := map[string]any{
		"request":  json.RawMessage(reqJSON),
		"latency":  latency.String(),
		"latency_us": latency.Microseconds(),
	}

	if err != nil {
		result["error"] = err.Error()
	} else if resp.StatusCode == http.StatusNoContent {
		result["response"] = "NO BID"
	} else {
		respJSON, _ := json.MarshalIndent(bidResp, "", "  ")
		result["response"] = json.RawMessage(respJSON)

		// Simulate win notice
		if len(bidResp.SeatBid) > 0 && len(bidResp.SeatBid[0].Bid) > 0 {
			bid := bidResp.SeatBid[0].Bid[0]
			clearPrice := bid.Price * 0.8 // second-price: clear at 80% of bid
			winURL := strings.Replace(bid.NURL, "${AUCTION_PRICE}", fmt.Sprintf("%.4f", clearPrice), 1)

			winResp, winErr := http.Post(winURL, "application/json", nil)
			if winErr != nil {
				result["win_notice"] = map[string]any{"error": winErr.Error()}
			} else {
				winBody, _ := io.ReadAll(winResp.Body)
				winResp.Body.Close()
				result["win_notice"] = map[string]any{
					"status":      winResp.StatusCode,
					"clear_price": clearPrice,
					"response":    string(winBody),
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleBurst(w http.ResponseWriter, r *http.Request) {
	n := 100
	var wg sync.WaitGroup
	bids, noBids, errors := atomic.Int64{}, atomic.Int64{}, atomic.Int64{}
	var latencies []int64
	var mu sync.Mutex

	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := randomBidRequest()
			t := time.Now()
			resp, _, err := sendBidRequest(req)
			lat := time.Since(t).Microseconds()

			mu.Lock()
			latencies = append(latencies, lat)
			mu.Unlock()

			if err != nil {
				errors.Add(1)
			} else if resp.StatusCode == http.StatusNoContent {
				noBids.Add(1)
			} else {
				bids.Add(1)
			}
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Calculate p50, p95, p99
	var sum int64
	for _, l := range latencies {
		sum += l
	}
	avg := sum / int64(len(latencies))

	result := map[string]any{
		"total":     n,
		"bids":      bids.Load(),
		"no_bids":   noBids.Load(),
		"errors":    errors.Load(),
		"elapsed":   elapsed.String(),
		"qps":       float64(n) / elapsed.Seconds(),
		"avg_us":    avg,
		"avg_ms":    float64(avg) / 1000.0,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleLoad(w http.ResponseWriter, r *http.Request) {
	// Parse optional params: ?qps=500&duration=10&threshold_p99_ms=50
	targetQPS := 1000
	if q := r.URL.Query().Get("qps"); q != "" {
		if v, err := fmt.Sscanf(q, "%d", &targetQPS); v == 1 && err == nil && targetQPS > 0 {
			// ok
		}
	}
	durationSec := 10
	if d := r.URL.Query().Get("duration"); d != "" {
		fmt.Sscanf(d, "%d", &durationSec)
	}
	thresholdP99Ms := 50.0
	if t := r.URL.Query().Get("threshold_p99_ms"); t != "" {
		fmt.Sscanf(t, "%f", &thresholdP99Ms)
	}

	duration := time.Duration(durationSec) * time.Second
	interval := time.Second / time.Duration(targetQPS)

	log.Printf("[LOAD] Starting %d QPS for %s (p99 threshold: %.0fms)", targetQPS, duration, thresholdP99Ms)

	resetCounters()
	var allLatencies []int64
	var latMu sync.Mutex

	start := time.Now()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	done := time.After(duration)
	for {
		select {
		case <-done:
			elapsed := time.Since(start)
			total := totalRequests.Load()
			avgUs := int64(0)
			if total > 0 {
				avgUs = totalLatencyUs.Load() / total
			}

			// Calculate percentiles
			latMu.Lock()
			sortedLats := make([]int64, len(allLatencies))
			copy(sortedLats, allLatencies)
			latMu.Unlock()
			sortInt64s(sortedLats)

			p50, p95, p99 := int64(0), int64(0), int64(0)
			if len(sortedLats) > 0 {
				p50 = percentile(sortedLats, 50)
				p95 = percentile(sortedLats, 95)
				p99 = percentile(sortedLats, 99)
			}

			p99Ms := float64(p99) / 1000.0
			passed := p99Ms <= thresholdP99Ms

			result := map[string]any{
				"duration":          elapsed.String(),
				"total":             total,
				"bids":              totalBids.Load(),
				"no_bids":           totalNoBids.Load(),
				"errors":            totalErrors.Load(),
				"actual_qps":        float64(total) / elapsed.Seconds(),
				"avg_latency_us":    avgUs,
				"avg_latency_ms":    float64(avgUs) / 1000.0,
				"p50_us":            p50,
				"p50_ms":            float64(p50) / 1000.0,
				"p95_us":            p95,
				"p95_ms":            float64(p95) / 1000.0,
				"p99_us":            p99,
				"p99_ms":            p99Ms,
				"threshold_p99_ms":  thresholdP99Ms,
				"passed":            passed,
			}

			verdict := "PASS"
			if !passed {
				verdict = "FAIL"
			}
			log.Printf("[LOAD] %s: %d requests, %.0f QPS, p50=%.2fms p95=%.2fms p99=%.2fms (threshold: %.0fms)",
				verdict, total, float64(total)/elapsed.Seconds(),
				float64(p50)/1000.0, float64(p95)/1000.0, p99Ms, thresholdP99Ms)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(result)
			return

		case <-ticker.C:
			go func() {
				req := randomBidRequest()
				t := time.Now()
				resp, _, err := sendBidRequest(req)
				lat := time.Since(t)

				totalRequests.Add(1)
				totalLatencyUs.Add(lat.Microseconds())

				latMu.Lock()
				allLatencies = append(allLatencies, lat.Microseconds())
				latMu.Unlock()

				if err != nil {
					totalErrors.Add(1)
				} else if resp.StatusCode == http.StatusNoContent {
					totalNoBids.Add(1)
				} else {
					totalBids.Add(1)
				}
			}()
		}
	}
}

func sortInt64s(a []int64) {
	// Simple insertion sort for latency arrays (typically <20k elements)
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func handleExchangeStats(w http.ResponseWriter, r *http.Request) {
	total := totalRequests.Load()
	avgUs := int64(0)
	if total > 0 {
		avgUs = totalLatencyUs.Load() / total
	}
	result := map[string]any{
		"total_requests": total,
		"total_bids":     totalBids.Load(),
		"total_no_bids":  totalNoBids.Load(),
		"total_errors":   totalErrors.Load(),
		"avg_latency_us": avgUs,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func handleBidderStats(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(bidderURL + "/stats")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	io.Copy(w, resp.Body)
}

func sendBidRequest(req *openrtb2.BidRequest) (*http.Response, *openrtb2.BidResponse, error) {
	body, _ := json.Marshal(req)
	resp, err := client.Post(bidderURL+"/bid", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return resp, nil, nil
	}

	var bidResp openrtb2.BidResponse
	if err := json.NewDecoder(resp.Body).Decode(&bidResp); err != nil {
		return resp, nil, err
	}
	return resp, &bidResp, nil
}

func randomBidRequest() *openrtb2.BidRequest {
	geos := []string{"CN", "US", "JP", "GB", "DE", "KR", "BR"}
	oses := []string{"iOS", "Android", "Windows", "macOS", "Linux"}

	w := int64(300)
	h := int64(250)

	return &openrtb2.BidRequest{
		ID: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Imp: []openrtb2.Imp{
			{
				ID: "1",
				Banner: &openrtb2.Banner{
					W: &w,
					H: &h,
				},
				BidFloor: 0.5,
			},
		},
		Site: &openrtb2.Site{
			Domain: "tech-blog.example.com",
			Page:   "https://tech-blog.example.com/article/rust-performance",
		},
		Device: &openrtb2.Device{
			OS: oses[rand.Intn(len(oses))],
			Geo: &openrtb2.Geo{
				Country: geos[rand.Intn(len(geos))],
			},
			UA: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		},
	}
}

func resetCounters() {
	totalRequests.Store(0)
	totalBids.Store(0)
	totalNoBids.Store(0)
	totalWins.Store(0)
	totalErrors.Store(0)
	totalLatencyUs.Store(0)
}
