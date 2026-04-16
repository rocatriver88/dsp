# Runtime behavior: failure handling & graceful shutdown

本文件是 DSP 服务运行时的**行为契约**,记录每个外部依赖在不可用时的处理策略,以及两个主进程(cmd/api、cmd/bidder)的优雅退出顺序。V5 §P1 lifecycle 的落地成果在这里归档,供 on-call、SRE、后续评审者参考。

契约原则:
- **fail-open**:依赖不可用时,服务仍然对外响应,以功能降级换可用性。适用于"缺了这个功能不致命"的路径。
- **fail-closed**:依赖不可用时,服务拒绝提供功能或直接退出。适用于"缺了这个会导致数据错误或安全漏洞"的路径。

每个依赖的选择要么启动时一次性判定(启动退出 vs 启动警告),要么请求时动态判定(handler 降级)。

---

## 依赖 × 主进程 矩阵

| 依赖 | cmd/api | cmd/bidder |
|---|---|---|
| **PostgreSQL** | fail-closed(`log.Fatal`) | fail-closed(`log.Fatal`) |
| **Redis** | fail-open 启动警告,`rdb = nil`,guardrail / pub/sub / budget 降级 | fail-closed(`log.Fatal`,bidder 必须有 Redis 做预算和频控) |
| **ClickHouse** | fail-open 启动警告,`reportStore = nil`,reporting 接口返回 503,autopause 早退 | fail-open 启动警告,`reportStore = nil`,statsCache 用默认 CTR/CVR 先验值 |
| **Kafka** | 不直接使用(api 侧没有 Kafka 生产者) | fail-open 带本地 disk buffer,producer 写失败落 `/tmp/dsp-kafka-buffer/*.jsonl`,启动时 `ReplayBuffer` 回放 |

> 所有决定都是"当前行为的记录",不是"新增约束"。任何要改变本表行为的 PR 必须同时更新本文件。

### PostgreSQL

- **策略**:fail-closed。
- **触发**:`pgxpool.New` 或 `db.Ping` 失败。
- **行为**:两个主进程都 `log.Fatal`,进程退出,systemd / k8s 负责重启。
- **理由**:Postgres 是 campaign / advertiser / billing 的**权威存储**。没有它任何业务操作都会产生不一致或丢失。
- **相关代码**:`cmd/api/main.go:58-65`、`cmd/bidder/main.go:55-62`。

### Redis

- **cmd/api 策略**:fail-open,功能降级。
- **cmd/api 触发**:`rdb.Ping` 失败 → log warning → 把本地 `rdb` 变量置 `nil` → 后续代码路径通过 `if rdb != nil` guard 跳过 Redis 相关功能。
- **cmd/api 降级面**:
  - `bidder.NotifyCampaignUpdate` pub/sub 发布被跳过(创建/编辑 campaign 时 bidder 要等 30s 全量 reload 才能看到改动——contract §1)
  - `internal/guardrail` 没初始化(全局预算上限不生效)
  - `internal/budget` 没初始化(handler 启动 campaign 时无法初始化当日预算)
  - `internal/autopause` 和 `internal/reconciliation` 分别检查 `rdb != nil` 才调用
- **cmd/bidder 策略**:fail-closed。
- **cmd/bidder 触发**:`rdb.Ping` 失败 → `log.Fatal`。
- **cmd/bidder 理由**:bidder 的出价热路径依赖 Redis 做**原子预算扣减**、**频控去重**、**反作弊计数**。没有 Redis 就没法保证"一个 win 只扣一次预算",属于数据正确性硬要求,必须 fail-closed。
- **相关代码**:`cmd/api/main.go:69-76`、`cmd/bidder/main.go:68-73`。

### ClickHouse

- **两个主进程共同策略**:fail-open,功能降级。
- **cmd/api 降级面**:
  - `GET /api/v1/reports/campaign/{id}/*` 五条报表接口:`if d.ReportStore == nil { 503 Service Unavailable }`
  - `GET /api/v1/reports/overview`:返回 `{today_spend_cents: 0}` 而不是 503(避免整个仪表盘白屏)
  - `autopause.Service.Start`:顶部 `if s.reportStore == nil { return }` 直接退出,不启动轮询
  - `reconciliation.StartHourlySchedule`:main.go 的启动条件包含 `reportStore != nil && rdb != nil`,条件不满足时不启动
- **cmd/bidder 降级面**:
  - `statsCache.refresh` 顶部 `if sc.reportStore == nil { return }` 早退
  - 实际出价时 `statsCache.Get` 返回零值 `CachedStats{}` → `EffectiveBidCPMCents` 使用硬编码先验值(默认 CTR 1%,默认 CVR 5%)→ 出价质量下降但继续竞价
