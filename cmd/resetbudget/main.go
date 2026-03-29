package main

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:6380", Password: "dsp_dev_password"})
	ctx := context.Background()
	date := time.Now().UTC().Format("2006-01-02")

	// Set budgets to actual daily budget values (cents)
	budgets := map[int64]int64{
		11: 50000,   // ¥500/day
		12: 80000,   // ¥800/day
		13: 80000,   // ¥800/day
		14: 200000,  // ¥2000/day
		15: 100000,  // ¥1000/day
	}

	for id, budget := range budgets {
		key := fmt.Sprintf("budget:daily:%d:%s", id, date)
		rdb.Set(ctx, key, budget, 25*time.Hour)
		fmt.Printf("Set %s = %d (¥%.0f)\n", key, budget, float64(budget)/100)
	}

	// Clear frequency caps and win dedup keys
	keys, _ := rdb.Keys(ctx, "freq:*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
		fmt.Printf("Cleared %d frequency keys\n", len(keys))
	}
	keys, _ = rdb.Keys(ctx, "win:dedup:*").Result()
	if len(keys) > 0 {
		rdb.Del(ctx, keys...)
		fmt.Printf("Cleared %d dedup keys\n", len(keys))
	}

	fmt.Println("Budget reset to real values complete")
}
