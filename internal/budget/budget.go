package budget

import (
	"context"
	"fmt"
	"time"

	"github.com/heartgryphon/dsp/internal/config"
	"github.com/heartgryphon/dsp/internal/observability"
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
// Used ONLY at win/click time (CheckAndDeductBudget), NOT at bid time.
var deductBudgetScript = redis.NewScript(`
local key = KEYS[1]
local amount = tonumber(ARGV[1])
local current = tonumber(redis.call('GET', key) or '0')
if current < amount then
  return -1
end
return redis.call('DECRBY', key, amount)
`)

// checkBudgetScript is a Lua script that checks budget WITHOUT deducting.
// Returns remaining budget if sufficient, or -1 if exhausted.
// Used at bid time (PipelineCheck) to avoid double-deduction: the real
// deduction happens at win/click time via deductBudgetScript.
var checkBudgetScript = redis.NewScript(`
local key = KEYS[1]
local amount = tonumber(ARGV[1])
local current = tonumber(redis.call('GET', key) or '0')
if current < amount then
  return -1
end
return current
`)

// CheckAndDeductBudget atomically checks and deducts from daily budget using Lua.
// Returns remaining budget in cents, or -1 if exhausted.
func (s *Service) CheckAndDeductBudget(ctx context.Context, campaignID int64, amountCents int64) (int64, error) {
	key := dailyBudgetKey(campaignID)
	result, err := deductBudgetScript.Run(ctx, s.rdb, []string{key}, amountCents).Int64()
	if err != nil {
		observability.RedisErrorsTotal.WithLabelValues("incr").Inc()
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
	if err != nil {
		observability.RedisErrorsTotal.WithLabelValues("get").Inc()
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
		observability.RedisErrorsTotal.WithLabelValues("incr").Inc()
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

// checkFreqScript is a Lua script that atomically checks frequency cap
// and increments only if under cap. Returns 1 if allowed, 0 if at cap.
// This avoids the race condition where INCR-then-check can over-count.
var checkFreqScript = redis.NewScript(`
local key = KEYS[1]
local cap = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])
local current = tonumber(redis.call('GET', key) or '0')
if current >= cap then
  return 0
end
redis.call('INCR', key)
if current == 0 then
  redis.call('EXPIRE', key, ttl)
end
return 1
`)

// PipelineCheck does budget check (no deduction) + frequency check at bid time.
//
// Budget is CHECK-ONLY here — the real deduction happens at win/click time
// via CheckAndDeductBudget. This prevents the double-deduction bug where
// bid-time deduction + win-time deduction would consume budget at 2x rate.
//
// Frequency cap uses an atomic Lua script that checks-then-increments to
// avoid the race condition where concurrent INCR-then-check could exceed cap.
func (s *Service) PipelineCheck(ctx context.Context, campaignID int64, userID string, bidAmountCents int64, freqCap int, freqPeriodHours int) (budgetOK bool, freqOK bool, err error) {
	// Check-only budget via Lua (no deduction)
	budgetKey := dailyBudgetKey(campaignID)
	remaining, err := checkBudgetScript.Run(ctx, s.rdb, []string{budgetKey}, bidAmountCents).Int64()
	if err != nil {
		observability.RedisErrorsTotal.WithLabelValues("check").Inc()
		return false, false, fmt.Errorf("redis budget check lua: %w", err)
	}
	budgetOK = remaining >= 0

	// Check frequency with atomic check-then-increment
	hasFreqCheck := userID != "" && freqCap > 0
	if !hasFreqCheck {
		freqOK = true
	} else {
		freqKeyStr := freqKey(campaignID, userID)
		ttlSeconds := int64(freqPeriodHours) * 3600
		allowed, ferr := checkFreqScript.Run(ctx, s.rdb, []string{freqKeyStr}, freqCap, ttlSeconds).Int64()
		if ferr != nil {
			observability.RedisErrorsTotal.WithLabelValues("freq").Inc()
			return false, false, fmt.Errorf("redis freq lua: %w", ferr)
		}
		freqOK = allowed == 1
	}

	return budgetOK, freqOK, nil
}

func dailyBudgetKey(campaignID int64) string {
	date := time.Now().In(config.CSTLocation).Format("2006-01-02")
	return fmt.Sprintf("budget:daily:%d:%s", campaignID, date)
}

func freqKey(campaignID int64, userID string) string {
	return fmt.Sprintf("freq:%d:%s", campaignID, userID)
}