- **理由**:ClickHouse 是**分析型**存储,不是权威账务存储。丢一段时间的报表数据不会破坏真实状态。出价用的 CTR/CVR 先验值即使不准,也比"不出价"好得多(机会成本远大于 bid 精度)。
- **相关代码**:`cmd/api/main.go:97-106`、`cmd/bidder/main.go:84-91`、`internal/reporting/store.go`。

### Kafka

- **策略**:fail-open + 本地 disk buffer。
- **触发**:`Producer.Send` 写 Kafka 失败。
- **行为**:
  - Producer 用 `Async: true` 和 `BatchTimeout: 10ms`,热路径非阻塞
  - Kafka 不可达时,事件 JSON 落盘到 `/tmp/dsp-kafka-buffer/{topic}.jsonl`(append-only)
  - Buffer 上限 1GB/节点(`internal/events/producer.go:16-28` 头注释)
  - 启动时 `producer.ReplayBuffer(processCtx)` 回放 buffer
- **buffer 满时行为**:达到 1GB 上限后,`producer.bufferToDisk`(`internal/events/producer.go:152`)直接拒绝写入当前新事件并 `log.Printf("buffer full ... dropping event")`——**丢新的,不丢最老的**。出价热路径因此不会被磁盘写入阻塞,但也意味着事故期超量的事件在 Kafka 恢复前无法回放。
- **理由**:dsp.bids / dsp.impressions 是分析用事件流,可以容忍秒级延迟,但不能阻塞出价 handler。disk buffer 把 Kafka 秒级不可用时的事件先落盘换一致性收益——在 1GB 上限内的事故窗口事件不丢(除非磁盘也坏),超过上限的新增事件则直接丢弃而不是阻塞出价。
- **相关代码**:`internal/events/producer.go`、`cmd/bidder/main.go:85-90`、`cmd/bidder/main.go:126-128`。

---

## 优雅退出顺序

两个主进程的 shutdown 序列都遵循 V5 §P1 lifecycle 的五条不变量。具体实现见 `cmd/api/main.go` 和 `cmd/bidder/main.go` 底部的 shutdown 块。

### 五条不变量

1. **新请求停止进入** — 由 `http.Server.Shutdown` 停止 Accept 原子完成
2. **worker 退出** — 通过 cancel `workerCtx` 触发后台 loop 的 `<-ctx.Done()` 分支
3. **inflight 请求排空** — 由 `http.Server.Shutdown` 的 drain 阶段完成
4. **producer flush** — bidder 专用。handler 和 engine 里所有 `go producer.SendXxx(...)` 的发送都通过 `producer.Go(...)` 进入 `sync.WaitGroup`,shutdown 在 HTTP drain 之后调用 `producer.WaitInflight(ctx, 5s)` 严格等待所有 tracked goroutine 退出,然后再 `producer.Close()` 关闭 Kafka writers。**严格** 满足 "producer flushes after last business write",不依赖 disk buffer 兜底(disk buffer 仍然是 Kafka 不可达时的最终一致性保护)
5. **存储连接关闭** — `db.Close` / `rdb.Close` 通过 top-of-main defer 触发(main 函数返回之后)

### 时间顺序

```
信号到达 (SIGINT/SIGTERM)
  │
  ├─ processCtx 自动 Done
  │
  └─ 主 goroutine 从 <-processCtx.Done() 解除阻塞
         │
         ├─ workerCancel()                  # Inv 2
         │    worker 开始退出(非阻塞)
         │
         ├─ srv.Shutdown(shutdownCtx)       # Inv 1 + 3
         │    停止 Accept;drain inflight;阻塞直到完成或 15s 超时
         │
         ├─ loader.Stop() / statsCache.Stop()   # bidder only,冗余但幂等
         │
         ├─ producer.WaitInflight(ctx, 5s)  # Inv 4a (bidder only)
         │    阻塞等待所有 producer.Go 跟踪的
         │    handler / engine 端发送 goroutine 退出
         ├─ producer.Close()                # Inv 4b (bidder only)
         │    关闭 Kafka writers,flush 已缓冲消息
         │
         └─ main() return
                │
                └─ defer rdb.Close() → defer db.Close()   # Inv 5
```

### 为什么 workerCancel 在 Shutdown 之前

worker(autopause、reconciliation、statscache、loader 周期刷新)可能和 inflight HTTP handler 写同一块 Redis / DB 状态。如果 HTTP drain 期间 worker 仍然在跑,draining 的 handler 完成后 worker 还能产生新的写入,**shutdown window 里 DB 状态持续变动**,事故复盘取证困难。

先 cancel workerCtx,再 drain HTTP,drain 期间 worker 已经在退出或已退出,drain 完成后 DB 状态稳定。

### 为什么 producer.Close 在 Shutdown 之后,且之前还要 WaitInflight

bidder 的 bid / click / convert / win handler 会调 `producer.SendXxx` 往 Kafka 写事件。发送操作是异步的——handler 在 spawn 完 goroutine 后立刻返回,goroutine 在后台往 Kafka 写。如果 producer 在 goroutine 还没写完时就被 Close,最后一批 inflight 事件就没了。

