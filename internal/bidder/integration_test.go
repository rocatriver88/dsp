package bidder_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/prebid/openrtb/v20/openrtb2"
)

// Full integration test: bid request → bid response → win notice → budget deduction
func TestFullBidLifecycle(t *testing.T) {
	b := bidder.New()

	// Set up HTTP handlers (same as cmd/bidder/main.go but in-process)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /bid", func(w http.ResponseWriter, r *http.Request) {
		var req openrtb2.BidRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
			return
		}
		result := b.ProcessRequest(&req)
		if result == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		resp := openrtb2.BidResponse{
			ID: req.ID,
			SeatBid: []openrtb2.SeatBid{{
				Bid: []openrtb2.Bid{{
					ID:      result.BidID,
					ImpID:   req.Imp[0].ID,
					Price:   result.BidPrice,
					AdM:     result.Campaign.AdMarkup,
					ADomain: []string{result.Campaign.AdvDomain},
				}},
				Seat: fmt.Sprintf("campaign-%d", result.Campaign.ID),
			}},
			Cur: "USD",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("POST /win", func(w http.ResponseWriter, r *http.Request) {
		cid, _ := strconv.Atoi(r.URL.Query().Get("campaign_id"))
		price, _ := strconv.ParseFloat(r.URL.Query().Get("price"), 64)
		ok := b.RecordWin(cid, price)
		if !ok {
			http.Error(w, `{"error":"budget exhausted"}`, http.StatusConflict)
			return
		}
		fmt.Fprintf(w, `{"status":"ok"}`)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Step 1: Send bid request (CN + iOS → should match campaign 3 "移动游戏下载" at CPM 12.0)
	w, h := int64(320), int64(50)
	bidReq := openrtb2.BidRequest{
		ID: "lifecycle-test-1",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "iOS",
			Geo: &openrtb2.Geo{Country: "CN"},
		},
	}

	reqBody, _ := json.Marshal(bidReq)
	resp, err := http.Post(srv.URL+"/bid", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("bid request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var bidResp openrtb2.BidResponse
	if err := json.NewDecoder(resp.Body).Decode(&bidResp); err != nil {
		t.Fatalf("decode bid response: %v", err)
	}

	if bidResp.ID != "lifecycle-test-1" {
		t.Errorf("response ID mismatch: %s", bidResp.ID)
	}
	if len(bidResp.SeatBid) == 0 || len(bidResp.SeatBid[0].Bid) == 0 {
		t.Fatal("expected at least one bid")
	}

	bid := bidResp.SeatBid[0].Bid[0]
	t.Logf("Received bid: id=%s price=%.4f seat=%s", bid.ID, bid.Price, bidResp.SeatBid[0].Seat)

	if bid.Price <= 0 {
		t.Error("bid price should be positive")
	}
	if bid.AdM == "" {
		t.Error("ad markup should not be empty")
	}

	// Step 2: Send win notice with clear price (80% of bid)
	clearPrice := bid.Price * 0.8
	// Extract campaign ID from seat (format: "campaign-N")
	seat := bidResp.SeatBid[0].Seat
	var campaignID int
	fmt.Sscanf(seat, "campaign-%d", &campaignID)

	winURL := fmt.Sprintf("%s/win?campaign_id=%d&price=%.4f", srv.URL, campaignID, clearPrice)
	winResp, err := http.Post(winURL, "application/json", nil)
	if err != nil {
		t.Fatalf("win notice failed: %v", err)
	}
	winResp.Body.Close()

	if winResp.StatusCode != http.StatusOK {
		t.Errorf("win notice: expected 200, got %d", winResp.StatusCode)
	}

	// Step 3: Verify budget was deducted
	stats := b.Stats()
	for _, s := range stats {
		if s["id"].(int) == campaignID {
			spent := s["spent"].(float64)
			if math.Abs(spent-clearPrice) > 0.0001 {
				t.Errorf("expected spent≈%.4f, got %.4f", clearPrice, spent)
			}
			t.Logf("Campaign %d spent: %.4f (budget remain: %.4f)",
				campaignID, spent, s["remain"].(float64))
		}
	}
}

func TestNoBidResponse(t *testing.T) {
	b := bidder.New()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /bid", func(w http.ResponseWriter, r *http.Request) {
		var req openrtb2.BidRequest
		json.NewDecoder(r.Body).Decode(&req)
		result := b.ProcessRequest(&req)
		if result == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Country ZZ matches no campaigns
	w, h := int64(300), int64(250)
	req := openrtb2.BidRequest{
		ID: "nobid-test",
		Imp: []openrtb2.Imp{{
			ID:     "imp-1",
			Banner: &openrtb2.Banner{W: &w, H: &h},
		}},
		Device: &openrtb2.Device{
			OS:  "macOS",
			Geo: &openrtb2.Geo{Country: "ZZ"},
		},
	}

	body, _ := json.Marshal(req)
	resp, err := http.Post(srv.URL+"/bid", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("expected 204 No Content, got %d", resp.StatusCode)
	}
}

func TestConcurrentBids(t *testing.T) {
	b := bidder.New()

	mux := http.NewServeMux()
	mux.HandleFunc("POST /bid", func(w http.ResponseWriter, r *http.Request) {
		var req openrtb2.BidRequest
		json.NewDecoder(r.Body).Decode(&req)
		result := b.ProcessRequest(&req)
		if result == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		resp := openrtb2.BidResponse{
			ID: req.ID,
			SeatBid: []openrtb2.SeatBid{{
				Bid: []openrtb2.Bid{{
					ID:    result.BidID,
					ImpID: req.Imp[0].ID,
					Price: result.BidPrice,
				}},
			}},
		}
		json.NewEncoder(w).Encode(resp)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Fire 100 concurrent requests
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			w, h := int64(300), int64(250)
			req := openrtb2.BidRequest{
				ID: fmt.Sprintf("concurrent-%d", n),
				Imp: []openrtb2.Imp{{
					ID:     "imp-1",
					Banner: &openrtb2.Banner{W: &w, H: &h},
				}},
				Device: &openrtb2.Device{
					OS:  "macOS",
					Geo: &openrtb2.Geo{Country: "CN"},
				},
			}
			body, _ := json.Marshal(req)
			resp, err := http.Post(srv.URL+"/bid", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Errorf("request %d failed: %v", n, err)
				done <- false
				return
			}
			resp.Body.Close()
			done <- resp.StatusCode == http.StatusOK
		}(i)
	}

	bids := 0
	for i := 0; i < 100; i++ {
		if <-done {
			bids++
		}
	}
	t.Logf("Concurrent test: %d/100 got bids", bids)
	if bids == 0 {
		t.Error("expected at least some bids from concurrent requests")
	}
}
