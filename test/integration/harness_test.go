//go:build integration

// Package integration holds the V5 §P2 regression tests that exercise the
// full handler stack against a real Postgres / Redis / ClickHouse backend.
//
// Run with:
//
//	./scripts/test-env.sh up
//	./scripts/test-env.sh migrate
//	go test -tags integration ./test/integration/... -count=1 -v
//
// These tests are gated behind the `integration` build tag so the default
// `go test ./...` short suite stays fast and offline.
package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/heartgryphon/dsp/internal/auth"
	"github.com/heartgryphon/dsp/internal/billing"
	"github.com/heartgryphon/dsp/internal/campaign"
	"github.com/heartgryphon/dsp/internal/handler"
	"github.com/heartgryphon/dsp/internal/reporting"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// testDeps holds the real backing services and the wired handler.Deps for a
// single integration test run. It is built once per TestMain and shared
// across every test case; each test is responsible for cleaning up any
// rows it created via the truncateAll helper.
type testDeps struct {
	Deps   *handler.Deps
	DB     *pgxpool.Pool
	Redis  *redis.Client
	Server *httptest.Server
}

var shared *testDeps

// TestMain connects to the dsp-test stack, builds a real handler.Deps,
// and stands up an httptest server wrapped in the production auth
// middleware. If any connection fails the integration suite is skipped
// with a clear hint about scripts/test-env.sh.
func TestMain(m *testing.M) {
	if os.Getenv("DSP_SKIP_INTEGRATION") != "" {
		fmt.Println("DSP_SKIP_INTEGRATION set; skipping integration suite")
		os.Exit(0)
	}

	setupDeps()
	if shared == nil {
		fmt.Println("integration setup failed; did you run ./scripts/test-env.sh up && migrate ?")
		os.Exit(0) // skip, don't fail the whole go test run
	}
	defer shared.teardown()

	os.Exit(m.Run())
}

func setupDeps() {
	dbHost := envOr("DB_HOST", "localhost")
	dbPort := envOr("DB_PORT", "6432")
	dbUser := envOr("DB_USER", "dsp")
	dbPass := envOr("DB_PASSWORD", "dsp_test_password")
	dbName := envOr("DB_NAME", "dsp_test")
	redisAddr := envOr("REDIS_ADDR", "localhost:7380")
	redisPass := envOr("REDIS_PASSWORD", "dsp_test_password")
	chAddr := envOr("CLICKHOUSE_ADDR", "localhost:10001")
	chUser := envOr("CLICKHOUSE_USER", "default")
	chPass := envOr("CLICKHOUSE_PASSWORD", "dsp_test_password")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPass, dbHost, dbPort, dbName)
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Printf("pgxpool connect: %v\n", err)
		return
	}
	if err := db.Ping(ctx); err != nil {
		fmt.Printf("pgxpool ping: %v\n", err)
		db.Close()
		return
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, Password: redisPass})
	if err := rdb.Ping(ctx).Err(); err != nil {
		fmt.Printf("redis ping: %v\n", err)
		db.Close()
		return
	}

	var reportStore *reporting.Store
	rs, chErr := reporting.NewStore(chAddr, chUser, chPass)
	if chErr != nil {
		fmt.Printf("clickhouse connect: %v (reports will 503 in tests)\n", chErr)
	} else {
		reportStore = rs
	}

	store := campaign.NewStore(db)
	billingSvc := billing.New(db)

	deps := &handler.Deps{
		Store:       store,
		ReportStore: reportStore,
		BillingSvc:  billingSvc,
		Redis:       rdb,
	}

	srv := httptest.NewServer(buildAuthedMux(deps, store))

	shared = &testDeps{
		Deps:   deps,
		DB:     db,
		Redis:  rdb,
		Server: srv,
	}

	if err := shared.truncateAll(ctx); err != nil {
		fmt.Printf("initial truncate: %v\n", err)
	}
}

