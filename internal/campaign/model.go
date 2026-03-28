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

// Billing model constants
const (
	BillingCPM  = "cpm"  // 按千次曝光计费
	BillingCPC  = "cpc"  // 按点击计费
	BillingOCPM = "ocpm" // 按目标转化成本出价, 按曝光计费 (优化CPM)
)

// BillingModelConfig describes each billing model.
var BillingModelConfig = map[string]struct {
	Label       string
	ChargeOn    string // impression or click
	Description string
}{
	BillingCPM:  {Label: "CPM", ChargeOn: "impression", Description: "按千次曝光计费，适合品牌曝光"},
	BillingCPC:  {Label: "CPC", ChargeOn: "click", Description: "按点击计费，适合效果导向"},
	BillingOCPM: {Label: "oCPM", ChargeOn: "impression", Description: "按目标转化成本智能出价，按曝光计费"},
}

type Campaign struct {
	ID                 int64           `json:"id" db:"id"`
	AdvertiserID       int64           `json:"advertiser_id" db:"advertiser_id"`
	Name               string          `json:"name" db:"name"`
	Status             Status          `json:"status" db:"status"`
	BillingModel       string          `json:"billing_model" db:"billing_model"`
	BudgetTotalCents   int64           `json:"budget_total_cents" db:"budget_total_cents"`
	BudgetDailyCents   int64           `json:"budget_daily_cents" db:"budget_daily_cents"`
	SpentCents         int64           `json:"spent_cents" db:"spent_cents"`
	BidCPMCents        int             `json:"bid_cpm_cents" db:"bid_cpm_cents"`
	BidCPCCents        int             `json:"bid_cpc_cents" db:"bid_cpc_cents"`
	OCPMTargetCPACents int             `json:"ocpm_target_cpa_cents" db:"ocpm_target_cpa_cents"`
	StartDate          *time.Time      `json:"start_date,omitempty" db:"start_date"`
	EndDate            *time.Time      `json:"end_date,omitempty" db:"end_date"`
	Targeting          json.RawMessage `json:"targeting" db:"targeting"`
	PauseReason        *string         `json:"pause_reason,omitempty" db:"pause_reason"`
	PausedAt           *time.Time      `json:"paused_at,omitempty" db:"paused_at"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

// EffectiveBidCPMCents returns the CPM-equivalent bid for auction ranking.
// CPM: use bid_cpm_cents directly
// CPC: estimate CPM from CPC * predicted CTR (default 1% if unknown)
// oCPM: estimate CPM from target CPA * predicted CVR * 1000
func (c *Campaign) EffectiveBidCPMCents(predictedCTR, predictedCVR float64) int {
	switch c.BillingModel {
	case BillingCPC:
		if predictedCTR <= 0 {
			predictedCTR = 0.01 // 1% default
		}
		return int(float64(c.BidCPCCents) * predictedCTR * 1000)
	case BillingOCPM:
		if predictedCTR <= 0 {
			predictedCTR = 0.01
		}
		if predictedCVR <= 0 {
			predictedCVR = 0.05 // 5% default
		}
		return int(float64(c.OCPMTargetCPACents) * predictedCTR * predictedCVR * 1000)
	default: // CPM
		return c.BidCPMCents
	}
}

// ChargeEvent returns what triggers a charge for this billing model.
func (c *Campaign) ChargeEvent() string {
	cfg, ok := BillingModelConfig[c.BillingModel]
	if !ok {
		return "impression"
	}
	return cfg.ChargeOn
}

// AdType constants
const (
	AdTypeSplash       = "splash"       // 开屏广告: 全屏, app启动时展示, 3-5秒
	AdTypeInterstitial = "interstitial" // 插屏广告: 全屏, 页面切换时展示
	AdTypeNative       = "native"       // 原生广告: 结构化数据(标题+描述+图标+图片+CTA), 融入内容流
	AdTypeBanner       = "banner"       // 横幅广告: 固定尺寸(300x250, 728x90等)
)

// AdTypeConfig defines format-specific constraints.
var AdTypeConfig = map[string]struct {
	Label       string
	Sizes       []string // allowed sizes, empty = flexible
	FullScreen  bool
	HasNative   bool // uses native fields instead of ad_markup
	MaxDuration int  // seconds, 0 = static
}{
	AdTypeSplash:       {Label: "开屏", Sizes: []string{"1080x1920", "1242x2208"}, FullScreen: true, MaxDuration: 5},
	AdTypeInterstitial: {Label: "插屏", Sizes: []string{"320x480", "768x1024"}, FullScreen: true, MaxDuration: 15},
	AdTypeNative:       {Label: "原生", HasNative: true},
	AdTypeBanner:       {Label: "横幅", Sizes: []string{"300x250", "728x90", "320x50", "300x600"}},
}

type Creative struct {
	ID             int64     `json:"id" db:"id"`
	CampaignID     int64     `json:"campaign_id" db:"campaign_id"`
	Name           string    `json:"name" db:"name"`
	AdType         string    `json:"ad_type" db:"ad_type"`
	Format         string    `json:"format" db:"format"`
	Size           string    `json:"size" db:"size"`
	AdMarkup       string    `json:"ad_markup,omitempty" db:"ad_markup"`
	DestinationURL string    `json:"destination_url" db:"destination_url"`
	Status         string    `json:"status" db:"status"`
	// Native ad fields (used when ad_type = "native")
	NativeTitle    string    `json:"native_title,omitempty" db:"native_title"`
	NativeDesc     string    `json:"native_desc,omitempty" db:"native_desc"`
	NativeIconURL  string    `json:"native_icon_url,omitempty" db:"native_icon_url"`
	NativeImageURL string    `json:"native_image_url,omitempty" db:"native_image_url"`
	NativeCTA      string    `json:"native_cta,omitempty" db:"native_cta"`
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
