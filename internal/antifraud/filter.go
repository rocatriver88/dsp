package antifraud

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Filter implements the 3-layer anti-fraud system from the design doc.
//
// Layer 1: Request-level (in bidder hot path, real-time)
//   - IP blacklist / datacenter IP filtering
//   - User-Agent anomaly detection
//   - Request frequency anomaly
//
// Layer 2: Event-level (async, near real-time) — deferred to consumer
// Layer 3: Aggregate-level (batch, T+1) — deferred to separate job
type Filter struct {
	rdb           *redis.Client
	ipBlacklist   map[string]bool
	dcRanges      []*net.IPNet // datacenter IP ranges
	badUA         []string
	mu            sync.RWMutex
}

// Result holds the filtering decision.
type Result struct {
	Allowed bool
	Reason  string
}

func NewFilter(rdb *redis.Client) *Filter {
	f := &Filter{
		rdb:         rdb,
		ipBlacklist: make(map[string]bool),
		badUA: []string{
			"bot", "crawler", "spider", "headless", "phantom",
			"selenium", "puppeteer", "playwright", "wget", "curl",
		},
	}

	// Known datacenter IP ranges (AWS, GCP, Azure partial)
	dcCIDRs := []string{
		"52.0.0.0/8",     // AWS partial
		"35.190.0.0/16",  // GCP
		"13.64.0.0/11",   // Azure partial
		"104.16.0.0/12",  // Cloudflare
	}
	for _, cidr := range dcCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			f.dcRanges = append(f.dcRanges, ipNet)
		}
	}

	return f
}

// Check runs Layer 1 checks on a bid request. Returns allowed=true if clean.
// This runs in the bidder hot path, must be fast (<1ms).
func (f *Filter) Check(ctx context.Context, ip, ua, deviceID string) Result {
	// 1. IP blacklist
	f.mu.RLock()
	if f.ipBlacklist[ip] {
		f.mu.RUnlock()
		return Result{Allowed: false, Reason: "ip_blacklisted"}
	}
	f.mu.RUnlock()

	// 2. Datacenter IP check
	if f.isDatacenterIP(ip) {
		return Result{Allowed: false, Reason: "datacenter_ip"}
	}

	// 3. User-Agent anomaly
	if f.isBadUA(ua) {
		return Result{Allowed: false, Reason: "suspicious_ua"}
	}

	// 4. Request frequency (Redis, per IP, 100 req/min max)
	if ip != "" {
		freqKey := fmt.Sprintf("fraud:freq:%s", ip)
		count, err := f.rdb.Incr(ctx, freqKey).Result()
		if err == nil {
			if count == 1 {
				f.rdb.Expire(ctx, freqKey, time.Minute)
			}
			if count > 100 {
				return Result{Allowed: false, Reason: "rate_limit_ip"}
			}
		}
	}

	// 5. Device ID frequency (per device, 50 req/min max)
	if deviceID != "" {
		devKey := fmt.Sprintf("fraud:dev:%s", deviceID)
		count, err := f.rdb.Incr(ctx, devKey).Result()
		if err == nil {
			if count == 1 {
				f.rdb.Expire(ctx, devKey, time.Minute)
			}
			if count > 50 {
				return Result{Allowed: false, Reason: "rate_limit_device"}
			}
		}
	}

	return Result{Allowed: true}
}

// AddToBlacklist adds an IP to the blacklist.
func (f *Filter) AddToBlacklist(ip string) {
	f.mu.Lock()
	f.ipBlacklist[ip] = true
	f.mu.Unlock()
	log.Printf("[ANTIFRAUD] Blacklisted IP: %s", ip)
}

// Stats returns current filter stats.
func (f *Filter) Stats() map[string]any {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return map[string]any{
		"blacklisted_ips":  len(f.ipBlacklist),
		"datacenter_ranges": len(f.dcRanges),
		"bad_ua_patterns":  len(f.badUA),
	}
}

func (f *Filter) isDatacenterIP(ip string) bool {
	if ip == "" {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range f.dcRanges {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

func (f *Filter) isBadUA(ua string) bool {
	if ua == "" {
		return true // no UA is suspicious
	}
	lower := strings.ToLower(ua)
	for _, bad := range f.badUA {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	return false
}
