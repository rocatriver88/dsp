package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealth_Live_AlwaysOK verifies that /health/live always returns 200
// regardless of backend state. Since liveness only confirms the process is
// running, it works even with completely nil deps.
func TestHealth_Live_AlwaysOK(t *testing.T) {
	d := &Deps{} // all nil — liveness must not probe anything

	req := httptest.NewRequest("GET", "/health/live", nil)
	w := httptest.NewRecorder()
	d.HandleHealthLive(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
	if body["time"] == "" {
		t.Error("expected non-empty time field")
	}
}

// TestHealth_BackwardCompat verifies that GET /health is registered as an
// alias for /health/live (backward compatibility with existing dashboards).
func TestHealth_BackwardCompat(t *testing.T) {
	d := &Deps{} // all nil — /health is liveness, no backend probes

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.HandleHealthLive)
	mux.HandleFunc("GET /health/live", d.HandleHealthLive)
	mux.HandleFunc("GET /health/ready", d.HandleHealthReady)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /health, got %d", w.Code)
	}
}

// TestHealth_Ready_NilDeps verifies that /health/ready returns 503 with
// meaningful detail when backends are not configured (nil deps).
func TestHealth_Ready_NilDeps(t *testing.T) {
	d := &Deps{} // all nil — every backend should report "not configured"

	req := httptest.NewRequest("GET", "/health/ready", nil)
	w := httptest.NewRecorder()
	d.HandleHealthReady(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var result HealthCheckResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.Status != "not_ready" {
		t.Errorf("expected status=not_ready, got %q", result.Status)
	}
	for _, backend := range []string{"postgres", "redis", "clickhouse"} {
		v, ok := result.Checks[backend]
		if !ok {
			t.Errorf("missing check for %s", backend)
			continue
		}
		if v == "ok" {
			t.Errorf("expected error for %s with nil deps, got ok", backend)
		}
	}
}

// TestHealth_Live_AuthExemption verifies that /health/live and /health/ready
// bypass the auth middleware (via WithAuthExemption).
func TestHealth_Live_AuthExemption(t *testing.T) {
	d := &Deps{} // nil deps — sufficient for liveness

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.HandleHealthLive)
	mux.HandleFunc("GET /health/live", d.HandleHealthLive)
	mux.HandleFunc("GET /health/ready", d.HandleHealthReady)

	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	h := WithAuthExemption(authed, mux)

	cases := []struct {
		path string
		want int
	}{
		{"/health", http.StatusOK},
		{"/health/live", http.StatusOK},
		// /health/ready with nil deps returns 503, but that proves it
		// bypassed auth (401 would mean auth blocked it).
		{"/health/ready", http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tc.want {
				t.Errorf("path %s: expected %d, got %d", tc.path, tc.want, w.Code)
			}
		})
	}
}
