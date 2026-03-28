package campaign

import (
	"encoding/json"
	"errors"
	"time"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusActive    Status = "active"
	StatusPaused    Status = "paused"
	StatusCompleted Status = "completed"
	StatusDeleted   Status = "deleted"
)

// Campaign state machine transitions:
//
//   draft ──→ active ──→ paused ──→ active
//     │         │           │
//     │         └──→ completed ←──┘
//     └──→ deleted
//
var validTransitions = map[Status][]Status{
	StatusDraft:  {StatusActive, StatusDeleted},
	StatusActive: {StatusPaused, StatusCompleted},
	StatusPaused: {StatusActive, StatusCompleted},
}

func ValidateTransition(from, to Status) error {
	allowed, ok := validTransitions[from]
	if !ok {
		return errors.New("no transitions from " + string(from))
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return errors.New("invalid transition: " + string(from) + " → " + string(to))
}

type Targeting struct {
	Geo          []string   `json:"geo,omitempty"`
	Device       []string   `json:"device,omitempty"`
	OS           []string   `json:"os,omitempty"`
	Browser      []string   `json:"browser,omitempty"`
	TimeSchedule []Schedule `json:"time_schedule,omitempty"`
	FrequencyCap *FreqCap   `json:"frequency_cap,omitempty"`
}

type Schedule struct {
	Day   int   `json:"day"`   // 0=Sun, 1=Mon, ...
	Hours []int `json:"hours"` // 0-23
}

type FreqCap struct {
	Count       int `json:"count"`
	PeriodHours int `json:"period_hours"`
}

type Campaign struct {
	ID               int64           `json:"id" db:"id"`
	AdvertiserID     int64           `json:"advertiser_id" db:"advertiser_id"`
	Name             string          `json:"name" db:"name"`
	Status           Status          `json:"status" db:"status"`
	BudgetTotalCents int64           `json:"budget_total_cents" db:"budget_total_cents"`
	BudgetDailyCents int64           `json:"budget_daily_cents" db:"budget_daily_cents"`
	SpentCents       int64           `json:"spent_cents" db:"spent_cents"`
	BidCPMCents      int             `json:"bid_cpm_cents" db:"bid_cpm_cents"`
	StartDate        *time.Time      `json:"start_date,omitempty" db:"start_date"`
	EndDate          *time.Time      `json:"end_date,omitempty" db:"end_date"`
	Targeting        json.RawMessage `json:"targeting" db:"targeting"`
	CreatedAt        time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at" db:"updated_at"`
}

type Creative struct {
	ID             int64     `json:"id" db:"id"`
	CampaignID     int64     `json:"campaign_id" db:"campaign_id"`
	Name           string    `json:"name" db:"name"`
	Format         string    `json:"format" db:"format"`
	Size           string    `json:"size" db:"size"`
	AdMarkup       string    `json:"ad_markup" db:"ad_markup"`
	DestinationURL string    `json:"destination_url" db:"destination_url"`
	Status         string    `json:"status" db:"status"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type Advertiser struct {
	ID           int64     `json:"id" db:"id"`
	CompanyName  string    `json:"company_name" db:"company_name"`
	ContactEmail string    `json:"contact_email" db:"contact_email"`
	APIKey       string    `json:"api_key" db:"api_key"`
	BalanceCents int64     `json:"balance_cents" db:"balance_cents"`
	BillingType  string    `json:"billing_type" db:"billing_type"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}
