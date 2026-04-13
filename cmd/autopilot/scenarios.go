package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ScenarioRunner executes verify mode scenarios.
type ScenarioRunner struct {
	client         *DSPClient
	exchangeSimURL string
	bidderURL      string
	adminURL       string
	browser        *Browser
	grafanaURL     string
	trafficWait    time.Duration

	// State populated during run
	advertiserID int64
	apiKey       string
	campaignID   int64
	creativeID   int64
}

func (s *ScenarioRunner) runStep(name string, fn func() (string, error)) StepResult {
	log.Printf("[STEP] %s ...", name)
	start := time.Now()
	detail, err := fn()
	duration := time.Since(start)

	result := StepResult{
		Name:     name,
		Duration: duration,
		Detail:   detail,
		Passed:   err == nil,
	}
	if err != nil {
		result.Error = err.Error()
		log.Printf("[FAIL] %s: %v (%.1fs)", name, err, duration.Seconds())
	} else {
		log.Printf("[PASS] %s (%.1fs)", name, duration.Seconds())
	}
	return result
}

func (s *ScenarioRunner) screenshot(name, path string) string {
	if s.browser == nil {
		return ""
	}
	file, err := s.browser.Screenshot(name, path)
	if err != nil {
		log.Printf("[WARN] screenshot failed for %s: %v", name, err)
		return ""
	}
	return file
}

func (s *ScenarioRunner) screenshotGrafana(name, dashPath string) string {
	if s.browser == nil || s.grafanaURL == "" {
		return ""
	}
	file, err := s.browser.ScreenshotGrafana(name, s.grafanaURL, dashPath)
	if err != nil {
		log.Printf("[WARN] grafana screenshot failed for %s: %v", name, err)
		return ""
	}
	return file
}

// RunNormalFlow executes steps 1-6: create advertiser, top up, create campaign, start, traffic, pause.
func (s *ScenarioRunner) RunNormalFlow() []StepResult {
	var steps []StepResult

	// Step 1: Create Advertiser (register flow: create invite → register → approve)
	step := s.runStep("Create Advertiser", func() (string, error) {
		companyName := fmt.Sprintf("Autopilot Test %s", time.Now().Format("0102-1504"))
		email := fmt.Sprintf("autopilot-%d@test.local", time.Now().UnixMilli())

		// 1a. Create invite code via admin API
		code, err := s.client.AdminCreateInviteCode(s.adminURL, 1)
		if err != nil {
			return "", fmt.Errorf("create invite code: %w", err)
		}

		// 1b. Register via public API (auth-exempt)
		regID, err := s.client.Register(companyName, email, code)
		if err != nil {
			return "", fmt.Errorf("register: %w", err)
		}

		// 1c. Approve via admin API
		adv, err := s.client.AdminApproveRegistration(s.adminURL, regID)
		if err != nil {
			return "", fmt.Errorf("approve: %w", err)
		}

		s.advertiserID = adv.ID
		s.apiKey = adv.APIKey
		s.client.APIKey = adv.APIKey
		return fmt.Sprintf("Advertiser id=%d, api_key=%s (invite=%s, reg=%d)", adv.ID, adv.APIKey, code, regID), nil
	})
	step.Screenshot = s.screenshot("01-dashboard-empty", "/")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 2: Top Up Balance
	step = s.runStep("Top Up Balance", func() (string, error) {
		balance, err := s.client.TopUp(s.advertiserID, 10000000, "autopilot verify top-up")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Balance: %d cents (%.2f CNY)", balance, float64(balance)/100), nil
	})
	step.Screenshot = s.screenshot("02-billing-topup", "/billing")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 3: Create Campaign + Creative
	step = s.runStep("Create Campaign + Creative", func() (string, error) {
		cid, err := s.client.CreateCampaign(CampaignRequest{
			AdvertiserID:     s.advertiserID,
			Name:             "Autopilot Verify Campaign",
			BillingModel:     "cpm",
			BudgetTotalCents: 5000000,
			BudgetDailyCents: 1000000,
			BidCPMCents:      500,
		})
		if err != nil {
			return "", fmt.Errorf("create campaign: %w", err)
		}
		s.campaignID = cid

		crID, err := s.client.CreateCreative(CreativeRequest{
			CampaignID:     cid,
			Name:           "Autopilot Test Banner",
			AdType:         "banner",
			Format:         "image",
			Size:           "300x250",
			AdMarkup:       `<div style="width:300px;height:250px;background:#2563eb;color:#fff;display:flex;align-items:center;justify-content:center;font-size:18px">Autopilot Test Ad</div>`,
			DestinationURL: "https://example.com/landing",
		})
		if err != nil {
			return "", fmt.Errorf("create creative: %w", err)
		}
		s.creativeID = crID

		return fmt.Sprintf("Campaign id=%d, Creative id=%d", cid, crID), nil
	})
	step.Screenshot = s.screenshot("03-campaign-detail", fmt.Sprintf("/campaigns/%d", s.campaignID))
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 4: Start Campaign + Send Traffic
	step = s.runStep("Start Campaign + Send Traffic", func() (string, error) {
		if err := s.client.StartCampaign(s.campaignID); err != nil {
			return "", fmt.Errorf("start campaign: %w", err)
		}

		result, err := s.client.TriggerExchangeSim(s.exchangeSimURL, "burst", nil)
		if err != nil {
			return "", fmt.Errorf("exchange-sim burst: %w", err)
		}

		bids, _ := result["bids"].(float64)
		total, _ := result["total"].(float64)
		return fmt.Sprintf("Campaign started. Burst: %d/%d bids", int(bids), int(total)), nil
	})
	step.Screenshot = s.screenshot("04-analytics-live", "/analytics")
	steps = append(steps, step)
	if !step.Passed {
		return steps
	}

	// Step 5: Wait for Data + Check Reports
	step = s.runStep("Wait for Data Accumulation", func() (string, error) {
		log.Printf("[INFO] Waiting %s for data pipeline...", s.trafficWait)
		time.Sleep(s.trafficWait)

		stats, err := s.client.GetCampaignStats(s.campaignID)
		if err != nil {
			return "", fmt.Errorf("get stats: %w", err)
		}

		overview, _ := s.client.GetOverviewStats()

		detail := fmt.Sprintf("Campaign stats: impressions=%d clicks=%d spend=%d cents",
			stats.Impressions, stats.Clicks, stats.SpendCents)
		if overview != nil {
			detail += fmt.Sprintf("\nOverview: impressions=%d clicks=%d spend=%d cents",
				overview.TodayImpressions, overview.TodayClicks, overview.TodaySpendCents)
		}
		return detail, nil
	})
	step.Screenshot = s.screenshot("05-reports", "/reports")
	steps = append(steps, step)

	// Step 6: Pause Campaign
	step = s.runStep("Pause Campaign", func() (string, error) {
		if err := s.client.PauseCampaign(s.campaignID); err != nil {
			return "", err
		}
		camp, err := s.client.GetCampaign(s.campaignID)
		if err != nil {
			return "", err
		}
		if camp.Status != "paused" {
			return "", fmt.Errorf("expected status paused, got %s", camp.Status)
		}
		return fmt.Sprintf("Campaign %d paused successfully", s.campaignID), nil
	})
	step.Screenshot = s.screenshot("06-campaign-paused", "/campaigns")
	steps = append(steps, step)

	return steps
}

