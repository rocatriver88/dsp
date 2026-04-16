//go:build e2e
// +build e2e

package handler_test

import (
	"net/http"
	"testing"

	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/handler"
)

// TestHandleCircuitStatus_UsesStandardCBSemantics verifies the V5.2A fix
// that circuit-breaker status strings follow the industry-standard convention:
//
//   "closed" = normal operation (breaker closed → circuit connected → traffic flowing)
//   "open"   = tripped (breaker open → circuit broken → traffic blocked)
//
// Previously the handler emitted "open" for normal and "tripped" for tripped,
// reversing the standard CB lexicon.
func TestHandleCircuitStatus_UsesStandardCBSemantics(t *testing.T) {
	d := mustDeps(t)
	if d.Redis == nil {
		t.Skip("redis not available")
	}

	// Wire up a real guardrail so the handler doesn't 503.
	d.Guardrail = guardrail.New(d.Redis, guardrail.Config{})

	// Clean up any stale circuit-breaker state from previous test runs.
	d.Guardrail.CB.Reset(t.Context())

	mux := handler.BuildAdminMux(d)

	// 1. Initially the circuit breaker should be closed (normal operation).
	{
		req := adminReq(t, http.MethodGet, "/api/v1/admin/circuit-status", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-status: want 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			CircuitBreaker string `json:"circuit_breaker"`
			Reason         string `json:"reason"`
		}
		decodeJSON(t, w, &resp)
		if resp.CircuitBreaker != "closed" {
			t.Fatalf("normal state: want circuit_breaker=%q, got %q", "closed", resp.CircuitBreaker)
		}
	}

	// 2. Trip the circuit breaker via the handler.
	{
		req := adminReq(t, http.MethodPost, "/api/v1/admin/circuit-break",
			map[string]string{"reason": "v5.2a test trip"})
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-break: want 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		decodeJSON(t, w, &resp)
		if resp.Status != "open" {
			t.Fatalf("trip response: want status=%q (breaker open), got %q", "open", resp.Status)
		}
	}

	// 3. After trip, circuit-status should report "open" (tripped).
	{
		req := adminReq(t, http.MethodGet, "/api/v1/admin/circuit-status", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-status: want 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			CircuitBreaker string `json:"circuit_breaker"`
			Reason         string `json:"reason"`
		}
		decodeJSON(t, w, &resp)
		if resp.CircuitBreaker != "open" {
			t.Fatalf("tripped state: want circuit_breaker=%q (open=tripped), got %q", "open", resp.CircuitBreaker)
		}
		if resp.Reason != "v5.2a test trip" {
			t.Fatalf("trip reason: want %q, got %q", "v5.2a test trip", resp.Reason)
		}
	}

	// 4. Reset and verify status returns to "closed".
	{
		req := adminReq(t, http.MethodPost, "/api/v1/admin/circuit-reset", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-reset: want 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			Status string `json:"status"`
		}
		decodeJSON(t, w, &resp)
		if resp.Status != "closed" {
			t.Fatalf("reset response: want status=%q (breaker closed), got %q", "closed", resp.Status)
		}
	}

	// 5. After reset, circuit-status should report "closed" again.
	{
		req := adminReq(t, http.MethodGet, "/api/v1/admin/circuit-status", nil)
		w := execAdmin(t, d, req)
		if w.Code != http.StatusOK {
			t.Fatalf("circuit-status: want 200, got %d body=%s", w.Code, w.Body.String())
		}
		var resp struct {
			CircuitBreaker string `json:"circuit_breaker"`
		}
		decodeJSON(t, w, &resp)
		if resp.CircuitBreaker != "closed" {
			t.Fatalf("after reset: want circuit_breaker=%q, got %q", "closed", resp.CircuitBreaker)
		}
	}

	_ = mux // mux built but we use execAdmin which builds its own
}
