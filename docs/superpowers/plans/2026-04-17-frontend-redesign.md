# Frontend Visual Redesign V2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Design Spec:** `docs/superpowers/specs/2026-04-17-frontend-redesign-design.md` — contains all CSS values, color tokens, spacing, and component specs. Reference it for every visual decision.
>
> **Figma Reference:** `docs/design/figma-screens/01_home.png` through `04_audiences.png`

**Goal:** Visually restructure all 14 existing frontend pages to DESIGN.md V2 + Figma quality with glassmorphism, gradient effects, entrance animations, and a brand-level split login page.

**Architecture:** Foundation CSS → shared components → layout shell (sidebar + TopBar + login) → tenant pages → admin pages. API clients (`lib/api.ts`, `lib/admin-api.ts`, `lib/api-types.ts`) stay untouched. No route changes, no backend changes.

**Tech Stack:** Next.js 16 App Router, React 19, TypeScript 5.9, Tailwind CSS 4, Recharts 3 (already installed), Lucide React, Inter font via Google Fonts.

---

## Phases

| Phase | Scope | Tasks |
|-------|-------|-------|
| 1 | Foundation: CSS tokens, utilities, animations, DESIGN.md update | 1 |
| 2 | Shared components: GlassCard, StatCard, TopBar, DataTable, LoadingState | 2 |
| 3 | Layout shell: Sidebar V3, split login page, root layout | 3 |
| 4 | Tenant pages: dashboard, campaigns (3), reports (2), analytics, billing | 4-7 |
| 5 | Admin: layout + 6 pages | 8-9 |
| 6 | Final verification | 10 |

---

## Phase 1: Foundation

### Task 1: CSS Variables, Utilities, Animations + DESIGN.md Update

**Files:**
- Modify: `web/app/globals.css`
- Modify: `DESIGN.md`

- [ ] **Step 1: Replace globals.css**

Replace the entire `web/app/globals.css` with:

