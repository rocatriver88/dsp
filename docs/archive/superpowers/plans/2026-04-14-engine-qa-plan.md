# Engine QA Round Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run a systematic QA round against the engine subsystem (bidder / consumer / events / budget / reconciliation / reporting / attribution), fill in the integration-test gap, fix any bugs found, and produce a test report with data + screenshots as evidence — all merged back via a single PR.

**Architecture:** Build an integration-test harness package `internal/qaharness` that hits the live compose stack on this worktree (ports offset by +12000). Add 7 `//go:build integration` test files. Two minimal refactors to `cmd/bidder/main.go` and `cmd/consumer/main.go` let handlers/readers be reused from tests. Three phases with S2 exit criteria (tests green + no Critical/Important review findings + 4-way data reconciliation).

**Tech Stack:** Go 1.22+, `pgx/v5`, `go-redis/v9`, `segmentio/kafka-go`, `clickhouse-go/v2`, `prebid/openrtb/v20`, `docker compose`.

**Spec source:** `docs/archive/superpowers/specs/2026-04-14-engine-qa-design.md` — every task below references its relevant `§` section. Read the referenced section before starting the task.

**Phase structure and gating:**
- **Phase 0** tasks T01–T08: build `internal/qaharness` infrastructure (no production code changes)
- **Phase 1** tasks T09–T12: data + config base (11 scenarios)
- **Phase 2** tasks T13–T19: bid + settlement e2e (22 scenarios, 2 refactors)
- **Phase 3** tasks T20–T23: consume + read side (10 scenarios)
- **Final** tasks T24–T26: full-branch review, smoke, report, PR

Each Phase ends with a verification-loop task (`exit criteria`) that runs `requesting-code-review` → `verification-before-completion` → manual curl smoke, and loops up to 5 rounds until an entire round is clean (CLAUDE.md hard rule).

---

## Prerequisites (run once before T01)

- [ ] **PR-0: Bring the compose stack up**

Working directory: `C:/Users/Roc/github/dsp/.worktrees/engine`

```bash
docker compose up -d
docker compose ps
```

Expected: every service in `State=running` and `Health=healthy` (postgres-engine, redis-engine, clickhouse-engine, kafka-engine, migrate-engine, api-engine, bidder-engine, consumer-engine, web-engine, prometheus-engine, grafana-engine). If any service is unhealthy, investigate before proceeding — do not start T01 on a broken stack.

- [ ] **PR-1: Sanity-check each backing service**

```bash
# Postgres
psql "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable" -c "SELECT 1"

# Redis
redis-cli -p 18380 PING

# ClickHouse native
clickhouse-client --host localhost --port 21001 -q "SELECT 1"

# Kafka (list topics)
docker exec kafka-engine kafka-topics --bootstrap-server localhost:9094 --list
```

Expected: each returns a trivial success. If any fail, fix before T01 — the rest of the plan depends on all four being reachable.

---

## Phase 0: `internal/qaharness` infrastructure (T01–T08)

Spec reference: `§3.1`, `§3.2`, `§3.3`. No production code is touched in this phase. Every file stays under ~150 lines.

### Task T01: `internal/qaharness/env.go` + `internal/qaharness/harness.go`

**Files:**
- Create: `internal/qaharness/env.go`
- Create: `internal/qaharness/harness.go`

These two files are created together because `harness.go` imports `env.go`'s constructors.

- [ ] **Step 1: Create `env.go`**

```go
package qaharness

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

// Env holds connection parameters for the compose stack of this worktree.
// Defaults match .worktrees/engine/docker-compose.override.yml (+12000 offset).
// Passwords default to the compose stack's dev defaults (dsp_dev_password).
type Env struct {
	PostgresDSN     string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	KafkaBrokers    []string
	ClickHouseAddr  string
	ClickHouseUser  string
	ClickHousePass  string
	BidderPublicURL string
}

func LoadEnv() *Env {
	return &Env{
		PostgresDSN:     getenv("QA_POSTGRES_DSN", "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable"),
		RedisAddr:       getenv("QA_REDIS_ADDR", "localhost:18380"),
		RedisPassword:   getenv("QA_REDIS_PASSWORD", "dsp_dev_password"),
		RedisDB:         getenvInt("QA_REDIS_DB", 15),
		KafkaBrokers:    []string{getenv("QA_KAFKA_BROKERS", "localhost:21094")},
		ClickHouseAddr:  getenv("QA_CLICKHOUSE_ADDR", "localhost:21001"),
		ClickHouseUser:  getenv("QA_CLICKHOUSE_USER", "default"),
		ClickHousePass:  getenv("QA_CLICKHOUSE_PASSWORD", "dsp_dev_password"),
		BidderPublicURL: getenv("QA_BIDDER_PUBLIC_URL", "http://localhost:20180"),
	}
}

func (e *Env) OpenPostgres(ctx context.Context) (*pgxpool.Pool, error) {
	return pgxpool.New(ctx, e.PostgresDSN)
}

func (e *Env) OpenRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     e.RedisAddr,
		Password: e.RedisPassword,
		DB:       e.RedisDB,
	})
}

func (e *Env) OpenClickHouse() (driver.Conn, error) {
	return clickhouse.Open(&clickhouse.Options{
		Addr: []string{e.ClickHouseAddr},
		Auth: clickhouse.Auth{Database: "default", Username: e.ClickHouseUser, Password: e.ClickHousePass},
	})
}

func (e *Env) KafkaDialer() *kafka.Dialer {
	return &kafka.Dialer{Timeout: 5_000_000_000}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// MustString panics with a testing message if s is empty.
// Used to fail fast when an env var was expected but missing.
func MustString(s, name string) string {
	if s == "" {
		panic(fmt.Sprintf("qaharness: %s is empty", name))
	}
	return s
}
```

- [ ] **Step 2: Create `harness.go`**

```go
package qaharness

import (
	"context"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// TestHarness bundles every backing-service client a test needs.
// Use New(t) in a test's setup block; Close is registered via t.Cleanup.
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

	t.Cleanup(func() {
		_ = h.Reset()
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

	if _, err := h.PG.Exec(ctx, `
		DELETE FROM creatives WHERE campaign_id IN (SELECT id FROM campaigns WHERE name LIKE 'qa-%');
	`); err != nil {
		return err
	}
	if _, err := h.PG.Exec(ctx, `DELETE FROM campaigns WHERE name LIKE 'qa-%'`); err != nil {
		return err
	}
	if _, err := h.PG.Exec(ctx, `DELETE FROM advertisers WHERE company_name LIKE 'qa-%'`); err != nil {
		return err
	}

	if err := h.RDB.FlushDB(ctx).Err(); err != nil {
		return err
	}

	// ClickHouse DELETE is async; Reset does not wait.
	if err := h.CH.Exec(ctx, `ALTER TABLE bid_log DELETE WHERE advertiser_id >= 900000`); err != nil {
		// Do not fail the test — mutations may be queued from previous runs.
		h.TestT.Logf("qaharness: CH delete warning: %v", err)
	}
	return nil
}
```

- [ ] **Step 3: Verify it builds**

Run: `go vet ./internal/qaharness/...`
Expected: no output (success).

- [ ] **Step 4: Commit**

```bash
git add internal/qaharness/env.go internal/qaharness/harness.go
git commit -m "feat(qaharness): env + TestHarness skeleton for engine QA round"
```

---

### Task T02: `internal/qaharness/campaign.go`

**Files:**
- Create: `internal/qaharness/campaign.go`

- [ ] **Step 1: Create `campaign.go`**

```go
package qaharness

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/heartgryphon/dsp/internal/campaign"
)

// SeedAdvertiser inserts a qa-prefixed advertiser and returns its id.
// id is constrained to the 900000+ range so ClickHouse Reset can target it.
func (h *TestHarness) SeedAdvertiser(name string) int64 {
	h.TestT.Helper()
	if name == "" {
		name = "qa-adv"
	}
	id := int64(900000 + rand.Int63n(99999))
	_, err := h.PG.Exec(h.Ctx, `
		INSERT INTO advertisers (id, company_name, contact_email, api_key, balance_cents, billing_type)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`, id, fmt.Sprintf("qa-%s-%d", name, id), fmt.Sprintf("qa-%d@test.local", id),
		fmt.Sprintf("qa-key-%d", id), int64(10_000_000), "prepay")
	if err != nil {
		h.TestT.Fatalf("SeedAdvertiser: %v", err)
	}
	return id
}

// CampaignSpec describes a campaign to seed. Zero values get safe defaults.
type CampaignSpec struct {
	AdvertiserID     int64
	Name             string
	Status           campaign.Status // default active
	BillingModel     string          // default cpm
	BidCPMCents      int             // default 1000 (10 CNY/CPM)
	BidCPCCents      int
	OCPMTargetCPACents int
	BudgetDailyCents int64 // default 100_000 (1000 CNY/day)
	TargetingGeo     []string // default ["CN"]
	TargetingOS      []string // default ["iOS"]
	FreqCap          int
	FreqPeriodHours  int
}

// SeedCampaign inserts a qa-prefixed campaign and returns its id.
// Also publishes campaign:updates with action=activated so the live bidder (if
// running) picks it up; tests may still rely on full reload for tighter timing.
func (h *TestHarness) SeedCampaign(spec CampaignSpec) int64 {
	h.TestT.Helper()
	if spec.Status == "" {
		spec.Status = campaign.StatusActive
	}
	if spec.BillingModel == "" {
		spec.BillingModel = campaign.BillingCPM
	}
	if spec.BidCPMCents == 0 && spec.BillingModel == campaign.BillingCPM {
		spec.BidCPMCents = 1000
	}
	if spec.BudgetDailyCents == 0 {
		spec.BudgetDailyCents = 100_000
	}
	if spec.TargetingGeo == nil {
		spec.TargetingGeo = []string{"CN"}
	}
	if spec.TargetingOS == nil {
		spec.TargetingOS = []string{"iOS"}
	}
	if spec.Name == "" {
		spec.Name = fmt.Sprintf("qa-camp-%d", time.Now().UnixNano())
	}

	targeting := map[string]any{
		"geo": spec.TargetingGeo,
		"os":  spec.TargetingOS,
	}
	if spec.FreqCap > 0 {
		targeting["frequency_cap"] = map[string]any{
			"count":        spec.FreqCap,
			"period_hours": spec.FreqPeriodHours,
		}
	}
	tjson, _ := json.Marshal(targeting)

	var id int64
	err := h.PG.QueryRow(h.Ctx, `
		INSERT INTO campaigns
		  (advertiser_id, name, status, billing_model,
		   budget_total_cents, budget_daily_cents,
		   bid_cpm_cents, bid_cpc_cents, ocpm_target_cpa_cents,
		   targeting, sandbox)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,false)
		RETURNING id
	`, spec.AdvertiserID, spec.Name, string(spec.Status), spec.BillingModel,
		spec.BudgetDailyCents*30, spec.BudgetDailyCents,
		spec.BidCPMCents, spec.BidCPCCents, spec.OCPMTargetCPACents,
		tjson).Scan(&id)
	if err != nil {
		h.TestT.Fatalf("SeedCampaign: %v", err)
	}
	// Pre-seed Redis daily budget so bidder's PipelineCheck has something to deduct from.
	key := fmt.Sprintf("budget:daily:%d:%s", id, time.Now().Format("2006-01-02"))
	if err := h.RDB.Set(h.Ctx, key, spec.BudgetDailyCents, 25*time.Hour).Err(); err != nil {
		h.TestT.Fatalf("SeedCampaign: init budget: %v", err)
	}
	return id
}

// SeedCreative adds a qa creative to a campaign.
func (h *TestHarness) SeedCreative(campaignID int64, adMarkup, destURL string) int64 {
	h.TestT.Helper()
	if adMarkup == "" {
		adMarkup = `<a href="${CLICK_URL}">qa-creative</a>`
	}
	if destURL == "" {
		destURL = "https://qa.example.invalid/landing"
	}
	var id int64
	err := h.PG.QueryRow(h.Ctx, `
		INSERT INTO creatives
		  (campaign_id, name, ad_format, width, height, ad_markup, destination_url, status)
		VALUES ($1, 'qa-creative', 'banner', 320, 50, $2, $3, 'approved')
		RETURNING id
	`, campaignID, adMarkup, destURL).Scan(&id)
	if err != nil {
		h.TestT.Fatalf("SeedCreative: %v", err)
	}
	return id
}

// UpdateCampaignStatus flips a campaign to a new status.
func (h *TestHarness) UpdateCampaignStatus(id int64, status campaign.Status) {
	h.TestT.Helper()
	_, err := h.PG.Exec(h.Ctx, `UPDATE campaigns SET status=$1 WHERE id=$2`, string(status), id)
	if err != nil {
		h.TestT.Fatalf("UpdateCampaignStatus: %v", err)
	}
}

var _ = context.Background
```

