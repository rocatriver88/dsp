package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/heartgryphon/dsp/internal/audit"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/registration"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/redis/go-redis/v9"
)

// Deps holds all dependencies injected into handlers.
type Deps struct {
	Store       *campaign.Store
	ReportStore *reporting.Store // nil if ClickHouse unavailable
	BillingSvc  *billing.Service
	RegSvc      *registration.Service
	BudgetSvc   *budget.Service      // nil if Redis unavailable
	Redis       *redis.Client        // nil if Redis unavailable
	Guardrail   *guardrail.Guardrail // nil if guardrails disabled
	AuditLog    *audit.Logger        // nil if audit disabled
	HMACSecret  []byte               // signs analytics SSE tokens (V5.1 P1-1)
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

func ParseDateRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -7)
	to := now

	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := r.URL.Query().Get("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed
		}
	}
	return from, to
}

func GenerateAPIKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "dsp_" + hex.EncodeToString(b)
}