```css
@import url("https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700;800&display=swap");
@import "tailwindcss";

:root {
  --primary: #8B5CF6;
  --primary-hover: #7C3AED;
  --primary-muted: rgba(139, 92, 246, 0.12);
  --bg-page: #0F0A1A;
  --bg-sidebar: #0A0610;
  --bg-card: #1A1225;
  --bg-card-elevated: #231830;
  --bg-input: #1A1225;
  --border: #2A2035;
  --border-subtle: #1F1730;
  --text-primary: #FFFFFF;
  --text-secondary: #A0A0B0;
  --text-muted: #6B6B80;
  --success: #22C55E;
  --warning: #EAB308;
  --error: #EF4444;
  --info: #3B82F6;
  --chart-1: #8B5CF6;
  --chart-2: #3B82F6;
  --chart-3: #22C55E;
  --chart-4: #EAB308;
  --chart-5: #EF4444;
  --chart-6: #EC4899;

  /* Sidebar-specific (V3 Figma-aligned) */
  --sidebar-text: #9898AC;
  --sidebar-text-hover: #C0C0D0;
  --sidebar-text-active: #8B5CF6;

  /* Glass card tokens */
  --glass-bg: rgba(26, 18, 37, 0.7);
  --glass-border: rgba(139, 92, 246, 0.1);
  --glass-border-hover: rgba(139, 92, 246, 0.25);
  --glass-light-edge: linear-gradient(90deg, transparent, rgba(139, 92, 246, 0.3), transparent);
}

body {
  background: var(--bg-page);
  color: var(--text-primary);
  font-family: "Inter", -apple-system, sans-serif;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}

.tabular-nums { font-variant-numeric: tabular-nums; }
nav a, nav button { min-height: 44px; display: flex; align-items: center; }

a:focus-visible, button:focus-visible, input:focus-visible, select:focus-visible, textarea:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

.sr-only {
  position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px;
  overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border-width: 0;
}

button, a { min-height: 44px; min-width: 44px; }
td a, p a, span a, .inline-link { min-height: auto; min-width: auto; }

/* ===== Glass card base ===== */
.glass-card {
  background: var(--glass-bg);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border: 1px solid var(--glass-border);
  border-radius: 14px;
  position: relative;
  overflow: hidden;
  transition: border-color 0.3s ease, transform 0.3s ease, box-shadow 0.3s ease;
}
.glass-card::before {
  content: '';
  position: absolute;
  top: 0; left: 0; right: 0;
  height: 1px;
  background: var(--glass-light-edge);
  z-index: 1;
}
.glass-card:hover {
  border-color: var(--glass-border-hover);
  transform: translateY(-1px);
  box-shadow: 0 8px 32px rgba(139, 92, 246, 0.08);
}

/* ===== Ambient glow (page-level) ===== */
.ambient-glow { position: relative; }
.ambient-glow::after {
  content: '';
  position: absolute;
  top: -100px; right: -100px;
  width: 400px; height: 400px;
  background: radial-gradient(circle, rgba(139, 92, 246, 0.06) 0%, transparent 70%);
  pointer-events: none;
  z-index: 0;
}

/* ===== Entrance animations ===== */
@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}
@keyframes breathe {
  0%, 100% { text-shadow: 0 0 0 transparent; }
  50% { text-shadow: 0 0 8px rgba(139, 92, 246, 0.4); }
}

.animate-fade-in-up { animation: fadeInUp 400ms ease-out both; }
.animate-fade-in { animation: fadeIn 300ms ease-out both; }
.animate-breathe { animation: breathe 600ms ease-in-out; }
.stagger-1 { animation-delay: 0ms; }
.stagger-2 { animation-delay: 50ms; }
.stagger-3 { animation-delay: 100ms; }
.stagger-4 { animation-delay: 150ms; }

/* ===== Button utilities ===== */
.btn-primary {
  background: linear-gradient(135deg, #8B5CF6, #7C3AED);
  color: #fff;
  font-weight: 600;
  border: none;
  border-radius: 8px;
  box-shadow: 0 4px 16px rgba(139, 92, 246, 0.25);
  transition: all 0.15s ease;
  cursor: pointer;
}
.btn-primary:hover {
  background: linear-gradient(135deg, #7C3AED, #6D28D9);
  box-shadow: 0 6px 24px rgba(139, 92, 246, 0.35);
}
.btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

.btn-ghost {
  background: transparent;
  color: var(--text-secondary);
  border: 1px solid var(--border);
  border-radius: 8px;
  transition: border-color 0.15s ease;
  cursor: pointer;
}
.btn-ghost:hover { border-color: var(--primary); }

/* ===== Reduced motion ===== */
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { animation-duration: 0.01ms !important; transition-duration: 0.01ms !important; }
}

/* ===== Scrollbar ===== */
::-webkit-scrollbar { width: 8px; height: 8px; }
::-webkit-scrollbar-track { background: var(--bg-page); }
::-webkit-scrollbar-thumb { background: var(--border); border-radius: 4px; }
::-webkit-scrollbar-thumb:hover { background: var(--text-muted); }
```

- [ ] **Step 2: Update DESIGN.md**

In `DESIGN.md`, update the Sidebar Navigation section:

1. Inactive text color: `#6B6B80` → `#9898AC` (sidebar-only override)
2. Inactive hover: `#A0A0B0` → `#C0C0D0`
3. Add explicit font-size `14px`, font-weight inactive `400` / active `500`
4. Add icon size `20px`
5. Add padding `11px 16px`

Add new sections to DESIGN.md:

6. **Top Bar** component pattern: 64px height, glassmorphism (`backdrop-filter: blur(12px)`), search input, notification bell, avatar
7. **Enhancement Effects**: glassmorphism cards, entrance animations, ambient gradients, data breathing, gradient accents
8. **Login Page**: split-layout brand login pattern
9. **Buttons**: update Primary to gradient

- [ ] **Step 3: Verify build**