Note: `rand` is math/rand, OK for test id generation. The `var _ = context.Background` is a deliberate import anchor so `context` stays imported if refactors remove other usages — remove it once other `h.Ctx` usages are present (it already is, so delete that trailing line before committing).

- [ ] **Step 2: Remove the placeholder line**

Delete the last line `var _ = context.Background`. It was only defensive padding.

- [ ] **Step 3: Verify build**

Run: `go vet ./internal/qaharness/...`
Expected: clean.

- [ ] **Step 4: Commit**

```bash
git add internal/qaharness/campaign.go
git commit -m "feat(qaharness): SeedAdvertiser/SeedCampaign/SeedCreative helpers"
```

---

### Task T03: `internal/qaharness/openrtb.go`

**Files:**
- Create: `internal/qaharness/openrtb.go`

- [ ] **Step 1: Create `openrtb.go`**

```go
package qaharness

import (
	"fmt"
	"time"

	"github.com/prebid/openrtb/v20/openrtb2"
)

// BuildBidRequestOpts parameterizes BuildBidRequest.
type BuildBidRequestOpts struct {
	ID        string
	Geo       string
	OS        string
	IFA       string
	Secure    bool
	BidFloor  float64 // in imp-level CNY
	Format    string  // "banner" (default), "video", "native"
	W, H      int64
}

// BuildBidRequest constructs a minimal OpenRTB 2.5 BidRequest suitable for
// hitting Engine.Bid. Defaults target CN + iOS banner 320x50 + secure=0.
func BuildBidRequest(opts BuildBidRequestOpts) *openrtb2.BidRequest {
	if opts.ID == "" {
		opts.ID = fmt.Sprintf("qa-req-%d", time.Now().UnixNano())
	}
	if opts.Geo == "" {
		opts.Geo = "CN"
	}
	if opts.OS == "" {
		opts.OS = "iOS"
	}
	if opts.IFA == "" {
		opts.IFA = "qa-ifa-" + opts.ID
	}
	if opts.Format == "" {
		opts.Format = "banner"
	}
	if opts.W == 0 {
		opts.W = 320
	}
	if opts.H == 0 {
		opts.H = 50
	}

	var secureVal int8
	if opts.Secure {
		secureVal = 1
	}

	imp := openrtb2.Imp{
		ID:       "imp-1",
		BidFloor: opts.BidFloor,
		Secure:   &secureVal,
	}
	switch opts.Format {
	case "video":
		imp.Video = &openrtb2.Video{W: &opts.W, H: &opts.H}
	case "native":
		imp.Native = &openrtb2.Native{Request: `{"ver":"1.2"}`}
	default:
		imp.Banner = &openrtb2.Banner{W: &opts.W, H: &opts.H}
	}

	return &openrtb2.BidRequest{
		ID:  opts.ID,
		Imp: []openrtb2.Imp{imp},
		Device: &openrtb2.Device{
			OS:  opts.OS,
			IFA: opts.IFA,
			Geo: &openrtb2.Geo{Country: opts.Geo},
			IP:  "203.0.113.1",
			UA:  "qa-test-agent/1.0",
		},
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go vet ./internal/qaharness/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/qaharness/openrtb.go
git commit -m "feat(qaharness): BuildBidRequest helper for OpenRTB 2.5 test input"
```

---

### Task T04: `internal/qaharness/kafka.go`

**Files:**
- Create: `internal/qaharness/kafka.go`

- [ ] **Step 1: Create `kafka.go`**

```go
package qaharness

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/segmentio/kafka-go"
)

// ReadMessagesFrom starts a fresh consumer at LastOffset and returns up to
// `count` messages whose request_id starts with `reqPrefix`, or error on timeout.
// Use a unique reqPrefix per test to avoid cross-test contamination.
func (h *TestHarness) ReadMessagesFrom(topic, reqPrefix string, count int, timeout time.Duration) []events.Event {
	h.TestT.Helper()
	groupID := fmt.Sprintf("qa-test-%s-%d", topic, time.Now().UnixNano())
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  h.Env.KafkaBrokers,
		Topic:    topic,
		GroupID:  groupID,
		MinBytes: 1,
		MaxBytes: 10_000_000,
		StartOffset: kafka.LastOffset,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(h.Ctx, timeout)
	defer cancel()

	var collected []events.Event
	for len(collected) < count {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			h.TestT.Fatalf("ReadMessagesFrom %s: waited for %d, got %d: %v",
				topic, count, len(collected), err)
		}
		var evt events.Event
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			continue
		}
		if reqPrefix != "" && !strings.HasPrefix(evt.RequestID, reqPrefix) {
			continue
		}
		collected = append(collected, evt)
	}
	return collected
}

// CountMessages returns how many events on topic match reqPrefix within timeout.
// Returns as soon as no new messages arrive for 500ms of idle time.
func (h *TestHarness) CountMessages(topic, reqPrefix string, timeout time.Duration) int {
	h.TestT.Helper()
	groupID := fmt.Sprintf("qa-count-%s-%d", topic, time.Now().UnixNano())
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     h.Env.KafkaBrokers,
		Topic:       topic,
		GroupID:     groupID,
		MinBytes:    1,
		MaxBytes:    10_000_000,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	defer reader.Close()

	ctx, cancel := context.WithTimeout(h.Ctx, timeout)
	defer cancel()

	n := 0
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			return n
		}
		var evt events.Event
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			continue
		}
		if reqPrefix == "" || strings.HasPrefix(evt.RequestID, reqPrefix) {
			n++
		}
	}
}
```

Note: `ReadMessagesFrom` uses `StartOffset: kafka.LastOffset` to skip history. `CountMessages` uses `FirstOffset` to scan all accumulated messages for assertions across a full test run.

- [ ] **Step 2: Verify build**

Run: `go vet ./internal/qaharness/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/qaharness/kafka.go
git commit -m "feat(qaharness): Kafka read/count helpers filtered by request_id prefix"
```

---

### Task T05: `internal/qaharness/clickhouse.go`

**Files:**
- Create: `internal/qaharness/clickhouse.go`

- [ ] **Step 1: Create `clickhouse.go`**

```go
package qaharness

import (
	"context"
	"fmt"
	"time"
)

// WaitForBidLogRows polls bid_log until it sees `want` rows matching the given
// (campaignID, eventType) pair, or fails the test on timeout.
func (h *TestHarness) WaitForBidLogRows(campaignID int64, eventType string, want int, timeout time.Duration) {
	h.TestT.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var n uint64
		row := h.CH.QueryRow(h.Ctx, `
			SELECT count() FROM bid_log
			WHERE campaign_id = ? AND event_type = ?
		`, uint64(campaignID), eventType)
		if err := row.Scan(&n); err != nil {
			h.TestT.Fatalf("WaitForBidLogRows: query: %v", err)
		}
		if int(n) >= want {
			return
		}
		if time.Now().After(deadline) {
			h.TestT.Fatalf("WaitForBidLogRows: campaign=%d type=%s want=%d got=%d (timeout)",
				campaignID, eventType, want, n)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// QueryCampaignSpend returns sum(charge_cents) for a campaign across event_types.
// If eventType is "", aggregates all types.
func (h *TestHarness) QueryCampaignSpend(campaignID int64, eventType string) uint64 {
	h.TestT.Helper()
	var q string
	var args []any
	if eventType == "" {
		q = `SELECT sum(charge_cents) FROM bid_log WHERE campaign_id = ?`
		args = []any{uint64(campaignID)}
	} else {
		q = `SELECT sum(charge_cents) FROM bid_log WHERE campaign_id = ? AND event_type = ?`
		args = []any{uint64(campaignID), eventType}
	}
	var sum uint64
	if err := h.CH.QueryRow(h.Ctx, q, args...).Scan(&sum); err != nil {
		h.TestT.Fatalf("QueryCampaignSpend: %v", err)
	}
	return sum
}

// InsertBidLogRow inserts a synthetic row directly. Used by tests that want to
// bypass the bidder/consumer pipeline and seed ClickHouse with a specific event.
func (h *TestHarness) InsertBidLogRow(
	campaignID, advertiserID, creativeID int64,
	eventType, requestID, deviceID string,
	bidPriceCents, clearPriceCents, chargeCents uint32,
	when time.Time,
) {
	h.TestT.Helper()
	err := h.CH.Exec(h.Ctx, `
		INSERT INTO bid_log (
			event_date, event_time, campaign_id, creative_id, advertiser_id,
			exchange_id, request_id, geo_country, device_os, device_id,
			bid_price_cents, clear_price_cents, charge_cents, event_type, loss_reason
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, when, when, uint64(campaignID), uint64(creativeID), uint64(advertiserID),
		"qa-exchange", requestID, "CN", "iOS", deviceID,
		bidPriceCents, clearPriceCents, chargeCents, eventType, "")
	if err != nil {
		h.TestT.Fatalf("InsertBidLogRow: %v", err)
	}
}

var _ = context.Background // anchor for context import; remove if other uses add up
```

- [ ] **Step 2: Delete the anchor line**

Remove the `var _ = context.Background` line once the file compiles.

- [ ] **Step 3: Verify build**

Run: `go vet ./internal/qaharness/...`
Expected: clean. If `context` becomes unused, remove the import — no file in this package should end up importing a package it doesn't use.

- [ ] **Step 4: Commit**

```bash
git add internal/qaharness/clickhouse.go
git commit -m "feat(qaharness): ClickHouse bid_log wait/query/insert helpers"
```

---

### Task T06: `internal/qaharness/redis.go`

**Files:**
- Create: `internal/qaharness/redis.go`

- [ ] **Step 1: Create `redis.go`**

```go
package qaharness

import (
	"encoding/json"
	"fmt"
	"time"
)

// GetBudgetRemaining reads budget:daily:{campaign_id}:{today}. Returns 0 if unset.
func (h *TestHarness) GetBudgetRemaining(campaignID int64) int64 {
	h.TestT.Helper()
	key := fmt.Sprintf("budget:daily:%d:%s", campaignID, time.Now().Format("2006-01-02"))
	v, err := h.RDB.Get(h.Ctx, key).Int64()
	if err != nil {
		return 0
	}
	return v
}

