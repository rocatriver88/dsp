package antifraud

import (
	"context"
	"testing"
)

func TestIsBadUA(t *testing.T) {
	f := &Filter{badUA: []string{"bot", "crawler", "spider", "headless"}}

	tests := []struct {
		ua   string
		bad  bool
	}{
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", false},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)", false},
		{"Googlebot/2.1 (+http://www.google.com/bot.html)", true},
		{"python-requests/2.28.0", false},
		{"HeadlessChrome/120.0.6099.109", true},
		{"Mozilla/5.0 (compatible; Baiduspider/2.0)", true},
		{"", true}, // empty UA is suspicious
	}

	for _, tt := range tests {
		got := f.isBadUA(tt.ua)
		if got != tt.bad {
			t.Errorf("isBadUA(%q) = %v, want %v", tt.ua, got, tt.bad)
		}
	}
}

func TestIsDatacenterIP(t *testing.T) {
	f := NewFilter(nil)

	tests := []struct {
		ip string
		dc bool
	}{
		{"52.10.20.30", true},   // AWS range
		{"35.190.1.1", true},    // GCP range
		{"192.168.1.1", false},  // private
		{"8.8.8.8", false},      // Google DNS (not in our DC list)
		{"", false},
		{"invalid", false},
	}

	for _, tt := range tests {
		got := f.isDatacenterIP(tt.ip)
		if got != tt.dc {
			t.Errorf("isDatacenterIP(%q) = %v, want %v", tt.ip, got, tt.dc)
		}
	}
}

func TestBlacklist(t *testing.T) {
	f := NewFilter(nil)
	f.AddToBlacklist("1.2.3.4")

	// Can't run full Check without Redis, but test blacklist lookup
	f.mu.RLock()
	if !f.ipBlacklist["1.2.3.4"] {
		t.Error("expected IP to be blacklisted")
	}
	if f.ipBlacklist["5.6.7.8"] {
		t.Error("unexpected IP in blacklist")
	}
	f.mu.RUnlock()
}

func TestCheckWithoutRedis(t *testing.T) {
	// Test the non-Redis checks (IP, UA, datacenter)
	f := &Filter{
		ipBlacklist: map[string]bool{"1.2.3.4": true},
		badUA:       []string{"bot", "crawler"},
		dcRanges:    nil,
	}

	tests := []struct {
		ip, ua, devID string
		allowed       bool
		reason        string
	}{
		{"1.2.3.4", "Mozilla/5.0", "dev1", false, "ip_blacklisted"},
		{"5.6.7.8", "Googlebot/2.1", "dev2", false, "suspicious_ua"},
		{"5.6.7.8", "", "dev3", false, "suspicious_ua"}, // empty UA
	}

	ctx := context.Background()
	for _, tt := range tests {
		r := f.Check(ctx, tt.ip, tt.ua, tt.devID)
		if r.Allowed != tt.allowed {
			t.Errorf("Check(ip=%s, ua=%s) allowed=%v, want %v (reason=%s)",
				tt.ip, tt.ua, r.Allowed, tt.allowed, r.Reason)
		}
		if !tt.allowed && r.Reason != tt.reason {
			t.Errorf("Check(ip=%s, ua=%s) reason=%s, want %s",
				tt.ip, tt.ua, r.Reason, tt.reason)
		}
	}
}
