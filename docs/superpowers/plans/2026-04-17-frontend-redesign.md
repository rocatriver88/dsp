# Frontend Redesign V2 — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Redesign the entire DSP Platform frontend from a light industrial theme to a dark purple premium theme, matching the Figma design in `docs/design/figma-screens/`.

**Architecture:** Update CSS design tokens → shared components → page-by-page conversion. No backend changes. No route changes. All existing API integrations preserved. Add recharts for data visualization. Convert campaign list from table to card layout.

**Tech Stack:** Next.js 16 (App Router), React 19, Tailwind CSS 4, TypeScript, recharts (new), lucide-react (existing), Inter font (replacing Geist + IBM Plex Sans)

**Design Source:** `DESIGN.md` V2 (2026-04-17), Figma screenshots in `docs/design/figma-screens/`

---

## Phases

| Phase | Scope | Tasks |
|-------|-------|-------|
| 1 | Foundation: tokens, fonts, shared components | 1-5 |
| 2 | Tenant pages: dashboard, campaigns, campaigns/new, campaigns/[id] | 6-10 |
| 3 | Tenant pages: analytics, billing, reports | 11-13 |
| 4 | Auth pages: login gate, admin login | 14-15 |
| 5 | Admin pages: layout, overview, agencies, creatives, invites, audit, users | 16-22 |

---

## Phase 1: Foundation

### Task 1: Install recharts + Inter font

**Files:**
- Modify: `web/package.json`
- Modify: `web/app/globals.css`
- Modify: `web/app/layout.tsx`

- [ ] **Step 1: Install recharts**

```bash
cd web && npm install recharts
```

- [ ] **Step 2: Update globals.css — replace font imports and CSS variables**

Replace the entire `web/app/globals.css` with:

```css
@import url("https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap");
@import "tailwindcss";

:root {
  /* Primary */
  --primary: #8B5CF6;
  --primary-hover: #7C3AED;
  --primary-muted: rgba(139, 92, 246, 0.12);

  /* Dark backgrounds */
  --bg-page: #0F0A1A;
  --bg-sidebar: #0A0610;
  --bg-card: #1A1225;
  --bg-card-elevated: #231830;
  --bg-input: #1A1225;

  /* Borders */
  --border: #2A2035;
  --border-subtle: #1F1730;

  /* Text */
  --text-primary: #FFFFFF;
  --text-secondary: #A0A0B0;
  --text-muted: #6B6B80;

  /* Semantic */
  --success: #22C55E;
  --warning: #EAB308;
  --error: #EF4444;
  --info: #3B82F6;

  /* Chart colors */
  --chart-1: #8B5CF6;
  --chart-2: #3B82F6;
  --chart-3: #22C55E;
  --chart-4: #EAB308;
  --chart-5: #EF4444;
  --chart-6: #EC4899;
}

body {
  background: var(--bg-page);
  color: var(--text-primary);
  font-family: "Inter", -apple-system, sans-serif;
  font-size: 14px;
  line-height: 1.6;
  margin: 0;
}

/* Tabular numbers for data alignment */
.tabular-nums {
  font-variant-numeric: tabular-nums;
}

/* Navigation items: minimum 44px touch target */
nav a, nav button {
  min-height: 44px;
  display: flex;
  align-items: center;
}

/* Accessibility: visible focus ring */
a:focus-visible,
button:focus-visible,
input:focus-visible,
select:focus-visible,
textarea:focus-visible {
  outline: 2px solid var(--primary);
  outline-offset: 2px;
}

/* Screen reader only utility */
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

/* Touch target minimum size */
button, a {
  min-height: 44px;
  min-width: 44px;
}

/* Exception: inline text links and table cell links */
td a, p a, span a, .inline-link {
  min-height: auto;
  min-width: auto;
}

/* Reduced motion preference */
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    transition-duration: 0.01ms !important;
  }
}

/* Scrollbar styling for dark theme */
::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}
::-webkit-scrollbar-track {
  background: var(--bg-page);
}
::-webkit-scrollbar-thumb {
  background: var(--border);
  border-radius: 4px;
}
::-webkit-scrollbar-thumb:hover {
  background: var(--text-muted);
}
```