```bash
cd web && npm run build
```

Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add web/app/globals.css DESIGN.md
git commit -m "feat(web): glass/animation CSS foundation + DESIGN.md V2 updates"
```

---

## Phase 2: Shared Components

### Task 2: GlassCard, StatCard, TopBar, DataTable, LoadingState

**Files:**
- Create: `web/app/_components/GlassCard.tsx`
- Create: `web/app/_components/TopBar.tsx`
- Create: `web/app/_components/DataTable.tsx`
- Modify: `web/app/_components/StatCard.tsx`
- Modify: `web/app/_components/LoadingState.tsx`

- [ ] **Step 1: Create GlassCard.tsx**

```tsx
"use client";

interface GlassCardProps {
  children: React.ReactNode;
  className?: string;
  padding?: string;
  hover?: boolean;
}

export default function GlassCard({ children, className = "", padding = "p-5", hover = true }: GlassCardProps) {
  const hoverClass = hover ? "" : "[&]:hover:transform-none [&]:hover:shadow-none [&]:hover:border-[var(--glass-border)]";
  return (
    <div className={`glass-card ${padding} ${hoverClass} ${className}`}>
      {children}
    </div>
  );
}
```

- [ ] **Step 2: Create TopBar.tsx**

```tsx
"use client";

import { Search, Bell } from "lucide-react";