// RunFaultScenarios executes steps 7-9: budget exhaustion, service restart, Kafka delay.
// Requires a running Docker environment and an active campaign from RunNormalFlow.
func (s *ScenarioRunner) RunFaultScenarios(faultInjector *FaultInjector) []StepResult {
	var steps []StepResult
	ctx := context.Background()

	// Skip all fault scenarios if normal flow didn't create a campaign
	if s.campaignID == 0 {
		steps = append(steps, StepResult{
			Name:   "Fault Scenarios",
			Passed: false,
			Error:  "skipped — normal flow did not create a campaign",
		})
		return steps
	}

	// Step 7: Budget Exhaustion
	step := s.runStep("Fault: Budget Exhaustion", func() (string, error) {
		err := s.client.UpdateCampaign(s.campaignID, map[string]any{
			"budget_daily_cents": 100,
		})
		if err != nil {
			return "", fmt.Errorf("set low budget: %w", err)
		}

		s.client.StartCampaign(s.campaignID)

		_, err = s.client.TriggerExchangeSim(s.exchangeSimURL, "load", map[string]string{
			"qps": "100", "duration": "5",
		})
		if err != nil {
			return "", fmt.Errorf("trigger load: %w", err)
		}

		time.Sleep(10 * time.Second)

		camp, err := s.client.GetCampaign(s.campaignID)
		if err != nil {
			return "", fmt.Errorf("get campaign: %w", err)
		}

		balance, _ := s.client.GetBalance(s.advertiserID)
		detail := fmt.Sprintf("Campaign status: %s, Remaining balance: %d cents", camp.Status, balance)

		if camp.Status != "paused" {
			return detail, fmt.Errorf("expected campaign to auto-pause, got status=%s", camp.Status)
		}
		return detail, nil
	})
	step.Screenshot = s.screenshot("07-budget-exhausted", "/campaigns")
	steps = append(steps, step)

	// Step 8: Service Restart (bidder)
	step = s.runStep("Fault: Bidder Service Restart", func() (string, error) {
		if faultInjector == nil {
			return "skipped (no Docker access)", nil
		}

		s.screenshotGrafana("08a-grafana-before", "/d/dsp-overview")

		if err := faultInjector.RestartContainer(ctx, "bidder"); err != nil {
			return "", fmt.Errorf("restart bidder: %w", err)
		}

		recoveryTime, err := WaitForHealthy(s.bidderURL, 60*time.Second)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Bidder restarted. Recovery time: %s", recoveryTime.Round(time.Millisecond)), nil
	})
	step.Screenshot = s.screenshotGrafana("08b-grafana-after", "/d/dsp-overview")
	steps = append(steps, step)

	// Step 9: Kafka Delay (pause consumer)
	step = s.runStep("Fault: Kafka Consumer Delay", func() (string, error) {
		if faultInjector == nil {
			return "skipped (no Docker access)", nil
		}

		if err := faultInjector.PauseContainer(ctx, "consumer"); err != nil {
			return "", fmt.Errorf("pause consumer: %w", err)
		}

		s.client.UpdateCampaign(s.campaignID, map[string]any{"budget_daily_cents": 1000000})
		s.client.StartCampaign(s.campaignID)

		s.client.TriggerExchangeSim(s.exchangeSimURL, "burst", nil)
		log.Printf("[INFO] Consumer paused — messages accumulating in Kafka...")
		time.Sleep(15 * time.Second)

		if err := faultInjector.UnpauseContainer(ctx, "consumer"); err != nil {
			return "", fmt.Errorf("unpause consumer: %w", err)
		}

		log.Printf("[INFO] Consumer resumed — waiting for catch-up...")
		time.Sleep(10 * time.Second)

		stats, err := s.client.GetCampaignStats(s.campaignID)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("Consumer caught up. Impressions: %d, Clicks: %d", stats.Impressions, stats.Clicks), nil
	})
	step.Screenshot = s.screenshot("09-analytics-catchup", "/analytics")
	steps = append(steps, step)

	return steps
}
