package audience

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_redis_password"})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("Redis not available: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		keys, _ := rdb.Keys(ctx, "segment:test-*").Result()
		if len(keys) > 0 {
			rdb.Del(ctx, keys...)
		}
		rdb.Close()
	})
	return rdb
}

func TestAddAndCheckMembership(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	svc.AddToSegment(ctx, "test-gamers", "device-001")
	svc.AddToSegment(ctx, "test-gamers", "device-002")

	if !svc.IsMember(ctx, "test-gamers", "device-001") {
		t.Error("device-001 should be member of test-gamers")
	}
	if svc.IsMember(ctx, "test-gamers", "device-003") {
		t.Error("device-003 should not be member of test-gamers")
	}
}

func TestMatchesSegments_AllRequired(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	svc.AddToSegment(ctx, "test-seg-a", "device-010")
	svc.AddToSegment(ctx, "test-seg-b", "device-010")

	if !svc.MatchesSegments(ctx, "device-010", []string{"test-seg-a", "test-seg-b"}, nil) {
		t.Error("device-010 should match both segments")
	}

	// Missing one segment
	if svc.MatchesSegments(ctx, "device-010", []string{"test-seg-a", "test-seg-c"}, nil) {
		t.Error("device-010 should not match (not in test-seg-c)")
	}
}

func TestMatchesSegments_Excluded(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	svc.AddToSegment(ctx, "test-buyers", "device-020")
	svc.AddToSegment(ctx, "test-churned", "device-020")

	// Should not match if in excluded segment
	if svc.MatchesSegments(ctx, "device-020", []string{"test-buyers"}, []string{"test-churned"}) {
		t.Error("device-020 should be excluded (in test-churned)")
	}
}

func TestMatchesSegments_EmptyDeviceID(t *testing.T) {
	svc := New(nil)
	ctx := context.Background()

	// No device ID with no required segments: should match
	if !svc.MatchesSegments(ctx, "", nil, nil) {
		t.Error("empty device with no requirements should match")
	}

	// No device ID with required segments: should not match
	if svc.MatchesSegments(ctx, "", []string{"test-seg"}, nil) {
		t.Error("empty device with requirements should not match")
	}
}

func TestSegmentSize(t *testing.T) {
	rdb := newTestRedis(t)
	svc := New(rdb)
	ctx := context.Background()

	svc.AddToSegment(ctx, "test-size", "d1")
	svc.AddToSegment(ctx, "test-size", "d2")
	svc.AddToSegment(ctx, "test-size", "d3")

	size, err := svc.SegmentSize(ctx, "test-size")
	if err != nil {
		t.Fatalf("segment size: %v", err)
	}
	if size != 3 {
		t.Errorf("expected size 3, got %d", size)
	}
}
