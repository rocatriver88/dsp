// cmd/autopilot/main.go
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: autopilot <verify|run>")
		fmt.Println("")
		fmt.Println("  verify  — Run full E2E verification, produce HTML report")
		fmt.Println("  run     — Start continuous simulation mode (24/7)")
		os.Exit(1)
	}

	cfg := LoadAutopilotConfig()

	switch os.Args[1] {
	case "verify":
		runVerify(cfg)
	case "run":
		runContinuous(cfg)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func newAlertSender(cfg *AutopilotConfig) alert.Sender {
	if cfg.WebhookURL == "" {
		return alert.Noop{}
	}
	switch cfg.WebhookType {
	case "feishu":
		return alert.NewFeishu(cfg.WebhookURL)
	default:
		return alert.NewDingTalk(cfg.WebhookURL)
	}
}

func runVerify(cfg *AutopilotConfig) {
	log.Println("=== DSP Autopilot: Verify Mode ===")
	log.Println("")

	alerter := newAlertSender(cfg)
	client := NewDSPClient(cfg.APIURL, "")
	client.AdminToken = cfg.AdminToken

	// Pre-flight: check services are running
	log.Println("[PRE-FLIGHT] Checking services...")
	services := map[string]string{
		"API":          cfg.APIURL,
		"Exchange-Sim": cfg.ExchangeSimURL,
	}
	for name, url := range services {
		if err := client.HealthCheck(url); err != nil {
			log.Fatalf("[PRE-FLIGHT] %s at %s is not reachable: %v", name, url, err)
		}
		log.Printf("[PRE-FLIGHT] %s OK", name)
	}

	// Start browser
	var browser *Browser
	browser = NewBrowser(cfg.FrontendURL, "", cfg.ScreenshotDir)
	if err := browser.Start(); err != nil {
		log.Printf("[WARN] Browser not available, screenshots disabled: %v", err)
		browser = nil
	} else {
		defer browser.Stop()
	}

	// Build scenario runner
	runner := &ScenarioRunner{
		client:         client,
		exchangeSimURL: cfg.ExchangeSimURL,
		bidderURL:      cfg.BidderURL,
		browser:        browser,
		grafanaURL:     cfg.GrafanaURL,
		trafficWait:    cfg.TrafficDuration,
	}

	report := &VerifyReport{
		StartTime: time.Now(),
	}

	// Run normal flow (steps 1-6)
	log.Println("")
	log.Println("=== Normal Flow ===")
	normalSteps := runner.RunNormalFlow()
	report.Steps = append(report.Steps, normalSteps...)

	// Update browser with the new API key
	if browser != nil && runner.apiKey != "" {
		browser.apiKey = runner.apiKey
	}

	// Run fault scenarios (steps 7-9)
	log.Println("")
	log.Println("=== Fault Injection ===")
	faultInjector, err := NewFaultInjector()
	if err != nil {
		log.Printf("[WARN] Docker not available, fault injection will be skipped: %v", err)
	} else {
		defer faultInjector.Close()
	}
	faultSteps := runner.RunFaultScenarios(faultInjector)
	report.Steps = append(report.Steps, faultSteps...)

	// Generate report
	report.EndTime = time.Now()
	reportFile := filepath.Join(cfg.ReportDir,
		fmt.Sprintf("verify-%s.html", time.Now().Format("2006-01-02-150405")))

	if err := GenerateHTMLReport(report, reportFile); err != nil {
		log.Fatalf("Failed to generate report: %v", err)
	}

	passed, failed := report.Summary()
	log.Println("")
	log.Println("=== Verify Complete ===")
	log.Printf("Results: %d passed, %d failed", passed, failed)
	log.Printf("Report: %s", reportFile)

	// Send alert summary
	alerter.Send("Autopilot Verify Complete",
		fmt.Sprintf("Passed: %d / Failed: %d\nReport: %s", passed, failed, reportFile))

	if failed > 0 {
		os.Exit(1)
	}
}
