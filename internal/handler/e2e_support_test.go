//go:build e2e
// +build e2e

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/heartgryphon/dsp/internal/audit"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/budget"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/handler"
	"github.com/heartgryphon/dsp/internal/registration"
	"github.com/heartgryphon/dsp/internal/reporting"
)

// --- env + defaults (match docker-compose.yml passwords for biz stack) ---

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// Defaults target main's docker-compose stack (ports 15432 / 16380 / 19001).
// Override via DSP_E2E_PG_DSN / DSP_E2E_REDIS_ADDR / DSP_E2E_CH_ADDR when
// pointing at a different stack. Pre-merge these pointed at the biz
// worktree stack (16432 / 17380 / 20001) which no longer exists on main.
func pgDSN() string {
	return envOr("DSP_E2E_PG_DSN", "postgres://dsp:dsp_dev_password@localhost:15432/dsp?sslmode=disable")
}

func redisAddr() string { return envOr("DSP_E2E_REDIS_ADDR", "localhost:16380") }

func redisPassword() string {
	if v, ok := os.LookupEnv("DSP_E2E_REDIS_PASSWORD"); ok {
		return v
	}
	return "dsp_dev_password"
}

// chAddr is the ClickHouse native protocol address used by both
// reporting.NewStore and mustCHConn. Main's stack exposes native on 19001.
func chAddr() string { return envOr("DSP_E2E_CH_ADDR", "localhost:19001") }

func chPassword() string {
	if v, ok := os.LookupEnv("DSP_E2E_CH_PASSWORD"); ok {
		return v
	}
	return "dsp_dev_password"
}

func adminToken() string { return envOr("DSP_E2E_ADMIN_TOKEN", "admin-secret") }

// --- test deps ---

// mustDeps connects to the real biz docker stack. Connection failures
// (postgres/redis unreachable) call t.Skipf so tests can run on hosts
// without docker. Structural failures (schema missing, queries rejected)
// call t.Fatalf so drift doesn't silently pass as skipped. ReportStore
// is left nil on clickhouse failure — tests that need it must guard.
func mustDeps(t *testing.T) *handler.Deps {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	db, err := pgxpool.New(ctx, pgDSN())
	if err != nil {
		if db != nil {
			db.Close()
		}
		t.Skipf("postgres not reachable (%v) — run `docker compose up -d`", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		t.Skipf("postgres ping failed (%v) — run `docker compose up -d`", err)
	}
	// Schema probe: if migrations didn't run, fail loudly instead of skipping.
	if _, err := db.Exec(ctx, "SELECT 1 FROM advertisers LIMIT 0"); err != nil {
		db.Close()
		t.Fatalf("schema probe failed on advertisers table (%v) — run migrations", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr(), Password: redisPassword()})
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		db.Close()
		t.Skipf("redis not reachable (%v) — run `docker compose up -d`", err)
	}

	// ClickHouse is optional: report tests use it, others don't.
	rs, err := reporting.NewStore(chAddr(), "default", chPassword())
	if err != nil {
		t.Logf("clickhouse not reachable (%v) — report tests will skip", err)
		rs = nil
	}

	t.Cleanup(func() {
		_ = rdb.Close()
		db.Close()
	})

	return &handler.Deps{
		Store:       campaign.NewStore(db),
		ReportStore: rs,
		BillingSvc:  billing.New(db),
		RegSvc:      registration.New(db),
		BudgetSvc:   budget.New(rdb),
		Redis:       rdb,
		AuditLog:    audit.NewLogger(db),
	}
}

