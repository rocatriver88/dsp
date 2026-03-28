package budget

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Service handles real-time budget tracking and frequency capping via Redis.
type Service struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Service {
	return &Service{rdb: rdb}
}

// Budget keys: budget:daily:{campaign_id}:{date}
// Freq keys:   freq:{campaign_id}:{user_id}

// CheckAndDeductBudget atomically checks and deducts from daily budget.
// Returns remaining budget in cents, or -1 if exhausted.
func (s *Service) CheckAndDeductBudget(ctx context.Context, campaignID int64, amountCents int64) (int64, error) {
	key := dailyBudgetKey(campaignID)
	remaining, err := s.rdb.DecrBy(ctx, key, amountCents).Result()
	if err != nil {
		return -1, fmt.Errorf("redis decrby: %w", err)
	}
	if remaining < 0 {
		// Overspent, refund
		s.rdb.IncrBy(ctx, key, amountCents)
		return -1, nil
	}
	return remaining, nil
}

// InitDailyBudget sets the daily budget for a campaign. Called at midnight or campaign start.
func (s *Service) InitDailyBudget(ctx context.Context, campaignID int64, budgetCents int64) error {
	key := dailyBudgetKey(campaignID)
	pipe := s.rdb.Pipeline()
	pipe.Set(ctx, key, budgetCents, 25*time.Hour) // TTL slightly over 24h
	_, err := pipe.Exec(ctx)
	return err
}

// GetDailyBudgetRemaining returns remaining daily budget in cents.
func (s *Service) GetDailyBudgetRemaining(ctx context.Context, campaignID int64) (int64, error) {
	key := dailyBudgetKey(campaignID)
	val, err := s.rdb.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	return val, err
}

// CheckFrequency checks if user has reached frequency cap. Returns true if under cap.
func (s *Service) CheckFrequency(ctx context.Context, campaignID int64, userID string, maxCount int, periodHours int) (bool, error) {
	if userID == "" {
		return true, nil // no user ID (GDPR), skip freq check
	}
	key := freqKey(campaignID, userID)
	count, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis incr: %w", err)
	}

	// Set TTL on first increment
	if count == 1 {
		s.rdb.Expire(ctx, key, time.Duration(periodHours)*time.Hour)
	}

	if count > int64(maxCount) {
		return false, nil // over cap
	}
	return true, nil
}

// PipelineCheck does budget + frequency check in one Redis round trip.
func (s *Service) PipelineCheck(ctx context.Context, campaignID int64, userID string, bidAmountCents int64, freqCap int, freqPeriodHours int) (budgetOK bool, freqOK bool, err error) {
	budgetKey := dailyBudgetKey(campaignID)

	pipe := s.rdb.Pipeline()
	budgetCmd := pipe.DecrBy(ctx, budgetKey, bidAmountCents)

	var freqCmd *redis.IntCmd
	hasFreqCheck := userID != "" && freqCap > 0
	freqKeyStr := ""
	if hasFreqCheck {
		freqKeyStr = freqKey(campaignID, userID)
		freqCmd = pipe.Incr(ctx, freqKeyStr)
	}

	_, err = pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return false, false, fmt.Errorf("redis pipeline: %w", err)
	}

	// Check budget
	remaining, _ := budgetCmd.Result()
	if remaining < 0 {
		// Refund
		s.rdb.IncrBy(ctx, budgetKey, bidAmountCents)
		budgetOK = false
	} else {
		budgetOK = true
	}

	// Check frequency
	if !hasFreqCheck {
		freqOK = true
	} else {
		count, _ := freqCmd.Result()
		if count == 1 {
			s.rdb.Expire(ctx, freqKeyStr, time.Duration(freqPeriodHours)*time.Hour)
		}
		freqOK = count <= int64(freqCap)
		if !freqOK && budgetOK {
			// Refund budget if freq cap hit
			s.rdb.IncrBy(ctx, budgetKey, bidAmountCents)
			budgetOK = false
		}
	}

	return budgetOK, freqOK, nil
}

func dailyBudgetKey(campaignID int64) string {
	date := time.Now().UTC().Format("2006-01-02")
	return fmt.Sprintf("budget:daily:%d:%s", campaignID, date)
}

func freqKey(campaignID int64, userID string) string {
	return fmt.Sprintf("freq:%d:%s", campaignID, userID)
}
