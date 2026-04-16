package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

// CircuitStatusResponse is the JSON shape returned by GET /admin/circuit-status.
type CircuitStatusResponse struct {
	CircuitBreaker       string `json:"circuit_breaker"`
	Reason               string `json:"reason"`
	GlobalSpendTodayCents int64 `json:"global_spend_today_cents"`
}

// HandleCircuitBreak godoc
// @Summary Trip circuit breaker
// @Description Opens the circuit breaker (standard CB terminology: open = tripped,
// @Description traffic blocked). All bidding stops until the breaker is reset.
// @Tags admin
// @Security AdminAuth
// @Accept json
// @Produce json
// @Param body body object{reason=string} true "Trip reason"
// @Success 200 {object} object{status=string,reason=string}
// @Router /admin/circuit-break [post]
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
		"status": "open", // V5.2A: standard CB — "open" means breaker is open (tripped)
		"reason": req.Reason,
	})
}

// HandleCircuitReset godoc
// @Summary Reset circuit breaker
// @Description Closes the circuit breaker (standard CB terminology: closed = normal,
// @Description traffic flowing). Bidding resumes.
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {object} object{status=string}
// @Router /admin/circuit-reset [post]
func (d *Deps) HandleCircuitReset(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	d.Guardrail.CB.Reset(r.Context())
	WriteJSON(w, http.StatusOK, map[string]string{"status": "closed"}) // V5.2A: standard CB — "closed" means breaker is closed (normal)
}

// HandleCircuitStatus godoc
// @Summary Get circuit breaker status
// @Description Returns the current circuit breaker state using standard CB terminology:
// @Description "closed" = normal operation (breaker closed, circuit connected, traffic flowing).
// @Description "open" = tripped (breaker open, circuit broken, fail-fast, traffic blocked).
// @Tags admin
// @Security AdminAuth
// @Produce json
// @Success 200 {object} handler.CircuitStatusResponse
// @Router /admin/circuit-status [get]
func (d *Deps) HandleCircuitStatus(w http.ResponseWriter, r *http.Request) {
	if d.Guardrail == nil {
		WriteError(w, http.StatusServiceUnavailable, "guardrails not configured")
		return
	}

	ctx := r.Context()
	biddingAllowed := d.Guardrail.CB.IsOpen(ctx)
	reason := d.Guardrail.CB.TripReason(ctx)
	globalSpend := d.Guardrail.GetGlobalSpend(ctx)

	// V5.2A: Standard circuit-breaker terminology.
	// CB.IsOpen() returns true when bidding is allowed (the internal naming
	// predates this fix). In standard CB lexicon:
	//   "closed" = breaker closed → circuit connected → traffic flowing (normal)
	//   "open"   = breaker open → circuit broken → traffic blocked (tripped)
	status := "closed"
	if !biddingAllowed {
		status = "open"
	}

	WriteJSON(w, http.StatusOK, CircuitStatusResponse{
		CircuitBreaker:        status,
		Reason:                reason,
		GlobalSpendTodayCents: globalSpend,
	})
}
