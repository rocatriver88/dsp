package qaharness

import (
	"context"
	"fmt"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TestHarness bundles every backing-service client a test needs.
// Use New(t) in a test's setup block; Close is registered via t.Cleanup.
//
// TestHarness is NOT safe for t.Parallel(). Reset() is globally destructive:
// it FLUSHDBs Redis DB 15 and wipes the isolated test database's mutable
// business tables. Two parallel harnesses will wipe each other's pre-test
// state and fail non-deterministically. Keep integration tests sequential
// within a package.
//
// The ClickHouse table is truncated as part of Reset, so tests get a clean
// reporting store even after autopilot or prior integration runs seed the
// same isolated stack.
type TestHarness struct {
	Env   *Env
	Ctx   context.Context
	PG    *pgxpool.Pool
	RDB   *redis.Client
	CH    driver.Conn
	TestT *testing.T
}

// New builds a TestHarness and registers a cleanup that closes all clients
// AND runs Reset to purge qa-scoped test data.
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

// Reset purges all mutable data from the isolated Postgres/Redis/ClickHouse
// test stack. This is intentionally broader than "qa-*" cleanup because
// autopilot and other test helpers can seed legitimate-looking rows whose IDs
// are not in the qaharness-owned range.
func (h *TestHarness) Reset() error {
	ctx := h.Ctx

	tx, err := h.PG.Begin(ctx)
	if err != nil {
		return fmt.Errorf("qaharness: begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		TRUNCATE TABLE
			audit_log,
			users,
			invite_codes,
			registration_requests,
			conversions,
			daily_reconciliation,
			invoices,
			transactions,
			creatives,
			campaigns,
			advertisers
		RESTART IDENTITY CASCADE;
	`); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("qaharness: commit tx: %w", err)
	}

	if err := h.RDB.FlushDB(ctx).Err(); err != nil {
		return err
	}

	if err := h.CH.Exec(ctx, `TRUNCATE TABLE bid_log`); err != nil {
		h.TestT.Logf("qaharness: CH delete warning: %v", err)
	}
	return nil
}

