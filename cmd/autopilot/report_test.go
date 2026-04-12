package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerateReport(t *testing.T) {
	steps := []StepResult{
		{
			Name:       "Create Advertiser",
			Passed:     true,
			Duration:   150 * time.Millisecond,
			Detail:     "Created advertiser id=1",
			Screenshot: "",
		},
		{
			Name:       "Top Up Balance",
			Passed:     true,
			Duration:   80 * time.Millisecond,
			Detail:     "Balance: 100000 cents",
			Screenshot: "topup.png",
		},
		{
			Name:     "Budget Exhaustion",
			Passed:   false,
			Duration: 2 * time.Second,
			Detail:   "Campaign did not auto-pause within timeout",
			Error:    "expected status paused, got active",
		},
	}

	report := &VerifyReport{
		StartTime: time.Now().Add(-5 * time.Minute),
		EndTime:   time.Now(),
		Steps:     steps,
	}

	tmpFile := os.TempDir() + "/test-report.html"
	defer os.Remove(tmpFile)

	err := GenerateHTMLReport(report, tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(tmpFile)
	html := string(data)

	if !strings.Contains(html, "Create Advertiser") {
		t.Error("report should contain step name")
	}
	if !strings.Contains(html, "PASS") {
		t.Error("report should contain PASS")
	}
	if !strings.Contains(html, "FAIL") {
		t.Error("report should contain FAIL for failed step")
	}
	if !strings.Contains(html, "topup.png") {
		t.Error("report should reference screenshot")
	}
}

func TestReportSummary(t *testing.T) {
	report := &VerifyReport{
		Steps: []StepResult{
			{Passed: true},
			{Passed: true},
			{Passed: false},
		},
	}
	passed, failed := report.Summary()
	if passed != 2 {
		t.Errorf("expected 2 passed, got %d", passed)
	}
	if failed != 1 {
		t.Errorf("expected 1 failed, got %d", failed)
	}
}
