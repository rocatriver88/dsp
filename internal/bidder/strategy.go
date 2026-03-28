package bidder

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/redis/go-redis/v9"
)

// BidStrategy adjusts bid prices based on win rate and pacing.
//
// Simple linear pacing (from eng review):
//   spent_so_far / expected_at_this_hour
//   ratio > 1.1 → reduce bid rate (random skip)
//   ratio < 0.9 → full bid
//
// Dynamic bid adjustment:
//   If win rate too high (>80%) → we're overpaying, lower bid
//   If win rate too low (<20%) → we're losing auctions, raise bid
//   Target: 40-60% win rate (sweet spot)
type BidStrategy struct {
	rdb *redis.Client
}

func NewBidStrategy(rdb *redis.Client) *BidStrategy {
	return &BidStrategy{rdb: rdb}
}

// AdjustedBid returns the adjusted bid price in CPM cents.
// It considers pacing (budget consumption rate) and win rate.
func (s *BidStrategy) AdjustedBid(ctx context.Context, campaignID int64, baseBidCPM int, dailyBudgetCents int64) int {
	pacingFactor := s.pacingFactor(ctx, campaignID, dailyBudgetCents)
	winRateFactor := s.winRateFactor(ctx, campaignID)

	adjusted := float64(baseBidCPM) * pacingFactor * winRateFactor

	// Clamp: never bid below 50% or above 150% of base
	minBid := float64(baseBidCPM) * 0.5
	maxBid := float64(baseBidCPM) * 1.5
	adjusted = math.Max(minBid, math.Min(maxBid, adjusted))

	return int(math.Round(adjusted))
}

// ShouldBid returns whether to participate in this auction based on pacing.
func (s *BidStrategy) ShouldBid(ctx context.Context, campaignID int64, dailyBudgetCents int64) bool {
	ratio := s.spendRatio(ctx, campaignID, dailyBudgetCents)
	if ratio > 1.1 {
		// Ahead of schedule: skip some auctions
		// The further ahead, the more we skip
		skipRate := math.Min(0.8, (ratio-1.0)*2) // at 1.5x, skip 100% → clamped to 80%
		return fastRand()%100 >= int(skipRate*100)
	}
	return true // behind or on schedule: always bid
}

// RecordBid increments the bid counter for win rate calculation.
func (s *BidStrategy) RecordBid(ctx context.Context, campaignID int64) {
	key := fmt.Sprintf("strategy:bids:%d:%s", campaignID, today())
	s.rdb.Incr(ctx, key)
	s.rdb.Expire(ctx, key, 25*time.Hour)
}

// RecordWin increments the win counter.
func (s *BidStrategy) RecordWin(ctx context.Context, campaignID int64) {
	key := fmt.Sprintf("strategy:wins:%d:%s", campaignID, today())
	s.rdb.Incr(ctx, key)
	s.rdb.Expire(ctx, key, 25*time.Hour)
}

// RecordSpend records spend for pacing calculation.
func (s *BidStrategy) RecordSpend(ctx context.Context, campaignID int64, amountCents int64) {
	key := fmt.Sprintf("strategy:spend:%d:%s", campaignID, today())
	s.rdb.IncrBy(ctx, key, amountCents)
	s.rdb.Expire(ctx, key, 25*time.Hour)
}

func (s *BidStrategy) pacingFactor(ctx context.Context, campaignID int64, dailyBudgetCents int64) float64 {
	ratio := s.spendRatio(ctx, campaignID, dailyBudgetCents)
	if ratio == 0 {
		return 1.0 // no data yet
	}
	// If spending too fast, lower bid to slow down
	// If spending too slow, raise bid to speed up
	if ratio > 1.1 {
		return 0.8 // overspending, bid lower
	}
	if ratio < 0.9 {
		return 1.1 // underspending, bid higher
	}
	return 1.0 // on track
}

func (s *BidStrategy) spendRatio(ctx context.Context, campaignID int64, dailyBudgetCents int64) float64 {
	if dailyBudgetCents <= 0 {
		return 0
	}

	spendKey := fmt.Sprintf("strategy:spend:%d:%s", campaignID, today())
	spent, err := s.rdb.Get(ctx, spendKey).Int64()
	if err != nil {
		return 0
	}

	// Expected spend at current hour
	hour := time.Now().UTC().Hour()
	if hour == 0 {
		hour = 1 // avoid division by zero at midnight
	}
	expectedSpent := dailyBudgetCents * int64(hour) / 24

	if expectedSpent == 0 {
		return 0
	}
	return float64(spent) / float64(expectedSpent)
}

func (s *BidStrategy) winRateFactor(ctx context.Context, campaignID int64) float64 {
	bidsKey := fmt.Sprintf("strategy:bids:%d:%s", campaignID, today())
	winsKey := fmt.Sprintf("strategy:wins:%d:%s", campaignID, today())

	bids, _ := s.rdb.Get(ctx, bidsKey).Int64()
	wins, _ := s.rdb.Get(ctx, winsKey).Int64()

	if bids < 100 {
		return 1.0 // not enough data for adjustment
	}

	winRate := float64(wins) / float64(bids)

	if winRate > 0.8 {
		return 0.9 // winning too much, we're overpaying
	}
	if winRate < 0.2 {
		return 1.15 // losing too much, bid higher
	}
	return 1.0 // sweet spot
}

func today() string {
	return time.Now().UTC().Format("2006-01-02")
}

// Simple fast pseudo-random (avoids sync overhead of math/rand)
var randState uint32 = uint32(time.Now().UnixNano())

func fastRand() int {
	randState ^= randState << 13
	randState ^= randState >> 17
	randState ^= randState << 5
	return int(randState)
}
