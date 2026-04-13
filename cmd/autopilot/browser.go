// cmd/autopilot/browser.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// Browser wraps chromedp for taking screenshots of the DSP frontend.
type Browser struct {
	frontendURL   string
	apiKey        string
	screenshotDir string
	allocCtx      context.Context
	allocCancel   context.CancelFunc
}

func NewBrowser(frontendURL, apiKey, screenshotDir string) *Browser {
	return &Browser{
		frontendURL:   frontendURL,
		apiKey:        apiKey,
		screenshotDir: screenshotDir,
	}
}

func (b *Browser) Start() error {
	if err := os.MkdirAll(b.screenshotDir, 0o755); err != nil {
		return fmt.Errorf("create screenshot dir: %w", err)
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.WindowSize(1440, 900),
	)
	b.allocCtx, b.allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	return nil
}

func (b *Browser) Stop() {
	if b.allocCancel != nil {
		b.allocCancel()
	}
}

// Screenshot navigates to a page, injects the API key into localStorage,
// waits for the page to load, and saves a full-page screenshot.
func (b *Browser) Screenshot(name, path string) (string, error) {
	ctx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filename := filepath.Join(b.screenshotDir, name+".png")
	var buf []byte

	url := b.frontendURL + path

	err := chromedp.Run(ctx,
		// First navigate to set localStorage
		chromedp.Navigate(b.frontendURL),
		chromedp.Evaluate(
			fmt.Sprintf(`localStorage.setItem("dsp_api_key", "%s")`, b.apiKey),
			nil,
		),
		// Navigate to target page
		chromedp.Navigate(url),
		chromedp.Sleep(2*time.Second),
		// Wait for content to render
		chromedp.WaitReady("body"),
		chromedp.Sleep(1*time.Second),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return "", fmt.Errorf("screenshot %s: %w", name, err)
	}

	if err := os.WriteFile(filename, buf, 0o644); err != nil {
		return "", fmt.Errorf("save screenshot %s: %w", name, err)
	}

	log.Printf("[SCREENSHOT] %s -> %s (%d bytes)", name, filename, len(buf))
	return filename, nil
}

// ScreenshotGrafana takes a screenshot of a Grafana dashboard.
func (b *Browser) ScreenshotGrafana(name, grafanaURL, dashboardPath string) (string, error) {
	ctx, cancel := chromedp.NewContext(b.allocCtx)
	defer cancel()

	ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	filename := filepath.Join(b.screenshotDir, name+".png")
	var buf []byte

	err := chromedp.Run(ctx,
		chromedp.Navigate(grafanaURL+dashboardPath),
		chromedp.Sleep(3*time.Second),
		chromedp.WaitReady("body"),
		chromedp.FullScreenshot(&buf, 90),
	)
	if err != nil {
		return "", fmt.Errorf("grafana screenshot %s: %w", name, err)
	}

	if err := os.WriteFile(filename, buf, 0o644); err != nil {
		return "", fmt.Errorf("save grafana screenshot %s: %w", name, err)
	}
	log.Printf("[SCREENSHOT] %s -> %s (%d bytes)", name, filename, len(buf))
	return filename, nil
}
