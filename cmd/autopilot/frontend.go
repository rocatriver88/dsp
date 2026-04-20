package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// FrontendManager ensures a local frontend is reachable for browser evidence.
// If the configured URL is already healthy it is left alone; otherwise a local
// Next.js dev server is started from ./web and stopped when the manager stops.
type FrontendManager struct {
	frontendURL string
	apiURL      string
	logDir      string
	cmd         *exec.Cmd
	started     bool
}

func NewFrontendManager(frontendURL, apiURL, logDir string) *FrontendManager {
	return &FrontendManager{
		frontendURL: frontendURL,
		apiURL:      apiURL,
		logDir:      logDir,
	}
}

func (m *FrontendManager) EnsureRunning() error {
	if m.frontendURL == "" {
		return nil
	}
	if err := waitForHTTP200(m.frontendURL, 3*time.Second); err == nil {
		return nil
	}

	u, err := url.Parse(m.frontendURL)
	if err != nil {
		return fmt.Errorf("parse frontend url: %w", err)
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	if err := os.MkdirAll(m.logDir, 0o755); err != nil {
		return fmt.Errorf("create frontend log dir: %w", err)
	}

	webDir := filepath.Join(".", "web")
	if _, err := os.Stat(webDir); err != nil {
		return fmt.Errorf("frontend not reachable and web dir missing: %w", err)
	}

	npx := "npx"
	if runtime.GOOS == "windows" {
		npx = "npx.cmd"
	}
	cmd := exec.Command(npx, "next", "dev", "-p", port)
	cmd.Dir = webDir
	cmd.Env = append(os.Environ(),
		"NEXT_PUBLIC_API_URL="+m.apiURL,
		"PORT="+port,
	)

	stdout, err := os.Create(filepath.Join(m.logDir, "frontend.out.log"))
	if err != nil {
		return fmt.Errorf("create frontend stdout log: %w", err)
	}
	stderr, err := os.Create(filepath.Join(m.logDir, "frontend.err.log"))
	if err != nil {
		_ = stdout.Close()
		return fmt.Errorf("create frontend stderr log: %w", err)
	}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("start frontend: %w", err)
	}
	m.cmd = cmd
	m.started = true

	if err := waitForHTTP200(m.frontendURL, 60*time.Second); err != nil {
		_ = m.Stop()
		return fmt.Errorf("frontend did not become healthy: %w", err)
	}
	return nil
}

func (m *FrontendManager) Stop() error {
	if !m.started || m.cmd == nil || m.cmd.Process == nil {
		return nil
	}
	_ = m.cmd.Process.Kill()
	_, _ = m.cmd.Process.Wait()
	m.started = false
	return nil
}

func waitForHTTP200(target string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{Timeout: 3 * time.Second}
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 400 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