export default function TopBar() {
  return (
    <div className="h-16 flex items-center justify-between px-8"
      style={{
        background: "rgba(15, 10, 26, 0.6)",
        backdropFilter: "blur(12px)",
        WebkitBackdropFilter: "blur(12px)",
        borderBottom: "1px solid var(--border)",
      }}>
      <div className="flex items-center gap-2 px-4 py-2 rounded-[10px] w-80 text-[13px]"
        style={{ background: "var(--bg-card)", border: "1px solid var(--border)", color: "var(--text-muted)" }}>
        <Search size={14} />
        <span>搜索广告系列、受众...</span>
      </div>
      <div className="flex items-center gap-4">
        <button className="relative w-9 h-9 rounded-full flex items-center justify-center transition-colors"
          style={{ color: "var(--text-muted)" }}
          aria-label="通知">
          <Bell size={18} />
          <span className="absolute top-1.5 right-2 w-1.5 h-1.5 rounded-full" style={{ background: "var(--error)" }} />
        </button>
        <div className="w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold cursor-pointer"
          style={{
            background: "linear-gradient(135deg, #8B5CF6, #3B82F6)",
            boxShadow: "0 0 0 2px var(--bg-page), 0 0 0 3px rgba(139,92,246,0.3)",
          }}>
          U
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Create DataTable.tsx**

```tsx
"use client";

interface Column<T> {
  key: string;
  header: string;
  align?: "left" | "right" | "center";
  render: (row: T) => React.ReactNode;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyFn: (row: T) => string | number;
  emptyMessage?: string;
}

export default function DataTable<T>({ columns, data, keyFn, emptyMessage = "暂无数据" }: DataTableProps<T>) {
  return (
    <div className="glass-card p-0 overflow-hidden animate-fade-in">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--bg-card-elevated)" }}>
              {columns.map((col) => (
                <th key={col.key}
                  className={`py-3 px-4 text-[11px] font-semibold uppercase tracking-wider ${col.align === "right" ? "text-right" : "text-left"}`}
                  style={{ color: "var(--text-muted)" }}>
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.length === 0 ? (
              <tr>
                <td colSpan={columns.length} className="py-12 text-center text-sm" style={{ color: "var(--text-secondary)" }}>
                  {emptyMessage}
                </td>
              </tr>
            ) : data.map((row) => (
              <tr key={keyFn(row)} className="transition-colors"
                style={{ borderTop: "1px solid var(--border-subtle)" }}
                onMouseEnter={(e) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; }}>
                {columns.map((col) => (
                  <td key={col.key}
                    className={`py-3 px-4 ${col.align === "right" ? "text-right tabular-nums" : ""}`}
                    style={{ color: "var(--text-secondary)" }}>
                    {col.render(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Rebuild StatCard.tsx**

Replace entire file. Key changes vs current:
- Use `glass-card` class instead of inline `bg-card` + border
- Add `animate-fade-in-up` + `stagger-N` classes
- Label: `text-[11px] font-medium uppercase tracking-wider`
- Icon container: `w-9 h-9 rounded-[10px]`
- Remove `HeroStatCard` export (dashboard will use regular StatCard grid)

```tsx
"use client";

import { TrendingUp, TrendingDown } from "lucide-react";
import type { LucideIcon } from "lucide-react";

interface StatCardProps {
  label: string;
  value: string;
  trend?: { value: string; positive: boolean };
  icon?: LucideIcon;
  iconColor?: string;
  stagger?: number;
}

export function StatCard({ label, value, trend, icon: Icon, iconColor = "#8B5CF6", stagger = 1 }: StatCardProps) {
  const iconBg = iconColor + "26";
  return (
    <div className={`glass-card p-5 animate-fade-in-up stagger-${stagger}`}>
      <div className="flex items-start justify-between mb-3">
        {Icon && (
          <div className="w-9 h-9 rounded-[10px] flex items-center justify-center" style={{ background: iconBg }}>
            <Icon size={18} style={{ color: iconColor }} />
          </div>
        )}
        {trend && (
          <div className="flex items-center gap-1 text-xs font-semibold"
            style={{ color: trend.positive ? "var(--success)" : "var(--error)" }}>
            {trend.positive ? <TrendingUp size={14} /> : <TrendingDown size={14} />}
            {trend.value}
          </div>
        )}
      </div>
      <p className="text-2xl font-bold tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</p>
      <p className="text-[11px] font-medium uppercase tracking-wider mt-1" style={{ color: "var(--text-muted)" }}>{label}</p>
    </div>
  );
}
```

- [ ] **Step 5: Update LoadingState.tsx**

Update all four exports to use `glass-card` class on containers and `btn-primary` on action buttons. Keep the same props/API. Reference current file for exact changes — swap `style={{ background: "var(--bg-card)", border: "..." }}` to `className="glass-card"`.

- [ ] **Step 6: Verify build**

```bash
cd web && npm run build
```

Expected: Build may warn about `HeroStatCard` import in `page.tsx` — that's expected, fixed in Task 4.

- [ ] **Step 7: Commit**

```bash
git add web/app/_components/GlassCard.tsx web/app/_components/TopBar.tsx web/app/_components/DataTable.tsx web/app/_components/StatCard.tsx web/app/_components/LoadingState.tsx
git commit -m "feat(web): glass shared components — StatCard, TopBar, DataTable, GlassCard, LoadingState"
```

---

## Phase 3: Layout Shell

### Task 3: Sidebar V3 + Split Login + Root Layout

**Files:**
- Modify: `web/app/_components/Sidebar.tsx`
- Modify: `web/app/_components/ApiKeyGate.tsx`
- Modify: `web/app/layout.tsx`

- [ ] **Step 1: Rebuild Sidebar.tsx (V3 Figma-aligned)**

Replace entire file. Changes from current:
- Nav item font: `text-sm` (14px) + `font-normal` (400 inactive) / `font-medium` (500 active)
- Nav item color: `var(--sidebar-text)` instead of `var(--text-muted)`
- Nav item hover: `var(--sidebar-text-hover)` + `background: rgba(255,255,255,0.04)`
- Icon size: `20` instead of `18`
- Padding: `px-4 py-[11px]` instead of `px-3 py-2.5`
- Gap between items: `mb-[3px]` instead of `mb-0.5`
- Active state: add left border bar with inline style:
  ```tsx
  style={{
    color: "var(--sidebar-text-active)",
    background: "var(--primary-muted)",
    borderLeft: "3px solid var(--primary)",
    boxShadow: "inset 3px 0 8px -3px rgba(139,92,246,0.4)",
  }}
  ```
- Logo icon: add `boxShadow: "0 4px 12px rgba(139,92,246,0.3)"`
- Logout: use `var(--sidebar-text)` color
- Mobile bottom nav: update icon size to `20`, color to `var(--sidebar-text)` / `var(--sidebar-text-active)`

- [ ] **Step 2: Rebuild ApiKeyGate.tsx — split login + TopBar shell**

Replace entire file. This is the biggest change. Two states:

**Unauthenticated → Split login page:**
- Full-screen flex layout: `min-h-screen flex`
- Left brand side (`flex-1`, bg `#0F0A1A`):
  - Two animated gradient orbs (define CSS keyframes inline or use `<style jsx>`)
  - Logo: 40px icon with gradient + shadow
  - Headline: "智能投放\n精准触达" — `text-[28px] font-extrabold`, gradient text via `background: linear-gradient(135deg, #fff 30%, #c4b5fd)` + `-webkit-background-clip: text`
  - Description: `text-[15px]` secondary color
  - Stats strip: 3 metrics (50M+ 日均竞价 / 200+ 活跃广告主 / 98.5% 可用性), purple values, border-top separator
- Right form side (`w-[440px]`, bg `var(--bg-card)`, border-left):
  - Centered form `max-w-xs`
  - Title "登录" 20px fw600
  - Email + password inputs with `#0F0A1A` background
  - Submit: `btn-primary` class, full-width
  - Divider + collapsible API Key fallback
  - **Keep all existing auth logic intact** (login(), isAuthenticated(), API key handling, admin redirect)

**Authenticated → Layout shell with TopBar:**
```tsx
import TopBar from "./TopBar";

// ... in the authenticated return:
return (
  <>
    {sidebar}
    <div className="flex-1 flex flex-col overflow-hidden">
      <TopBar />
      <main id="main-content" className="flex-1 overflow-auto ambient-glow" role="main">
        <div className="relative z-10 max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-7">
          {children}
        </div>
      </main>
    </div>
  </>
);
```

- [ ] **Step 3: Update layout.tsx**

No structural changes — keep existing file as-is. It already delegates to `<ApiKeyGate sidebar={<Sidebar />}>`.

- [ ] **Step 4: Verify build**

```bash
cd web && npm run build
```

Expected: May fail if `page.tsx` still imports `HeroStatCard`. If so, that's OK — fixed in Task 4.

- [ ] **Step 5: Commit**

```bash
git add web/app/_components/Sidebar.tsx web/app/_components/ApiKeyGate.tsx web/app/layout.tsx
git commit -m "feat(web): layout shell — V3 sidebar, split brand login, TopBar integration"
```

---

## Phase 4: Tenant Pages

### Task 4: Dashboard (/)

**Files:**
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Rewrite dashboard page**

Replace entire file. Key changes:
- Remove `HeroStatCard` import — use 4× `StatCard` in `grid-cols-2 md:grid-cols-4` grid
- Each StatCard: `icon`, `iconColor`, `trend`, `stagger` (1-4)
  - 总展示量: Eye, `#8B5CF6`, stagger 1
  - 点击次数: MousePointer, `#3B82F6`, stagger 2
  - 转化率: Target, `#F97316`, stagger 3
  - 总收入: DollarSign, `#22C55E`, stagger 4
- Charts: wrap in `<GlassCard>` with `p-6` padding
- Chart tooltips: `contentStyle={{ background: "#231830", border: "1px solid #2A2035", borderRadius: 12, color: "#fff", fontSize: 12 }}`
- Campaign table: wrap in `glass-card`, use uppercase muted headers
- Low balance warning: keep styling (already dark theme)
- Keep all existing API calls and data logic

- [ ] **Step 2: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 3: Commit**

```bash
git add web/app/page.tsx
git commit -m "feat(web): redesign dashboard — glass StatCards, enhanced charts"
```

---

### Task 5: Campaign Pages (/campaigns, /campaigns/new, /campaigns/[id])

**Files:**
- Modify: `web/app/campaigns/page.tsx`
- Modify: `web/app/campaigns/new/page.tsx`
- Modify: `web/app/campaigns/[id]/page.tsx`

- [ ] **Step 1: Read all three campaign pages**

Read each file fully before making changes.

- [ ] **Step 2: Rewrite campaigns list (/campaigns)**

Key changes:
- Filter tabs: active uses `btn-primary` styling (gradient bg, white text), inactive uses transparent + secondary text
- Campaign cards: use `glass-card` class on each card container
- "创建广告系列" CTA: use `btn-primary` class
- Filter button: use `btn-ghost` class
- MetricCell: remove `font-geist` reference, use Inter (default)
- Keep all existing API calls, action handlers, filtering logic

- [ ] **Step 3: Rewrite campaign wizard (/campaigns/new)**

Key changes:
- Wizard container: `glass-card` with `p-6` or `p-8`
- Step indicators: numbered circles — completed/active filled with gradient, future with border
- Form inputs: `style={{ background: "var(--bg-input)", border: "1px solid var(--border)" }}`, focus → purple border
- Select elements: same input styling
- Submit/next: `btn-primary`, back: `btn-ghost`
- Keep all form state, validation, and API submission logic

- [ ] **Step 4: Rewrite campaign detail (/campaigns/[id])**

Key changes:
- Campaign info: `glass-card` container
- Stats: use `StatCard` components
- Creatives list: `glass-card` per creative
- Add creative form: `glass-card` with form inputs
- Bid simulator: `glass-card`, slider/input with purple accent
- Keep all API calls and state logic

- [ ] **Step 5: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 6: Commit**

```bash
git add web/app/campaigns/
git commit -m "feat(web): redesign campaign pages — glass cards, gradient tabs, enhanced wizard"
```

---

### Task 6: Report Pages (/reports, /reports/[id])

**Files:**
- Modify: `web/app/reports/page.tsx`
- Modify: `web/app/reports/[id]/page.tsx`

- [ ] **Step 1: Read both report pages**

- [ ] **Step 2: Rewrite reports selector (/reports)**

- Each campaign rendered as clickable `glass-card` with name, status badge, key metrics
- Page title: 24px fw700

- [ ] **Step 3: Rewrite report detail (/reports/[id])**

- Stats: `StatCard` grid
- Charts: `GlassCard` containers, recharts with dark tooltip
- Bid table: use `DataTable` component
- CSV export buttons: `btn-ghost`
- Keep all API calls

- [ ] **Step 4: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 5: Commit**

```bash
git add web/app/reports/
git commit -m "feat(web): redesign report pages — glass containers, DataTable"
```

---

### Task 7: Analytics + Billing

**Files:**
- Modify: `web/app/analytics/page.tsx`
- Modify: `web/app/billing/page.tsx`

- [ ] **Step 1: Read both pages fully**

Analytics has complex SSE logic — read the entire file to understand token minting, refresh, and event source management.

- [ ] **Step 2: Rewrite analytics page**

**CRITICAL: DO NOT modify SSE connection logic, token minting, refresh timer, or EventSource management. Only change JSX and styling.**

Key styling changes:
- Stat cards: use `StatCard` components
- Live campaign table: use `DataTable` component
- Connection status: small indicator dot in glass-card container
- Data breathing: when SSE pushes new values, add `animate-breathe` class to updated cells. Implementation: track previous values in a ref, compare on update, add class via state, remove after animation.
- Keep all existing SSE code paths

- [ ] **Step 3: Rewrite billing page**

Key changes:
- Balance card: `glass-card` with large number (text-3xl fw700), billing type badge
- Top-up form: `glass-card`, inputs with standard styling, `btn-primary` submit
- Transaction history: use `DataTable` with columns for type, amount, balance_after, description, date
- Transaction type badges: topup=green, spend=red, adjustment=blue, refund=yellow
- Keep all API calls and state logic

- [ ] **Step 4: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 5: Commit**

```bash
git add web/app/analytics/page.tsx web/app/billing/page.tsx
git commit -m "feat(web): redesign analytics (data breathing) and billing pages"
```

---

## Phase 5: Admin Pages

### Task 8: Admin Layout

**Files:**
- Modify: `web/app/admin/layout.tsx`

- [ ] **Step 1: Read current admin/layout.tsx**

- [ ] **Step 2: Rebuild admin layout**

Key changes:

**AdminSidebar:**
- Apply V3 Figma-aligned styling (same tokens as tenant Sidebar):
  - Font: 14px, fw400 inactive, fw500 active
  - Colors: `var(--sidebar-text)`, `var(--sidebar-text-hover)`, `var(--sidebar-text-active)`
  - Icon size: 20px (bare icons, remove the old rounded icon wrapper background)
  - Active state: left border with glow shadow
  - Logo area: "DSP 管理后台" with gradient logo icon + subtitle

**AdminAuthGate login:**
- Use `glass-card` on login container
- Inputs: `#0F0A1A` background
- Submit: `btn-primary` class
- Keep all auth logic (checkAuth, handleAdminLogin, token verification)

**Authenticated shell:**
- Import and render `TopBar` above `<main>`
- Wrap main content in `ambient-glow` container
- Keep "← 广告主后台" link in sidebar footer

- [ ] **Step 3: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 4: Commit**

```bash
git add web/app/admin/layout.tsx
git commit -m "feat(web): redesign admin layout — V3 sidebar, glass login, TopBar"
```

---

### Task 9: Admin Pages (6 pages)

**Files:**
- Modify: `web/app/admin/page.tsx`
- Modify: `web/app/admin/agencies/page.tsx`
- Modify: `web/app/admin/users/page.tsx`
- Modify: `web/app/admin/creatives/page.tsx`
- Modify: `web/app/admin/invites/page.tsx`
- Modify: `web/app/admin/audit/page.tsx`

- [ ] **Step 1: Read all 6 admin pages**

- [ ] **Step 2: Rewrite admin overview (/admin)**

- StatCards: use `StatCard` for system health metrics
- Circuit breaker: `glass-card`, status indicator, trip/reset buttons (`btn-primary` / `btn-ghost`)
- System health: `glass-card` with colored service status dots

- [ ] **Step 3: Rewrite admin agencies (/admin/agencies)**

- Advertiser list: use `DataTable`
- Create form / top-up form: `glass-card` with form inputs
- Pending registrations: use `DataTable`

- [ ] **Step 4: Rewrite admin users (/admin/users)**

- User list: use `DataTable`
- Create/edit form: `glass-card`
- Role badges: purple for admin, blue for advertiser

- [ ] **Step 5: Rewrite admin creatives (/admin/creatives)**

- Creative list: `glass-card` per creative (card layout for visual review)
- Approve/reject buttons: green/red variants
- Status badges: reuse `StatusBadge`

- [ ] **Step 6: Rewrite admin invites (/admin/invites)**

- Invite list: use `DataTable`
- Create form: `glass-card`

- [ ] **Step 7: Rewrite admin audit (/admin/audit)**

- Audit log: use `DataTable`
- Pagination buttons: `btn-ghost`

- [ ] **Step 8: Verify build**

```bash
cd web && npm run build
```

- [ ] **Step 9: Commit**

```bash
git add web/app/admin/
git commit -m "feat(web): redesign all 6 admin pages — glass DataTables and components"
```

---

## Phase 6: Final Verification

### Task 10: Build + Lint + Smoke Test

- [ ] **Step 1: Full build**

```bash
cd web && npm run build
```

Expected: Zero errors

- [ ] **Step 2: Lint**

```bash
cd web && npm run lint
```

Expected: No errors (warnings OK)

- [ ] **Step 3: Type check**

```bash
cd web && npx tsc --noEmit
```

Expected: No type errors

- [ ] **Step 4: Visual smoke test**

```bash
cd web && npm run dev
```

Verify in browser at http://localhost:4000:
1. Login page: split layout with brand side + form
2. Dashboard: glass stat cards + charts
3. Campaigns: card-based layout with filter tabs
4. Analytics: live SSE data table
5. Admin: unified design language
6. Entrance animations play on each page
7. Glass card hover effects work
8. Mobile bottom nav renders

- [ ] **Step 5: Commit any final fixes**

```bash
git add web/
git commit -m "fix(web): final polish for frontend V2 redesign"
```