// SetBudgetRemaining forces budget:daily to a specific value with 25h TTL.
func (h *TestHarness) SetBudgetRemaining(campaignID int64, cents int64) {
	h.TestT.Helper()
	key := fmt.Sprintf("budget:daily:%d:%s", campaignID, time.Now().Format("2006-01-02"))
	if err := h.RDB.Set(h.Ctx, key, cents, 25*time.Hour).Err(); err != nil {
		h.TestT.Fatalf("SetBudgetRemaining: %v", err)
	}
}

// GetFreqCount returns the value of freq:{campaign}:{user}, or 0 if unset.
func (h *TestHarness) GetFreqCount(campaignID int64, userID string) int64 {
	h.TestT.Helper()
	key := fmt.Sprintf("freq:%d:%s", campaignID, userID)
	v, err := h.RDB.Get(h.Ctx, key).Int64()
	if err != nil {
		return 0
	}
	return v
}

// PublishCampaignUpdate publishes to campaign:updates with the given action.
func (h *TestHarness) PublishCampaignUpdate(campaignID int64, action string) {
	h.TestT.Helper()
	payload, _ := json.Marshal(map[string]any{
		"campaign_id": campaignID,
		"action":      action,
	})
	if err := h.RDB.Publish(h.Ctx, "campaign:updates", payload).Err(); err != nil {
		h.TestT.Fatalf("PublishCampaignUpdate: %v", err)
	}
}

// PublishRaw publishes an arbitrary raw string, used to test malformed payload handling.
func (h *TestHarness) PublishRaw(channel, raw string) {
	h.TestT.Helper()
	if err := h.RDB.Publish(h.Ctx, channel, raw).Err(); err != nil {
		h.TestT.Fatalf("PublishRaw: %v", err)
	}
}
```

- [ ] **Step 2: Verify build**

Run: `go vet ./internal/qaharness/...`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add internal/qaharness/redis.go
git commit -m "feat(qaharness): Redis budget/freq/pubsub helpers"
```

---

### Task T07: `internal/qaharness/assert.go` + smoke test

**Files:**
- Create: `internal/qaharness/assert.go`
- Create: `internal/qaharness/harness_smoke_test.go`

- [ ] **Step 1: Create `assert.go`**

```go
package qaharness

import (
	"time"
)

// AssertKafkaEqCH checks that the count of Kafka messages matches CH row count
// for a (campaign, eventType) tuple. Uses CountMessages + WaitForBidLogRows.
func (h *TestHarness) AssertKafkaEqCH(topic, reqPrefix string, campaignID int64, eventType string, want int) {
	h.TestT.Helper()
	kafkaN := h.CountMessages(topic, reqPrefix, 3*time.Second)
	if kafkaN != want {
		h.TestT.Fatalf("Kafka count mismatch: topic=%s prefix=%s want=%d got=%d",
			topic, reqPrefix, want, kafkaN)
	}
	h.WaitForBidLogRows(campaignID, eventType, want, 5*time.Second)
}

// AssertBudgetDelta checks that (before - after) equals an expected delta in cents.
// Tolerance of ±1 cent absorbs fencepost rounding in float math.
func (h *TestHarness) AssertBudgetDelta(campaignID int64, before int64, wantDeltaCents int64) {
	h.TestT.Helper()
	after := h.GetBudgetRemaining(campaignID)
	got := before - after
	diff := got - wantDeltaCents
	if diff < -1 || diff > 1 {
		h.TestT.Fatalf("Budget delta mismatch: before=%d after=%d want=%d got=%d",
			before, after, wantDeltaCents, got)
	}
}

// AssertSpendConsistency checks Redis budget delta == CH sum(charge_cents) for win events.
func (h *TestHarness) AssertSpendConsistency(campaignID int64, before int64) {
	h.TestT.Helper()
	after := h.GetBudgetRemaining(campaignID)
	redisDelta := uint64(before - after)
	chSpend := h.QueryCampaignSpend(campaignID, "win")
	if redisDelta != chSpend {
		h.TestT.Fatalf("Spend inconsistency: redisDelta=%d CH(win)=%d", redisDelta, chSpend)
	}
}
```

- [ ] **Step 2: Create smoke test `harness_smoke_test.go`**

```go
//go:build integration

package qaharness_test

import (
	"testing"

	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

func TestHarnessSmoke(t *testing.T) {
	h := qaharness.New(t)

	// Seed an advertiser + campaign + creative
	advID := h.SeedAdvertiser("smoke")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-smoke-campaign",
		BidCPMCents:  500,
	})
	h.SeedCreative(campID, "", "")

	// Budget pre-seeded by SeedCampaign
	if got := h.GetBudgetRemaining(campID); got != 100_000 {
		t.Errorf("expected 100000, got %d", got)
	}

	// Flip status and verify Postgres row updated
	h.UpdateCampaignStatus(campID, campaign.StatusPaused)
	var status string
	if err := h.PG.QueryRow(h.Ctx, `SELECT status FROM campaigns WHERE id=$1`, campID).Scan(&status); err != nil {
		t.Fatalf("query status: %v", err)
	}
	if status != "paused" {
		t.Errorf("expected paused, got %s", status)
	}

	// Reset is called automatically on cleanup.
}
```

- [ ] **Step 3: Run the smoke test**

```bash
go test -tags=integration -v -run TestHarnessSmoke ./internal/qaharness/...
```

Expected: PASS. If anything fails, investigate before proceeding — T08 onward depends on this smoke passing.

- [ ] **Step 4: Commit**

```bash
git add internal/qaharness/assert.go internal/qaharness/harness_smoke_test.go
git commit -m "feat(qaharness): assertion helpers + smoke test"
```

---

### Task T08: `internal/qaharness` self-review

**Files:**
- Read all: `internal/qaharness/*.go`

- [ ] **Step 1: Spec compliance review**

Compare every file against spec `§3.1`:
- [ ] Every helper promised by spec is present
- [ ] No file exceeds ~150 lines
- [ ] Only `*_integration_test.go` consumers will use this package (no production code imports)

Run:
```bash
grep -rn "heartgryphon/dsp/internal/qaharness" cmd/ internal/ 2>/dev/null | grep -v '_test.go'
```
Expected: empty. If any production file imports qaharness, remove the import.

- [ ] **Step 2: Code quality review**

Run `requesting-code-review` on the qaharness package diff. Fix Critical/Important findings.

- [ ] **Step 3: Verification — run integration smoke against the compose stack**

```bash
go test -tags=integration -v -timeout 2m ./internal/qaharness/...
```
Expected: `TestHarnessSmoke PASS`. No residual qa-% rows in Postgres (`TestHarness.Cleanup` should have purged them).

Verify no residue:
```bash
psql "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable" -c "SELECT count(*) FROM campaigns WHERE name LIKE 'qa-%';"
```
Expected: `0`.

- [ ] **Step 4: Commit if any fixes were needed**

```bash
git add -A internal/qaharness/
git commit -m "chore(qaharness): fixes from phase 0 self-review"
```

Or skip if no changes.

---

## Phase 1: Data + Config Base (T09–T12)

Spec reference: `§4`. 11 scenarios in 2 test files.

### Task T09: `loader_integration_test.go` — Scenarios 1-7

**Files:**
- Create: `internal/bidder/loader_integration_test.go`

**Scenarios covered**: 1, 2, 3a/3b/3c, 4, 5, 6, 7 from spec `§4.1` and `§4.2`.

- [ ] **Step 1: Read spec `§4.1` and `§4.2` fully**

All 7 scenario definitions + S2 assertions must be loaded into the implementer's context.

- [ ] **Step 2: Write the test file with scenario 1 as complete pattern**

```go
//go:build integration

package bidder_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// Scenario 1 — startup full load: seed 3 active + 2 paused + 1 draft,
// start loader, expect exactly 3 in the cache.
func TestLoader_InitialFullLoad(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("loader")

	active := make([]int64, 0, 3)
	for i := 0; i < 3; i++ {
		id := h.SeedCampaign(qaharness.CampaignSpec{
			AdvertiserID: advID,
			Name:         fmt.Sprintf("qa-loader-active-%d", i),
			BidCPMCents:  1000,
		})
		h.SeedCreative(id, "", "")
		active = append(active, id)
	}
	paused := []int64{
		h.SeedCampaign(qaharness.CampaignSpec{AdvertiserID: advID, Name: "qa-loader-paused-a", Status: campaign.StatusPaused, BidCPMCents: 1000}),
		h.SeedCampaign(qaharness.CampaignSpec{AdvertiserID: advID, Name: "qa-loader-paused-b", Status: campaign.StatusPaused, BidCPMCents: 1000}),
	}
	_ = h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID, Name: "qa-loader-draft", Status: campaign.StatusDraft, BidCPMCents: 1000,
	})

	cl := bidder.NewCampaignLoader(h.PG, h.RDB)
	ctx, cancel := context.WithCancel(h.Ctx)
	defer cancel()
	if err := cl.Start(ctx); err != nil {
		t.Fatalf("loader start: %v", err)
	}
	defer cl.Stop()

	cached := cl.GetActiveCampaigns()
	if len(cached) != 3 {
		t.Fatalf("expected 3 active, got %d", len(cached))
	}

	seen := map[int64]bool{}
	for _, c := range cached {
		seen[c.ID] = true
	}
	for _, id := range active {
		if !seen[id] {
			t.Errorf("active campaign %d missing from cache", id)
		}
	}
	for _, id := range paused {
		if seen[id] {
			t.Errorf("paused campaign %d should not be in cache", id)
		}
	}
}
```

- [ ] **Step 3: Implement scenarios 2-7 in the same file**

Each follows the same shape: build harness, seed data, start loader, act, assert cache state. Key specifics per spec:

- **Scenario 2 (`pub/sub activated`)**: seed paused → `UpdateCampaignStatus(id, StatusActive)` → `PublishCampaignUpdate(id, "activated")` → poll `cl.GetCampaign(id)` up to 1s, expect non-nil.
- **Scenario 3a/3b/3c (`paused`/`completed`/`deleted`)**: run as 3 subtests (`t.Run`). Each seeds an active campaign + creative, publishes the action, polls `GetCampaign(id)` up to 1s, expects nil.
- **Scenario 4 (`updated`)**: seed active with `TargetingGeo=[]string{"US"}` → direct DB update to geo=[CN] (via a one-off `UPDATE campaigns SET targeting = ...`) → `PublishCampaignUpdate(id, "updated")` → poll until `cl.GetCampaign(id).Targeting.Geo[0] == "CN"`.
- **Scenario 5 (`30s reload fallback`)**: seed an `active` campaign but do **not** publish pub/sub. Wait `35 * time.Second`. Assert `cl.GetCampaign(id)` non-nil afterwards. Mark test with a comment `// slow: 35s wait, fallback reload interval`.
- **Scenario 6 (`bad JSON payload`)**: `h.PublishRaw("campaign:updates", "{not json")` → assert no panic (test continues) → publish a valid `updated` message for a different campaign and assert it still works.
- **Scenario 7 (`unknown action`)**: publish a valid JSON with `action="weird"` → assert cache unchanged 500ms later.

Poll helper to avoid flakiness:

```go
func waitForCache(t *testing.T, cl *bidder.CampaignLoader, id int64, want bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := cl.GetCampaign(id) != nil
		if got == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("waitForCache: id=%d want=%v after %v", id, want, timeout)
}
```

- [ ] **Step 4: Run the tests**

```bash
go test -tags=integration -v -timeout 3m -run TestLoader ./internal/bidder/...
```

Expected: all 7 subtests PASS. Scenario 5 takes ~35s — the 3m timeout accommodates it. If any fail:

1. Apply `superpowers:systematic-debugging` — identify root cause
2. If the cause is a bug in `internal/bidder/loader.go`, fix it in place (record the fix in the candidate bug table in the report later)
3. If the cause is a test wiring error, fix the test
4. Re-run until PASS

- [ ] **Step 5: Commit**

```bash
git add internal/bidder/loader_integration_test.go
# + any loader.go fixes
git commit -m "test(bidder): integration tests for CampaignLoader sync (P1 scenarios 1-7)"
```

---

### Task T10: `budget_integration_test.go` — Scenarios 8-11

**Files:**
- Create: `internal/budget/budget_integration_test.go`

- [ ] **Step 1: Read spec `§4.3` and `§4.4` fully**

- [ ] **Step 2: Write the test file**

```go
//go:build integration

package budget_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// Scenario 8 — simple deduct.
func TestBudget_SimpleDeduct(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900001)
	svc := budget.New(h.RDB)

	if err := svc.InitDailyBudget(h.Ctx, campID, 10_000); err != nil {
		t.Fatal(err)
	}
	remain, err := svc.CheckAndDeductBudget(h.Ctx, campID, 300)
	if err != nil {
		t.Fatal(err)
	}
	if remain != 9700 {
		t.Errorf("expected 9700, got %d", remain)
	}
	if got := h.GetBudgetRemaining(campID); got != 9700 {
		t.Errorf("redis GET returned %d, expected 9700", got)
	}
}

// Scenario 9 — exhaustion returns -1 without over-deducting.
func TestBudget_Exhaustion(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900002)
	svc := budget.New(h.RDB)

	_ = svc.InitDailyBudget(h.Ctx, campID, 100)
	for i, amt := range []int64{50, 50} {
		r, err := svc.CheckAndDeductBudget(h.Ctx, campID, amt)
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if r < 0 {
			t.Fatalf("step %d unexpected -1", i)
		}
	}
	r, err := svc.CheckAndDeductBudget(h.Ctx, campID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r != -1 {
		t.Errorf("expected -1 on third deduct, got %d", r)
	}
	if got := h.GetBudgetRemaining(campID); got != 0 {
		t.Errorf("budget should be 0 after exhaustion, got %d", got)
	}
}

// Scenario 10 — 100 concurrent deducts, exact math.
func TestBudget_ConcurrentAtomicity(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900003)
	svc := budget.New(h.RDB)

	_ = svc.InitDailyBudget(h.Ctx, campID, 10_000)

	var successes, failures atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			r, err := svc.CheckAndDeductBudget(ctx, campID, 50)
			if err != nil {
				failures.Add(1)
				return
			}
			if r < 0 {
				failures.Add(1)
			} else {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()

	total := successes.Load() + failures.Load()
	if total != 100 {
		t.Fatalf("expected 100 total, got %d", total)
	}
	expectedRemaining := int64(10_000) - 50*successes.Load()
	if got := h.GetBudgetRemaining(campID); got != expectedRemaining {
		t.Errorf("budget mismatch: expected %d (10000 - 50 * %d successes), got %d",
			expectedRemaining, successes.Load(), got)
	}
	fmt.Printf("concurrent test: successes=%d failures=%d remaining=%d\n",
		successes.Load(), failures.Load(), h.GetBudgetRemaining(campID))
}

// Scenario 11 — PipelineCheck rolls budget back when freq cap is exceeded.
func TestBudget_PipelineFreqRollback(t *testing.T) {
	h := qaharness.New(t)
	campID := int64(900004)
	svc := budget.New(h.RDB)
	_ = svc.InitDailyBudget(h.Ctx, campID, 10_000)

	user := "qa-user-1"

	// 1st call: both OK; budget becomes 9950
	bOK, fOK, err := svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil || !bOK || !fOK {
		t.Fatalf("1st: err=%v bOK=%v fOK=%v", err, bOK, fOK)
	}
	// 2nd call: both OK; budget becomes 9900
	bOK, fOK, err = svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil || !bOK || !fOK {
		t.Fatalf("2nd: err=%v bOK=%v fOK=%v", err, bOK, fOK)
	}
	// 3rd call: freq cap hit, budget rolled back.
	bOK, fOK, err = svc.PipelineCheck(h.Ctx, campID, user, 50, 2, 24)
	if err != nil {
		t.Fatal(err)
	}
	if fOK {
		t.Error("expected freq cap to block 3rd call")
	}
	if got := h.GetBudgetRemaining(campID); got != 9900 {
		t.Errorf("budget should remain at 9900 after freq rollback, got %d", got)
	}
	if got := h.GetFreqCount(campID, user); got != 3 {
		t.Errorf("freq count should be 3, got %d", got)
	}
}
```

- [ ] **Step 3: Run the tests**

```bash
go test -tags=integration -v -timeout 2m ./internal/budget/...
```

Expected: all 4 tests PASS. If the concurrent test shows non-zero failures (meaning some deducts returned -1 before total hit 10000), that is **expected behavior** — the Lua script MUST reject the over-budget attempts. What must NOT happen is the final `remaining != 10000 - 50 * successes` (that would indicate a race).

- [ ] **Step 4: Commit**

```bash
git add internal/budget/budget_integration_test.go
git commit -m "test(budget): integration tests for Lua atomicity + freq rollback (P1 scenarios 8-11)"
```

---

### Task T11: Phase 1 exit criteria verification (review loop)

**Files:**
- Read all: `internal/bidder/loader_integration_test.go`, `internal/budget/budget_integration_test.go`, plus any loader/budget fix diffs

- [ ] **Step 1: Run the full Phase 1 suite**

```bash
go test -tags=integration -timeout 5m ./internal/bidder/... ./internal/budget/...
```

