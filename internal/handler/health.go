package handler

import (
	"context"
	"net/http"
	"time"
)

// HealthCheckResult is the JSON response for /health/ready.
type HealthCheckResult struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// HandleHealthLive returns 200 as long as the process is running.
// No backend probes — this is the Kubernetes liveness probe target.
func (d *Deps) HandleHealthLive(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleHealthReady probes Postgres, Redis, and ClickHouse. Returns 200 if
// all backends are reachable, 503 with per-backend detail if any is down.
// This is the Kubernetes readiness probe target.
func (d *Deps) HandleHealthReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	checks := make(map[string]string, 3)
	allOK := true

	// Postgres
	if d.Store != nil {
		if err := d.Store.Ping(ctx); err != nil {
			checks["postgres"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["postgres"] = "ok"
		}
	} else {
		checks["postgres"] = "error: not configured"
		allOK = false
	}

	// Redis
	if d.Redis != nil {
		if err := d.Redis.Ping(ctx).Err(); err != nil {
			checks["redis"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["redis"] = "ok"
		}
	} else {
		checks["redis"] = "error: not configured"
		allOK = false
	}

	// ClickHouse
	if d.ReportStore != nil {
		if err := d.ReportStore.Ping(ctx); err != nil {
			checks["clickhouse"] = "error: " + err.Error()
			allOK = false
		} else {
			checks["clickhouse"] = "ok"
		}
	} else {
		checks["clickhouse"] = "error: not configured"
		allOK = false
	}

	result := HealthCheckResult{
		Checks: checks,
	}
	if allOK {
		result.Status = "ready"
		WriteJSON(w, http.StatusOK, result)
	} else {
		result.Status = "not_ready"
		WriteJSON(w, http.StatusServiceUnavailable, result)
	}
}
