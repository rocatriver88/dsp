# biz ↔ engine 契约

本文档定义业务系统(api / web)与投放引擎(bidder / consumer)之间的跨系统数据契约。两侧代码、测试、文档的权威来源,任何跨边界改动必须先更新本文件。

**适用范围**
- biz 侧:`cmd/api`、`web/`、`internal/handler`、`internal/campaign`、`internal/billing`、`internal/reporting`
- engine 侧:`cmd/bidder`、`cmd/consumer`、`internal/bidder`、`internal/events`
- 共享基础设施:PostgreSQL、Redis、Kafka、ClickHouse

---

## 1. biz → engine:业务数据同步

**目的**:让 bidder 在出价时能拿到最新的广告主、广告、创意、预算、定向数据。

### 数据载体

| 类型 | 对象 | 文件 / 表 | 写方 | 读方 |
|---|---|---|---|---|
| Postgres 表 | `advertisers` | `migrations/001_init.sql` | api(`internal/campaign/store.go`) | bidder(加载时) |
| Postgres 表 | `campaigns`(含 `targeting` JSONB 列) | `001_init.sql` + `007_campaign_pause.sql` + `010_phase3.sql` | api | bidder |
| Postgres 表 | `creatives` | `001_init.sql` + `005_ad_formats.sql` | api | bidder |
| Postgres 表 | billing / balance | `003_billing.sql` + `006_billing_model.sql` | api(`internal/billing`) | bidder(读余额判断能否出价) |

### 同步机制:Redis pub/sub + 周期 reload 双保险

**通道**:`campaign:updates`

**Payload(JSON)**
```json
{
  "campaign_id": 12345,
  "action": "updated"
}
```

**合法 `action` 枚举**(严格对应 `internal/bidder/loader.go:240-262` 的 switch 分支):
| action | 语义 | loader 行为 |
|---|---|---|
| `activated` | 广告启动 | 从 DB 拉取,若 status=active 则载入缓存 |
| `updated` | 广告/创意属性变更 | 从 DB 拉取,若 status=active 则替换缓存条目 |
| `paused` | 广告暂停 | 从缓存移除 |
| `completed` | 广告完成 | 从缓存移除 |
| `deleted` | 广告删除 | 从缓存移除 |

**兜底**:bidder `CampaignLoader` 每 30 秒执行一次 `fullLoad`,调用 `store.ListActiveCampaigns` 全量刷新缓存(`internal/bidder/loader.go:80-165`)。pub/sub 丢消息不会导致永久不一致,但会引入最多 30 秒延迟。

### 发布点(权威清单)

biz 侧任何对影响 **已激活广告** 投放行为的写入都 **必须** 发布到 `campaign:updates`。发布统一通过 `bidder.NotifyCampaignUpdate(ctx, rdb, campaignID, action)`(定义在 `internal/bidder/loader.go:269-277`)。

| 路由 | Handler | action | 说明 |
|---|---|---|---|
| `POST /api/v1/campaigns` | `HandleCreateCampaign` | `updated` | 新建为 draft;publish 为未来状态扭转预热 |
| `PUT /api/v1/campaigns/{id}` | `HandleUpdateCampaign` | `updated` | 修改名称/出价/日预算/定向 |
| `POST /api/v1/campaigns/{id}/start` | `HandleStartCampaign` | `activated` | 进入 active 状态 |
| `POST /api/v1/campaigns/{id}/pause` | `HandlePauseCampaign` | `paused` | 进入 paused 状态 |
| `internal/autopause/service.go:117` | Autopause service | `paused` | 自动暂停触发时 |

### 已知缺口(非本轮 QA 范围)

以下写入路径 **当前不发布** 到 `campaign:updates`,仅依赖 30s 全量 reload 兜底,可能导致最多 30s 的行为不一致:

- `POST /api/v1/creatives`(新增创意):新创意要等下次 full reload 才会进 bidder 的 `CampaignWithCreatives` 缓存
- `PUT /api/v1/creatives/{id}`(编辑创意):ad_markup / destination_url 变更同上
- `DELETE /api/v1/creatives/{id}`(删除创意):bidder 可能继续使用已被删除的创意最多 30s

这些路径不在本轮 biz QA 的修复范围内;biz QA 在编写"创意 CRUD → 投放验证"用例时,需要么(a)只测 30s 后状态,要么(b)标注为"eventually consistent within 30s"。后续若确认需要修复,可在 biz 或后续轮次单独处理。

---

## 2. engine → biz:投放数据回收

