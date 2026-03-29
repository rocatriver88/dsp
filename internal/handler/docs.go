package handler

import (
	"net/http"
)

// HandleAPIDocs returns the API documentation as a JSON schema.
func (d *Deps) HandleAPIDocs(w http.ResponseWriter, r *http.Request) {
	docs := map[string]any{
		"openapi": "3.0.0",
		"info": map[string]any{
			"title":       "DSP API",
			"version":     "1.0.0",
			"description": "Demand-Side Platform API for campaign management, bidding, and reporting.",
		},
		"servers": []map[string]string{
			{"url": "http://localhost:8181", "description": "Development"},
		},
		"security": []map[string]any{
			{"ApiKeyAuth": []string{}},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"ApiKeyAuth": map[string]string{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
		},
		"paths": map[string]any{
			"/api/v1/advertisers": map[string]any{
				"post": endpoint("Create Advertiser", "Create a new advertiser account. Returns API key."),
			},
			"/api/v1/campaigns": map[string]any{
				"get":  endpoint("List Campaigns", "List all campaigns for the authenticated advertiser."),
				"post": endpoint("Create Campaign", "Create a new campaign in draft status. Requires name, billing_model, budget, and bid price."),
			},
			"/api/v1/campaigns/{id}": map[string]any{
				"get": endpoint("Get Campaign", "Get campaign details by ID."),
				"put": endpoint("Update Campaign", "Update campaign name, bid, budget, or targeting."),
			},
			"/api/v1/campaigns/{id}/start": map[string]any{
				"post": endpoint("Start Campaign", "Transition campaign from draft/paused to active. Requires at least one creative."),
			},
			"/api/v1/campaigns/{id}/pause": map[string]any{
				"post": endpoint("Pause Campaign", "Pause an active campaign. Stops bidding immediately."),
			},
			"/api/v1/creatives": map[string]any{
				"post": endpoint("Create Creative", "Upload a creative (ad markup). Supports banner, native, splash, interstitial."),
			},
			"/api/v1/billing/topup": map[string]any{
				"post": endpoint("Top Up", "Add funds to advertiser balance. Amount in cents."),
			},
			"/api/v1/billing/transactions": map[string]any{
				"get": endpoint("List Transactions", "Transaction history with pagination. Query params: advertiser_id, limit, offset."),
			},
			"/api/v1/billing/balance/{id}": map[string]any{
				"get": endpoint("Get Balance", "Current balance and billing type for an advertiser."),
			},
			"/api/v1/reports/campaign/{id}/stats": map[string]any{
				"get": endpoint("Campaign Stats", "Aggregated stats: impressions, clicks, conversions, CTR, CVR, spend. Query params: from, to (YYYY-MM-DD)."),
			},
			"/api/v1/reports/campaign/{id}/hourly": map[string]any{
				"get": endpoint("Hourly Stats", "Per-hour breakdown. Query param: date (YYYY-MM-DD)."),
			},
			"/api/v1/reports/campaign/{id}/geo": map[string]any{
				"get": endpoint("Geo Breakdown", "Stats by country. Query params: from, to."),
			},
			"/api/v1/reports/campaign/{id}/bids": map[string]any{
				"get": endpoint("Bid Transparency", "Individual bid records with prices. Query params: from, to, limit, offset."),
			},
			"/api/v1/reports/campaign/{id}/attribution": map[string]any{
				"get": endpoint("Conversion Attribution", "Multi-touch attribution paths. Query params: from, to, model (last_click/first_click/linear), limit."),
			},
			"/api/v1/reports/overview": map[string]any{
				"get": endpoint("Overview", "Today's totals: spend, impressions, clicks across all campaigns."),
			},
			"/api/v1/analytics/stream": map[string]any{
				"get": endpoint("Analytics Stream (SSE)", "Real-time campaign stats via Server-Sent Events. Updates every 5 seconds."),
			},
			"/api/v1/analytics/snapshot": map[string]any{
				"get": endpoint("Analytics Snapshot", "Single-request version of the analytics stream."),
			},
			"/api/v1/register": map[string]any{
				"post": endpoint("Self-Register", "Submit advertiser registration request. No API key required. Rate limited."),
			},
			"/api/v1/ad-types": map[string]any{
				"get": endpoint("List Ad Types", "Available ad formats: banner, native, splash, interstitial."),
			},
			"/api/v1/billing-models": map[string]any{
				"get": endpoint("List Billing Models", "Available billing models: CPM, CPC, oCPM."),
			},
		},
	}

	WriteJSON(w, http.StatusOK, docs)
}

func endpoint(summary, description string) map[string]string {
	return map[string]string{
		"summary":     summary,
		"description": description,
	}
}
