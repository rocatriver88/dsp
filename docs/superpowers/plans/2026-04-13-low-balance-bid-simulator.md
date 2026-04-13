# Low Balance Alert + Bid Simulator Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add low-balance warning on dashboard + bid outcome simulator on campaign detail page.

**Architecture:** Phase A is pure frontend (compute balance vs daily budget from existing data). Phase B adds a ClickHouse query for historical bid simulation, a new API endpoint, and a slider UI on campaign detail.

**Tech Stack:** Go 1.26, ClickHouse SQL, Next.js 16 / React 19 / TypeScript

---

## File Structure

**Phase A (Low Balance Alert):**
```
web/app/page.tsx                    # Add warning banner
cmd/autopilot/continuous.go         # Add balance check in health loop
```

**Phase B (Bid Simulator):**
```
internal/reporting/store.go         # Add SimulateBid query method
internal/handler/report.go          # Add HandleBidSimulate handler
cmd/api/main.go                     # Register route
web/app/campaigns/[id]/page.tsx     # Add simulator UI
```

---

## Phase A: Low Balance Alert

### Task 1: Dashboard Warning Banner

**Files:**
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Add low balance detection and banner**

After the existing `const totalBudget = ...` line (~line 27), add balance check logic and a warning banner in the JSX:

```tsx
// After line 27
const activeDailyBudget = active.reduce((sum, c) => sum + c.budget_daily_cents, 0);
const balanceCents = overview?.balance_cents || 0;
const isLowBalance = balanceCents > 0 && activeDailyBudget > 0 && balanceCents < activeDailyBudget;
```

In the JSX, before the stat cards grid (after the `<h2>概览</h2>` heading), add:

```tsx
{isLowBalance && (
  <div className="mb-4 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 text-sm text-yellow-800">
    <span className="font-medium">⚠ 余额不足：</span>
    当前余额 ¥{(balanceCents / 100).toLocaleString()}，
    活跃 Campaign 日预算总计 ¥{(activeDailyBudget / 100).toLocaleString()}。
    请及时<Link href="/billing" className="text-blue-600 underline hover:text-blue-700">充值</Link>以避免投放中断。
  </div>
)}
```

- [ ] **Step 2: Verify**

Run: `cd web && npx tsc --noEmit 2>&1 | grep "app/page.tsx" || echo "No new errors"`

- [ ] **Step 3: Commit**

```bash
git add web/app/page.tsx
git commit -m "feat(web): add low balance warning banner on dashboard"
```

---

### Task 2: Autopilot Balance Check

**Files:**
- Modify: `cmd/autopilot/continuous.go`

- [ ] **Step 1: Add checkBalances method**

After the `checkHealth()` method, add:

```go
func (s *ContinuousSimulator) checkBalances() {
	advs, err := s.client.ListAdvertisers()
	if err != nil {
		log.Printf("[BALANCE] Failed to list advertisers: %v", err)
		return
	}

	for _, adv := range advs {
		campaigns, err := s.client.ListCampaignsForAdvertiser(adv.ID)
		if err != nil {
			continue
		}
		var activeDailyBudget int64
		for _, c := range campaigns {
			if c.Status == "active" {
				activeDailyBudget += c.BudgetDailyCents
			}
		}
		if activeDailyBudget > 0 && adv.BalanceCents < activeDailyBudget {
			msg := fmt.Sprintf("Advertiser %d (%s): balance ¥%.2f < daily budget ¥%.2f",
				adv.ID, adv.CompanyName, float64(adv.BalanceCents)/100, float64(activeDailyBudget)/100)
			log.Printf("[BALANCE] LOW: %s", msg)
			s.alerter.Send("Low Balance Alert", msg)
		}
	}
}
```

Note: `ListAdvertisers` and `ListCampaignsForAdvertiser` need to be added to the autopilot client. However, the autopilot client already has `ListCampaigns()` (returns campaigns for the current API key). For the admin-level check, we need a new method that calls the admin API.

Add to `cmd/autopilot/client.go`:

```go
type AdminAdvertiser struct {
	ID              int64  `json:"id"`
	CompanyName     string `json:"company_name"`
	BalanceCents    int64  `json:"balance_cents"`
	ActiveCampaigns int    `json:"active_campaigns"`
	TotalSpentCents int64  `json:"total_spent_cents"`
}

func (c *DSPClient) AdminListAdvertisers(adminURL string) ([]AdminAdvertiser, error) {
	req, _ := http.NewRequest("GET", adminURL+"/api/v1/admin/advertisers?limit=500", nil)
	req.Header.Set("X-Admin-Token", c.AdminToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("admin list advertisers: status %d", resp.StatusCode)
	}
	var result []AdminAdvertiser
	json.Unmarshal(data, &result)
	return result, nil
}
```