Expected: 11 test cases (`TestLoader_*` + `TestBudget_*`) all PASS. Total wallclock ~40s (scenario 5's 35s wait dominates).

- [ ] **Step 2: Run `requesting-code-review` on the Phase 1 diff**

Scope = `git diff origin/main -- internal/bidder/loader_integration_test.go internal/bidder/loader.go internal/budget/budget_integration_test.go internal/budget/budget.go internal/qaharness/`.

Fix Critical/Important findings. If a fix changes production code under loader/budget, the next step is mandatory.

- [ ] **Step 3: Residue check**

```bash
psql "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable" -c "SELECT count(*) FROM campaigns WHERE name LIKE 'qa-%';"
redis-cli -p 18380 -n 15 DBSIZE
```
Expected: both `0`.

- [ ] **Step 4: Loop decision**

If step 2 or 3 found any issue, loop: go back to Step 1 and run an entire round from scratch. Maximum 5 rounds. If any round's 3 steps all return clean, Phase 1 is closed.

- [ ] **Step 5: Update report draft**

Create `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md` if it does not exist; otherwise append a `## Phase 1 Result` section including:
- The 11-scenario pass/fail table
- Any bug found + fix commit SHA
- Link to `data/p1-budget-lua-concurrency.txt` (capture the stdout of `TestBudget_ConcurrentAtomicity` via `go test -v`)

Commit the report draft:

```bash
git add docs/archive/superpowers/reports/
git commit -m "docs(qa-report): phase 1 results"
```

---

## Phase 2: Bid + Settlement e2e (T12–T19)

Spec reference: `§5`. 22 scenarios + 2 refactors.

### Task T12: Refactor `cmd/bidder/main.go` — extract `RegisterRoutes`

**Files:**
- Modify: `cmd/bidder/main.go`
- Create: `cmd/bidder/routes.go`

The goal: enable `httptest` to serve the exact same handlers the production binary uses. Currently handler funcs reference package-level globals (`engine`, `budgetSvc`, `loader`, `rdb`, `producer`, `strategySvc`, `guard`, `exchangeRegistry`). We collapse those into a `Deps` struct.

- [ ] **Step 1: Create `cmd/bidder/routes.go` with the new structure**

```go
package main

import (
	"net/http"

	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/redis/go-redis/v9"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Deps is the full set of collaborators every handler needs.
// Production main() constructs one; integration tests construct one with test clients.
type Deps struct {
	Engine           *bidder.Engine
	BudgetSvc        *budget.Service
	StrategySvc      *bidder.BidStrategy
	Loader           *bidder.CampaignLoader
	Producer         *events.Producer
	RDB              *redis.Client
	ExchangeRegistry *exchange.Registry
	Guard            *guardrail.Guardrail
	HMACSecret       string
	PublicURL        string
}

// RegisterRoutes wires the bidder's HTTP handlers against a Deps bundle.
// Both production main() and integration tests call this.
func RegisterRoutes(mux *http.ServeMux, d *Deps) {
	mux.HandleFunc("POST /bid", d.handleBid)
	mux.HandleFunc("POST /bid/{exchange_id}", d.handleExchangeBid)
	mux.HandleFunc("POST /win", d.handleWin)
	mux.HandleFunc("GET /win", d.handleWin)
	mux.HandleFunc("GET /click", d.handleClick)
	mux.HandleFunc("GET /convert", d.handleConvert)
	mux.HandleFunc("GET /stats", d.handleStats)
	mux.HandleFunc("GET /health", d.handleHealth)
	mux.Handle("GET /metrics", promhttp.Handler())
}
```

- [ ] **Step 2: Convert handler funcs in `main.go` to methods on `*Deps`**

In `cmd/bidder/main.go`:

1. Delete the package-level variable block at the top (lines 32-42 in current file: `engine`, `budgetSvc`, `strategySvc`, `statsCache`, `loader`, `producer`, `rdb`, `exchangeRegistry`, `guard`). Keep any that are only used by `main()` internally.
2. Change every `func handleBid(w, r)` to `func (d *Deps) handleBid(w http.ResponseWriter, r *http.Request)`.
3. Replace every reference to the global (e.g. `engine.Bid(...)`) with `d.Engine.Bid(...)`, `budgetSvc.CheckAndDeductBudget` → `d.BudgetSvc.CheckAndDeductBudget`, and so on.
4. Replace `config.Load().BidderPublicURL` and `config.Load().BidderHMACSecret` inside handlers with `d.PublicURL` and `d.HMACSecret` (these are captured once in `main()`).
5. In `main()`, construct `deps := &Deps{...}` right before creating the mux, then:

```go
mux := http.NewServeMux()
RegisterRoutes(mux, deps)
```

- [ ] **Step 3: Build and run existing tests**

```bash
go build ./cmd/bidder/...
go test ./cmd/bidder/...
```

Expected: compiles clean. Existing `cmd/bidder/main_test.go` (if any) still passes. If you break legacy `internal/bidder/integration_test.go`, remember it tests the **legacy** `bidder.New()` path, not your refactor — leave it alone.

- [ ] **Step 4: Smoke the refactored binary**

```bash
go run ./cmd/bidder/ &
BIDDER_PID=$!
sleep 2
curl -sS http://localhost:8180/health
kill $BIDDER_PID
```

Expected: `{"status":"ok",...}`. This verifies that the live `main()` still starts via the new wiring.

- [ ] **Step 5: Commit**

```bash
git add cmd/bidder/main.go cmd/bidder/routes.go
git commit -m "refactor(bidder): extract handlers onto Deps + RegisterRoutes for test reuse"
```

---

### Task T13: Refactor `cmd/consumer/main.go` — extract `RunConsumer`

**Files:**
- Modify: `cmd/consumer/main.go`
- Create: `cmd/consumer/runner.go`

- [ ] **Step 1: Create `cmd/consumer/runner.go`**

```go
package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/segmentio/kafka-go"
)

// RunnerDeps bundles everything RunConsumer needs.
type RunnerDeps struct {
	Brokers     []string
	Topics      []string
	GroupID     string
	Store       *reporting.Store
	DLQProducer *events.Producer
}

// RunConsumer spawns a reader per topic and blocks until ctx is done.
// Each topic's reader runs in its own goroutine; the function waits for
// all of them to return (which they do only when ctx is cancelled).
func RunConsumer(ctx context.Context, deps RunnerDeps) {
	done := make(chan struct{}, len(deps.Topics))
	for _, topic := range deps.Topics {
		t := topic
		go func() {
			defer func() { done <- struct{}{} }()
			reader := kafka.NewReader(kafka.ReaderConfig{
				Brokers:  deps.Brokers,
				Topic:    t,
				GroupID:  deps.GroupID,
				MinBytes: 1,
				MaxBytes: 10e6,
				MaxWait:  1 * time.Second,
			})
			defer reader.Close()
			log.Printf("[CONSUMER] Listening on topic: %s", t)

			for {
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[CONSUMER] %s read error: %v", t, err)
					time.Sleep(time.Second)
					continue
				}

				var evt events.Event
				if err := json.Unmarshal(msg.Value, &evt); err != nil {
					log.Printf("[CONSUMER] %s unmarshal error: %v", t, err)
					continue
				}

				bidEvt := reporting.BidEvent{
					EventTime:       evt.Timestamp,
					CampaignID:      uint64(evt.CampaignID),
					CreativeID:      uint64(evt.CreativeID),
					AdvertiserID:    uint64(evt.AdvertiserID),
					RequestID:       evt.RequestID,
					GeoCountry:      evt.GeoCountry,
					DeviceOS:        evt.DeviceOS,
					DeviceID:        evt.DeviceID,
					BidPriceCents:   uint32(evt.BidPrice*100 + 0.5),
					ClearPriceCents: uint32(evt.ClearPrice*100 + 0.5),
					ChargeCents:     uint32(evt.AdvertiserCharge*100 + 0.5),
					EventType:       evt.Type,
				}

				if err := deps.Store.InsertEvent(ctx, bidEvt); err != nil {
					log.Printf("[CONSUMER] %s insert error: %v (sending to DLQ)", t, err)
					deps.DLQProducer.SendToDeadLetter(ctx, t, msg.Value, err.Error())
					continue
				}
			}
		}()
	}

	for range deps.Topics {
		<-done
	}
}
```

- [ ] **Step 2: Slim down `cmd/consumer/main.go`**

Replace the reader construction + goroutine loop with a single call:

```go
func main() {
	cfg := config.Load()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	store, err := reporting.NewStore(cfg.ClickHouseAddr, cfg.ClickHouseUser, cfg.ClickHousePassword)
	if err != nil {
		log.Fatalf("connect clickhouse: %v", err)
	}
	defer store.Close()
	log.Println("Connected to ClickHouse")

	brokers := strings.Split(cfg.KafkaBrokers, ",")
	dlqProducer := events.NewProducer(brokers, "/tmp/dsp-kafka-buffer-consumer")
	defer dlqProducer.Close()

	deps := RunnerDeps{
		Brokers:     brokers,
		Topics:      []string{"dsp.bids", "dsp.impressions"},
		GroupID:     "dsp-clickhouse-consumer",
		Store:       store,
		DLQProducer: dlqProducer,
	}

	log.Println("Kafka → ClickHouse consumer running. Press Ctrl+C to stop.")
	RunConsumer(ctx, deps)
	log.Println("Shutting down consumer...")
}
```

- [ ] **Step 3: Build**

```bash
go build ./cmd/consumer/...
```

Expected: clean.

- [ ] **Step 4: Smoke**

Start the compose stack consumer via `docker compose up -d consumer` and confirm logs show `Listening on topic: dsp.bids` and `dsp.impressions` (the compose-built binary is separate, but this confirms your source refactor didn't break the build chain used by the container).

- [ ] **Step 5: Commit**

```bash
git add cmd/consumer/main.go cmd/consumer/runner.go
git commit -m "refactor(consumer): extract RunConsumer for in-process test reuse"
```

---

### Task T14: P2 Engine.Bid integration tests — scenarios 12-21

**Files:**
- Create: `internal/bidder/engine_integration_test.go`

- [ ] **Step 1: Read spec `§5.1` fully**

- [ ] **Step 2: Write scenario 12 as complete pattern**

```go
//go:build integration

package bidder_test

import (
	"context"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

type engineFixture struct {
	*qaharness.TestHarness
	engine *bidder.Engine
	loader *bidder.CampaignLoader
}

func newEngineFixture(t *testing.T) *engineFixture {
	h := qaharness.New(t)
	loader := bidder.NewCampaignLoader(h.PG, h.RDB)
	ctx, cancel := context.WithCancel(h.Ctx)
	t.Cleanup(cancel)
	if err := loader.Start(ctx); err != nil {
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(loader.Stop)

	budgetSvc := budget.New(h.RDB)
	strategySvc := bidder.NewBidStrategy(h.RDB)
	statsCache := bidder.NewStatsCache(h.RDB, nil, loader.GetActiveCampaigns)
	fraudFilter := antifraud.NewFilter(h.RDB)
	guard := guardrail.New(h.RDB, guardrail.Config{})
	producer := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	t.Cleanup(producer.Close)

	eng := bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)
	return &engineFixture{
		TestHarness: h,
		engine:      eng,
		loader:      loader,
	}
}

// Scenario 12 — normal bid succeeds, event lands in Kafka and ClickHouse.
func TestEngine_BidHappyPath(t *testing.T) {
	f := newEngineFixture(t)
	advID := f.SeedAdvertiser("bid-happy")
	campID := f.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-engine-happy",
		BidCPMCents:  2000,
	})
	f.SeedCreative(campID, "", "")
	// Force a full reload so the newly seeded campaign is visible.
	time.Sleep(200 * time.Millisecond)
	f.PublishCampaignUpdate(campID, "activated")
	time.Sleep(200 * time.Millisecond)

	req := qaharness.BuildBidRequest(qaharness.BuildBidRequestOpts{})
	resp, err := f.engine.Bid(f.Ctx, req)
	if err != nil {
		t.Fatalf("Bid error: %v", err)
	}
	if resp == nil || len(resp.SeatBid) == 0 {
		t.Fatal("expected a bid response")
	}
	bid := resp.SeatBid[0].Bid[0]
	if bid.Price <= 0 {
		t.Errorf("bid.Price should be > 0, got %v", bid.Price)
	}

	// Kafka: dsp.bids should have 1 event for this request_id
	kCount := f.CountMessages("dsp.bids", req.ID, 3*time.Second)
	if kCount != 1 {
		t.Errorf("dsp.bids count: want 1 got %d", kCount)
	}
	// ClickHouse: bid_log should have a row (consumer runs in compose stack)
	f.WaitForBidLogRows(campID, "bid", 1, 5*time.Second)
}
```

Note the fixture pattern — it's re-used by every `TestEngine_*`.

- [ ] **Step 3: Implement scenarios 13-21**

Per spec `§5.1`:

- **Scenario 13 (multi-candidate pick highest)**: seed 3 campaigns with `BidCPMCents` 500/1000/2000 → Bid → assert `resp.SeatBid[0].Seat == "campaign-<id of 2000 one>"`.
- **Scenario 14 (geo mismatch → 204)**: seed CN targeting, request `Geo="US"` → `resp == nil, err == nil`.
- **Scenario 15 (no Device → 204)**: build a request with `Device: nil` → `resp == nil`.
- **Scenario 16 (no Banner/Video/Native → 204)**: build request where `Imp[0]` has all three nil → `resp == nil`.
- **Scenario 17 (bidfloor filter)**: `BidFloor: 999.0` in the request, low BidCPM → `resp == nil`.
- **Scenario 18 (guardrail pre-check denies)**: construct fixture with a `guardrail` pre-configured so `PreCheck` returns `Allowed=false` (set `MinBalanceCents` very high and pre-seed a low global balance key). Alternative: directly instantiate a mock guardrail. Start simple and fail the test if it's too intrusive.
- **Scenario 19 (bid ceiling)**: `guardrail.Config{MaxBidCPMCents: 10}` in fixture → campaign with BidCPMCents=2000 → `resp == nil`.
- **Scenario 20 (budget exhausted)**: `SetBudgetRemaining(campID, 0)` before calling Bid → `resp == nil`.
- **Scenario 21 (CPC + StatsCache consistency — CB6 probe)**: seed CPC campaign with `BidCPCCents=100`. Force statsCache CTR via direct Redis SET on key used by `statsCache` (see `internal/bidder/statscache.go` for the key format — read that file first). Call Bid. Read the `dsp.bids` event from Kafka. Assert `kafkaEvt.BidPrice == engineExpectedBidPrice` where `engineExpectedBidPrice = float64(lc.EffectiveBidCPMCents(ctr, 0)) * 0.90 / 100 / 1000` with the **same CTR** statsCache was holding. **If the assertion fails, CB6 is confirmed** — file a bug, then fix `cmd/bidder/main.go:391` and/or `engine.go` to compute `bidPrice` consistently, add a regression test, re-run.

- [ ] **Step 4: Run**

```bash
go test -tags=integration -v -timeout 5m -run TestEngine_ ./internal/bidder/...
```

Expected: all 10 PASS. Scenario 21 may fail first run → apply fix → re-run until PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/bidder/engine_integration_test.go
# + any engine.go or main.go fixes
git commit -m "test(bidder): Engine.Bid integration tests (P2 scenarios 12-21)"
```

If a bug fix was applied as part of scenario 21, commit that separately first:

```bash
git commit -m "fix(bidder): align win-event bidPrice with Engine.Bid computed value (CB6)"
```

---

### Task T15: P2 handler integration tests — scenarios 22-27

**Files:**
- Create: `cmd/bidder/handlers_integration_test.go`

- [ ] **Step 1: Read spec `§5.2` fully**

- [ ] **Step 2: Build the handler fixture**

```go
//go:build integration

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/antifraud"
	"github.com/heartgryphon/dsp/internal/bidder"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/exchange"
	"github.com/heartgryphon/dsp/internal/guardrail"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

type handlerFixture struct {
	*qaharness.TestHarness
	deps *Deps
	srv  *httptest.Server
}

func newHandlerFixture(t *testing.T) *handlerFixture {
	h := qaharness.New(t)

	loader := bidder.NewCampaignLoader(h.PG, h.RDB)
	if err := loader.Start(h.Ctx); err != nil {
		t.Fatalf("loader start: %v", err)
	}
	t.Cleanup(loader.Stop)

	producer := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	t.Cleanup(producer.Close)

	budgetSvc := budget.New(h.RDB)
	strategySvc := bidder.NewBidStrategy(h.RDB)
	statsCache := bidder.NewStatsCache(h.RDB, nil, loader.GetActiveCampaigns)
	fraudFilter := antifraud.NewFilter(h.RDB)
	guard := guardrail.New(h.RDB, guardrail.Config{})
	eng := bidder.NewEngine(loader, budgetSvc, strategySvc, statsCache, producer, fraudFilter, guard)

	deps := &Deps{
		Engine:           eng,
		BudgetSvc:        budgetSvc,
		StrategySvc:      strategySvc,
		Loader:           loader,
		Producer:         producer,
		RDB:              h.RDB,
		ExchangeRegistry: exchange.DefaultRegistry("http://test-bidder"),
		Guard:            guard,
		HMACSecret:       "qa-test-secret",
		PublicURL:        "http://test-bidder",
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &handlerFixture{TestHarness: h, deps: deps, srv: srv}
}
```

- [ ] **Step 3: Write scenarios 22-27**

Each follows the same shape: seed → fire a request via `srv.Client()` → inspect response and side effects. Specifics:

- **Scenario 22 (`/win` normal CPM)**: seed active campaign, `beforeBudget := f.GetBudgetRemaining(campID)`. Run Bid → extract NURL → replace `${AUCTION_PRICE}` with `0.05` in NURL → fire GET to `NURL` via `srv.Client().Get(...)`. Assert HTTP 200. Assert `beforeBudget - f.GetBudgetRemaining(campID)` equals `int64(0.05 / 0.9 * 100)` (i.e. 5). Assert Kafka `dsp.bids` has a `type=win` event and `dsp.impressions` has a `type=impression` event for the request_id.
- **Scenario 23 (HMAC invalid)**: build a `/win` URL manually with a bogus token. Assert HTTP 403. Assert Redis budget unchanged.
- **Scenario 24 (win dedup)**: do scenario 22 once. Immediately fire the same URL 2 more times. Assert responses: first `ok`, second & third `"duplicate"`. Assert Redis budget decremented once only. Assert Kafka count of `type=win` events == 1.
- **Scenario 25 (money edge)**: use `price=0.00123`. Record `beforeBudget`. Fire `/win`. `int64(0.00123 / 0.9 * 100) == 0`. Assert Redis budget is unchanged. Log the computed value (`int64(price/0.9*100)`) in `t.Logf` — this is the evidence that CB2 is real. If the product decision is "small wins round to 0 and that's acceptable," the assertion is the test's baseline (not a failure). If not, file a bug.
- **Scenario 26 (CPC click)**: seed CPC campaign with `BidCPCCents=10`. `/bid` → extract the click URL from the returned ad markup (look for `href="...click?..."`). Fire GET on it. Assert Redis budget reduced by 10. Assert Kafka `dsp.impressions` has a `type=click` event with `AdvertiserCharge == 0.10`.
- **Scenario 27 (convert HMAC invalid)**: `/convert` with bogus token → 403, Kafka no conversion events.

- [ ] **Step 4: Run**

```bash
go test -tags=integration -v -timeout 5m -run TestHandlers_ ./cmd/bidder/...
```

- [ ] **Step 5: Commit**

```bash
git add cmd/bidder/handlers_integration_test.go
# + any main.go fixes
git commit -m "test(bidder): handler integration tests (P2 scenarios 22-27)"
```

---

### Task T16: P2 Producer integration tests — scenarios 28-30

**Files:**
- Create: `internal/events/producer_integration_test.go`

- [ ] **Step 1: Read spec `§5.3` fully**

- [ ] **Step 2: Write the test**

```go
//go:build integration

package events_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/qaharness"
)

// Scenario 28 — normal publish, 100 events land in Kafka, buffer stays empty.
func TestProducer_NormalPublish(t *testing.T) {
	h := qaharness.New(t)
	bufDir := t.TempDir()
	p := events.NewProducer(h.Env.KafkaBrokers, bufDir)
	defer p.Close()

	prefix := fmt.Sprintf("qa-prod-%d", time.Now().UnixNano())
	for i := 0; i < 100; i++ {
		p.SendBid(h.Ctx, events.Event{
			CampaignID: 900050,
			RequestID:  fmt.Sprintf("%s-%d", prefix, i),
			BidPrice:   0.05,
		})
	}
	// Let Async writer flush
	time.Sleep(2 * time.Second)

	got := h.CountMessages("dsp.bids", prefix, 5*time.Second)
	if got != 100 {
		t.Errorf("expected 100 messages, got %d", got)
	}

	entries, _ := os.ReadDir(bufDir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".jsonl" {
			info, _ := e.Info()
			if info.Size() > 0 {
				t.Errorf("disk buffer should be empty but %s has %d bytes", e.Name(), info.Size())
			}
		}
	}
}

// Scenario 29 — Kafka unreachable → disk buffer must fill. Probes CB4.
// NOTE: kafka.Writer.Async=true means WriteMessages returns nil before the
// background flusher attempts delivery, so bufferToDisk never fires.
// If this test fails, CB4 is confirmed and the producer must be fixed.
func TestProducer_AsyncFailureBuffers(t *testing.T) {
	bufDir := t.TempDir()
	// Point to a port guaranteed unreachable (nothing listens on 1).
	p := events.NewProducer([]string{"127.0.0.1:1"}, bufDir)
	defer p.Close()

	for i := 0; i < 10; i++ {
		p.SendBid(context.Background(), events.Event{
			CampaignID: 900051,
			RequestID:  fmt.Sprintf("qa-buf-%d", i),
			BidPrice:   0.01,
		})
	}
	// Wait for Async flusher to fail and fallback to buffer
	time.Sleep(5 * time.Second)

	path := filepath.Join(bufDir, "dsp.bids.jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("disk buffer file missing: %v (CB4 confirmed: Async producer silently drops events)", err)
	}
	if info.Size() == 0 {
		t.Fatalf("disk buffer is empty (CB4 confirmed)")
	}

	data, _ := os.ReadFile(path)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 10 {
		t.Errorf("expected 10 buffered events, got %d", lines)
	}
}

// Scenario 30 — ReplayBuffer sends buffered events once Kafka returns.
func TestProducer_ReplayBuffer(t *testing.T) {
	h := qaharness.New(t)
	bufDir := t.TempDir()
	path := filepath.Join(bufDir, "dsp.bids.jsonl")

	prefix := fmt.Sprintf("qa-replay-%d", time.Now().UnixNano())
	f, _ := os.Create(path)
	for i := 0; i < 5; i++ {
		line := fmt.Sprintf(`{"type":"bid","campaign_id":900052,"request_id":"%s-%d","bid_price":0.05}`+"\n",
			prefix, i)
		f.WriteString(line)
	}
	f.Close()

	p := events.NewProducer(h.Env.KafkaBrokers, bufDir)
	defer p.Close()
	if err := p.ReplayBuffer(h.Ctx); err != nil {
		t.Fatalf("replay: %v", err)
	}

	got := h.CountMessages("dsp.bids", prefix, 5*time.Second)
	if got != 5 {
		t.Errorf("replay: expected 5, got %d", got)
	}

	// Replayed file should be renamed
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("original buffer file should be renamed after replay")
	}
	if _, err := os.Stat(path + ".replayed"); err != nil {
		t.Errorf("expected .replayed marker, got %v", err)
	}
}
```

- [ ] **Step 3: Run**

```bash
go test -tags=integration -v -timeout 5m -run TestProducer_ ./internal/events/...
```

Expected behavior for scenario 29 matters:
- **If test PASSES**: buffer works → CB4 is disproved, spec update.
- **If test FAILS** ("disk buffer file missing" or "empty"): CB4 confirmed. Apply fix: either set `Async=false`, OR add a completion callback that writes to buffer on failure. Re-run until pass, then commit the fix in a separate commit.

- [ ] **Step 4: Commit**

```bash
git add internal/events/producer_integration_test.go
# + any producer.go fixes
git commit -m "test(events): producer integration tests (P2 scenarios 28-30)"
```

---

### Task T17: P2 Consumer → ClickHouse tests — scenarios 31-33

**Files:**
- Create: `cmd/consumer/consumer_integration_test.go`

- [ ] **Step 1: Read spec `§5.4` fully**

- [ ] **Step 2: Write the tests using `RunConsumer`**

```go
//go:build integration

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/events"
	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// Scenario 31 — all 5 event types land correctly; event_time == original Timestamp.
// Probes CB3 (Timestamp overwritten in Send).
func TestConsumer_AllEventTypesLand(t *testing.T) {
	h := qaharness.New(t)

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatalf("reporting store: %v", err)
	}
	defer store.Close()

	dlq := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	defer dlq.Close()

	// Start the consumer in the background with a unique groupID so it doesn't
	// share offsets with the production consumer.
	ctx, cancel := context.WithCancel(h.Ctx)
	defer cancel()
	groupID := fmt.Sprintf("qa-consumer-%d", time.Now().UnixNano())
	go RunConsumer(ctx, RunnerDeps{
		Brokers:     h.Env.KafkaBrokers,
		Topics:      []string{"dsp.bids", "dsp.impressions"},
		GroupID:     groupID,
		Store:       store,
		DLQProducer: dlq,
	})
	time.Sleep(500 * time.Millisecond)

	// Publish 1 event of each type with a distinct campaign.
	prod := events.NewProducer(h.Env.KafkaBrokers, t.TempDir())
	defer prod.Close()

	campID := int64(900060)
	eventTS := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	mkEvt := func(reqSuffix string) events.Event {
		return events.Event{
			CampaignID:   campID,
			AdvertiserID: 900060,
			CreativeID:   1,
			RequestID:    fmt.Sprintf("qa-types-%s", reqSuffix),
			BidPrice:     0.05,
			Timestamp:    eventTS,
		}
	}
	prod.SendBid(h.Ctx, mkEvt("bid"))
	prod.SendWin(h.Ctx, mkEvt("win"))
	prod.SendImpression(h.Ctx, mkEvt("imp"))
	prod.SendClick(h.Ctx, mkEvt("click"))
	prod.SendConversion(h.Ctx, mkEvt("conv"))

	// Wait for all 5 to land. Note: dsp.billing is ALSO written by SendWin but
	// no consumer subscribes to it (contract §3.1), so bid_log should still be 5.
	h.WaitForBidLogRows(campID, "bid", 1, 10*time.Second)
	h.WaitForBidLogRows(campID, "win", 1, 10*time.Second)
	h.WaitForBidLogRows(campID, "impression", 1, 10*time.Second)
	h.WaitForBidLogRows(campID, "click", 1, 10*time.Second)
	h.WaitForBidLogRows(campID, "conversion", 1, 10*time.Second)

	// CB3 probe: event_time in CH must equal original eventTS, not "now".
	var storedTime time.Time
	row := h.CH.QueryRow(h.Ctx, `
		SELECT event_time FROM bid_log WHERE campaign_id = ? AND event_type = 'bid' LIMIT 1
	`, uint64(campID))
	if err := row.Scan(&storedTime); err != nil {
		t.Fatalf("scan event_time: %v", err)
	}
	if diff := storedTime.Sub(eventTS); diff < -2*time.Second || diff > 2*time.Second {
		t.Fatalf("CB3 confirmed: event_time=%v, expected ≈%v (producer is overwriting Timestamp)",
			storedTime, eventTS)
	}
}