- [ ] **Step 3: Update layout.tsx — switch to Inter font**

Replace `web/app/layout.tsx` with:

```tsx
import type { Metadata } from "next";
import ApiKeyGate from "./_components/ApiKeyGate";
import Sidebar from "./_components/Sidebar";
import "./globals.css";

export const metadata: Metadata = {
  title: "DSP Platform",
  description: "Demand-Side Platform — 广告主管理后台",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" className="h-full">
      <body className="h-full flex flex-col md:flex-row" style={{ background: "var(--bg-page)", color: "var(--text-primary)" }}>
        <a href="#main-content"
          className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:top-2 focus:left-2 focus:px-4 focus:py-2 focus:rounded focus:text-sm"
          style={{ background: "var(--primary)", color: "#fff" }}>
          跳转到主内容
        </a>

        <ApiKeyGate
          sidebar={<Sidebar />}
        >
          {children}
        </ApiKeyGate>
      </body>
    </html>
  );
}
```

- [ ] **Step 4: Verify build compiles**

```bash
cd web && npx next build 2>&1 | tail -5
```

Expected: build succeeds (warnings OK, no errors)

- [ ] **Step 5: Commit**

```bash
git add web/package.json web/package-lock.json web/app/globals.css web/app/layout.tsx
git commit -m "feat(web): install recharts, switch to Inter font, dark theme CSS tokens"
```

---

### Task 2: Redesign Sidebar component

**Files:**
- Modify: `web/app/_components/Sidebar.tsx`

- [ ] **Step 1: Replace Sidebar.tsx**

Replace the entire file with:

```tsx
"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { logout } from "@/lib/api";
import { LayoutDashboard, Megaphone, BarChart3, Activity, Wallet, LogOut } from "lucide-react";
import type { LucideIcon } from "lucide-react";

const navItems: { href: string; label: string; icon: LucideIcon }[] = [
  { href: "/", label: "仪表板", icon: LayoutDashboard },
  { href: "/campaigns", label: "广告系列", icon: Megaphone },
  { href: "/reports", label: "报表", icon: BarChart3 },
  { href: "/analytics", label: "数据分析", icon: Activity },
  { href: "/billing", label: "账户", icon: Wallet },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <>
      {/* Mobile bottom nav */}
      <nav aria-label="移动端导航"
        className="md:hidden fixed bottom-0 left-0 right-0 z-50 flex items-center justify-around px-2 py-2"
        style={{ background: "var(--bg-sidebar)", borderTop: "1px solid var(--border)" }}>
        {navItems.map((item) => {
          const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
          return (
            <Link key={item.href} href={item.href}
              className="flex flex-col items-center gap-0.5 px-3 py-1.5 text-[10px] rounded-lg transition-colors"
              style={{ color: isActive ? "var(--primary)" : "var(--text-muted)" }}>
              <item.icon size={20} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Desktop sidebar */}
      <nav aria-label="主导航"
        className="hidden md:flex flex-shrink-0 flex-col"
        style={{ width: 220, minHeight: "100vh", background: "var(--bg-sidebar)" }}>
        <div className="px-5 py-5" style={{ borderBottom: "1px solid var(--border)" }}>
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center"
              style={{ background: "linear-gradient(135deg, #8B5CF6, #6D28D9)" }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <div>
              <h1 className="text-sm font-semibold" style={{ color: "var(--text-primary)" }}>DSP Platform</h1>
            </div>
          </div>
        </div>

        <div className="flex-1 py-3 px-3" role="list">
          {navItems.map((item) => {
            const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
            return (
              <Link key={item.href} href={item.href} role="listitem"
                className="flex items-center gap-3 px-3 py-2.5 text-sm rounded-lg mb-0.5 transition-colors"
                style={{
                  color: isActive ? "#8B5CF6" : "var(--text-muted)",
                  background: isActive ? "var(--primary-muted)" : "transparent",
                }}>
                <item.icon size={18} />
                {item.label}
              </Link>
            );
          })}
        </div>

        <div className="px-5 py-4" style={{ borderTop: "1px solid var(--border)" }}>
          <button
            onClick={() => { logout(); }}
            className="flex items-center gap-2 text-sm transition-colors w-full"
            style={{ color: "var(--text-muted)" }}>
            <LogOut size={16} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}
```

