package main

import (
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"
)

//go:embed report.html
var reportTemplateFS embed.FS

type StepResult struct {
	Name       string
	Passed     bool
	Duration   time.Duration
	Detail     string
	Error      string
	Screenshot string // relative path to screenshot file
}

type VerifyReport struct {
	StartTime time.Time
	EndTime   time.Time
	Steps     []StepResult
}

func (r *VerifyReport) Summary() (passed, failed int) {
	for _, s := range r.Steps {
		if s.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}

type reportData struct {
	StartTime   time.Time
	EndTime     time.Time
	DurationStr string
	TotalSteps  int
	PassedCount int
	FailedCount int
	Steps       []StepResult
}

func GenerateHTMLReport(report *VerifyReport, outputPath string) error {
	tmplData, _ := reportTemplateFS.ReadFile("report.html")
	tmpl, err := template.New("report").Parse(string(tmplData))
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}

	passed, failed := report.Summary()
	data := reportData{
		StartTime:   report.StartTime,
		EndTime:     report.EndTime,
		DurationStr: report.EndTime.Sub(report.StartTime).Round(time.Second).String(),
		TotalSteps:  len(report.Steps),
		PassedCount: passed,
		FailedCount: failed,
		Steps:       report.Steps,
	}

	os.MkdirAll(filepath.Dir(outputPath), 0o755)
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}
