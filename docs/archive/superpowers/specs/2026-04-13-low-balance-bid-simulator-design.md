# 余额低告警 + Bid Simulator 设计

## 概述

两个独立功能，分两个 Phase 实现：
- **Phase A:** 余额低告警 — 前端 banner + autopilot 定时检测
- **Phase B:** Bid Simulator — 历史数据回放模拟出价效果

---

## Phase A: 余额低告警

### 触发条件

广告主余额 < 其所有活跃 campaign 的日预算总和（即"不够撑一天"）

### 前端

Dashboard (`/`) 顶部显示黄色 warning banner：

```
⚠ 账户余额不足：当前余额 ¥2,000，活跃 Campaign 日预算总计 ¥5,000。请及时充值以避免投放中断。
```

逻辑：`balance_cents < sum(active_campaigns.budget_daily_cents)`

数据来源：Dashboard 页面已有 `overview` API 返回 `balance_cents`，campaigns 列表已有 `budget_daily_cents`。纯前端计算，不需要新 API。

### 后端（autopilot continuous 模式）

每小时检测所有广告主余额，低于阈值时发钉钉/飞书告警。

在 `ContinuousSimulator.checkHealth()` 中增加余额检测逻辑：
- 获取广告主列表
- 对比余额 vs 活跃 campaign 日预算总和
- 低余额时调 `alerter.Send()`

### 文件

- 修改: `web/app/page.tsx` — 加 warning banner
- 修改: `cmd/autopilot/continuous.go` — 加余额检测

---

## Phase B: Bid Simulator

### API

`GET /api/v1/reports/campaign/{id}/simulate?bid_cpm_cents=N`

**Response:**
```json
{
  "current_bid_cpm_cents": 500,
  "simulated_bid_cpm_cents": 800,
  "total_bids": 1000,
  "actual_wins": 420,
  "current_win_rate": 0.42,
  "simulated_wins": 650,
  "simulated_win_rate": 0.65,
  "simulated_spend_cents": 5200,
  "median_clear_price_cents": 350,
  "max_clear_price_cents": 1200,
  "data_days": 7
}
```

### ClickHouse 查询

```sql
SELECT
  count()                                            AS total_bids,
  countIf(event_type = 'win')                        AS actual_wins,
  countIf(clear_price_cents > 0
    AND clear_price_cents <= {simulated_cpm})         AS simulated_wins,
  sumIf(clear_price_cents,
    clear_price_cents > 0
    AND clear_price_cents <= {simulated_cpm})         AS simulated_spend_cents,
  quantileExact(0.5)(clear_price_cents)              AS median_clear_price,
  max(clear_price_cents)                              AS max_clear_price
FROM bid_log
WHERE campaign_id = {campaign_id}
  AND event_date >= today() - 7
```

数据不足时返回全 0，不报错。

### 前端

Campaign 详情页 (`/campaigns/[id]`) 底部加"出价模拟器"区域：
- 滑块或输入框设置模拟出价
- 实时显示：模拟胜率、预估曝光、预估花费
- 和当前出价的对比（差值、箭头方向）

### 文件

- 修改: `internal/reporting/store.go` — 加 `SimulateBid` 查询方法
- 修改: `internal/handler/reports.go` — 加 `HandleBidSimulate` handler
- 修改: `cmd/api/main.go` — 注册路由
- 修改: `web/app/campaigns/[id]/page.tsx` — 加模拟器 UI