Simplify the balance check — use the `active_campaigns` count from the admin advertisers list (already includes JOIN data from I7 fix). Since we don't have per-advertiser daily budget total from the admin API, use a config threshold instead:

Actually, simpler approach: the admin advertisers endpoint now returns `active_campaigns` and `total_spent_cents`. But it doesn't return daily budget sum. The simplest approach for autopilot is to use the `LowBalanceAlertCents` config value from guardrails:

```go
func (s *ContinuousSimulator) checkBalances() {
	if s.lowBalanceThreshold <= 0 {
		return
	}
	advs, err := s.client.AdminListAdvertisers(s.adminURL)
	if err != nil {
		log.Printf("[BALANCE] Failed to list advertisers: %v", err)
		return
	}
	for _, adv := range advs {
		if adv.ActiveCampaigns > 0 && adv.BalanceCents < s.lowBalanceThreshold {
			msg := fmt.Sprintf("Advertiser %d (%s): balance ¥%.2f below threshold ¥%.2f, %d active campaigns",
				adv.ID, adv.CompanyName,
				float64(adv.BalanceCents)/100,
				float64(s.lowBalanceThreshold)/100,
				adv.ActiveCampaigns)
			log.Printf("[BALANCE] LOW: %s", msg)
			s.alerter.Send("Low Balance Alert", msg)
		}
	}
}
```

Add `lowBalanceThreshold int64` field to `ContinuousSimulator` and `AUTOPILOT_LOW_BALANCE_CENTS` to config (default 100000 = ¥1000).

- [ ] **Step 2: Wire into health check loop**

In the `Run()` method, call `s.checkBalances()` alongside `s.checkHealth()` in the health ticker case:

```go
		case <-healthTicker.C:
			go s.checkHealth()
			go s.checkBalances()
```

- [ ] **Step 3: Run tests**

Run: `go test ./cmd/autopilot/ -short -v -count=1 2>&1 | tail -5`

- [ ] **Step 4: Commit**

```bash
git add cmd/autopilot/continuous.go cmd/autopilot/client.go cmd/autopilot/config.go
git commit -m "feat(autopilot): add low balance alert in continuous mode"
```

---

## Phase B: Bid Simulator

### Task 3: ClickHouse SimulateBid Query

**Files:**
- Modify: `internal/reporting/store.go`

- [ ] **Step 1: Add SimulateBid method and result type**

