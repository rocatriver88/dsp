package qaharness

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TestHarness bundles every backing-service client a test needs.
// Use New(t) in a test's setup block; Close is registered via t.Cleanup.
//
// TestHarness is NOT safe for t.Parallel(). Reset() is globally destructive:
// it FLUSHDBs Redis DB 15 and ALTER TABLE DELETEs bid_log rows with
// advertiser_id >= 900000. Two parallel harnesses will wipe each other's
// pre-test state and fail non-deterministically. Keep integration tests
// sequential within a package.
//
// The ClickHouse DELETE is async (mutation-queued); concurrent tests within
// a single sequential run may still see each other's rows for a short window
// after Reset(). Scope queries by a per-test advertiser_id in [900000, ∞) —
// later QA harness helpers (T07) will codify this pattern.
type TestHarness struct {
	Env   *Env
	Ctx   context.Context
	PG    *pgxpool.Pool
	RDB   *redis.Client
	CH    driver.Conn
	TestT *testing.T
}

// New builds a TestHarness and registers a cleanup that closes all clients
// AND runs Reset to purge qa-prefixed test data.
func New(t *testing.T) *TestHarness {
	t.Helper()
	ctx := context.Background()
	env := LoadEnv()

	if env.RedisDB < 10 {
		t.Fatalf("qaharness: QA_REDIS_DB=%d is too low; must be >= 10 to avoid wiping shared data", env.RedisDB)
	}

	pg, err := env.OpenPostgres(ctx)
	if err != nil {
		t.Fatalf("qaharness: open postgres: %v", err)
	}
	rdb := env.OpenRedis()
	if err := rdb.Ping(ctx).Err(); err != nil {
		t.Fatalf("qaharness: ping redis: %v", err)
	}
	ch, err := env.OpenClickHouse()
	if err != nil {
		t.Fatalf("qaharness: open clickhouse: %v", err)
	}
	if err := ch.Ping(ctx); err != nil {
		t.Fatalf("qaharness: ping clickhouse: %v", err)
	}

	h := &TestHarness{
		Env:   env,
		Ctx:   ctx,
		PG:    pg,
		RDB:   rdb,
		CH:    ch,
		TestT: t,
	}

	// Registered before pre-flight Reset so a failing setup still closes connections.
	t.Cleanup(func() {
		if err := h.Reset(); err != nil {
			t.Logf("qaharness: cleanup reset: %v", err)
		}
		pg.Close()
		_ = rdb.Close()
		_ = ch.Close()
	})

	// Purge leftover data from any prior test that crashed without cleanup.
	if err := h.Reset(); err != nil {
		t.Fatalf("qaharness: reset: %v", err)
	}

	return h
}

// Reset purges all qa-prefixed data from Postgres, Redis DB 15, and
// ClickHouse bid_log (advertiser_id >= 900000).
func (h *TestHarness) Reset() error {
	ctx := h.Ctx

	tx, err := h.PG.Begin(ctx)
	if err != nil {
		return fmt.Errorf("qaharness: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		DELETE FROM creatives WHERE campaign_id IN (SELECT id FROM campaigns WHERE name LIKE 'qa-%');
	`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM campaigns WHERE name LIKE 'qa-%'`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM advertisers WHERE company_name LIKE 'qa-%'`); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("qaharness: commit tx: %w", err)
	}

	if err := h.RDB.FlushDB(ctx).Err(); err != nil {
		return err
	}

	// Force the mutation to complete before Reset returns. The default async
	// ALTER TABLE DELETE can race the next test: the previous test's cleanup
	// mutation may still be materializing while the next test inserts fresh QA
	// rows, which then get swept away. `mutations_sync = 1` makes the delete
	// appear synchronous for the harness.
	if err := h.CH.Exec(ctx, `
		ALTER TABLE bid_log
		DELETE WHERE advertiser_id >= 900000
		SETTINGS mutations_sync = 1
	`); err != nil {
		h.TestT.Logf("qaharness: CH delete warning: %v", err)
	}
	return nil
}

// waitForCHMutations polls system.mutations until no in-progress mutations
// on bid_log remain, or the timeout fires. Called from Reset to make the
// async ALTER TABLE DELETE appear synchronous for the next test.
func (h *TestHarness) waitForCHMutations(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var inProgress uint64
		err := h.CH.QueryRow(ctx, `
			SELECT count() FROM system.mutations
			WHERE database = 'default' AND table = 'bid_log' AND is_done = 0
		`).Scan(&inProgress)
		if err != nil {
			return err
		}
		if inProgress == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("CH mutations on bid_log did not finish within %v", timeout)
}
