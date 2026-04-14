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

// AdvertiserCreatedResponse is the one-time response for POST /advertisers.
// It carries the fresh api_key so the caller can persist it; no read path
// ever returns this shape.
type AdvertiserCreatedResponse struct {
	ID      int64  `json:"id"`
	APIKey  string `json:"api_key"`
	Message string `json:"message,omitempty"`
}

// RegistrationApprovedResponse is the one-time response for admin approval
// of a registration request. It returns the new advertiser id and its
// fresh api key, and nothing else — no read path ever returns this shape.
type RegistrationApprovedResponse struct {
	AdvertiserID int64  `json:"advertiser_id"`
	APIKey       string `json:"api_key"`
	Message      string `json:"message,omitempty"`
}
