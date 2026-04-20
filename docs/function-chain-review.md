**Findings**
- High: `/bid/{exchange_id}` 这条链没有注入 click tracker，导致 exchange 流量下 `/click` 根本不会被触发，CPC 计费和 click/conversion 分析链会直接断掉。直接路径在 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:309>) 和 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:314>) 会生成 click URL 并调用 `injectClickTracker`，但 exchange 路径在 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:373>) 到 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:388>) 只补了 `NURL`，没有对应 click 装饰。当前测试也没有覆盖 exchange bid 响应里的 click tracking。
- High: campaign 启动在余额查询失败时是 fail-open，会继续返回 `active`。问题在 [internal/handler/campaign.go](</C:/Users/Roc/github/dsp/internal/handler/campaign.go:327>) 到 [internal/handler/campaign.go](</C:/Users/Roc/github/dsp/internal/handler/campaign.go:332>)：只有 `err == nil && balance < budget_daily` 才会阻止启动，`BillingSvc.GetBalance` 一旦报错就直接放行。`GetBalance` 本身是数据库查询 [internal/billing/service.go](</C:/Users/Roc/github/dsp/internal/billing/service.go:241>)，所以账务库抖动时会把“无法验证余额”错误误判成“允许启动”。
- Medium: activation 不是原子链路，DB 已切到 `active` 后，Redis 侧失败会留下“API 说已启动，但 bidder 暂时不可投”的裂缝状态。状态切换先发生在 [internal/handler/campaign.go](</C:/Users/Roc/github/dsp/internal/handler/campaign.go:335>)，随后才做 `InitDailyBudget` 和 `NotifyCampaignUpdate` [internal/handler/campaign.go](</C:/Users/Roc/github/dsp/internal/handler/campaign.go:340>) [internal/handler/campaign.go](</C:/Users/Roc/github/dsp/internal/handler/campaign.go:346>)。而预算检查在缺少 daily key 时会按 0 处理并 no-bid [internal/budget/budget.go](</C:/Users/Roc/github/dsp/internal/budget/budget.go:64>) [internal/budget/budget.go](</C:/Users/Roc/github/dsp/internal/budget/budget.go:190>)。如果 publish 失败，bidder 只能等 loader 的 30s 全量刷新 [internal/bidder/loader.go](</C:/Users/Roc/github/dsp/internal/bidder/loader.go:265>) 才看到状态变更。
- Medium: 直接 `/bid` 热路径没有请求体大小上限，而 `/bid/{exchange_id}` 有 1MB 限制，导致两个入口的资源保护不一致。直接路径在 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:270>) 直接 `json.NewDecoder(r.Body)`，exchange 路径则显式 `io.LimitReader(..., 1<<20)` [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:343>)。这会让公开 `/bid` 更容易被超大 body 拖慢或打爆内存。
- Medium: win 事件写入 ClickHouse 时使用的是“重算后的当前 campaign 信息”，不是真实赢标时的信息，报表透明度会偏。问题在 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:514>) 到 [cmd/bidder/main.go](</C:/Users/Roc/github/dsp/cmd/bidder/main.go:529>)：`bidPrice` 由当前 `EffectiveBidCPMCents(0, 0)` 重算，`creativeID` 直接取 `c.Creatives[0].ID`。如果同一 campaign 有多个 creative，或者 bid strategy/CTR-CVR 在 bid 与 win 之间变化，`bid_log` 中的 win 行就和真实竞价响应不一致，而透明度查询会直接把这些行读出来 [internal/reporting/store.go](</C:/Users/Roc/github/dsp/internal/reporting/store.go:227>)。

**Open Questions**
- 假设 `/bid/{exchange_id}` 预期与直接 `/bid` 共享同一套 CPC/click 计费语义。如果某些 exchange 会在适配层自行改写点击链，第一条的严重度会下降，但现在代码和测试里没有看到这个约束。
- 假设 `POST /campaigns/{id}/start` 的 200 语义是“启动后应立即可投”。如果产品接受“最多 30 秒最终一致”，第三条更像契约不清，而不是纯 bug。

**改进建议**
- 把 direct bid 和 exchange bid 的响应装饰抽成一个共享函数，统一补 `NURL`、click URL 和 `AdM` 改写，并补一条 exchange integration test。
- `HandleStartCampaign` 对余额查询改成 fail-closed；至少在 `GetBalance` 出错时返回 `503`，不要默默放行。
- 启动链改成“先准备 Redis side effects，再提交 active 状态”，或者引入 outbox/pending_activation，避免 DB 与 Redis 分叉。
- 给直接 `/bid` 加 `http.MaxBytesReader` 或 `io.LimitReader`，并补 oversized request test。
- 如果报表需要真实 win 元数据，就在 bid 响应生成时把 `request_id -> creative_id/bid_price` 落一个短 TTL cache，win 回调按 `request_id` 取回真实值，而不是现场重算。

这次是 review only，我没有改代码。