- [ ] **Step 2: Verify build**

```bash
cd web && npx next build 2>&1 | tail -5
```

- [ ] **Step 3: Commit**

```bash
git add web/app/_components/Sidebar.tsx
git commit -m "feat(web): redesign sidebar — dark purple theme with gradient logo"
```

---

### Task 3: Redesign StatCard component

**Files:**
- Modify: `web/app/_components/StatCard.tsx`

- [ ] **Step 1: Replace StatCard.tsx with new dark theme + trend indicators**

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
  className?: string;
}

export function StatCard({ label, value, trend, icon: Icon, iconColor = "#8B5CF6", className }: StatCardProps) {
  const iconBg = iconColor + "26"; // ~15% opacity

  return (
    <div className={`rounded-[14px] p-5 ${className || ""}`}
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <div className="flex items-start justify-between mb-3">
        {Icon && (
          <div className="w-10 h-10 rounded-lg flex items-center justify-center"
            style={{ background: iconBg }}>
            <Icon size={20} style={{ color: iconColor }} />
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
      <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>{label}</p>
    </div>
  );
}

export function HeroStatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="col-span-2 rounded-[14px] p-6"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <p className="text-xs font-medium mb-2" style={{ color: "var(--text-muted)" }}>{label}</p>
      <p className="text-4xl font-bold tracking-tight tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</p>
      {sub && <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>{sub}</p>}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/_components/StatCard.tsx
git commit -m "feat(web): redesign StatCard — dark cards with icon circles and trend indicators"
```

---

### Task 4: Redesign StatusBadge component

**Files:**
- Modify: `web/app/_components/StatusBadge.tsx`

- [ ] **Step 1: Replace StatusBadge.tsx with dark theme badges**

```tsx
"use client";

const styles: Record<string, { bg: string; text: string }> = {
  draft: { bg: "rgba(107,107,128,0.15)", text: "#6B6B80" },
  active: { bg: "rgba(34,197,94,0.15)", text: "#22C55E" },
  paused: { bg: "rgba(234,179,8,0.15)", text: "#EAB308" },
  completed: { bg: "rgba(59,130,246,0.15)", text: "#3B82F6" },
};

const zhLabels: Record<string, string> = {
  draft: "草稿",
  active: "运行中",
  paused: "已暂停",
  completed: "已完成",
};

export function StatusBadge({ status }: { status: string }) {
  const s = styles[status] || styles.draft;
  return (
    <span className="inline-block px-2.5 py-0.5 text-[11px] font-semibold rounded-full"
      style={{ background: s.bg, color: s.text }}>
      {zhLabels[status] || status}
    </span>
  );
}

const eventStyles: Record<string, { bg: string; text: string }> = {
  bid: { bg: "rgba(59,130,246,0.15)", text: "#3B82F6" },
  win: { bg: "rgba(34,197,94,0.15)", text: "#22C55E" },
  loss: { bg: "rgba(239,68,68,0.15)", text: "#EF4444" },
  impression: { bg: "rgba(139,92,246,0.15)", text: "#8B5CF6" },
  click: { bg: "rgba(249,115,22,0.15)", text: "#F97316" },
};

export function EventBadge({ type }: { type: string }) {
  const s = eventStyles[type] || { bg: "rgba(107,107,128,0.15)", text: "#6B6B80" };
  return (
    <span className="inline-block px-1.5 py-0.5 text-[11px] font-medium rounded"
      style={{ background: s.bg, color: s.text }}>
      {type}
    </span>
  );
}

export function TypeBadge({ type }: { type: string }) {
  return (
    <span className="inline-block px-2 py-0.5 text-[11px] font-medium rounded-full"
      style={{ background: "rgba(139,92,246,0.15)", color: "#A78BFA" }}>
      {type}
    </span>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/_components/StatusBadge.tsx
git commit -m "feat(web): redesign StatusBadge — dark theme with Chinese labels"
```

---

### Task 5: Redesign LoadingState component

**Files:**
- Modify: `web/app/_components/LoadingState.tsx`

- [ ] **Step 1: Replace LoadingState.tsx with dark theme variants**

```tsx
"use client";

export function LoadingSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="h-10 rounded-lg animate-pulse"
          style={{ background: "var(--bg-card)" }} />
      ))}
    </div>
  );
}

