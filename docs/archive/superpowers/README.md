# superpowers 归档

本目录保存已完成或被新版本替代的 superpowers 产物，顶层 `docs/superpowers/` 只保留当期在做的。

## 目录约定

| 子目录 | 内容 | 产出时机 |
|--------|------|---------|
| `plans/` | 实现计划（执行前产出） | `superpowers:writing-plans` 生成 |
| `specs/` | 设计规范（brainstorming 后落地） | `superpowers:brainstorming` → spec 收敛 |
| `reports/` | 执行报告（对复杂 Phase 的复盘） | 可选，通常只在重大 Phase 或 QA 任务后产出 |

## plans / specs / reports 三者关系

- **plans 与 specs 通常成对出现**：同一主题会有一个 plan（怎么做）+ 一个 spec（做什么）。命名上一般是 `YYYY-MM-DD-<topic>.md` 与 `YYYY-MM-DD-<topic>-design.md` 对应。
- **reports 不是强制产出**：只有需要独立复盘的任务（比如 engine-qa、大版本 Phase 收官）才产生 report。大多数 plans 执行完不会有对应 report。
- **当期在做**：位于上一级 `docs/superpowers/`（不在 archive）。一旦 plan 执行完毕且被新版本取代，整对 plan + spec 移动到本归档目录。

## 命名规范

`YYYY-MM-DD-<kebab-case-topic>.md`

- plan:`2026-04-14-biz-qa.md`
- spec:`2026-04-14-biz-qa-design.md`
- report:`2026-04-14-biz-qa-report.md`（如果有）

日期是产出日期,不是执行日期。同一主题的 plan/spec/report 共用同一日期前缀便于配对查找。
