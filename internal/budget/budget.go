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

// deductBudgetScript is a Lua script that atomically checks and deducts budget.
// Returns remaining budget if sufficient, or -1 if exhausted (no deduction made).
var deductBudgetScript = redis.NewScript(`
local key = KEYS[1]
local amount = tonumber(ARGV[1])
local current = tonumber(redis.call('GET', key) or '0')
if current < amount then
  return -1
end
return redis.call('DECRBY', key, amount)
`)

// CheckAndDeductBudget atomically checks and deducts from daily budget using Lua.
// Returns remaining budget in cents, or -1 if exhausted.
func (s *Service) CheckAndDeductBudget(ctx context.Context, campaignID int64, amountCents int64) (int64, error) {
	key := dailyBudgetKey(campaignID)
	result, err := deductBudgetScript.Run(ctx, s.rdb, []string{key}, amountCents).Int64()
	if err != nil {
		return -1, fmt.Errorf("redis budget lua: %w", err)
	}
	return result, nil
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

// PipelineCheck does budget (atomic Lua) + frequency check.
func (s *Service) PipelineCheck(ctx context.Context, campaignID int64, userID string, bidAmountCents int64, freqCap int, freqPeriodHours int) (budgetOK bool, freqOK bool, err error) {
	// Atomic budget check via Lua
	budgetKey := dailyBudgetKey(campaignID)
	remaining, err := deductBudgetScript.Run(ctx, s.rdb, []string{budgetKey}, bidAmountCents).Int64()
	if err != nil {
		return false, false, fmt.Errorf("redis budget lua: %w", err)
	}
	budgetOK = remaining >= 0

	// Check frequency
	hasFreqCheck := userID != "" && freqCap > 0
	if !hasFreqCheck {
		freqOK = true
	} else {
		freqKeyStr := freqKey(campaignID, userID)
		count, ferr := s.rdb.Incr(ctx, freqKeyStr).Result()
		if ferr != nil {
			// Refund budget on freq check failure
			if budgetOK {
				s.rdb.IncrBy(ctx, budgetKey, bidAmountCents)
			}
			return false, false, fmt.Errorf("redis freq: %w", ferr)
		}
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
	loc, _ := time.LoadLocation("Asia/Shanghai")
	date := time.Now().In(loc).Format("2006-01-02")
	return fmt.Sprintf("budget:daily:%d:%s", campaignID, date)
}

func freqKey(campaignID int64, userID string) string {
	return fmt.Sprintf("freq:%d:%s", campaignID, userID)
}