// buildAuthedMux registers the routes exercised by the P2 regression suite
// and wraps the mux in the production APIKeyMiddleware. This duplicates a
// subset of cmd/api/main.go's route table; the V5 §P2 tests assert on the
// routes declared here so adding a new tenant-scoped handler requires both
// a cmd/api registration and an entry in this function — drift surfaces
// as a missing test, which is the intended behavior.
func buildAuthedMux(d *handler.Deps, store *campaign.Store) http.Handler {
	mux := http.NewServeMux()

	// Advertisers (self-service + create)
	mux.HandleFunc("POST /api/v1/advertisers", d.HandleCreateAdvertiser)
	mux.HandleFunc("GET /api/v1/advertisers/{id}", d.HandleGetAdvertiser)

	// Campaigns (self-scoped)
	mux.HandleFunc("POST /api/v1/campaigns", d.HandleCreateCampaign)
	mux.HandleFunc("GET /api/v1/campaigns/{id}", d.HandleGetCampaign)
	mux.HandleFunc("PUT /api/v1/campaigns/{id}", d.HandleUpdateCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/start", d.HandleStartCampaign)
	mux.HandleFunc("POST /api/v1/campaigns/{id}/pause", d.HandlePauseCampaign)

	// Creatives
	mux.HandleFunc("GET /api/v1/campaigns/{id}/creatives", d.HandleListCreatives)
	mux.HandleFunc("POST /api/v1/creatives", d.HandleCreateCreative)
	mux.HandleFunc("PUT /api/v1/creatives/{id}", d.HandleUpdateCreative)
	mux.HandleFunc("DELETE /api/v1/creatives/{id}", d.HandleDeleteCreative)

	// Billing
	mux.HandleFunc("POST /api/v1/billing/topup", d.HandleTopUp)
	mux.HandleFunc("GET /api/v1/billing/transactions", d.HandleTransactions)
	mux.HandleFunc("GET /api/v1/billing/balance", d.HandleBalance)
	mux.HandleFunc("GET /api/v1/billing/balance/{id}", d.HandleBalance) // legacy alias

	// Reports (all five in V5 §P0 plus simulate which also enforces owner check)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/stats", d.HandleCampaignStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/hourly", d.HandleHourlyStats)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/geo", d.HandleGeoBreakdown)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/bids", d.HandleBidTransparency)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/attribution", d.HandleAttribution)
	mux.HandleFunc("GET /api/v1/reports/campaign/{id}/simulate", d.HandleBidSimulate)

	// Export (already had owner check but Round 1 I1 made sure they're
	// under regression coverage too).
	mux.HandleFunc("GET /api/v1/export/campaign/{id}/stats", d.HandleExportCampaignCSV)
	mux.HandleFunc("GET /api/v1/export/campaign/{id}/bids", d.HandleExportBidsCSV)

	// Admin routes (HandleListAdvertisers, etc.) are intentionally NOT
	// registered here: in production they live behind AdminAuthMiddleware
	// on a separate mux, not APIKeyMiddleware. Tests that need to verify
	// admin handler behavior invoke the handler method directly via
	// httptest.NewRecorder, bypassing middleware — see
	// TestListAdvertisers_OmitsAPIKey in admin_list_test.go.

	// APIKeyMiddleware routes auth-free requests to 401, which is what the
	// 401-path regression tests actually want. It also turns a valid key
	// into an auth context entry that the handlers then read.
	apiKeyLookup := func(ctx context.Context, key string) (int64, string, string, error) {
		adv, err := store.GetAdvertiserByAPIKey(ctx, key)
		if err != nil {
			return 0, "", "", err
		}
		return adv.ID, adv.CompanyName, adv.ContactEmail, nil
	}
	return auth.APIKeyMiddleware(apiKeyLookup)(mux)
}

// truncateAll wipes every table the integration suite touches. Tests are
// expected to create the state they need and rely on truncation at the
// boundaries; this keeps cases independent without per-case fixtures.
func (d *testDeps) truncateAll(ctx context.Context) error {
	tables := []string{
		"audit_log",
		"conversions",
		"daily_reconciliation",
		"transactions",
		"invoices",
		"creatives",
		"campaigns",
		"registration_requests",
		"invite_codes",
		"advertisers",
	}
	for _, t := range tables {
		if _, err := d.DB.Exec(ctx, "TRUNCATE TABLE "+t+" CASCADE"); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	// Flush anything dedup-related in Redis so tests start clean.
	if err := d.Redis.FlushDB(ctx).Err(); err != nil {
		return fmt.Errorf("redis flushdb: %w", err)
	}
	return nil
}

func (d *testDeps) teardown() {
	if d.Server != nil {
		d.Server.Close()
	}
	if d.Redis != nil {
		_ = d.Redis.Close()
	}
	if d.DB != nil {
		d.DB.Close()
	}
}

// createAdvertiser inserts an advertiser via the Store and returns its
// id and API key. The key is generated by the same helper used in
// production so the auth middleware lookup works unchanged.
func (d *testDeps) createAdvertiser(t *testing.T, company, email string) (int64, string) {
	t.Helper()
	apiKey := handler.GenerateAPIKey()
	adv := &campaign.Advertiser{
		CompanyName:  company,
		ContactEmail: email,
		APIKey:       apiKey,
		BalanceCents: 1_000_000, // 10k yuan — enough to top up and start campaigns
		BillingType:  "prepaid",
	}
	id, err := d.Deps.Store.CreateAdvertiser(context.Background(), adv)
	if err != nil {
		t.Fatalf("create advertiser %q: %v", company, err)
	}
	return id, apiKey
}

// createCampaign inserts a minimal active-ready campaign for the given
// advertiser and returns its id. It's a draft — tests that want the
// campaign to be active should POST /campaigns/{id}/start after adding
// a creative.
func (d *testDeps) createCampaign(t *testing.T, advID int64, name string) int64 {
	t.Helper()
	c := &campaign.Campaign{
		AdvertiserID:     advID,
		Name:             name,
		BillingModel:     "cpm",
		BudgetTotalCents: 100_000,
		BudgetDailyCents: 10_000,
		BidCPMCents:      500,
		Targeting:        []byte(`{}`),
	}
	id, err := d.Deps.Store.CreateCampaign(context.Background(), c)
	if err != nil {
		t.Fatalf("create campaign %q: %v", name, err)
	}
	return id
}

// createCreative inserts a creative under the given campaign.
func (d *testDeps) createCreative(t *testing.T, campaignID int64, name string) int64 {
	t.Helper()
	cr := &campaign.Creative{
		CampaignID:     campaignID,
		Name:           name,
		AdType:         "banner",
		Format:         "banner", // one of banner/native/video per 001_init.sql
		Size:           "300x250",
		AdMarkup:       "<div>test</div>",
		DestinationURL: "https://example.test",
	}
	id, err := d.Deps.Store.CreateCreative(context.Background(), cr)
	if err != nil {
		t.Fatalf("create creative %q: %v", name, err)
	}
	return id
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