```go
type BidSimulation struct {
	CurrentBidCPMCents    int     `json:"current_bid_cpm_cents"`
	SimulatedBidCPMCents  int     `json:"simulated_bid_cpm_cents"`
	TotalBids             int64   `json:"total_bids"`
	ActualWins            int64   `json:"actual_wins"`
	CurrentWinRate        float64 `json:"current_win_rate"`
	SimulatedWins         int64   `json:"simulated_wins"`
	SimulatedWinRate      float64 `json:"simulated_win_rate"`
	SimulatedSpendCents   int64   `json:"simulated_spend_cents"`
	MedianClearPriceCents int     `json:"median_clear_price_cents"`
	MaxClearPriceCents    int     `json:"max_clear_price_cents"`
	DataDays              int     `json:"data_days"`
}

func (s *Store) SimulateBid(ctx context.Context, campaignID uint64, simulatedCPMCents int) (*BidSimulation, error) {
	var result BidSimulation
	result.SimulatedBidCPMCents = simulatedCPMCents
	result.DataDays = 7

	err := s.conn.QueryRow(ctx, `
		SELECT
			count()                                                      AS total_bids,
			countIf(event_type = 'win')                                  AS actual_wins,
			countIf(clear_price_cents > 0 AND clear_price_cents <= $2)   AS simulated_wins,
			sumIf(clear_price_cents, clear_price_cents > 0 AND clear_price_cents <= $2) AS simulated_spend_cents,
			toUInt32(quantileExactIf(0.5)(clear_price_cents, clear_price_cents > 0)) AS median_clear_price,
			max(clear_price_cents)                                       AS max_clear_price
		FROM bid_log
		WHERE campaign_id = $1
		  AND event_date >= today() - 7
	`, campaignID, simulatedCPMCents).Scan(
		&result.TotalBids,
		&result.ActualWins,
		&result.SimulatedWins,
		&result.SimulatedSpendCents,
		&result.MedianClearPriceCents,
		&result.MaxClearPriceCents,
	)
	if err != nil {
		return nil, fmt.Errorf("simulate bid: %w", err)
	}

	if result.TotalBids > 0 {
		result.CurrentWinRate = float64(result.ActualWins) / float64(result.TotalBids)
		result.SimulatedWinRate = float64(result.SimulatedWins) / float64(result.TotalBids)
	}

	return &result, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `go vet ./internal/reporting/`

- [ ] **Step 3: Commit**

```bash
git add internal/reporting/store.go
git commit -m "feat(reporting): add SimulateBid ClickHouse query"
```

---

### Task 4: API Handler + Route

**Files:**
- Modify: `internal/handler/report.go`
- Modify: `cmd/api/main.go`

- [ ] **Step 1: Add HandleBidSimulate handler**

Append to `internal/handler/report.go`:

```go
// HandleBidSimulate godoc
// @Summary Simulate bid outcome
// @Tags reports
// @Security ApiKeyAuth
// @Produce json
// @Param id path int true "Campaign ID"
// @Param bid_cpm_cents query int true "Simulated CPM bid in cents"
// @Success 200 {object} reporting.BidSimulation
// @Router /reports/campaign/{id}/simulate [get]
func (d *Deps) HandleBidSimulate(w http.ResponseWriter, r *http.Request) {
	if d.ReportStore == nil {
		WriteError(w, http.StatusServiceUnavailable, "ClickHouse not connected")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}

	bidStr := r.URL.Query().Get("bid_cpm_cents")
	bidCPM, err := strconv.Atoi(bidStr)
	if err != nil || bidCPM <= 0 {
		WriteError(w, http.StatusBadRequest, "bid_cpm_cents must be a positive integer")
		return
	}

	// Get campaign to fill current bid
	advID := auth.AdvertiserIDFromContext(r.Context())
	camp, err := d.Store.GetCampaignForAdvertiser(r.Context(), id, advID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "campaign not found")
		return
	}

	sim, err := d.ReportStore.SimulateBid(r.Context(), uint64(id), bidCPM)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sim.CurrentBidCPMCents = camp.BidCPMCents

	WriteJSON(w, http.StatusOK, sim)
}
```

Add `"strconv"` to imports if not present (already there).

- [ ] **Step 2: Register route**

In `cmd/api/main.go`, in the public mux routes section, add after the attribution route:

```go
publicMux.HandleFunc("GET /api/v1/reports/campaign/{id}/simulate", h.HandleBidSimulate)
```

- [ ] **Step 3: Build**

Run: `go build ./cmd/api/`

- [ ] **Step 4: Commit**

```bash
git add internal/handler/report.go cmd/api/main.go
git commit -m "feat(api): add bid simulation endpoint"
```

---

### Task 5: Frontend Bid Simulator UI

**Files:**
- Modify: `web/app/campaigns/[id]/page.tsx`
- Modify: `web/lib/api.ts`

- [ ] **Step 1: Add API method**

In `web/lib/api.ts`, add:

```typescript
interface BidSimulation {
  current_bid_cpm_cents: number;
  simulated_bid_cpm_cents: number;
  total_bids: number;
  actual_wins: number;
  current_win_rate: number;
  simulated_wins: number;
  simulated_win_rate: number;
  simulated_spend_cents: number;
  median_clear_price_cents: number;
  max_clear_price_cents: number;
  data_days: number;
}