**目的**:把投放过程中产生的 bid / impression / click / conversion / spend 事件回流到业务系统,供广告主看到实时的投放数据和扣费。

### 数据载体

| 机制 | 对象 | 文件 | 写方 | 读方 |
|---|---|---|---|---|
| Kafka topic | `dsp.bids` | `internal/events/producer.go:57` | bidder | consumer |
| Kafka topic | `dsp.impressions` | `internal/events/producer.go:57` | bidder | consumer |
| Kafka topic | `dsp.dead-letter` | `internal/events/producer.go:57` | consumer(失败回写) | 人工 |
| ClickHouse 表 | `bid_log` | `migrations/002_clickhouse.sql` + `008_clickhouse_attribution.sql` | consumer | api(`internal/reporting`) |
| Redis key | 日预算计数器 | `internal/budget` | bidder(win 时扣) | api(读余额/对账) |

### 消息 schema:`events.Event`

定义于 `internal/events/producer.go:35-48`,JSON 序列化:

```go
type Event struct {
    Type             string    `json:"type"`              // bid | win | loss | impression | click | conversion
    CampaignID       int64     `json:"campaign_id"`
    CreativeID       int64     `json:"creative_id,omitempty"`
    AdvertiserID     int64     `json:"advertiser_id,omitempty"`
    RequestID        string    `json:"request_id"`
    BidPrice         float64   `json:"bid_price,omitempty"`         // 出价(元)
    ClearPrice       float64   `json:"clear_price,omitempty"`       // 成交价(元, ADX 结算价)
    AdvertiserCharge float64   `json:"advertiser_charge,omitempty"` // 广告主实际扣费(元)
    GeoCountry       string    `json:"geo_country,omitempty"`
    DeviceOS         string    `json:"device_os,omitempty"`
    DeviceID         string    `json:"device_id,omitempty"`         // IDFA/GAID/OAID
    Timestamp        time.Time `json:"ts"`
}
```

**不变量**
- `request_id` 非空,同一 bid 生命周期内的 bid / win / impression / click 事件共享同一个 `request_id`
- `Type` 严格使用上面 6 个枚举值之一
- 金额字段单位是 **元(float64)**,不是分;consumer 在写 ClickHouse 时转换为 `*_cents UInt32`

### 发布点(bidder)

| 路由 | Handler | 发布 topic(s) | 事件类型 |
|---|---|---|---|
| `POST /bid` | 出价 | `dsp.bids` | `bid` |
| `GET\|POST /win` | 赢竞价通知 | `dsp.bids` + `dsp.billing`(见缺口) | `win` |
| `GET /click` | 点击上报 | `dsp.impressions` | `click` |
| 内部曝光路径 | impression 回调 | `dsp.impressions` | `impression` |
| `GET /convert` | 转化上报 | `dsp.impressions` | `conversion` |

### 消费与落盘(consumer)

- `cmd/consumer/main.go:39` 订阅 `dsp.bids` 和 `dsp.impressions`
- 收到事件 → 映射为 `reporting.BidEvent`(`cmd/consumer/main.go:73-86`)→ 写入 ClickHouse `bid_log`(`internal/reporting/store.go:34-46`)
- 写入失败 → 发送到 `dsp.dead-letter`(`internal/events/producer.go:163-192`)

### ClickHouse `bid_log` 表

合并自 `migrations/002_clickhouse.sql` 和 `migrations/008_clickhouse_attribution.sql`:

| 列 | 类型 | 说明 |
|---|---|---|
| event_date | Date | 分区键 |
| event_time | DateTime | 事件时间 |
| campaign_id | UInt64 | |
| creative_id | UInt64 | |
| advertiser_id | UInt64 | |
| exchange_id | String | |
| request_id | String | 生命周期关联 ID |
| geo_country | String | |
| device_os | String | |
| device_id | String | IDFA/GAID/OAID,归因用 |
| bid_price_cents | UInt32 | 分 |
| clear_price_cents | UInt32 | 分 |
| charge_cents | UInt32 | 广告主扣费,分 |
| event_type | Enum8 | `bid=1, win=2, loss=3, impression=4, click=5, conversion=6` |
| loss_reason | String | 默认空 |

**TTL**:6 个月。biz 的报表 / 归因查询必须假定 6 个月以前的数据会被清理。

### biz 侧读方

- `internal/reporting/store.go` - 基础报表查询(投放量、花费、CTR)
- `internal/reporting/attribution.go:59-60` - `GetAttributionReport`,按 `device_id` 关联 impression/click/conversion
- `internal/reconciliation/reconciliation.go:43-60` - 每小时对比 Redis 日预算 vs ClickHouse `bid_log` 累计花费,由 `cmd/api/main.go:105-109` 启动

