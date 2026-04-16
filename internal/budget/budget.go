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

// Budget keys: budget:daily:{campaign_id}:{date}  (daily remaining, TTL 25h)
//              budget:total:{campaign_id}          (total remaining, no TTL)
// Freq keys:   freq:{campaign_id}:{user_id}

// deductBudgetScript atomically checks and deducts from BOTH daily and total
// budget counters. If either is insufficient, neither is deducted.
// KEYS[1] = daily budget key, KEYS[2] = total budget key
// ARGV[1] = amount to deduct
// Returns: remaining daily budget if both sufficient, -1 if daily exhausted,
// -2 if total exhausted.
// Used ONLY at win/click time (CheckAndDeductBudget), NOT at bid time.
var deductBudgetScript = redis.NewScript(`
local dailyKey = KEYS[1]
local totalKey = KEYS[2]
local amount = tonumber(ARGV[1])
local dailyCurrent = tonumber(redis.call('GET', dailyKey) or '0')
if dailyCurrent < amount then
  return -1
end
local totalCurrent = tonumber(redis.call('GET', totalKey) or '0')
if totalCurrent > 0 and totalCurrent < amount then
  return -2
end
if totalCurrent > 0 then
  redis.call('DECRBY', totalKey, amount)
end
return redis.call('DECRBY', dailyKey, amount)
`)

// checkBudgetScript checks BOTH daily and total budget WITHOUT deducting.
// KEYS[1] = daily budget key, KEYS[2] = total budget key
// ARGV[1] = amount needed
// Returns: remaining daily budget if both sufficient, -1 if daily exhausted,
// -2 if total exhausted.
// Used at bid time (PipelineCheck) to avoid double-deduction.
var checkBudgetScript = redis.NewScript(`
local dailyKey = KEYS[1]
local totalKey = KEYS[2]
local amount = tonumber(ARGV[1])
local dailyCurrent = tonumber(redis.call('GET', dailyKey) or '0')
if dailyCurrent < amount then
  return -1
end
local totalCurrent = tonumber(redis.call('GET', totalKey) or '0')
if totalCurrent > 0 and totalCurrent < amount then
  return -2
end
return dailyCurrent
`)

// CheckAndDeductBudget atomically checks and deducts from both daily and total
// budget using Lua. Returns remaining daily budget in cents, or -1 if daily
// exhausted, or -2 if total exhausted.
func (s *Service) CheckAndDeductBudget(ctx context.Context, campaignID int64, amountCents int64) (int64, error) {
	dailyKey := dailyBudgetKey(campaignID)
	totalKey := totalBudgetKey(campaignID)
	result, err := deductBudgetScript.Run(ctx, s.rdb, []string{dailyKey, totalKey}, amountCents).Int64()
	if err != nil {
		observability.RedisErrorsTotal.WithLabelValues("deduct").Inc()
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

// InitTotalBudget sets the total (lifetime) budget for a campaign in Redis.
// Called when a campaign is loaded/started. Uses SetNX so it doesn't reset
// a counter that's already been partially spent.
func (s *Service) InitTotalBudget(ctx context.Context, campaignID int64, budgetCents int64) error {
	if budgetCents <= 0 {
		return nil // no total budget constraint
	}
	key := totalBudgetKey(campaignID)
	// SetNX: only set if not already present, so reloads don't reset spent budget
	return s.rdb.SetNX(ctx, key, budgetCents, 0).Err()
}

// GetTotalBudgetRemaining returns remaining total (lifetime) budget in cents.
func (s *Service) GetTotalBudgetRemaining(ctx context.Context, campaignID int64) (int64, error) {
	key := totalBudgetKey(campaignID)
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
	// Check-only budget via Lua (no deduction) — checks both daily and total
	dailyKey := dailyBudgetKey(campaignID)
	totalKey := totalBudgetKey(campaignID)
	remaining, err := checkBudgetScript.Run(ctx, s.rdb, []string{dailyKey, totalKey}, bidAmountCents).Int64()
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

func totalBudgetKey(campaignID int64) string {
	return fmt.Sprintf("budget:total:%d", campaignID)
}

func freqKey(campaignID int64, userID string) string {
	return fmt.Sprintf("freq:%d:%s", campaignID, userID)
}