两层保障:

- **HTTP drain 结束 ≠ 所有 goroutine 已经退出**。`srv.Shutdown` 只等 handler 的同步返回,不等它们 spawn 的异步 goroutine。所以 drain 完成后,还可能有 bid / win / click 的 SendXxx goroutine 正在往 Kafka 写。
- **解决方案**:所有这些 goroutine 都通过 `producer.Go(fn)` 启动,内部用 `sync.WaitGroup` 跟踪。shutdown 在 HTTP drain 之后调 `producer.WaitInflight(ctx, 5s)` 阻塞等每一个 tracked 都返回,然后才 `producer.Close()` 关 writers。5 秒 cap 防止病态 case 让关闭无限 hang,超时的最后残留事件落 disk buffer,下次启动 `ReplayBuffer` 回放。

**关键**:`producer.Go` 必须在**所有**异步发送位点使用,含 `cmd/bidder/main.go` 的 handler 级调用 **和** `internal/bidder/engine.go` 里 bid 热路径的 `SendBid`。任何漏网的 `go producer.SendXxx(...)` 都会绕过 WaitInflight 跟踪,让不变量 4 的保证降级为 disk buffer 兜底。Round 2 review 抓到过一次这种疏漏(engine.go:238 的 SendBid)。

### 为什么请求 ctx 不绑定 workerCtx

这是 V5 codex V4 评审中明确纠正 Claude V3 的要点。如果把 HTTP request 的 ctx 和 workerCtx 都派生自同一个 root context,cancel root 会**同时**取消 inflight HTTP 请求。handler 在 drain 期间看到 `r.Context().Done()`,返回错误,客户端收到 5xx,重试,下一次请求发给可能已经没在 Accept 的服务器——级联故障。

正确做法:
- request ctx 由 `http.Server` 独立管理(内部为每个请求派生一个短生命周期 ctx,handler 返回时自动 cancel)
- worker ctx 是一个独立的 `context.WithCancel(context.Background())`,和 request ctx 之间没有派生关系
- 两个 ctx 的"退出信号"都由 `processCtx` 触发,但触发的是**不同的关闭路径**(Shutdown vs workerCancel)

### 超时边界

- `http.Server.Shutdown` 的超时是 `15s`(两个进程一致)
- 15s 内没排空的 inflight 请求被强制丢弃,client 见 connection reset
- 超过 15s 的 HTTP handler 是性能 bug,应当单独分析

---

## on-call 快速诊断

| 症状 | 可能原因 | 应对 |
|---|---|---|
| api 进程起不来 `config validation failed` | 生产 secret 缺失(`BIDDER_HMAC_SECRET` / `ADMIN_TOKEN` / `CORS_ALLOWED_ORIGINS` 走 dev 默认) | 补 env → 重启 |
| api 进程起不来 `connect to postgres` | Postgres 不可达 | 检查 DB 连通性;api 和 bidder 都 fail-closed |
| api warning `Redis not available, pub/sub notifications disabled` | Redis 不可达 | api 继续跑但 guardrail/pub-sub/budget 降级;修好 Redis 后需要 api 重启才能重新启用 |
| bidder 进程起不来 `connect redis` | Redis 不可达 | bidder 必须 Redis,修好后重启 |
| reports 接口返回 503 | ClickHouse 不可达 | 非致命,api 其他接口正常;修好 ClickHouse 即恢复 |
| bidder 出价质量下降(CTR 回归到 1% 默认) | ClickHouse 不可达 → statsCache 用先验值 | 非致命;修好 ClickHouse 后 5 分钟内恢复(下次 refresh) |
| shutdown 卡 15s 后被强制 kill | 有 handler 超过 15s 没返回 | 从 access log 找最慢路由;通常是长轮询或误用的同步 SSE |
| shutdown 后 `/tmp/dsp-kafka-buffer` 里还有未回放的 jsonl | Kafka 在事故期间不可达,producer fallback 到 disk | 下次 bidder 启动会 `ReplayBuffer` 回放;如果回放失败要人工处理 |

---

## 变更守则

- 本文件记录**现状**,不是**规划**。任何改变依赖处理策略的 PR(比如把 Redis 在 api 改为 fail-closed)必须在同一 PR 里更新这里。
- 任何新增的长生命周期后台 goroutine 必须:(a)接受 `context.Context`;(b)在 for-select 里处理 `<-ctx.Done()`;(c)在 `cmd/*/main.go` 里传入 `workerCtx` 而不是 `processCtx`;(d)在 `docs/runtime.md` 的"优雅退出顺序"段加一行说明它属于哪个阶段退出。
- shutdown 顺序的五条不变量是**行为约束**而非代码形态要求——任何结构都可以,只要满足这五条。但**不要改变先后关系**,每条的"why"在 `cmd/api/main.go` 和 `cmd/bidder/main.go` 底部注释里说明了。
