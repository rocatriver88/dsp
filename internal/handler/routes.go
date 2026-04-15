package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/observability"
	"github.com/heartgryphon/dsp/internal/ratelimit"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// BuildPublicMux returns the bare public router (no middleware).
// Use in tests that want to bypass auth/ratelimit.
func BuildPublicMux(d *Deps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/advertisers", d.HandleCreateAdvertiser)
	mux.HandleFunc("GET /api/v1/advertisers/{id}", d.HandleGetAdvertiser)
	mux.HandleFunc("POST /api/v1/campaigns", d.HandleCreateCampaign)
	mux.HandleFunc("GET /api/v1/campaigns", d.HandleListCampaigns)
	mux.HandleFunc("GET /api/v1/campaigns/{id}", d.HandleGetCampaign)
	mux.HandleFunc("PUT /api/v1/campaigns/{id}", d.HandleUpdateCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/start", d.HandleStartCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/pause", d.HandlePauseCampaign)
	mux.HandleFunc("GET /api/v1/campaigns/{id}/creatives", d.HandleListCreatives)
	mux.HandleFunc("POST /api/v1/creatives", d.HandleCreateCreative)
	mux.HandleFunc("PUT /api/v1/creatives/{id}", d.HandleUpdateCreative)
	mux.HandleFunc("DELETE /api/v1/creatives/{id}", d.HandleDeleteCreative)
	mux.HandleFunc("GET /api/v1/ad-types", d.HandleAdTypes)
	mux.HandleFunc("GET /api/v1/billing-models", d.HandleBillingModels)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/stats", d.HandleCampaignStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/hourly", d.HandleHourlyStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/geo", d.HandleGeoBreakdown)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/bids", d.HandleBidTransparency)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/attribution", d.HandleAttribution)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/simulate", d.HandleBidSimulate)
	mux.HandleFunc("GET /api/v1/reports/overview", d.HandleOverviewStats)
	mux.HandleFunc("GET /api/v1/export/campaign/{id}/stats", d.HandleExportCampaignCSV)
	mux.HandleFunc("GET /api/v1/export/campaign/{id}/bids", d.HandleExportBidsCSV)
	mux.HandleFunc("GET /api/v1/audit-log", d.HandleMyAuditLog)
	mux.HandleFunc("GET /api/v1/analytics/stream", d.HandleAnalyticsStream)
	mux.HandleFunc("GET /api/v1/analytics/snapshot", d.HandleAnalyticsSnapshot)
	mux.HandleFunc("POST /api/v1/billing/topup", d.HandleTopUp)
	mux.HandleFunc("GET /api/v1/billing/transactions", d.HandleTransactions)
	mux.HandleFunc("GET /api/v1/billing/balance", d.HandleBalance)
	// Legacy alias kept for backward compatibility with clients (notably
	// cmd/autopilot and the Batch 6 tenant isolation test suite) that still
	// send GET /billing/balance/{id}. Routed through the dedicated stub
	// HandleBalanceLegacyByID so swag emits this path as a distinct
	// @Deprecated entry in the generated OpenAPI contract. The stub
	// delegates to HandleBalance, which enforces pathID == authID → else
	// 404 per V5 §P0 (Round 1 review reverted commit 4faa8c9's permissive
	// "silently ignore path id" shortcut).
	mux.HandleFunc("GET /api/v1/billing/balance/{id}", d.HandleBalanceLegacyByID)
	mux.HandleFunc("POST /api/v1/upload", d.HandleUpload)
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", UploadFileServer()))
	mux.HandleFunc("POST /api/v1/register", d.HandleRegister)
	mux.HandleFunc("GET /api/v1/docs", d.HandleAPIDocs)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})
	return mux
}

// BuildAdminMux returns the bare admin router (no auth middleware).
// Use in tests that want to bypass the X-Admin-Token check.
func BuildAdminMux(d *Deps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/active-campaigns", d.HandleActiveCampaigns)
	mux.HandleFunc("GET /api/v1/admin/registrations", d.HandleListRegistrations)
	mux.HandleFunc("POST /api/v1/admin/registrations/{id}/approve", d.HandleApproveRegistration)
	mux.HandleFunc("POST /api/v1/admin/registrations/{id}/reject", d.HandleRejectRegistration)
	mux.HandleFunc("GET /api/v1/admin/health", d.HandleSystemHealth)
	mux.HandleFunc("GET /api/v1/admin/creatives", d.HandleListCreativesForReview)
	mux.HandleFunc("POST /api/v1/admin/creatives/{id}/approve", d.HandleApproveCreative)
	mux.HandleFunc("POST /api/v1/admin/creatives/{id}/reject", d.HandleRejectCreative)
	mux.HandleFunc("POST /api/v1/admin/circuit-break", d.HandleCircuitBreak)
	mux.HandleFunc("POST /api/v1/admin/circuit-reset", d.HandleCircuitReset)
	mux.HandleFunc("GET /api/v1/admin/circuit-status", d.HandleCircuitStatus)
	mux.HandleFunc("GET /api/v1/admin/advertisers", d.HandleListAdvertisers)
	mux.HandleFunc("POST /api/v1/admin/topup", d.HandleAdminTopUp)
	mux.HandleFunc("POST /api/v1/admin/invite-codes", d.HandleCreateInviteCode)
	mux.HandleFunc("GET /api/v1/admin/invite-codes", d.HandleListInviteCodes)
	mux.HandleFunc("GET /api/v1/admin/audit-log", d.HandleAuditLog)
	return mux
}

// BuildPublicHandler returns the full production public handler chain:
// CORS -> RequestID -> Logging -> AuthExemption(RateLimit(APIKey(publicMux))).
// This preserves the exact middleware order from cmd/api/main.go prior to
// the biz routes.go refactor.
func BuildPublicHandler(cfg *config.Config, d *Deps) http.Handler {
	publicMux := BuildPublicMux(d)
	apiKeyLookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := d.Store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}
	limiter := ratelimit.New(d.Redis)
	authed := auth.APIKeyMiddleware(apiKeyLookup)(publicMux)
	rateLimited := ratelimit.Middleware(limiter, ratelimit.APIKeyFunc, 100, time.Minute)(authed)
	withExemption := WithAuthExemption(rateLimited, publicMux)
	return WithCORS(cfg, observability.RequestIDMiddleware(observability.LoggingMiddleware(withExemption)))
}

// BuildInternalHandler returns the full internal (admin) handler chain:
// CORS -> Logging -> {admin auth for /internal/ and /api/v1/admin/} + /metrics + /health.
func BuildInternalHandler(cfg *config.Config, d *Deps) http.Handler {
	adminMux := BuildAdminMux(d)
	internalMux := http.NewServeMux()
	internalMux.Handle("GET /metrics", promhttp.Handler())
	internalMux.Handle("/internal/", AdminAuthMiddleware(adminMux))
	internalMux.Handle("/api/v1/admin/", AdminAuthMiddleware(adminMux))
	internalMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"status":"ok","port":"internal","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})
	return WithCORS(cfg, observability.LoggingMiddleware(internalMux))
}
