//go:build e2e
// +build e2e

package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/heartgryphon/dsp/internal/handler"
	"github.com/redis/go-redis/v9"
)

// TestHealth_Live_AlwaysOK_E2E exercises /health/live through the full public
// mux wiring (BuildPublicMux) to confirm it returns 200 regardless of
// backend state.
func TestHealth_Live_AlwaysOK_E2E(t *testing.T) {
	d := mustDeps(t)
	mux := handler.BuildPublicMux(d)

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
}

// TestHealth_BackwardCompat_E2E verifies /health still works as an alias for
// /health/live through the full mux.
func TestHealth_BackwardCompat_E2E(t *testing.T) {
	d := mustDeps(t)
	mux := handler.BuildPublicMux(d)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health, got %d (body: %s)", w.Code, w.Body.String())
	}
}

// TestHealth_Ready_AllHealthy_E2E verifies that /health/ready returns 200
// when all backends (Postgres, Redis, ClickHouse) are reachable.
func TestHealth_Ready_AllHealthy_E2E(t *testing.T) {
	d := mustDeps(t)
	if d.ReportStore == nil {
		t.Skip("clickhouse not available — cannot test full readiness")
	}
	mux := handler.BuildPublicMux(d)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var result handler.HealthCheckResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != "ready" {
		t.Errorf("expected status=ready, got %q", result.Status)
	}
	for _, backend := range []string{"postgres", "redis", "clickhouse"} {
		if result.Checks[backend] != "ok" {
			t.Errorf("expected %s=ok, got %q", backend, result.Checks[backend])
		}
	}
}

// TestHealth_Ready_FailsOnBackendOutage verifies that /health/ready returns
// 503 when a backend is down. We simulate this by injecting a Redis client
// pointing at a non-existent address.
func TestHealth_Ready_FailsOnBackendOutage(t *testing.T) {
	d := mustDeps(t)

	// Replace the real Redis client with one pointing at a dead address.
	// This simulates a Redis outage without stopping the real Redis.
	deadRedis := redis.NewClient(&redis.Options{
		Addr:     "localhost:1", // nothing listens here
		Password: "irrelevant",
	})
	defer deadRedis.Close()

	original := d.Redis
	d.Redis = deadRedis
	defer func() { d.Redis = original }()

	mux := handler.BuildPublicMux(d)

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d (body: %s)", w.Code, w.Body.String())
	}

	var result handler.HealthCheckResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != "not_ready" {
		t.Errorf("expected status=not_ready, got %q", result.Status)
	}
	if result.Checks["redis"] == "ok" {
		t.Error("expected redis check to report an error, got ok")
	}
	// Postgres should still be ok (we only broke Redis).
	if result.Checks["postgres"] != "ok" {
		t.Errorf("expected postgres=ok (not broken), got %q", result.Checks["postgres"])
	}
}
