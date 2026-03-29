package audience

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Service manages audience segments for behavioral targeting.
//
// Segments are stored in Redis sets keyed by segment ID.
// Each set contains device IDs (IDFA/GAID/OAID) that belong to the segment.
//
// Segment membership can be populated from:
//   - Conversion data (e.g., "users who converted in last 30 days")
//   - Click behavior (e.g., "users who clicked gaming ads")
//   - External DMP imports (future)
//
// Key format: segment:{segment_id} → Redis SET of device IDs
type Service struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Service {
	return &Service{rdb: rdb}
}

// AddToSegment adds a device ID to a segment.
func (s *Service) AddToSegment(ctx context.Context, segmentID, deviceID string) error {
	if deviceID == "" {
		return nil // no device tracking
	}
	key := segmentKey(segmentID)
	s.rdb.SAdd(ctx, key, deviceID)
	s.rdb.Expire(ctx, key, 30*24*time.Hour) // 30-day TTL
	return nil
}

// RemoveFromSegment removes a device ID from a segment.
func (s *Service) RemoveFromSegment(ctx context.Context, segmentID, deviceID string) error {
	return s.rdb.SRem(ctx, segmentKey(segmentID), deviceID).Err()
}

// IsMember checks if a device ID belongs to a segment.
func (s *Service) IsMember(ctx context.Context, segmentID, deviceID string) bool {
	if deviceID == "" {
		return false
	}
	ok, err := s.rdb.SIsMember(ctx, segmentKey(segmentID), deviceID).Result()
	return err == nil && ok
}

// MatchesSegments checks if a device ID matches ALL required segments
// and NONE of the excluded segments.
func (s *Service) MatchesSegments(ctx context.Context, deviceID string, required, excluded []string) bool {
	if deviceID == "" {
		return len(required) == 0 // no device ID: only match if no segments required
	}

	// Check required segments (must be member of ALL)
	for _, seg := range required {
		if !s.IsMember(ctx, seg, deviceID) {
			return false
		}
	}

	// Check excluded segments (must NOT be member of ANY)
	for _, seg := range excluded {
		if s.IsMember(ctx, seg, deviceID) {
			return false
		}
	}

	return true
}

// SegmentSize returns the number of device IDs in a segment.
func (s *Service) SegmentSize(ctx context.Context, segmentID string) (int64, error) {
	return s.rdb.SCard(ctx, segmentKey(segmentID)).Result()
}

// ListSegments returns all known segment IDs with their sizes.
func (s *Service) ListSegments(ctx context.Context) ([]SegmentInfo, error) {
	keys, err := s.rdb.Keys(ctx, "segment:*").Result()
	if err != nil {
		return nil, err
	}

	var segments []SegmentInfo
	for _, key := range keys {
		id := key[8:] // strip "segment:" prefix
		size, _ := s.rdb.SCard(ctx, key).Result()
		segments = append(segments, SegmentInfo{ID: id, Size: size})
	}
	return segments, nil
}

type SegmentInfo struct {
	ID   string `json:"id"`
	Size int64  `json:"size"`
}

func segmentKey(segmentID string) string {
	return fmt.Sprintf("segment:%s", segmentID)
}
