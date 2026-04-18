# Layout Polish — Design Spec

**Date:** 2026-04-18
**Status:** Approved
**Scope:** 8 layout/sizing fixes across all pages

## Fixes

### 1. PageHeader Component (P0)

Create `web/app/_components/PageHeader.tsx`:
- Props: `title: string`, `subtitle?: string`, `action?: ReactNode`
- Title: `text-2xl font-bold text-primary`
- Subtitle: `text-[13px] text-secondary`, below title with `mb-6`
- Action: right-aligned, vertically centered with title
- Layout: `flex items-start justify-between` wrapper, title/subtitle in left div, action in right
- Replace all 14 pages' ad-hoc title+subtitle markup with this component

### 2. Content Area Padding Consistency (P0)

Both tenant ApiKeyGate and admin layout content wrappers use identical padding:
`px-6 py-6 md:px-8 md:py-7`

Files:
- `web/app/_components/ApiKeyGate.tsx` — authenticated shell content wrapper
- `web/app/admin/layout.tsx` — admin authenticated shell content wrapper

### 3. TopBar Search Responsive (P0)

In `web/app/_components/TopBar.tsx`:
- Change search container from `w-80` to `max-w-xs flex-1`
- Prevents overlap with right-side elements on narrow viewports

### 6. Login Page 2:1 Ratio (P1)

In `web/app/_components/ApiKeyGate.tsx` login section:
- Left brand side: `flex-[2]` (replaces `flex-1`)
- Right form side: `flex-1` (replaces `w-[440px]`)
- Remove `flex-shrink-0` from right side
- Maintain `min-w-[360px]` on right side to prevent collapse

### 7. StatCard Label Proportion (P1)

In `web/app/_components/StatCard.tsx`:
- Label: `text-[11px] font-medium uppercase tracking-wider` → `text-xs` (12px, no uppercase, no tracking)
- Value: keep `text-2xl font-bold`
- Ratio improves from 2.2x to 2x

Also update `web/app/globals.css` — remove `.stagger-N` classes' uppercase if any are applied (they're not, stagger only affects animation-delay).

### 11. Admin Overview StatCard Icons (P2)

In `web/app/admin/page.tsx`:
- Replace local `StatCard` component with imported shared `StatCard` from `_components/StatCard`
- Add icons: 广告主数 → Building2 (#8B5CF6), 活跃 Campaign → Target (#3B82F6), 今日花费 → DollarSign (#F97316), 平台总余额 → Wallet (#22C55E)

### 12. Circuit Breaker Button Style (P2)

In `web/app/admin/page.tsx`:
- "手动熔断" button: solid red bg → `background: rgba(239,68,68,0.15); color: #EF4444`
- Matches the reject button pattern used elsewhere

### 13. Invite Form Enhancement (P2)

In `web/app/admin/invites/page.tsx`:
- Wrap form section in `max-w-lg` to prevent full-width stretch
- Add "备注" text input field
- Add "过期时间" date input field
- Keep "最大使用次数" and "生成邀请码" button

## What Does NOT Change

- Colors, fonts, glass effects — already established
- API logic — no backend changes
- Route structure — no changes
- StatusBadge, DataTable, GlassCard components — no changes