### 预算 / 扣费回路(特殊说明)

花费数据 **不走 Kafka**。真实回路是:

1. bidder 在处理 win/click 时,直接调用 `internal/budget` 扣减 Redis 日预算计数器(`cmd/bidder/main.go:344-384`)
2. 同时发 `dsp.bids` 事件到 Kafka,consumer 写入 `bid_log`
3. api 每小时跑 `reconciliation`,对比 Redis 计数 vs `bid_log` 聚合,发现偏差则告警 / 修正

**biz 侧如果想看"实时花费"**:读 Redis 计数(bidder 刚扣完的值)。
**biz 侧如果想看"可信花费"**:读 ClickHouse `bid_log` 聚合(最终事实来源,但可能有若干秒滞后)。

---

## 3. 已知缺口(非 QA 范围,已接受)

### 3.1 `dsp.billing` topic:produced but unconsumed

- **现状**:bidder `SendWin` 会同时写 `dsp.bids` 和 `dsp.billing`(`internal/events/producer.go:106-111`)
- **问题**:没有任何 consumer 订阅 `dsp.billing`。注释标注"for billing service",但该 service 不存在
- **决定**:本轮 QA **不覆盖** `dsp.billing`。engine QA 只验证 `dsp.bids` 和 `dsp.impressions`。未来如需独立结算服务,从这个 topic 重建即可
- **相关代码**:`cmd/consumer/main.go:37-38` 注释、`internal/events/producer.go:106-111`

### 3.2 `SendLoss` 定义但未调用

- **现状**:`events.Producer.SendLoss`(`producer.go:113-117`)定义完整,但 `cmd/bidder/main.go` 中无调用点
- **影响**:loss 事件目前完全不写 Kafka,`bid_log` 里只有 `event_type=bid/win/impression/click/conversion`,没有 `loss=3`
- **决定**:本轮 QA **不覆盖** loss 事件。engine QA 的测试数据集里不要期望看到 loss 行。未来要埋点时,从 `handleBid` 的 "未中标/被过滤" 分支注入
- **相关代码**:`internal/events/producer.go:113-117`

### 3.3 创意 CRUD 不发 pub/sub

见 §1 "已知缺口",biz QA 需明确这一点,或者在本轮 QA 过程中发现确有阻塞时追加修复。

---

## 4. 两条 QA 线的责任边界

| 边界点 | biz(业务系统)负责 | engine(投放引擎)负责 |
|---|---|---|
| Postgres 广告主/广告/创意 CRUD | ✅ 正确性 + 发 pub/sub | 读(作为前置条件) |
| Redis `campaign:updates` 发布 | ✅ 发布覆盖面、payload 正确 | — |
| Redis `campaign:updates` 订阅 + loader 缓存 | — | ✅ 缓存一致性、full reload 正确性 |
| 30s 兜底 full reload | — | ✅ |
| 出价逻辑 / 过滤 / 预算判断 | — | ✅ |
| Kafka `dsp.bids` / `dsp.impressions` 生产 | — | ✅ schema 正确、不丢消息 |
| Kafka consumer → ClickHouse `bid_log` 落盘 | — | ✅ |
| ClickHouse `bid_log` 读取 → 报表 | ✅ 查询正确性、字段语义 | schema 稳定性 |
| Redis 日预算扣减 | 读取余额、余额不足的上游拦截 | ✅ 原子扣减、并发安全 |
| `reconciliation` 每小时对账 | ✅ | 不动 |

**跨边界 e2e(两条线都要配合)**:
- 从 biz 创建 campaign → engine 投放 → 数据回流 biz 报表 的完整链路,建议放在 `cmd/autopilot/` harness 里统一跑,不归属单一线。本轮两条线各自独立完成后,合回 main 再跑一次 autopilot 做联合验证。

---

## 5. 修改本契约的流程

1. 在 main 分支提出改动(新增 topic、新增 pub/sub 字段、修改 `bid_log` 列等)
2. 更新本文件
3. 同步更新相关代码注释(`internal/events/producer.go`、`internal/bidder/loader.go`)
4. 两条线的 worktree 从 main rebase 以拿到新契约
5. 任何一侧的 QA 测试代码直接引用本文件的章节号作为 spec 来源

契约漂移是最大的集成 bug 源。任何"我记得以前是这样"的修改都要先改这里。
