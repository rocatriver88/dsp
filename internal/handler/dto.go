package handler

import (
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
)

// AdvertiserResponse is the public-facing view of an advertiser.
// It never contains the API key — api_key is only disclosed once, at
// creation or admin-approval, via dedicated response shapes below.
type AdvertiserResponse struct {
	ID              int64     `json:"id"`
	CompanyName     string    `json:"company_name"`
	ContactEmail    string    `json:"contact_email"`
	BalanceCents    int64     `json:"balance_cents"`
	BillingType     string    `json:"billing_type"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	ActiveCampaigns int       `json:"active_campaigns,omitempty"`
	TotalSpentCents int64     `json:"total_spent_cents,omitempty"`
}

// NewAdvertiserResponse converts a persistence-layer Advertiser into its
// sanitized response shape. The api_key field is deliberately dropped.
func NewAdvertiserResponse(adv *campaign.Advertiser) AdvertiserResponse {
	return AdvertiserResponse{
		ID:              adv.ID,
		CompanyName:     adv.CompanyName,
		ContactEmail:    adv.ContactEmail,
		BalanceCents:    adv.BalanceCents,
		BillingType:     adv.BillingType,
		CreatedAt:       adv.CreatedAt,
		UpdatedAt:       adv.UpdatedAt,
		ActiveCampaigns: adv.ActiveCampaigns,
		TotalSpentCents: adv.TotalSpentCents,
	}
}

// NewAdvertiserResponseList converts a slice of persistence-layer
// Advertisers into sanitized response shapes. Always returns a non-nil
// slice so callers see `[]` instead of `null` when the source is empty
// — this keeps the admin list endpoint's JSON contract stable.
//
// This helper exists because Round 1's final code review caught a
// Critical: HandleListAdvertisers was returning []*campaign.Advertiser
// directly, which still carries json:"api_key". One admin GET on a
// leaked ADMIN_TOKEN would have exfiltrated every tenant's plaintext
// API key. Use this helper for every admin path that returns advertiser
// collections.
func NewAdvertiserResponseList(advs []*campaign.Advertiser) []AdvertiserResponse {
	out := make([]AdvertiserResponse, 0, len(advs))
	for _, adv := range advs {
		out = append(out, NewAdvertiserResponse(adv))
	}
	return out
}

// AdvertiserCreatedResponse is the one-time response for POST /advertisers.
// It carries the fresh api_key so the caller can persist it; no read path
// ever returns this shape.
type AdvertiserCreatedResponse struct {
	ID      int64  `json:"id"`
	APIKey  string `json:"api_key"`
	Message string `json:"message,omitempty"`
}

// RegistrationApprovedResponse is the one-time response for admin approval
// of a registration request. It returns the new advertiser id, its fresh
// api key, and the temp login credentials for the auto-seeded advertiser
// user. No read path ever returns this shape.
//
// user_email and temp_password are disclosed exactly once at approval time.
// The admin is expected to relay them to the advertiser out-of-band; the
// plaintext password is never retrievable after this response is sent.
type RegistrationApprovedResponse struct {
	AdvertiserID int64  `json:"advertiser_id"`
	APIKey       string `json:"api_key"`
	UserEmail    string `json:"user_email"`
	TempPassword string `json:"temp_password"`
	Message      string `json:"message,omitempty"`
}
