package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	WriteJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "bad input")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

func TestParseeDateRange_Defaults(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	from, to := ParseDateRange(req)

	if from.IsZero() || to.IsZero() {
		t.Error("expected non-zero dates")
	}
	if !from.Before(to) {
		t.Error("from should be before to")
	}
}

func TestParseDateRange_CustomRange(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?from=2026-01-01&to=2026-01-31", nil)
	from, to := ParseDateRange(req)

	if from.Year() != 2026 || from.Month() != 1 || from.Day() != 1 {
		t.Errorf("expected 2026-01-01, got %s", from.Format("2006-01-02"))
	}
	if to.Year() != 2026 || to.Month() != 1 || to.Day() != 31 {
		t.Errorf("expected 2026-01-31, got %s", to.Format("2006-01-02"))
	}
}

func TestParseDateRange_InvalidDates(t *testing.T) {
	req := httptest.NewRequest("GET", "/test?from=invalid&to=also-invalid", nil)
	from, to := ParseDateRange(req)

	// Should fall back to defaults
	if from.IsZero() || to.IsZero() {
		t.Error("expected fallback to defaults, not zero")
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	key := GenerateAPIKey()
	if len(key) < 10 {
		t.Error("key too short")
	}
	if key[:4] != "dsp_" {
		t.Errorf("expected dsp_ prefix, got %s", key[:4])
	}
}

func TestGenerateAPIKey_Unique(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 10; i++ {
		k := GenerateAPIKey()
		if keys[k] {
			t.Fatalf("duplicate key generated: %s", k)
		}
		keys[k] = true
	}
}

func TestWithAuthExemption_Health(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	h := WithAuthExemption(authed, mux)

	// Health endpoint should bypass auth
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health should bypass auth, got %d", w.Code)
	}
}

func TestWithAuthExemption_Register(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("registered"))
	})

	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	h := WithAuthExemption(authed, mux)

	req := httptest.NewRequest("POST", "/api/v1/register", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("register should bypass auth, got %d", w.Code)
	}
}

func TestWithAuthExemption_Protected(t *testing.T) {
	mux := http.NewServeMux()

	authed := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	h := WithAuthExemption(authed, mux)

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("campaigns should require auth, got %d", w.Code)
	}
}
