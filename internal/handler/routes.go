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
	// V5.1 P1-2: POST /api/v1/advertisers is NOT registered here. It
	// used to be a dev-bootstrap handler that let any authenticated
	// tenant create a new advertiser with a client-settable
	// balance_cents — a privilege-escalation path where tenant A
	// could POST /advertisers with balance_cents=100_000_000 and
	// receive a fresh api_key for a new advertiser, then use that
	// key to spend ADX dollars. The handler now lives on the admin
	// mux at POST /api/v1/admin/advertisers (see BuildAdminMux).
	// The legitimate tenant bootstrap path is POST /api/v1/register
	// → admin approval → api_key delivery.
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
	// Analytics SSE streaming routes (/analytics/stream, /analytics/snapshot)
	// are NOT registered here. They live in BuildAnalyticsSSEMux, behind
	// SSETokenMiddleware, because EventSource cannot send custom headers and
	// putting the long-lived X-API-Key in URL query leaks tenant credentials
	// into proxy logs / browser history / referrer headers (V5.1 P1-1).
	// The token-issue endpoint below stays in publicMux so it gets the
	// normal APIKeyMiddleware treatment.
	// DO NOT add /analytics/stream or /analytics/snapshot to this mux —
	// they live on BuildAnalyticsSSEMux behind SSETokenMiddleware (V5.1 P1-1).
	mux.HandleFunc("POST /api/v1/analytics/token", d.HandleAnalyticsToken)
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
	// V5.1 P1-2: relocated from the public mux. See BuildPublicMux for
	// the rationale. Admin token required.
	mux.HandleFunc("POST /api/v1/admin/advertisers", d.HandleCreateAdvertiser)
	mux.HandleFunc("POST /api/v1/admin/topup", d.HandleAdminTopUp)
	mux.HandleFunc("POST /api/v1/admin/invite-codes", d.HandleCreateInviteCode)
	mux.HandleFunc("GET /api/v1/admin/invite-codes", d.HandleListInviteCodes)
	mux.HandleFunc("GET /api/v1/admin/audit-log", d.HandleAuditLog)
	return mux
}

// BuildAnalyticsSSEMux returns a dedicated mux for analytics SSE endpoints.
// These routes authenticate via short-lived HMAC tokens (?token=) rather
// than X-API-Key, because EventSource cannot set custom headers and
// putting the long-lived tenant key in URL query leaks credentials into
// proxy logs, browser history, and referrer headers (V5.1 P1-1).
//
// Clients mint a token via POST /api/v1/analytics/token (authenticated
// via X-API-Key header, routed through BuildPublicMux + APIKeyMiddleware)
// and then use the returned token in ?token= query of the stream /
// snapshot endpoints below.
func BuildAnalyticsSSEMux(d *Deps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/analytics/stream", d.HandleAnalyticsStream)
	mux.HandleFunc("GET /api/v1/analytics/snapshot", d.HandleAnalyticsSnapshot)
	return mux
}

// BuildPublicHandler returns the full production public handler chain:
// CORS -> RequestID -> Logging -> dispatcher { analytics SSE -> SSETokenMiddleware,
//   everything else -> AuthExemption(RateLimit(APIKey(publicMux))) }.
//
// Analytics SSE endpoints (/analytics/stream, /analytics/snapshot) use HMAC
// token auth instead of X-API-Key (V5.1 P1-1). Token minting lives at
// POST /api/v1/analytics/token inside publicMux, so it still flows through
// APIKeyMiddleware. The dispatcher uses exact-path matching so /token is NOT
// accidentally routed through SSETokenMiddleware.
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

	// Analytics SSE sub-chain: SSETokenMiddleware reads ?token= and injects
	// the advertiser into context using the same advertiserKey as
	// APIKeyMiddleware, so HandleAnalyticsStream / HandleAnalyticsSnapshot
	// work unchanged. Rate limit runs BEFORE SSE token validation so
	// unauthenticated attackers cannot cheaply burn HMAC-verification CPU
	// by spraying garbage tokens; the per-IP fallback in APIKeyFunc means
	// missing X-API-Key (always the case for EventSource) buckets by
	// RemoteAddr. Per-advertiser SSE rate limiting is Phase 2C debt.
	analyticsSSEMux := BuildAnalyticsSSEMux(d)
	analyticsSSEAuth := auth.SSETokenMiddleware(d.SSETokenSecret)(analyticsSSEMux)
	analyticsSSE := ratelimit.Middleware(limiter, ratelimit.APIKeyFunc, 100, time.Minute)(analyticsSSEAuth)

	dispatcher := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/api/v1/analytics/stream" || p == "/api/v1/analytics/snapshot" {
			analyticsSSE.ServeHTTP(w, r)
			return
		}
		withExemption.ServeHTTP(w, r)
	})

	return WithCORS(cfg, observability.RequestIDMiddleware(observability.LoggingMiddleware(dispatcher)))
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
