# 手工 QA checklist — V5 §P2 前端回归

本文件是 V5 §P2 §3.4 "前端回归" 的执行步骤。Batch 6 的自动集成测试(`test/integration/`)覆盖了后端 handler 层的租户隔离和 api_key 泄露回归;本文档列出的手工步骤覆盖自动测试覆盖不到的**前端交互路径**,尤其是 Batch 1 P0 前端清理(去掉 `advertiser = 1` 硬编码 + 改造 `web/lib/api.ts` billing 函数签名)的视觉和功能验证。

执行方法:每次前端或 billing/campaign/creative 路径的代码改动后,人工走一遍;或者在 biz worktree 的 QA session 里作为最终合入前的门槛。

## 准备

1. 主栈(dsp-main)或任一 biz worktree 栈必须在运行:
   ```
   docker compose up -d
   ```
2. 浏览器打开前端(main: http://localhost:14000;biz: http://localhost:15000;engine: http://localhost:16000)
3. 至少有一个广告主账号 + API key,能完成登录
4. 清空浏览器缓存或使用无痕窗口,避免旧 JS 被缓存

## 必查项 —— 与 Batch 1 前端修复直接相关

### A. billing 页面不再依赖 advertiser `1` 硬编码

这是 Batch 1 明确修掉的 3 处硬编码(`web/app/billing/page.tsx` L33, L34, L125)。

- [ ] **打开 /billing**,登录为**非 1 号**广告主。期望:余额、交易流水加载正确(显示当前登录者的数据,不是广告主 1 的数据)
- [ ] **充值**:选一个金额点"确认充值",期望:
  - 页面显示"充值成功"
  - 余额卡片立刻刷新到新值
  - 下面的交易流水表立刻出现一条"充值"记录
  - **关键**:再次登录为广告主 1(如果存在)检查,广告主 1 的余额和流水**未被更新** —— 钱应该进了当前登录者的账户
- [ ] **刷新页面**(F5):重新拉取余额和流水,数值保持一致
- [ ] **查 Network 标签**:请求应是 `GET /api/v1/billing/balance`(无 `/{id}`)和 `GET /api/v1/billing/transactions`(无 `?advertiser_id=`)。请求头含 `X-API-Key`

### B. 前端 API 签名已经去掉 advertiserId 参数

这是 `web/lib/api.ts` 的 Batch 1 改动。

- [ ] 在浏览器控制台里执行(只在开发环境):
  ```js
  // 期望:这些调用应该能工作
  await api.getBalance()
  await api.getTransactions()
  await api.topUp(100, "test")
  ```
- [ ] 如果有任何地方还在用旧签名 `api.getBalance(1)`,TypeScript 应该已经编译失败(不会跑到运行时)—— 如果跑到了运行时,说明有个调用点漏改

### C. 跨租户越权的 UI 体感检查

Batch 6 的自动测试覆盖了 API 层的 404/400/401,这里补一个**用户体感**层的检查:点击一个广告主看不到的资源,UI 应该优雅降级。

- [ ] 以广告主 A 登录,打开 URL `/campaigns/{B_campaign_id}`(手工拼一个另一广告主的 campaign id)。期望:
  - 页面不崩(不应出现白屏或 React 报错)
  - 显示"Campaign not found"或"未找到"类友好错误
- [ ] 对 `/reports/{B_campaign_id}` 做同样的事
- [ ] 打开浏览器控制台,不应出现 unexpected unhandled exception

## 应查项 —— 与 V5 整体目标相关

### D. 充值回路闭环

Batch 6 自动测试已验证 topup body 带他人 id 返回 400,手工验证 UI 处理:

- [ ] 如果前端以任何方式允许在 topup 请求 body 里注入自定义 `advertiser_id`(不应该有 UI,但为防万一),后端返回 400,前端应显示"advertiser_id mismatch"或类似错误,不崩溃
- [ ] 如果没有这种 UI 路径(正常情况),跳过

### E. 广告主 / 创意 CRUD 视觉

- [ ] 打开 /campaigns,登录后看到**自己的** campaign 列表(不是所有人的)
- [ ] 点击 "新建 campaign",填表单,确认提交 → 返回创建成功,新 campaign 出现在列表里
- [ ] 打开一个 campaign 的详情页面,点 "启动",确认状态变成 active
- [ ] 打开创意管理,新增一个 creative,编辑,删除,每步都应成功,且 campaign:updates pub/sub 通知应发出(bidder 日志应显示收到通知,如果 bidder 在跑)
- [ ] DESIGN.md 对齐检查:字体(Geist)、主色(#2563EB)、间距 4px 倍数、表单输入框圆角、按钮样式 —— 和 DESIGN.md 的规范比对,任何偏差标记为 bug

### F. 管理后台(如果你有 admin token)

- [ ] 设置 `X-Admin-Token: <your-token>` header 访问 `/api/v1/admin/registrations`,能看到待审核列表
- [ ] 尝试在 URL 里加 `?admin_token=...`(不带 header),应该 **401**(Batch 2 删了 query 参数支持)
- [ ] 尝试用 `admin-secret` 作为 token,应该 **401**(Batch 2 删了 fallback)
- [ ] 如果 ADMIN_TOKEN 环境变量没设,**服务不应启动**(Batch 2 的 Config.Validate 要求)

## 已知限制

这些是手工 QA 能验证但自动测试目前不覆盖的项;Batch 6 自动测试套件覆盖不到的地方,这份 checklist 就是兜底。

- **视觉合规**:DESIGN.md 的细节只有人眼能判断(字重、阴影、过渡时长、响应式断点)。自动测试无法判断"这个按钮的 hover 颜色和规范差了 5%"
- **浏览器兼容**:测试栈只跑 headless Chromium,真实的 Safari / 旧 Chrome 行为需手工验证
- **交互流畅度**:API 响应时间、loading 状态、error 状态的用户感知,自动测试不参与
- **中文文案**:中文字符渲染、换行、字体降级等本地化问题需要中文 reviewer 看

## 失败处理

任何一条没通过:
1. 记录具体步骤(URL、输入、实际 vs 期望)
2. 截图(尤其是视觉合规类)
3. 在 biz worktree 的 QA 报告里记为一个 issue
4. 评估是阻塞合并的(P0 / P1)还是可延后的(P2)
5. 如果是阻塞项,修复并回到这份 checklist 重新走一遍

## 更新这份文档

这份文档写于 Batch 6 落地时(V5 P2 §3.4 交付品)。后续如果前端有任何新增页面或 API 路径变化,应当同步更新这份 checklist,不要让它漂移成"半年前的状态"。
