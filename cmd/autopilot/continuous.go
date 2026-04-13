package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
)

type ContinuousSimulator struct {
	client         *DSPClient
	exchangeSimURL string
	browser        *Browser
	grafanaURL     string
	alerter        alert.Sender
	apiURL         string
	bidderURL      string

	dayStartHour   int
	dayEndHour     int
	dayQPS         int
	nightQPS       int
	healthInterval time.Duration
	reportHour     int
	reportDir      string

	// State
	mu           sync.Mutex
	advertiserID int64
	apiKey       string
	campaignIDs  []int64
}

func (s *ContinuousSimulator) currentQPS(now time.Time) int {
	hour := now.Hour()
	if hour >= s.dayStartHour && hour < s.dayEndHour {
		return s.dayQPS
	}
	return s.nightQPS
}

func (s *ContinuousSimulator) shouldGenerateReport(now time.Time, lastReport time.Time) bool {
	if now.Hour() != s.reportHour {
		return false
	}
	return now.Day() != lastReport.Day() || now.Month() != lastReport.Month()
}

func (s *ContinuousSimulator) Run(ctx context.Context) {
	log.Println("[CONTINUOUS] Starting continuous simulation...")

	if err := s.setup(); err != nil {
		log.Fatalf("[CONTINUOUS] Setup failed: %v", err)
	}

	trafficTicker := time.NewTicker(10 * time.Second)
	healthTicker := time.NewTicker(s.healthInterval)
	operationTicker := time.NewTicker(5 * time.Minute)
	reportCheck := time.NewTicker(1 * time.Minute)
	defer trafficTicker.Stop()
	defer healthTicker.Stop()
	defer operationTicker.Stop()
	defer reportCheck.Stop()

	lastReport := time.Time{}

	for {
		select {
		case <-ctx.Done():
			log.Println("[CONTINUOUS] Shutting down...")
			return

		case <-trafficTicker.C:
			qps := s.currentQPS(time.Now())
			if qps > 0 {
				go s.sendTraffic(qps)
			}

		case <-healthTicker.C:
			go s.checkHealth()

		case <-operationTicker.C:
			go s.randomOperation()

		case <-reportCheck.C:
			now := time.Now()
			if s.shouldGenerateReport(now, lastReport) {
				s.generateDailyReport()
				lastReport = now
			}
		}
	}
}

func (s *ContinuousSimulator) setup() error {
	adv, err := s.client.CreateAdvertiser(
		fmt.Sprintf("Autopilot Continuous %s", time.Now().Format("0102")),
		"autopilot-continuous@test.local",
	)
	if err != nil {
		return fmt.Errorf("create advertiser: %w", err)
	}
	s.advertiserID = adv.ID
	s.apiKey = adv.APIKey
	s.client.APIKey = adv.APIKey

	if s.browser != nil {
		s.browser.apiKey = adv.APIKey
	}

	_, err = s.client.TopUp(adv.ID, 100000000, "autopilot continuous initial")
	if err != nil {
		return fmt.Errorf("topup: %w", err)
	}

	cid, err := s.client.CreateCampaign(CampaignRequest{
		AdvertiserID:     adv.ID,
		Name:             "Continuous Campaign 1",
		BillingModel:     "cpm",
		BudgetTotalCents: 10000000,
		BudgetDailyCents: 1000000,
		BidCPMCents:      500,
	})
	if err != nil {
		return fmt.Errorf("create campaign: %w", err)
	}
	s.mu.Lock()
	s.campaignIDs = append(s.campaignIDs, cid)
	s.mu.Unlock()

	s.client.CreateCreative(CreativeRequest{
		CampaignID:     cid,
		Name:           "Continuous Banner",
		AdType:         "banner",
		Format:         "banner",
		Size:           "300x250",
		AdMarkup:       `<div style="width:300px;height:250px;background:#2563eb;color:#fff;display:flex;align-items:center;justify-content:center">Continuous Test</div>`,
		DestinationURL: "https://example.com",
	})

	s.client.StartCampaign(cid)
	log.Printf("[CONTINUOUS] Setup complete: advertiser=%d, campaign=%d", adv.ID, cid)
	return nil
}

func (s *ContinuousSimulator) sendTraffic(qps int) {
	_, err := s.client.TriggerExchangeSim(s.exchangeSimURL, "load", map[string]string{
		"qps":      fmt.Sprintf("%d", qps),
		"duration": "9",
	})
	if err != nil {
		log.Printf("[TRAFFIC] Error: %v", err)
	}
}

func (s *ContinuousSimulator) checkHealth() {
	services := map[string]string{
		"API":    s.apiURL,
		"Bidder": s.bidderURL,
	}
	for name, url := range services {
		if err := s.client.HealthCheck(url); err != nil {
			msg := fmt.Sprintf("Service %s is DOWN: %v", name, err)
			log.Printf("[HEALTH] ALERT: %s", msg)
			s.alerter.Send("DSP Service Down", msg)
		}
	}
}