// Scenario 32 — malformed JSON skipped, valid event still processed.
func TestConsumer_MalformedJSONSkipped(t *testing.T) {
	// Implementation per spec §5.4 scenario 32. See TestConsumer_AllEventTypesLand
	// for the RunConsumer setup pattern.
	t.Skip("TODO: implement per spec §5.4 scenario 32")
}

// Scenario 33 — CH write failure routes to DLQ.
func TestConsumer_CHFailureDLQ(t *testing.T) {
	// Implementation per spec §5.4 scenario 33 (stop ClickHouse mid-test or
	// inject a wrapper store that always errors). See TestConsumer_AllEventTypesLand
	// for the RunConsumer setup pattern.
	t.Skip("TODO: implement per spec §5.4 scenario 33")
}
```

- [ ] **Step 3: Replace the two `t.Skip` placeholders with real implementations**

**Scenario 32**: same setup as 31, but publish two events — one with `[]byte("{not json")` by using `kafka.Writer` directly via `kafka-go`, and one via `prod.SendBid(...)`. Wait for exactly 1 bid_log row for the valid request_id. Consumer should log an error for the malformed one and keep running.

**Scenario 33**: wrap `reporting.Store` with a local adapter whose `InsertEvent` always returns `fmt.Errorf("qa: forced failure")`. Swap the deps so `RunConsumer` uses it. Publish 1 event. Poll `dsp.dead-letter` for 1 message. Assert the DLQ payload's `original_topic` field matches.

The wrapper pattern:

```go
type failingStore struct {
	real *reporting.Store
}

