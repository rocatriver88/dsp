package main

import (
	"fmt"
	"log"
	"time"
)

// ScenarioRunner executes verify mode scenarios.
type ScenarioRunner struct {
	client         *DSPClient
	exchangeSimURL string
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

	// Step 1: Create Advertiser
	step := s.runStep("Create Advertiser", func() (string, error) {
		adv, err := s.client.CreateAdvertiser(
			fmt.Sprintf("Autopilot Test %s", time.Now().Format("0102-1504")),
			"autopilot@test.local",
		)
		if err != nil {
			return "", err
		}
		s.advertiserID = adv.ID
		s.apiKey = adv.APIKey
		s.client.APIKey = adv.APIKey
		return fmt.Sprintf("Advertiser id=%d, api_key=%s", adv.ID, adv.APIKey), nil
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