func (s *ContinuousSimulator) randomOperation() {
	ops := []string{"create_campaign", "pause_campaign", "adjust_budget"}
	op := ops[rand.Intn(len(ops))]

	switch op {
	case "create_campaign":
		cid, err := s.client.CreateCampaign(CampaignRequest{
			AdvertiserID:     s.advertiserID,
			Name:             fmt.Sprintf("Auto Campaign %s", time.Now().Format("150405")),
			BillingModel:     "cpm",
			BudgetTotalCents: 5000000,
			BudgetDailyCents: 500000,
			BidCPMCents:      300 + rand.Intn(500),
		})
		if err == nil {
			s.mu.Lock()
			s.campaignIDs = append(s.campaignIDs, cid)
			s.mu.Unlock()
			s.client.CreateCreative(CreativeRequest{
				CampaignID: cid, Name: "Auto Creative", AdType: "banner",
				Format: "banner", Size: "300x250",
				AdMarkup:       `<div style="width:300px;height:250px;background:#059669;color:#fff;display:flex;align-items:center;justify-content:center">Auto</div>`,
				DestinationURL: "https://example.com",
			})
			s.client.StartCampaign(cid)
			log.Printf("[OP] Created and started campaign %d", cid)
		}

	case "pause_campaign":
		s.mu.Lock()
		pauseLen := len(s.campaignIDs)
		var pauseCID int64
		if pauseLen > 1 {
			pauseCID = s.campaignIDs[rand.Intn(pauseLen)]
		}
		s.mu.Unlock()
		if pauseLen > 1 {
			s.client.PauseCampaign(pauseCID)
			log.Printf("[OP] Paused campaign %d", pauseCID)
		}

	case "adjust_budget":
		s.mu.Lock()
		adjLen := len(s.campaignIDs)
		var adjCID int64
		if adjLen > 0 {
			adjCID = s.campaignIDs[rand.Intn(adjLen)]
		}
		s.mu.Unlock()
		if adjLen > 0 {
			newBudget := 200000 + rand.Intn(1000000)
			s.client.UpdateCampaign(adjCID, map[string]any{"budget_daily_cents": newBudget})
			log.Printf("[OP] Adjusted campaign %d daily budget to %d", adjCID, newBudget)
		}
	}
}

func (s *ContinuousSimulator) generateDailyReport() {
	log.Println("[REPORT] Generating daily report...")

	overview, err := s.client.GetOverviewStats()
	if err != nil || overview == nil {
		overview = &OverviewStats{}
	}

	var steps []StepResult
	steps = append(steps, StepResult{
		Name:   "Daily Overview",
		Passed: true,
		Detail: fmt.Sprintf("Impressions: %d, Clicks: %d, Spend: %d cents",
			overview.TodayImpressions, overview.TodayClicks, overview.TodaySpendCents),
	})

	if s.browser != nil {
		ss, _ := s.browser.Screenshot("daily-dashboard", "/")
		steps[0].Screenshot = ss
	}

	// Circuit breaker status
	if circuitStatus, err := s.client.GetCircuitStatus(); err == nil {
		steps = append(steps, StepResult{
			Name:   "Circuit Breaker Status",
			Passed: circuitStatus.Status == "open",
			Detail: fmt.Sprintf("Status: %s, Global spend: %d cents",
				circuitStatus.Status, circuitStatus.GlobalSpend),
			Error: circuitStatus.Reason,
		})
	}

	s.mu.Lock()
	campaignIDs := make([]int64, len(s.campaignIDs))
	copy(campaignIDs, s.campaignIDs)
	s.mu.Unlock()

	for _, cid := range campaignIDs {
		stats, err := s.client.GetCampaignStats(cid)
		if err != nil {
			continue
		}
		steps = append(steps, StepResult{
			Name:   fmt.Sprintf("Campaign %d Stats", cid),
			Passed: true,
			Detail: fmt.Sprintf("Impressions: %d, Clicks: %d, Spend: %d cents, WinRate: %.2f%%",
				stats.Impressions, stats.Clicks, stats.SpendCents, stats.WinRate*100),
		})
	}

	report := &VerifyReport{
		StartTime: time.Now().Add(-24 * time.Hour),
		EndTime:   time.Now(),
		Steps:     steps,
	}

	reportFile := filepath.Join(s.reportDir,
		fmt.Sprintf("daily-%s.html", time.Now().Format("2006-01-02")))
	GenerateHTMLReport(report, reportFile)

	passed, _ := report.Summary()
	s.alerter.Send("DSP Daily Report",
		fmt.Sprintf("Date: %s\nCampaigns: %d\nReport: %s",
			time.Now().Format("2006-01-02"), len(campaignIDs), reportFile))

	log.Printf("[REPORT] Daily report: %d steps, all passed=%v, file=%s", len(steps), passed == len(steps), reportFile)
}

// runContinuous is called from main.go
func runContinuous(cfg *AutopilotConfig) {
	log.Println("=== DSP Autopilot: Continuous Mode ===")

	alerter := newAlertSender(cfg)
	client := NewDSPClient(cfg.APIURL, "")
	client.AdminToken = cfg.AdminToken

	var browser *Browser
	browser = NewBrowser(cfg.FrontendURL, "", cfg.ScreenshotDir)
	if err := browser.Start(); err != nil {
		log.Printf("[WARN] Browser not available: %v", err)
		browser = nil
	} else {
		defer browser.Stop()
	}

	sim := &ContinuousSimulator{
		client:         client,
		exchangeSimURL: cfg.ExchangeSimURL,
		browser:        browser,
		grafanaURL:     cfg.GrafanaURL,
		alerter:        alerter,
		apiURL:         cfg.APIURL,
		bidderURL:      cfg.BidderURL,
		dayStartHour:   cfg.DayStartHour,
		dayEndHour:     cfg.DayEndHour,
		dayQPS:         cfg.DayQPS,
		nightQPS:       cfg.NightQPS,
		healthInterval: cfg.HealthInterval,
		reportHour:     cfg.ReportHour,
		reportDir:      cfg.ReportDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go sim.Run(ctx)

	alerter.Send("Autopilot Started", "Continuous simulation mode activated")
	log.Println("[CONTINUOUS] Running. Press Ctrl+C to stop.")

	<-quit
	cancel()
	alerter.Send("Autopilot Stopped", "Continuous simulation mode deactivated")
	log.Println("[CONTINUOUS] Stopped.")
}