func (f *failingStore) InsertEvent(ctx context.Context, e reporting.BidEvent) error {
	return fmt.Errorf("qa: forced failure")
}
```

Since `RunnerDeps.Store` is `*reporting.Store`, you'll need to either (a) change `RunnerDeps.Store` to an interface `type BidLogStore interface { InsertEvent(ctx, BidEvent) error }` (preferred — tiny interface extraction), or (b) inject failure via shutting down the compose `clickhouse-engine` container and restarting afterwards. Prefer (a) — it's cleaner. Document the tiny interface in `cmd/consumer/runner.go` with one sentence.

- [ ] **Step 4: Run**

```bash
go test -tags=integration -v -timeout 5m -run TestConsumer_ ./cmd/consumer/...
```

Expected: all 3 PASS. If scenario 31 fails (CB3 confirmed), apply the fix: in `internal/events/producer.go:75` change `evt.Timestamp = time.Now().UTC()` to only set it when it is the zero value:

```go
if evt.Timestamp.IsZero() {
	evt.Timestamp = time.Now().UTC()
}
```

Re-run until PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/consumer/consumer_integration_test.go cmd/consumer/runner.go
# + internal/events/producer.go if CB3 fix was applied
git commit -m "test(consumer): integration tests (P2 scenarios 31-33)"
```

If CB3 fix was applied, commit it first:

```bash
git commit -m "fix(events): preserve caller-supplied Event.Timestamp in producer (CB3)"
```

---

### Task T18: Phase 2 interim smoke run

**Files:**
- Read: all Phase 2 tests

- [ ] **Step 1: Run the full Phase 2 suite**

```bash
go test -tags=integration -timeout 15m ./internal/bidder/... ./cmd/bidder/... ./internal/events/... ./cmd/consumer/...
```

Expected: 22 Phase 2 scenarios + 7 Phase 1 scenarios + 4 Phase 1 budget scenarios = 33 test cases PASS.

- [ ] **Step 2: 4-way reconciliation spot check**

Pick any campaign from the Phase 2 runs and verify:

```bash
# Kafka bids count
docker exec kafka-engine kafka-console-consumer --bootstrap-server localhost:9094 \
  --topic dsp.bids --from-beginning --timeout-ms 3000 2>/dev/null | grep 'campaign_id":900' | wc -l

# ClickHouse bid count
clickhouse-client --host localhost --port 21001 -q \
  "SELECT count() FROM bid_log WHERE campaign_id >= 900000 AND event_type = 'bid'"
```

The two numbers should be equal (or CH >= Kafka by a constant, if the consumer has drained extra history). Any drift indicates a bug — investigate before proceeding.

- [ ] **Step 3: If everything is clean, move on**

No commit needed if no code changed.

---

### Task T19: Phase 2 exit criteria verification (review loop)

- [ ] **Step 1: Run the full Phase 2 suite**

Same as T18 step 1.

- [ ] **Step 2: `requesting-code-review`**

Scope: everything changed since Phase 1 closed. Fix Critical/Important findings.

- [ ] **Step 3: Residue check**

```bash
psql "postgres://dsp:dsp_dev_password@localhost:17432/dsp?sslmode=disable" -c "SELECT count(*) FROM campaigns WHERE name LIKE 'qa-%';"
redis-cli -p 18380 -n 15 DBSIZE
```
Expected: both 0.

- [ ] **Step 4: Loop decision**

If any fix, restart the round from Step 1. Max 5 rounds.

- [ ] **Step 5: Append Phase 2 results to report**

Update `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md` with a `## Phase 2 Result` section:
- 22-scenario pass/fail table
- 4-way reconciliation numbers (Kafka vs CH vs Redis vs engine metric)
- Money consistency observations from scenario 25 (CB2 baseline)
- Candidate bug status updates (CB2, CB3, CB4, CB6)
- Link/dump of `data/p2-4way-reconciliation.csv` and `data/p2-money-consistency.csv` (created from the test output)

```bash
git add docs/archive/superpowers/reports/
git commit -m "docs(qa-report): phase 2 results + CB status updates"
```

---

## Phase 3: Consume Path + Read-side (T20–T23)

Spec reference: `§6`.

### Task T20: Reconciliation integration tests — scenarios 34-37

**Files:**
- Create: `internal/reconciliation/reconciliation_integration_test.go`

- [ ] **Step 1: Read spec `§6.1` fully**

- [ ] **Step 2: Write the tests**

```go
//go:build integration

package reconciliation_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/alert"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reconciliation"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// fakeAlerter captures the last message instead of sending anywhere.
type fakeAlerter struct {
	sent []string
}

func (f *fakeAlerter) Send(subject, body string) {
	f.sent = append(f.sent, subject+" | "+body)
}

func newReconcileSvc(t *testing.T, h *qaharness.TestHarness, alerter alert.Sender) *reconciliation.Service {
	t.Helper()
	store := campaign.NewStore(h.PG)
	rs, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatalf("reporting store: %v", err)
	}
	t.Cleanup(func() { _ = rs.Close() })
	billSvc := billing.NewService(h.PG, h.RDB) // verify actual constructor signature
	return reconciliation.New(h.RDB, store, rs, billSvc, alerter)
}

// Scenario 34 — consistent Redis and CH, no alert.
func TestReconcile_Consistent(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-ok")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-ok",
		BudgetDailyCents: 10_000,
	})
	h.SetBudgetRemaining(campID, 7_000) // 3000 spent

	h.InsertBidLogRow(campID, advID, 1, "win", "req-1", "dev-1", 0, 0, 3_000, time.Now())

	alerter := &fakeAlerter{}
	svc := newReconcileSvc(t, h, alerter)
	results, err := svc.RunHourly(h.Ctx, 1.0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range results {
		if r.CampaignID == campID {
			found = true
			if r.DiffPercent() != 0 {
				t.Errorf("DiffPercent: want 0 got %v", r.DiffPercent())
			}
		}
	}
	if !found {
		t.Fatal("campaign missing from results")
	}
	if len(alerter.sent) != 0 {
		t.Errorf("expected no alert, got %d", len(alerter.sent))
	}
}

// Scenario 35 — drift beyond threshold triggers alert.
func TestReconcile_DriftAlerts(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("recon-drift")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID:     advID,
		Name:             "qa-recon-drift",
		BudgetDailyCents: 10_000,
	})
	h.SetBudgetRemaining(campID, 7_000) // 3000 spent per redis

	h.InsertBidLogRow(campID, advID, 1, "win", "req-a", "dev-a", 0, 0, 3_500, time.Now())

	alerter := &fakeAlerter{}
	svc := newReconcileSvc(t, h, alerter)
	_, err := svc.RunHourly(h.Ctx, 5.0)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerter.sent) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerter.sent))
	}
	if !containsAny(alerter.sent[0], fmt.Sprintf("%d", campID)) {
		t.Errorf("alert should mention campaign id %d: %q", campID, alerter.sent[0])
	}
}

func containsAny(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 3: Implement scenarios 36 and 37 in the same file**

- **Scenario 36 (CB5 probe — SQL aggregation semantics)**: seed a campaign; insert 1 `bid`(charge=0) + 1 `win`(charge=300) + 1 `click`(charge=50, simulating CPC) + 1 `conversion`(charge=0) into `bid_log`. Call `GetCampaignStats` directly. Assert `stats.SpendCents == 300` **or** `== 350` — whichever is the spec-intended value. The test must be explicit: pick the assertion that matches the current reconciliation model (Redis side deducts both `win` for CPM and `click` for CPC, so the honest answer is 350). If the real result differs, CB5 is confirmed — either fix `store.go` to constrain `event_type IN ('win','click')` in `sum(charge_cents)`, or acknowledge the semantics mismatch in the report (not both).
- **Scenario 37 (CH failure does not panic)**: close `reportStore` before calling `RunHourly` OR pass an obviously bad `time.Time{}` for `now`. Assert the function returns an error (or empty results) without panicking. Use `defer func() { if r := recover(); r != nil { t.Fatalf("panicked: %v", r) } }()` as a safety net.

- [ ] **Step 4: Run**

```bash
go test -tags=integration -v -timeout 3m -run TestReconcile_ ./internal/reconciliation/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/reconciliation/reconciliation_integration_test.go
# + any store.go fixes if CB5 confirmed
git commit -m "test(reconciliation): integration tests (P3 scenarios 34-37)"
```

---

### Task T21: Attribution integration tests — scenarios 38-41

**Files:**
- Create: `internal/reporting/attribution_integration_test.go`

- [ ] **Step 1: Read spec `§6.2` fully**

- [ ] **Step 2: Write the tests**

```go
//go:build integration

package reporting_test

import (
	"math"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
)

func seedTouchpoints(h *qaharness.TestHarness, campID, advID int64, deviceID string, events []struct {
	typ string
	t   time.Time
}) {
	for i, e := range events {
		h.InsertBidLogRow(campID, advID, 1, e.typ, pastReq(i), deviceID, 0, 0, 0, e.t)
	}
}

func pastReq(i int) string {
	return []string{"qa-tp-1", "qa-tp-2", "qa-tp-3", "qa-tp-4"}[i]
}

// Scenario 38 — last_click: 100% credit to last touchpoint.
func TestAttribution_LastClick(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("attr-last")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-attr-last",
	})

	base := time.Now().UTC().Add(-2 * time.Hour)
	seedTouchpoints(h, campID, advID, "dA", []struct {
		typ string
		t   time.Time
	}{
		{"impression", base},
		{"click", base.Add(10 * time.Minute)},
		{"impression", base.Add(20 * time.Minute)},
		{"conversion", base.Add(30 * time.Minute)},
	})

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), base.Add(-time.Hour), time.Now(), reporting.ModelLastClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.AttributedPaths) != 1 {
		t.Fatalf("paths: want 1 got %d", len(report.AttributedPaths))
	}
	path := report.AttributedPaths[0]
	if len(path.Credit) != 3 { // 3 touchpoints (excluding the conversion itself)
		t.Fatalf("credits: want 3 got %d", len(path.Credit))
	}
	var sum float64
	for _, c := range path.Credit {
		sum += c.Credit
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Errorf("credit sum: want 1.0 got %v", sum)
	}
	if path.Credit[len(path.Credit)-1].Credit != 1.0 {
		t.Errorf("last_click should put 1.0 on last touchpoint")
	}
}
```

- [ ] **Step 3: Implement scenarios 39, 40, 41**

- **Scenario 39 (`first_click`)**: same data, `ModelFirstClick`. Assert `Credit[0].Credit == 1.0`, others 0, sum == 1.0.
- **Scenario 40 (`linear`)**: same data, `ModelLinear`. Assert each `Credit[i].Credit ≈ 1/3` with ±1e-9 tolerance, sum ≈ 1.0.
- **Scenario 41 (empty touchpoints skip)**: insert a conversion but no impression/click for that device_id. Run the report. Assert `TotalConversions == 0`, no panic.

- [ ] **Step 4: Run**

```bash
go test -tags=integration -v -timeout 3m -run TestAttribution_ ./internal/reporting/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/reporting/attribution_integration_test.go
git commit -m "test(reporting): attribution integration tests (P3 scenarios 38-41)"
```

---

### Task T22: GetCampaignStats schema tests — scenarios 42-43

**Files:**
- Create (or modify T21 file): `internal/reporting/stats_integration_test.go`

- [ ] **Step 1: Read spec `§6.3` fully**

- [ ] **Step 2: Write the tests**

```go
//go:build integration

