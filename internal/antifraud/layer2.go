package antifraud

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Layer2 performs async event-level fraud checks in the Kafka consumer pipeline.
//
// Checks:
//   - Click timing anomaly: impression-to-click interval < 1s or > 24h
//   - Impression flooding: same device_id > 100 impressions/min for a campaign
//
// Suspicious events are flagged (not dropped) so they appear in reports
// but can be filtered or discounted.
type Layer2 struct {
	rdb *redis.Client
}

func NewLayer2(rdb *redis.Client) *Layer2 {
	return &Layer2{rdb: rdb}
}

// CheckResult holds the fraud check outcome.
type CheckResult struct {
	Suspicious bool
	Reason     string
}

// CheckClickTiming detects anomalous click timing.
// impressionTime is when the impression was recorded.
// clickTime is when the click occurred.
func (l *Layer2) CheckClickTiming(impressionTime, clickTime time.Time) CheckResult {
	interval := clickTime.Sub(impressionTime)

	if interval < time.Second {
		return CheckResult{
			Suspicious: true,
			Reason:     fmt.Sprintf("click too fast: %dms after impression", interval.Milliseconds()),
		}
	}
	if interval > 24*time.Hour {
		return CheckResult{
			Suspicious: true,
			Reason:     fmt.Sprintf("click too late: %.1fh after impression", interval.Hours()),
		}
	}
	return CheckResult{}
}

// CheckImpressionFlood detects if a device is receiving too many impressions
// for a single campaign in a short time window. Returns suspicious if > threshold/min.
func (l *Layer2) CheckImpressionFlood(ctx context.Context, campaignID int64, deviceID string, threshold int) CheckResult {
	if deviceID == "" {
		return CheckResult{} // no device tracking (GDPR)
	}

	key := fmt.Sprintf("fraud:flood:%d:%s", campaignID, deviceID)
	count, err := l.rdb.Incr(ctx, key).Result()
	if err != nil {
		return CheckResult{} // fail-open on Redis error
	}

	if count == 1 {
		l.rdb.Expire(ctx, key, time.Minute)
	}

	if count > int64(threshold) {
		return CheckResult{
			Suspicious: true,
			Reason:     fmt.Sprintf("impression flood: %d impressions/min for device %s", count, deviceID[:8]),
		}
	}
	return CheckResult{}
}

// LogSuspicious logs a suspicious event for reporting/audit.
func (l *Layer2) LogSuspicious(campaignID int64, eventType, reason string) {
	log.Printf("[FRAUD-L2] campaign=%d event=%s reason=%s", campaignID, eventType, reason)
}