export function LoadingCards({ count = 4 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="rounded-[14px] h-28 animate-pulse"
          style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }} />
      ))}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="text-center py-12 rounded-[14px]"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <p className="text-sm mb-3" style={{ color: "var(--error)" }}>{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="px-4 py-2 text-sm font-semibold text-white rounded-lg transition-colors"
          style={{ background: "var(--primary)" }}>
          重试
        </button>
      )}
    </div>
  );
}

export function EmptyState({ heading, message, actionLabel, actionHref, onAction }: {
  heading?: string;
  message: string;
  actionLabel?: string;
  actionHref?: string;
  onAction?: () => void;
}) {
  return (
    <div className="rounded-[14px] p-12 text-center"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      {heading && <p className="text-base font-semibold mb-2" style={{ color: "var(--text-primary)" }}>{heading}</p>}
      <p className="text-sm mb-1" style={{ color: "var(--text-secondary)" }}>{message}</p>
      {actionLabel && actionHref && (
        <a href={actionHref}
          className="inline-block mt-4 px-6 py-2.5 text-sm font-semibold text-white rounded-lg"
          style={{ background: "var(--primary)" }}>
          {actionLabel}
        </a>
      )}
      {actionLabel && onAction && !actionHref && (
        <button onClick={onAction}
          className="mt-4 px-6 py-2.5 text-sm font-semibold text-white rounded-lg"
          style={{ background: "var(--primary)" }}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/_components/LoadingState.tsx
git commit -m "feat(web): redesign LoadingState — dark theme skeletons and states"
```

---

## Phase 2: Tenant Pages (Dashboard + Campaigns)

### Task 6: Redesign Dashboard (home page)

**Files:**
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Replace page.tsx with dark theme dashboard + chart placeholders**

Replace `web/app/page.tsx`. Key changes:
- Dark card backgrounds for all stat cards
- Add icon + trend indicators to stat cards (use Eye, MousePointer, Target, DollarSign from lucide-react)
- Replace white bg with `var(--bg-card)` everywhere
- Replace text-gray-* with `var(--text-*)` tokens
- Replace bg-blue-600 with `var(--primary)` for buttons
- Campaign table: dark header, dark rows, purple links
- Low balance warning: dark card with warning border

NOTE: Full code for this task is ~160 lines. The pattern is: for every `bg-white` → `style={{ background: "var(--bg-card)" }}`, for every `text-gray-500` → `style={{ color: "var(--text-secondary)" }}`, for every `bg-blue-600` → `style={{ background: "var(--primary)" }}`, for every `border-gray-*` → `style={{ borderColor: "var(--border)" }}`.

The implementer should apply these substitutions systematically to the existing `page.tsx` while preserving all business logic.

- [ ] **Step 2: Verify the page renders in browser**

```bash
cd web && npm run dev
# Open http://localhost:4000 in browser, verify dark theme
```

- [ ] **Step 3: Commit**

```bash
git add web/app/page.tsx
git commit -m "feat(web): redesign dashboard — dark theme with stat cards and trend indicators"
```

---

### Task 7: Redesign Campaign List (table → cards)

**Files:**
- Modify: `web/app/campaigns/page.tsx`

- [ ] **Step 1: Convert campaign list from table to card layout**

Key changes:
- Replace `<table>` with campaign cards (dark card bg, 14px radius)
- Each card: header (name + status badge + type badge), date range, metrics row, action buttons
- Add filter tabs: 全部 / 运行中 / 已暂停 / 草稿
- Action buttons as icon buttons (Pause, Copy, Edit, Trash2, MoreHorizontal from lucide-react)
- Keep all existing API logic (handleAction, load, etc.)
- Dark theme colors throughout

- [ ] **Step 2: Verify in browser**
- [ ] **Step 3: Commit**

```bash
git add web/app/campaigns/page.tsx
git commit -m "feat(web): redesign campaigns — card layout with filter tabs and dark theme"
```

---

### Task 8: Redesign Campaign Creation Wizard

**Files:**
- Modify: `web/app/campaigns/new/page.tsx`

- [ ] **Step 1: Apply dark theme to 3-step wizard**

Key changes:
- All form inputs: dark background (`var(--bg-input)`), border `var(--border)`, white text
- Step indicators: purple for active step, muted for inactive
- Buttons: primary purple, ghost with dark border
- Targeting geo/OS toggles: purple when selected
- Dark card backgrounds for each step section

- [ ] **Step 2: Verify wizard flow in browser (click through all 3 steps)**
- [ ] **Step 3: Commit**

```bash
git add web/app/campaigns/new/page.tsx
git commit -m "feat(web): redesign campaign wizard — dark theme forms and step indicators"
```

---

### Task 9: Redesign Campaign Detail Page

**Files:**
- Modify: `web/app/campaigns/[id]/page.tsx`

- [ ] **Step 1: Apply dark theme to campaign detail**

Key changes:
- Header with campaign name + status badge
- Dark stat cards for campaign metrics
- Creative list with dark cards
- Edit form inputs: dark theme
- All buttons: purple primary

- [ ] **Step 2: Commit**

```bash
git add "web/app/campaigns/[id]/page.tsx"
git commit -m "feat(web): redesign campaign detail — dark theme with stat cards"
```

---

### Task 10: Add recharts to Dashboard

**Files:**
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Add performance trend chart (area line) and budget allocation chart (bar)**

Add to dashboard below stat cards:
- "投放表现趋势" area chart: purple gradient fill, dark grid, last 7 days
- "预算分配" bar chart: purple + blue bars, campaign names on x-axis
- Both use recharts `ResponsiveContainer`, `AreaChart`/`BarChart`
- Dark tooltip styling
- Data sourced from existing API data (campaigns array)

```tsx
// Example chart component pattern:
import { AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from "recharts";

// Inside component:
<div className="rounded-[14px] p-6" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
  <h3 className="text-base font-semibold mb-1" style={{ color: "var(--text-primary)" }}>投放表现趋势</h3>
  <p className="text-xs mb-4" style={{ color: "var(--text-muted)" }}>最近7天的数据概览</p>
  <ResponsiveContainer width="100%" height={240}>
    <AreaChart data={trendData}>
      <defs>
        <linearGradient id="purpleGrad" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#8B5CF6" stopOpacity={0.3} />
          <stop offset="95%" stopColor="#8B5CF6" stopOpacity={0} />
        </linearGradient>
      </defs>
      <CartesianGrid stroke="#1F1730" strokeDasharray="3 3" />
      <XAxis dataKey="date" stroke="#6B6B80" fontSize={11} />
      <YAxis stroke="#6B6B80" fontSize={11} />
      <Tooltip contentStyle={{ background: "#231830", border: "1px solid #2A2035", borderRadius: 12, color: "#fff" }} />
      <Area type="monotone" dataKey="value" stroke="#8B5CF6" fill="url(#purpleGrad)" strokeWidth={2} />
    </AreaChart>
  </ResponsiveContainer>
</div>
```

- [ ] **Step 2: Verify charts render in browser**
- [ ] **Step 3: Commit**

```bash
git add web/app/page.tsx
git commit -m "feat(web): add recharts to dashboard — trend area chart + budget bar chart"
```

---

## Phase 3: Analytics, Billing, Reports

### Task 11: Redesign Analytics Page

**Files:**
- Modify: `web/app/analytics/page.tsx`

- [ ] **Step 1: Apply dark theme + add chart components**

Key changes:
- Dark stat cards with KPI values (转化率/跳出率/CPC/CPA)
- Multi-line trend chart (综合表现趋势) with recharts
- Donut chart (设备分布) with recharts PieChart
- Channel analysis table with dark styling
- Keep existing SSE real-time connection logic

- [ ] **Step 2: Commit**

```bash
git add web/app/analytics/page.tsx
git commit -m "feat(web): redesign analytics — dark theme with multi-line chart and donut"
```

---

### Task 12: Redesign Billing Page

**Files:**
- Modify: `web/app/billing/page.tsx`

- [ ] **Step 1: Apply dark theme to billing**

Key changes:
- Balance card: dark bg, large white number
- Top-up form: dark inputs, purple quick-amount buttons when selected
- Transaction table: dark header/rows
- Amount badges: green for income, red for expense

- [ ] **Step 2: Commit**

```bash
git add web/app/billing/page.tsx
git commit -m "feat(web): redesign billing — dark theme with styled transaction table"
```

---

### Task 13: Redesign Reports Pages

**Files:**
- Modify: `web/app/reports/page.tsx`
- Modify: `web/app/reports/[id]/page.tsx`

- [ ] **Step 1: Apply dark theme to reports list and detail**
- [ ] **Step 2: Commit**

```bash
git add web/app/reports/page.tsx "web/app/reports/[id]/page.tsx"
git commit -m "feat(web): redesign reports — dark theme"
```

---

## Phase 4: Auth Pages

### Task 14: Redesign Tenant Login (ApiKeyGate)

**Files:**
- Modify: `web/app/_components/ApiKeyGate.tsx`

- [ ] **Step 1: Dark theme login screen**

Key changes:
- Full-page dark background (`var(--bg-page)`)
- Login card: dark card bg, purple accent
- Input: dark bg, border on focus → purple
- Button: purple primary
- Logo area with gradient icon

- [ ] **Step 2: Commit**

```bash
git add web/app/_components/ApiKeyGate.tsx
git commit -m "feat(web): redesign tenant login — dark theme with purple accent"
```

---

### Task 15: Redesign Admin Login + Layout

**Files:**
- Modify: `web/app/admin/layout.tsx`

- [ ] **Step 1: Dark theme admin login + admin sidebar**

Key changes:
- Admin login: same dark card style as tenant login
- Admin sidebar: dark bg with admin nav items, purple active state
- Admin mobile nav: dark bottom bar

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/layout.tsx
git commit -m "feat(web): redesign admin login and layout — dark theme"
```

---

## Phase 5: Admin Pages

### Task 16: Redesign Admin Overview

**Files:**
- Modify: `web/app/admin/page.tsx`

- [ ] **Step 1: Dark theme admin dashboard**

Key changes:
- Stat cards: dark bg with purple/blue/green/orange icons
- Circuit breaker panel: dark card
- System health: dark card with status indicators

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/page.tsx
git commit -m "feat(web): redesign admin overview — dark theme"
```

---

### Task 17: Redesign Admin Agencies

**Files:**
- Modify: `web/app/admin/agencies/page.tsx`

- [ ] **Step 1: Dark theme agencies management**

Key changes:
- Pending registrations table: dark header/rows
- Advertiser list table: dark theme
- Top-up modal: dark card, dark inputs
- Approve/reject buttons: green/red on dark bg

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/agencies/page.tsx
git commit -m "feat(web): redesign admin agencies — dark theme tables and modal"
```

---

### Task 18: Redesign Admin Creatives

**Files:**
- Modify: `web/app/admin/creatives/page.tsx`

- [ ] **Step 1: Dark theme creatives review**
- [ ] **Step 2: Commit**

```bash
git add web/app/admin/creatives/page.tsx
git commit -m "feat(web): redesign admin creatives — dark theme"
```

---

### Task 19: Redesign Admin Invites

**Files:**
- Modify: `web/app/admin/invites/page.tsx`

- [ ] **Step 1: Dark theme invite management**
- [ ] **Step 2: Commit**

```bash
git add web/app/admin/invites/page.tsx
git commit -m "feat(web): redesign admin invites — dark theme"
```

---

### Task 20: Redesign Admin Audit

**Files:**
- Modify: `web/app/admin/audit/page.tsx`

- [ ] **Step 1: Dark theme audit log**
- [ ] **Step 2: Commit**

```bash
git add web/app/admin/audit/page.tsx
git commit -m "feat(web): redesign admin audit — dark theme"
```

---

### Task 21: Redesign Admin Users

**Files:**
- Modify: `web/app/admin/users/page.tsx`

- [ ] **Step 1: Dark theme user management**
- [ ] **Step 2: Commit**

```bash
git add web/app/admin/users/page.tsx
git commit -m "feat(web): redesign admin users — dark theme"
```

---

### Task 22: Final E2E Verification

**Files:**
- Modify: `test/e2e/test_design_compliance.py` (update color references)

- [ ] **Step 1: Update E2E design compliance tests for dark theme**

Update `DESIGN` dict in `test_design_compliance.py`:
- primary_color: "#8b5cf6"
- sidebar_bg: "#0a0610"
- page_bg: "#0f0a1a"

- [ ] **Step 2: Run full E2E test suite**

```bash
cd test/e2e && bash run.sh
```

- [ ] **Step 3: Run E2E business flow test**

```bash
PYTHONIOENCODING=utf-8 python test/e2e/test_e2e_flow.py
```

- [ ] **Step 4: Commit test updates**

```bash
git add test/e2e/test_design_compliance.py
git commit -m "test(e2e): update design compliance tests for dark purple theme"
```

---

## Design Token Quick Reference (for all tasks)

When converting any page from light → dark, apply these substitutions:

| Old (light) | New (dark) |
|-------------|-----------|
| `bg-white` | `style={{ background: "var(--bg-card)" }}` |
| `bg-gray-50` | `style={{ background: "var(--bg-page)" }}` |
| `bg-gray-100` | `style={{ background: "var(--bg-card)" }}` |
| `text-gray-500`, `text-gray-600` | `style={{ color: "var(--text-secondary)" }}` |
| `text-gray-400`, `text-gray-300` | `style={{ color: "var(--text-muted)" }}` |
| `text-gray-700`, `text-gray-800`, `text-gray-900` | `style={{ color: "var(--text-primary)" }}` |
| `border-gray-100`, `border-gray-200` | `style={{ borderColor: "var(--border)" }}` |
| `bg-blue-600` | `style={{ background: "var(--primary)" }}` |
| `hover:bg-blue-700` | onHover → `var(--primary-hover)` |
| `text-blue-600` | `style={{ color: "var(--primary)" }}` |
| `bg-green-50 text-green-700` | `style={{ background: "rgba(34,197,94,0.15)", color: "#22C55E" }}` |
| `bg-yellow-50 text-yellow-700` | `style={{ background: "rgba(234,179,8,0.15)", color: "#EAB308" }}` |
| `bg-red-50 text-red-700` | `style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}` |
| `rounded-lg` | `rounded-[14px]` (cards) or `rounded-lg` (small elements) |
| `font-geist` | remove (Inter is the only font now) |