// Add to the api object:
async simulateBid(campaignId: number, bidCPMCents: number): Promise<BidSimulation> {
  return request(`/api/v1/reports/campaign/${campaignId}/simulate?bid_cpm_cents=${bidCPMCents}`);
},
```

- [ ] **Step 2: Add BidSimulator component to campaign detail**

In `web/app/campaigns/[id]/page.tsx`, add a `BidSimulator` component before the `InfoRow` component:

```tsx
function BidSimulator({ campaignId, currentBidCPM }: { campaignId: number; currentBidCPM: number }) {
  const [simBid, setSimBid] = useState(currentBidCPM);
  const [result, setResult] = useState<{
    current_win_rate: number;
    simulated_win_rate: number;
    simulated_wins: number;
    simulated_spend_cents: number;
    total_bids: number;
    actual_wins: number;
  } | null>(null);
  const [loading, setLoading] = useState(false);

  const runSimulation = useCallback(() => {
    if (simBid <= 0) return;
    setLoading(true);
    api.simulateBid(campaignId, simBid)
      .then(setResult)
      .catch(() => setResult(null))
      .finally(() => setLoading(false));
  }, [campaignId, simBid]);

  useEffect(() => {
    const timer = setTimeout(runSimulation, 500);
    return () => clearTimeout(timer);
  }, [runSimulation]);

  const winDelta = result ? result.simulated_win_rate - result.current_win_rate : 0;

  return (
    <div className="rounded-lg bg-white p-5 mt-6">
      <h3 className="text-sm font-semibold mb-4">出价模拟器</h3>
      <div className="flex items-center gap-4 mb-4">
        <label className="text-sm text-gray-500 flex-shrink-0">模拟 CPM</label>
        <input
          type="range"
          min={100} max={Math.max(currentBidCPM * 3, 2000)} step={50}
          value={simBid}
          onChange={(e) => setSimBid(Number(e.target.value))}
          className="flex-1"
        />
        <span className="text-sm font-geist tabular-nums w-20 text-right">
          ¥{(simBid / 100).toFixed(2)}
        </span>
      </div>

      {loading ? (
        <p className="text-xs text-gray-400">计算中...</p>
      ) : result && result.total_bids > 0 ? (
        <div className="grid grid-cols-3 gap-4 text-center">
          <div>
            <p className="text-xs text-gray-500 mb-1">预估胜率</p>
            <p className="text-lg font-geist tabular-nums font-semibold">
              {(result.simulated_win_rate * 100).toFixed(1)}%
            </p>
            <p className={`text-xs ${winDelta >= 0 ? "text-green-600" : "text-red-500"}`}>
              {winDelta >= 0 ? "+" : ""}{(winDelta * 100).toFixed(1)}%
            </p>
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">预估曝光</p>
            <p className="text-lg font-geist tabular-nums font-semibold">
              {result.simulated_wins.toLocaleString()}
            </p>
            <p className="text-xs text-gray-400">/ {result.total_bids.toLocaleString()} 竞价</p>
          </div>
          <div>
            <p className="text-xs text-gray-500 mb-1">预估花费</p>
            <p className="text-lg font-geist tabular-nums font-semibold">
              ¥{(result.simulated_spend_cents / 100).toFixed(2)}
            </p>
            <p className="text-xs text-gray-400">过去 7 天</p>
          </div>
        </div>
      ) : (
        <p className="text-xs text-gray-400">暂无历史竞价数据，投放后可使用模拟器</p>
      )}
    </div>
  );
}
```

- [ ] **Step 3: Wire into campaign detail page**

In the campaign detail page JSX, after the "素材" section and before the "查看详细报表" link, add:

```tsx
{campaign.billing_model === "cpm" && (
  <BidSimulator campaignId={campaign.id} currentBidCPM={campaign.bid_cpm_cents} />
)}
```

Add `useCallback` to the imports if not present.

- [ ] **Step 4: Verify**

Run: `cd web && npx tsc --noEmit 2>&1 | grep "campaigns" || echo "No new errors"`

- [ ] **Step 5: Commit**

```bash
git add web/lib/api.ts web/app/campaigns/\[id\]/page.tsx
git commit -m "feat(web): add bid simulator UI with slider on campaign detail"
```

---

### Task 6: Regenerate OpenAPI + API Types

- [ ] **Step 1: Regenerate**

```bash
swag init -g cmd/api/main.go -o docs/ --parseDependency --parseInternal
cd web && npx swagger2openapi ../docs/swagger.yaml -o ../docs/openapi3.yaml
cd web && npx openapi-typescript ../docs/openapi3.yaml -o lib/api-types.ts
```

- [ ] **Step 2: Commit**

```bash
git add docs/ web/lib/api-types.ts
git commit -m "chore: regenerate OpenAPI spec + TypeScript types after bid simulator"
```

---

### Task 7: Smoke Test

- [ ] **Step 1: Go tests**

Run: `go test ./internal/reporting/ ./internal/handler/ ./cmd/autopilot/ -short -v -count=1 2>&1 | tail -10`

- [ ] **Step 2: Build all**

Run: `go build ./cmd/api/ && go build ./cmd/autopilot/`

- [ ] **Step 3: Go vet**

Run: `go vet ./...`

---

## Task Dependency Graph

```
Task 1 (dashboard banner) ── independent
Task 2 (autopilot balance) ── independent
Task 3 (ClickHouse query) ── independent
Task 4 (API handler) ──────── depends on Task 3
Task 5 (frontend UI) ──────── depends on Task 4
Task 6 (regenerate types) ──── depends on Task 4
Task 7 (smoke test) ─────────── depends on all
```

Tasks 1, 2, 3 can run in parallel. Task 4 depends on 3. Task 5 depends on 4.