// mustPool returns a raw pgxpool for tests that need to issue ad-hoc SQL
// (e.g. looking up ids by email for registration tests). Caller gets cleanup
// via t.Cleanup.
func mustPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, pgDSN())
	if err != nil {
		if db != nil {
			db.Close()
		}
		t.Skipf("postgres not reachable (%v)", err)
	}
	if err := db.Ping(ctx); err != nil {
		db.Close()
		t.Skipf("postgres ping failed (%v)", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// --- fixture builders ---

// fixtureSeq is a process-wide monotonic counter used to make fixture
// identifiers unique even across parallel subtests that would otherwise
// race on time.Now().UnixNano() within the same nanosecond.
var fixtureSeq atomic.Uint64

// safeName normalises a test name for use in an email local-part.
// Whitelist-based: only keep [A-Za-z0-9_-] and replace everything else
// with '-'. This covers '/' (subtests), spaces, ':', '(', ',', etc.
func safeName(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// newAdvertiser creates a unique advertiser and returns (id, apiKey).
// The advertiser starts with a 1_000_000 cent balance so campaign-start
// and topup tests don't fail on insufficient balance. Use this for any
// test that needs an authenticated context.
func newAdvertiser(t *testing.T, d *handler.Deps) (int64, string) {
	t.Helper()
	ctx := context.Background()
	key := handler.GenerateAPIKey()
	seq := fixtureSeq.Add(1)
	email := fmt.Sprintf("qa-%s-%d-%d@test.local", safeName(t.Name()), time.Now().UnixNano(), seq)
	adv := &campaign.Advertiser{
		CompanyName:  "QA-" + safeName(t.Name()),
		ContactEmail: email,
		APIKey:       key,
		BalanceCents: 1_000_000,
		BillingType:  "prepaid",
	}
	id, err := d.Store.CreateAdvertiser(ctx, adv)
	if err != nil {
		t.Fatalf("create advertiser: %v", err)
	}
	return id, key
}

// newCampaign creates a draft campaign for the given advertiser with enough
// budget that HandleStartCampaign's preconditions can be satisfied without
// test-side SQL patches. BudgetTotalCents >= BudgetDailyCents is required by
// the handler; we seed them 10:1 so multi-day runs in tests behave sensibly.
func newCampaign(t *testing.T, d *handler.Deps, advID int64) int64 {
	t.Helper()
	c := &campaign.Campaign{
		AdvertiserID:     advID,
		Name:             "QA-" + safeName(t.Name()),
		Status:           campaign.StatusDraft,
		BillingModel:     campaign.BillingCPM,
		BidCPMCents:      100,
		BudgetDailyCents: 10000,
		BudgetTotalCents: 100000,
	}
	id, err := d.Store.CreateCampaign(context.Background(), c)
	if err != nil {
		t.Fatalf("create campaign: %v", err)
	}
	return id
}

// newCreative creates a banner creative for the given campaign and marks it
// APPROVED so HandleStartCampaign's "has at least one approved creative" check
// passes. This matches the effective behavior of HandleCreateCreative in dev
// (it auto-approves via an ENV check), which store.CreateCreative bypasses.
func newCreative(t *testing.T, d *handler.Deps, campaignID int64) int64 {
	t.Helper()
	ctx := context.Background()
	cr := &campaign.Creative{
		CampaignID:     campaignID,
		Name:           "QA-" + safeName(t.Name()),
		AdType:         campaign.AdTypeBanner,
		Format:         "banner",
		Size:           "300x250",
		AdMarkup:       `<a href="https://example.com">ad</a>`,
		DestinationURL: "https://example.com",
	}
	id, err := d.Store.CreateCreative(ctx, cr)
	if err != nil {
		t.Fatalf("create creative: %v", err)
	}
	if err := d.Store.UpdateCreativeStatus(ctx, id, "approved"); err != nil {
		t.Fatalf("approve creative: %v", err)
	}
	return id
}

// --- request helpers ---

// authedReq builds an httptest.Request with JSON body + X-API-Key header.
// body may be nil for GET/DELETE calls.
func authedReq(t *testing.T, method, path string, body any, apiKey string) *http.Request {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// adminReq builds an httptest.Request with JSON body + X-Admin-Token header.
func adminReq(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("X-Admin-Token", adminToken())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// execPublic calls the bare public mux (no middleware, no auth check).
// Use this for tests that care about handler logic, not middleware.
func execPublic(t *testing.T, d *handler.Deps, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := handler.BuildPublicMux(d)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// execAuthed calls the public mux wrapped in the real auth.APIKeyMiddleware.
// Use this for handlers that read AdvertiserIDFromContext (campaign CRUD,
// list, start/pause, reports, etc.). The lookup closure mirrors
// routes.go:BuildPublicHandler so the handler sees the same advertiser
// identity it would in production, just without rate limiting. Requires the
// request to carry a valid X-API-Key (use authedReq(t, ..., apiKey)).
func execAuthed(t *testing.T, d *handler.Deps, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := handler.BuildPublicMux(d)
	lookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := d.Store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}
	chain := authMiddlewareImpl(lookup, mux)
	w := httptest.NewRecorder()
	chain.ServeHTTP(w, req)
	return w
}

// execAdmin calls the bare admin mux (no admin token check).
// Use this for handler-logic tests. Pair with execAdminWithAuth for the 401 case.
func execAdmin(t *testing.T, d *handler.Deps, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := handler.BuildAdminMux(d)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// execAdminWithAuth calls the admin mux behind AdminAuthMiddleware so 401
// branches can be tested.
func execAdminWithAuth(t *testing.T, d *handler.Deps, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	mux := handler.BuildAdminMux(d)
	withAuth := handler.AdminAuthMiddleware(mux)
	w := httptest.NewRecorder()
	withAuth.ServeHTTP(w, req)
	return w
}

// --- pub/sub subscribe helper ---

// subscribeUpdates returns a wait function that reports whether a
// campaign:updates message for campaignID arrived within d. Subscription is
// established (via pubsub.Receive) BEFORE the function returns, so callers
// can trigger handlers immediately after without racing the subscribe.
func subscribeUpdates(t *testing.T, rdb *redis.Client, campaignID int64) func(time.Duration) bool {
	t.Helper()
	ctx := context.Background()
	pubsub := rdb.Subscribe(ctx, "campaign:updates")
	recvCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := pubsub.Receive(recvCtx); err != nil {
		_ = pubsub.Close()
		t.Fatalf("subscribe confirm: %v", err)
	}
	got := make(chan struct{}, 1)
	go func() {
		for msg := range pubsub.Channel() {
			var p struct {
				CampaignID int64  `json:"campaign_id"`
				Action     string `json:"action"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &p); err != nil {
				continue
			}
			if p.CampaignID == campaignID {
				select {
				case got <- struct{}{}:
				default:
				}
				return
			}
		}
	}()
	t.Cleanup(func() { _ = pubsub.Close() })
	return func(d time.Duration) bool {
		select {
		case <-got:
			return true
		case <-time.After(d):
			return false
		}
	}
}

// subscribeUpdatesAction is like subscribeUpdates but only matches messages
// with a specific action ("updated" / "activated" / "paused" / etc.).
func subscribeUpdatesAction(t *testing.T, rdb *redis.Client, campaignID int64, action string) func(time.Duration) bool {
	t.Helper()
	ctx := context.Background()
	pubsub := rdb.Subscribe(ctx, "campaign:updates")
	recvCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := pubsub.Receive(recvCtx); err != nil {
		_ = pubsub.Close()
		t.Fatalf("subscribe confirm: %v", err)
	}
	got := make(chan struct{}, 1)
	go func() {
		for msg := range pubsub.Channel() {
			var p struct {
				CampaignID int64  `json:"campaign_id"`
				Action     string `json:"action"`
			}
			if err := json.Unmarshal([]byte(msg.Payload), &p); err != nil {
				continue
			}
			if p.CampaignID == campaignID && p.Action == action {
				select {
				case got <- struct{}{}:
				default:
				}
				return
			}
		}
	}()
	t.Cleanup(func() { _ = pubsub.Close() })
	return func(d time.Duration) bool {
		select {
		case <-got:
			return true
		case <-time.After(d):
			return false
		}
	}
}

// --- ClickHouse fixture helper ---

// mustCHConn opens a raw clickhouse-go native conn for fixture inserts.
// Uses chAddr() (native protocol, default localhost:20001 on the biz stack)
// with the password from chPassword().
func mustCHConn(t *testing.T) chdriver.Conn {
	t.Helper()
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{chAddr()},
		Auth: clickhouse.Auth{Database: "default", Username: "default", Password: chPassword()},
	})
	if err != nil {
		t.Skipf("clickhouse not reachable: %v", err)
	}
	if err := conn.Ping(context.Background()); err != nil {
		t.Skipf("clickhouse ping failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// insertBidLog seeds N rows into bid_log for (advertiserID, campaignID,
// creativeID) with the given event type. Used by report tests that need
// pre-existing analytics data without going through the bidder.
//
// eventType ∈ {"bid","win","loss","impression","click","conversion"}.
func insertBidLog(t *testing.T, conn chdriver.Conn, advertiserID, campaignID, creativeID int64, eventType string, n int) {
	t.Helper()
	validTypes := map[string]bool{
		"bid":        true,
		"win":        true,
		"loss":       true,
		"impression": true,
		"click":      true,
		"conversion": true,
	}
	if !validTypes[eventType] {
		t.Fatalf("unknown event type: %s", eventType)
	}
	batch, err := conn.PrepareBatch(context.Background(),
		`INSERT INTO bid_log (event_date, event_time, campaign_id, creative_id, advertiser_id, exchange_id, request_id, geo_country, device_os, device_id, bid_price_cents, clear_price_cents, charge_cents, event_type, loss_reason)`)
	if err != nil {
		t.Fatalf("prepare batch: %v", err)
	}
	now := time.Now()
	for i := 0; i < n; i++ {
		if err := batch.Append(
			now, now,
			uint64(campaignID), uint64(creativeID), uint64(advertiserID),
			"qa-exchange",
			fmt.Sprintf("%s-%d-%d", safeName(t.Name()), now.UnixNano(), i),
			"US", "iOS", fmt.Sprintf("device-%d", i),
			uint32(100), uint32(80), uint32(80), eventType, "",
		); err != nil {
			t.Fatalf("append row %d: %v", i, err)
		}
	}
	if err := batch.Send(); err != nil {
		t.Fatalf("send batch: %v", err)
	}
}

// --- misc ---

func nowNano() int64              { return time.Now().UnixNano() }
func contains(s, sub string) bool { return strings.Contains(s, sub) }

// decodeJSON is a convenience wrapper for reading response bodies.
func decodeJSON(t *testing.T, r *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(out); err != nil {
		t.Fatalf("decode json (body=%s): %v", r.Body.String(), err)
	}
}
