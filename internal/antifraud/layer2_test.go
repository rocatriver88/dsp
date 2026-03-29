package antifraud

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_redis_password"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	return rdb
}

func TestCheckClickTiming_Normal(t *testing.T) {
	l := NewLayer2(nil)
	imp := time.Now().Add(-5 * time.Second)
	click := time.Now()

	result := l.CheckClickTiming(imp, click)
	if result.Suspicious {
		t.Errorf("5s interval should be normal, got: %s", result.Reason)
	}
}

func TestCheckClickTiming_TooFast(t *testing.T) {
	l := NewLayer2(nil)
	imp := time.Now()
	click := imp.Add(500 * time.Millisecond)

	result := l.CheckClickTiming(imp, click)
	if !result.Suspicious {
		t.Error("500ms interval should be suspicious")
	}
}

func TestCheckClickTiming_TooLate(t *testing.T) {
	l := NewLayer2(nil)
	imp := time.Now().Add(-25 * time.Hour)
	click := time.Now()

	result := l.CheckClickTiming(imp, click)
	if !result.Suspicious {
		t.Error("25h interval should be suspicious")
	}
}

func TestCheckImpressionFlood_Normal(t *testing.T) {
	rdb := newTestRedis(t)
	l := NewLayer2(rdb)
	ctx := context.Background()

	rdb.Del(ctx, "fraud:flood:999:device-test-1")

	result := l.CheckImpressionFlood(ctx, 999, "device-test-1", 100)
	if result.Suspicious {
		t.Error("first impression should not be suspicious")
	}
}

func TestCheckImpressionFlood_Exceeded(t *testing.T) {
	rdb := newTestRedis(t)
	l := NewLayer2(rdb)
	ctx := context.Background()

	rdb.Del(ctx, "fraud:flood:998:device-test-2")

	// Simulate 5 impressions with threshold of 3
	for i := 0; i < 5; i++ {
		l.CheckImpressionFlood(ctx, 998, "device-test-2", 3)
	}

	result := l.CheckImpressionFlood(ctx, 998, "device-test-2", 3)
	if !result.Suspicious {
		t.Error("6th impression with threshold 3 should be suspicious")
	}
}

func TestCheckImpressionFlood_EmptyDeviceID(t *testing.T) {
	l := NewLayer2(nil)
	result := l.CheckImpressionFlood(context.Background(), 999, "", 100)
	if result.Suspicious {
		t.Error("empty device ID (GDPR) should not be checked")
	}
}
