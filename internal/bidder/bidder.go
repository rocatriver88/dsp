package bidder

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// Campaign represents a hardcoded advertiser campaign for Phase 0.
type Campaign struct {
	ID          int
	Name        string
	BidCPM      float64 // bid price in CPM (cost per mille)
	DailyBudget float64
	Spent       float64
	TargetGeo   []string // country codes
	TargetOS    []string
	AdMarkup    string
	AdvDomain   string
	mu          sync.Mutex
}

// Bidder handles OpenRTB bid requests against a set of campaigns.
type Bidder struct {
	campaigns []*Campaign
}

// New creates a bidder with 5 hardcoded campaigns.
func New() *Bidder {
	return &Bidder{
		campaigns: []*Campaign{
			{
				ID: 1, Name: "Rust编程教程推广",
				BidCPM: 5.0, DailyBudget: 1000, Spent: 0,
				TargetGeo: []string{"CN", "US"}, TargetOS: []string{"Windows", "macOS", "Linux"},
				AdMarkup:  `<div style="width:300px;height:250px;background:#1a1a2e;color:#e94560;font-family:monospace;display:flex;align-items:center;justify-content:center;font-size:18px">学Rust，写高性能代码</div>`,
				AdvDomain: "rust-tutorial.example.com",
			},
			{
				ID: 2, Name: "云服务器促销",
				BidCPM: 8.0, DailyBudget: 2000, Spent: 0,
				TargetGeo: []string{"CN"}, TargetOS: []string{"Windows", "macOS", "Linux"},
				AdMarkup:  `<div style="width:300px;height:250px;background:#0f3460;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;font-size:16px">云服务器 1核2G ¥99/年</div>`,
				AdvDomain: "cloud.example.com",
			},
			{
				ID: 3, Name: "移动游戏下载",
				BidCPM: 12.0, DailyBudget: 5000, Spent: 0,
				TargetGeo: []string{"CN", "JP", "KR"}, TargetOS: []string{"iOS", "Android"},
				AdMarkup:  `<div style="width:320px;height:50px;background:#e94560;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;font-size:14px">全新RPG手游 - 立即下载</div>`,
				AdvDomain: "game.example.com",
			},
			{
				ID: 4, Name: "SaaS工具推广",
				BidCPM: 15.0, DailyBudget: 3000, Spent: 0,
				TargetGeo: []string{"US", "GB", "DE"}, TargetOS: []string{"Windows", "macOS"},
				AdMarkup:  `<div style="width:728px;height:90px;background:#16213e;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;font-size:16px">Project Management — Free Trial</div>`,
				AdvDomain: "saas-tool.example.com",
			},
			{
				ID: 5, Name: "电商大促",
				BidCPM: 3.0, DailyBudget: 10000, Spent: 0,
				TargetGeo: []string{"CN"}, TargetOS: []string{"iOS", "Android", "Windows", "macOS"},
				AdMarkup:  `<div style="width:300px;height:250px;background:#ff6b6b;color:#fff;font-family:sans-serif;display:flex;align-items:center;justify-content:center;font-size:20px">限时特惠 全场5折</div>`,
				AdvDomain: "shop.example.com",
			},
		},
	}
}

// BidResult contains the auction outcome.
type BidResult struct {
	Campaign *Campaign
	BidPrice float64 // actual bid price for this impression (CPM / 1000)
	BidID    string
}

// ProcessRequest evaluates a bid request and returns a bid if eligible.
func (b *Bidder) ProcessRequest(req *openrtb2.BidRequest) *BidResult {
	if req == nil || len(req.Imp) == 0 {
		return nil
	}

	imp := req.Imp[0]

	// Extract device info
	var deviceOS string
	if req.Device != nil && req.Device.OS != "" {
		deviceOS = req.Device.OS
	}

	// Extract geo
	var geoCountry string
	if req.Device != nil && req.Device.Geo != nil && req.Device.Geo.Country != "" {
		geoCountry = req.Device.Geo.Country
	}

	// Find best matching campaign (highest CPM)
	var best *Campaign
	for _, c := range b.campaigns {
		if !c.matches(geoCountry, deviceOS, imp) {
			continue
		}
		c.mu.Lock()
		budgetOK := c.Spent < c.DailyBudget
		c.mu.Unlock()
		if !budgetOK {
			continue
		}
		if best == nil || c.BidCPM > best.BidCPM {
			best = c
		}
	}

	if best == nil {
		return nil
	}

	bidPrice := best.BidCPM / 1000.0 // CPM to per-impression price

	return &BidResult{
		Campaign: best,
		BidPrice: bidPrice,
		BidID:    fmt.Sprintf("bid-%d-%d", best.ID, time.Now().UnixNano()),
	}
}

// RecordWin deducts spend for a winning bid.
func (b *Bidder) RecordWin(campaignID int, clearPrice float64) bool {
	for _, c := range b.campaigns {
		if c.ID == campaignID {
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.Spent >= c.DailyBudget {
				return false // budget exhausted
			}
			c.Spent += clearPrice
			return true
		}
	}
	return false
}

// Stats returns current campaign stats.
func (b *Bidder) Stats() []map[string]any {
	var stats []map[string]any
	for _, c := range b.campaigns {
		c.mu.Lock()
		stats = append(stats, map[string]any{
			"id":      c.ID,
			"name":    c.Name,
			"bid_cpm": c.BidCPM,
			"budget":  c.DailyBudget,
			"spent":   c.Spent,
			"remain":  c.DailyBudget - c.Spent,
		})
		c.mu.Unlock()
	}
	return stats
}

func (c *Campaign) matches(geo, os string, imp openrtb2.Imp) bool {
	// Geo check
	if len(c.TargetGeo) > 0 && geo != "" {
		found := false
		for _, g := range c.TargetGeo {
			if g == geo {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// OS check
	if len(c.TargetOS) > 0 && os != "" {
		found := false
		for _, o := range c.TargetOS {
			if o == os {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Format check: if imp has banner, campaign must have banner-sized markup
	// Phase 0: skip strict size matching, just check format exists
	if imp.Banner == nil && imp.Video == nil && imp.Native == nil {
		return false
	}

	return true
}

func init() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
}