package reporting_test

import (
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/qaharness"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// Scenario 42 — mixed event counts aggregate correctly.
func TestStats_MixedCounts(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("stats-mix")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-stats-mix",
	})

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		h.InsertBidLogRow(campID, advID, 1, "bid", "req-bid", "d", 500, 0, 0, now)
	}
	for i := 0; i < 5; i++ {
		h.InsertBidLogRow(campID, advID, 1, "win", "req-win", "d", 500, 450, 500, now)
	}
	for i := 0; i < 5; i++ {
		h.InsertBidLogRow(campID, advID, 1, "impression", "req-imp", "d", 0, 0, 0, now)
	}
	for i := 0; i < 2; i++ {
		h.InsertBidLogRow(campID, advID, 1, "click", "req-click", "d", 0, 0, 0, now)
	}

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	stats, err := store.GetCampaignStats(h.Ctx, uint64(campID), now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if stats.Bids != 10 || stats.Wins != 5 || stats.Impressions != 5 || stats.Clicks != 2 {
		t.Errorf("counts: bids=%d wins=%d imps=%d clicks=%d (want 10/5/5/2)",
			stats.Bids, stats.Wins, stats.Impressions, stats.Clicks)
	}
	// CTR = clicks / impressions * 100 = 40
	if stats.CTR != 40.0 {
		t.Errorf("CTR: want 40 got %v", stats.CTR)
	}
	// WinRate = wins / bids * 100 = 50
	if stats.WinRate != 50.0 {
		t.Errorf("WinRate: want 50 got %v", stats.WinRate)
	}
}

// Scenario 43 — boundary: clear_price_cents at UInt32 max, empty device_id excluded.
func TestStats_FieldBoundaries(t *testing.T) {
	h := qaharness.New(t)
	advID := h.SeedAdvertiser("stats-bound")
	campID := h.SeedCampaign(qaharness.CampaignSpec{
		AdvertiserID: advID,
		Name:         "qa-stats-bound",
	})

	now := time.Now().UTC()
	// UInt32 max
	h.InsertBidLogRow(campID, advID, 1, "win", "req-max", "dX", 0, 0xFFFFFFFF, 500, now)
	// empty device_id
	h.InsertBidLogRow(campID, advID, 1, "conversion", "req-empty", "", 0, 0, 0, now)

	store, err := reporting.NewStore(h.Env.ClickHouseAddr, h.Env.ClickHouseUser, h.Env.ClickHousePass)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	_, err = store.GetCampaignStats(h.Ctx, uint64(campID), now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GetCampaignStats: %v", err)
	}

	// Attribution report should exclude the empty-device row.
	report, err := store.GetAttributionReport(h.Ctx, uint64(campID), now.Add(-time.Hour), now.Add(time.Hour), reporting.ModelLastClick, 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalConversions != 0 {
		t.Errorf("empty device_id should be excluded from attribution, got %d", report.TotalConversions)
	}
}
```

- [ ] **Step 3: Run**

```bash
go test -tags=integration -v -timeout 3m -run TestStats_ ./internal/reporting/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/reporting/stats_integration_test.go
git commit -m "test(reporting): GetCampaignStats schema tests (P3 scenarios 42-43)"
```

---

### Task T23: Phase 3 exit criteria verification (review loop)

- [ ] **Step 1: Run the full Phase 3 suite**

```bash
go test -tags=integration -timeout 5m ./internal/reconciliation/... ./internal/reporting/...
```

- [ ] **Step 2: `requesting-code-review`**

Fix Critical/Important.

- [ ] **Step 3: Residue check** (same commands as T11 Step 3)

- [ ] **Step 4: Loop decision** — max 5 rounds until clean round

- [ ] **Step 5: Append Phase 3 results to report**

Update `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md` with a `## Phase 3 Result` section:
- 10-scenario pass/fail table
- Three attribution credit-sum tables
- CB5 status update
- Dump of `data/p3-reconciliation-diff.csv` and `data/p3-attribution-credits.json`

```bash
git add docs/archive/superpowers/reports/
git commit -m "docs(qa-report): phase 3 results"
```

---

## Final: Full-branch review + smoke + report + PR (T24–T26)

### Task T24: Full-branch verification

- [ ] **Step 1: Run everything**

```bash
go test ./...                                      # unit tests still green
go test -tags=integration -timeout 15m ./...       # all 43 scenarios
```

Expected: both commands exit 0. If any single test fails → fix root cause → restart from Step 1.

- [ ] **Step 2: `final-code-review`**

Scope: `git diff main...HEAD`. This is a full-branch review, not a Phase-scoped one.

Fix Critical/Important. If any fix, loop back to Step 1.

- [ ] **Step 3: Autopilot long run**

```bash
go run ./cmd/autopilot -duration=3m -mode=continuous
```

Expected: no panics, no leaked goroutines, no residual qa-prefixed data after exit. If autopilot reports red lines, fix before proceeding.

- [ ] **Step 4: Manual curl smoke (replaces /qa)**

```bash
# Save output to final-smoke-curl.log
{
  echo '=== /health ==='
  curl -sS http://localhost:20180/health
  echo
  echo '=== /stats ==='
  curl -sS http://localhost:20180/stats | head -50
  echo
  echo '=== /metrics ==='
  curl -sS http://localhost:20180/metrics | head -50
  echo
  echo '=== /bid (CN + iOS) ==='
  curl -sS -X POST http://localhost:20180/bid \
    -H 'Content-Type: application/json' \
    -d '{"id":"manual-smoke","imp":[{"id":"i1","banner":{"w":320,"h":50}}],"device":{"os":"iOS","geo":{"country":"CN"},"ifa":"smoke"}}'
  echo
} > docs/archive/superpowers/reports/2026-04-14-engine-qa-report/data/final-smoke-curl.log 2>&1
```

Inspect the log. Expected: `/health` returns ok, `/stats` returns JSON, `/metrics` returns prometheus text, `/bid` returns either 200 with a bid or 204. If 400/500, investigate and fix.

- [ ] **Step 5: Loop**

If any fix was applied in steps 2, 3, or 4, go back to Step 1. Max 5 rounds.

---

### Task T25: Test report finalization + screenshots

**Files:**
- Modify: `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md`
- Create: `docs/archive/superpowers/reports/2026-04-14-engine-qa-report/screenshots/*.png`

- [ ] **Step 1: Use `/browse` skill to capture screenshots**

Capture at least 5 screenshots (spec §8.3 hard requirement). Target URLs:
- `http://localhost:16100` — Grafana home (or a dashboard if one exists)
- `http://localhost:22090` — Prometheus home
- `http://localhost:20180/stats` — bidder stats JSON
- `http://localhost:20180/metrics` — bidder Prometheus metrics
- `http://localhost:16000/campaigns` — biz web campaign list (verifies engine data reaches biz frontend)

Save each PNG into `docs/archive/superpowers/reports/2026-04-14-engine-qa-report/screenshots/` with descriptive names.

**Grafana fallback**: if no dashboard exists, skip Grafana and capture 2 screenshots from Prometheus instead (Targets page + a graph page for `bidder_*` metrics). Still aim for ≥5 total.

- [ ] **Step 2: Finalize the report markdown**

Fill in the `## Executive summary`, `## Environment`, `## Candidate bug status`, `## Final smoke result`, `## Known leftovers`, and `## Conclusion` sections. Cross-reference screenshots and data files.

- [ ] **Step 3: Commit**

```bash
git add docs/archive/superpowers/reports/
git commit -m "docs(qa-report): final engine QA report with screenshots + data"
```

---

### Task T26: Finishing — single PR

- [ ] **Step 1: Invoke the `superpowers:finishing-a-development-branch` skill**

It walks through final checks. Present the "single PR" option (spec §11).

- [ ] **Step 2: Push and open the PR**

```bash
git push -u origin engine
gh pr create --base main --title "engine QA round: 43 integration tests + bug fixes + QA report" --body "$(cat <<'EOF'
## Summary

Systematic QA round for the engine subsystem. 43 integration scenarios across 3 phases, 2 minimal refactors to make handlers testable, candidate bug verdicts inline.

## What's in this PR

- `internal/qaharness/` — new test-only package for live compose-stack integration tests
- 7 `*_integration_test.go` files covering bidder, handlers, producer, consumer, reconciliation, reporting, attribution
- Refactors: `cmd/bidder/main.go` → `Deps` + `RegisterRoutes`; `cmd/consumer/main.go` → `RunConsumer`
- Bug fixes for candidate bugs confirmed during the QA round (see report §6)
- Spec: `docs/archive/superpowers/specs/2026-04-14-engine-qa-design.md`
- Plan: `docs/archive/superpowers/plans/2026-04-14-engine-qa-plan.md`
- Report: `docs/archive/superpowers/reports/2026-04-14-engine-qa-report.md` (with screenshots + data)

## Test plan

- [x] `go test ./...` green (unit)
- [x] `go test -tags=integration -timeout 15m ./...` green (43 scenarios)
- [x] `cmd/autopilot -duration=3m -mode=continuous` green
- [x] Manual curl smoke on /bid /win /click /convert /stats /health /metrics
- [x] Data residue check: Postgres/Redis/CH clean after test run
- [x] Final code review (no Critical/Important)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Report the PR URL to the user**

---

## Self-Review Checklist (run after writing this plan)

- [ ] **Spec coverage**: Every section of the spec has at least one task that implements it.
  - `§1` (goal/scope): captured in plan header ✓
  - `§2` (5 decisions): implicit in task structure ✓
  - `§3.1` qaharness files: T01-T07 cover env/harness/campaign/openrtb/kafka/clickhouse/redis/assert ✓
  - `§3.2` test file layout: T09, T10, T14, T15, T16, T17, T20, T21, T22 map 1-to-1 ✓
  - `§3.3` data isolation: baked into `TestHarness.Reset` (T01) ✓
  - `§3.4` refactors: T12, T13 ✓
  - `§4` Phase 1 11 scenarios: T09 (loader 1-7) + T10 (budget/freq 8-11) ✓
  - `§5` Phase 2 22 scenarios: T14 (engine 12-21) + T15 (handlers 22-27) + T16 (producer 28-30) + T17 (consumer 31-33) ✓
  - `§6` Phase 3 10 scenarios: T20 (recon 34-37) + T21 (attrib 38-41) + T22 (stats 42-43) ✓
  - `§7` final loop: T24 ✓
  - `§8` report: T11, T19, T23, T25 (incremental) ✓
  - `§9` candidate bugs: CB2/3/4/5/6 mapped to scenarios 25, 31, 29, 36, 21 respectively ✓
  - `§10` known leftovers: referenced in T25 report ✓
  - `§11` finishing: T26 ✓
  - `§12` test commands: referenced throughout ✓

- [ ] **Placeholder scan**: no "TBD", "TODO" without follow-up, no `implement later`. T17's `t.Skip` placeholders are explicitly followed by Step 3 that replaces them with real implementations.

- [ ] **Type consistency**: `Deps`, `RegisterRoutes`, `RunConsumer`, `RunnerDeps`, `TestHarness`, `CampaignSpec`, `BuildBidRequest`, `BuildBidRequestOpts`, `CountMessages`, `WaitForBidLogRows` — names match across all references.

- [ ] **Every code step shows code**: qaharness tasks have full file bodies; integration test tasks show scenario-1 pattern + explicit instructions for the remaining scenarios (not "similar to X").
