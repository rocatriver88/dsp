package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

func (d *Deps) HandleCircuitBreak(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	var req struct {
		Reason string `json:"reason"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	if req.Reason == "" {
		req.Reason = "manual: admin triggered at " + time.Now().Format(time.RFC3339)
	}

	d.Guardrail.CB.Trip(r.Context(), req.Reason)
	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "tripped",
		"reason": req.Reason,
	})
}

func (d *Deps) HandleCircuitReset(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	d.Guardrail.CB.Reset(r.Context())
	WriteJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (d *Deps) HandleCircuitStatus(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	ctx := r.Context()
	open := d.Guardrail.CB.IsOpen(ctx)
	reason := d.Guardrail.CB.TripReason(ctx)
	globalSpend := d.Guardrail.GetGlobalSpend(ctx)

	status := "open"
	if !open {
		status = "tripped"
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"circuit_breaker":          status,
		"reason":                   reason,
		"global_spend_today_cents": globalSpend,
	})
}
