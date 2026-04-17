# Frontend Visual Redesign — Design Spec

**Date:** 2026-04-17
**Status:** Approved
**Scope:** Visual restructuring of all 14 existing pages to DESIGN.md V2 + Figma quality with enhanced effects

## Overview

Rebuild the entire DSP Platform frontend to match the DESIGN.md V2 dark purple design system and Figma design mockups, with additional premium visual enhancements (glassmorphism, gradient effects, entrance animations). No new pages, no backend changes, no route changes.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scope | A: Visual restructuring of all 14 pages | Full platform consistency, no new pages without API backing |
| Admin treatment | A: Unified design language | One platform, one visual language — admin pages are simple tables/buttons, alignment cost is low |
| Login page | A: Brand-level split layout | Left brand area (slogan + stats + animated gradient orbs) + right login form. First touchpoint = premium impression |
| Design direction | B: Figma base + advanced enhancements | Glassmorphism, gradient mesh, entrance animations, data breathing. Tone: "high-end tech" not "clean utility" |
| Sidebar font | V3: Figma-aligned | 14px / fw400 inactive / fw500 active / 20px icons / 11px 16px padding — relaxed, readable, matches Figma |

## Layout Shell

### Sidebar (220px fixed, desktop)

- **Background:** `#0A0610`
- **Logo:** 32px rounded square with `linear-gradient(135deg, #8B5CF6, #6D28D9)`, `box-shadow: 0 4px 12px rgba(139,92,246,0.3)`
- **Nav items (inactive):**
  - Font: Inter 14px, font-weight 400, letter-spacing 0.01em
  - Color: `#9898AC` (updated from DESIGN.md's `#6B6B80` for sidebar-specific contrast)
  - Icon: 20px, opacity 0.55
  - Padding: 11px 16px, gap between items: 3px
- **Nav items (active):**
  - Font: Inter 14px, font-weight 500
  - Color: `#8B5CF6`
  - Background: `rgba(139,92,246,0.12)`
  - Left border accent: 3px, `#8B5CF6`, `box-shadow: 0 0 8px rgba(139,92,246,0.4)`
  - Icon: opacity 1
- **Nav items (hover):** Color `#C0C0D0`, background `rgba(255,255,255,0.04)`
- **Footer:** Logout button, same `#9898AC` color, hover `#C0C0D0`
- **Mobile (<768px):** Bottom tab navigation, 5 items, icons + 10px labels
- **Tablet (768-1279px):** Collapsed sidebar 64px, icons only

### TopBar (NEW — 64px height)

- **Background:** `rgba(15,10,26,0.6)` + `backdrop-filter: blur(12px)`
- **Border:** bottom 1px `#2A2035`
- **Left:** Search input (UI-only, no backend search API — visual placeholder for future functionality)
  - 320px width, `#1A1225` background, `#2A2035` border, 10px radius
  - Magnifying glass icon + placeholder "搜索广告系列、受众..."
  - Focus: border → `#8B5CF6`
- **Right:** Notification bell (UI-only, no notification backend — renders static with red dot) + user avatar (32px circle, `linear-gradient(135deg, #8B5CF6, #3B82F6)`, double ring shadow, displays first letter of user name from auth context)

### Content Area

- **Padding:** 28px vertical, 32px horizontal
- **Max-width:** ~1200px centered
- **Ambient gradient:** Right-top `radial-gradient(circle, rgba(139,92,246,0.06), transparent 70%)`, pointer-events: none

## Login Page (Split Layout)

### Left Brand Side

- **Background:** `#0F0A1A`
- **Animated orbs:** Two `radial-gradient` blobs (purple 15% opacity, blue 10% opacity) with slow float animations (8-10s ease-in-out infinite)
- **Logo:** 40px icon with gradient, stronger shadow
- **Headline:** "智能投放\n精准触达" — 28px fw800, `linear-gradient(135deg, #fff 30%, #c4b5fd)` text gradient
- **Description:** 15px `#A0A0B0`, max-width 380px
- **Stats strip:** Bottom of brand area, border-top `rgba(139,92,246,0.15)`, three stats (日均竞价请求 / 活跃广告主 / 服务可用性) with purple values

### Right Form Side

- **Background:** `#1A1225`, 440px width, border-left `#2A2035`
- **Form:** Max-width 320px centered
- **Title:** "登录" 20px fw600
- **Inputs:** `#0F0A1A` background, `#2A2035` border, focus → `#8B5CF6`
- **Button:** `linear-gradient(135deg, #8B5CF6, #7C3AED)`, `box-shadow: 0 4px 16px rgba(139,92,246,0.25)`
- **API Key fallback:** Collapsible section below divider

## Component System

### Glass StatCard

- **Background:** `rgba(26,18,37,0.7)` + `backdrop-filter: blur(16px)`
- **Border:** 1px `rgba(139,92,246,0.1)`
- **Top light edge:** 1px `linear-gradient(90deg, transparent, rgba(139,92,246,0.3), transparent)`
- **Border-radius:** 14px
- **Padding:** 20px
- **Hover:** border-color `rgba(139,92,246,0.25)`, translateY(-1px), `box-shadow: 0 8px 32px rgba(139,92,246,0.08)`
- **Label:** 11px fw500 uppercase tracking 0.8px `#6B6B80`
- **Value:** 24px fw700 tabular-nums
- **Trend:** 12px fw600, green `#22C55E` for positive, red `#EF4444` for negative
- **Icon:** 36px rounded square, category-colored background (purple/blue/orange/green at 15% opacity)

### CampaignCard (per Figma)

- **Container:** Glass card (same as StatCard background treatment)
- **Header row:** Campaign name (16px fw700) + StatusBadge + TypeBadge + action buttons (right-aligned)
- **Sub-header:** Date range (12px `#6B6B80`)
- **Metrics row:** 8-column grid — 预算 / 已花费 / 展示量 / 点击量 / 点击率 / 转化数 / ROI / 进度
  - Labels: 11px uppercase muted
  - Values: 14px fw700 tabular-nums
  - ROI: green when positive
  - Progress: percentage + thin bar (purple fill, red when ≥90%)
- **Action buttons:** 32px icon buttons (pause/play, copy, edit, delete, more), `#6B6B80` → `#A0A0B0` on hover

### DataTable

- **Container:** Glass card
- **Header:** 11px uppercase fw600, letter-spacing 0.8px, `#6B6B80`
- **Row height:** 48px
- **Row border:** 1px `#1F1730`
- **Numbers:** tabular-nums, right-aligned
- **Links:** `#8B5CF6`, no underline
- **Row hover:** subtle background lighten (`rgba(255,255,255,0.02)`)

### StatusBadge (unchanged)

- Pill shape (border-radius 9999px), 11px fw600
- Active: `rgba(34,197,94,0.15)` bg, `#22C55E` text
- Paused: `rgba(234,179,8,0.15)` bg, `#EAB308` text
- Draft: `rgba(107,107,128,0.15)` bg, `#6B6B80` text

### Charts (recharts)

- Inherit card background (transparent)
- Grid: `#1F1730` dashed
- Axis labels: 11px `#6B6B80`
- Tooltip: glass card style — `#231830` bg, `backdrop-filter: blur(8px)`, 1px `#2A2035` border, 12px radius
- Area fill: linear-gradient from chart color (0.3 opacity) to transparent
- Bar radius: 4px top corners

### Buttons

- **Primary:** `linear-gradient(135deg, #8B5CF6, #7C3AED)`, white text, 8-10px radius, `box-shadow: 0 4px 16px rgba(139,92,246,0.25)`. Hover: deepen gradient + expand shadow
- **Secondary/Ghost:** transparent bg, `#A0A0B0` text, 1px `#2A2035` border, hover → border `#8B5CF6`
- **Icon button:** 36px circle, transparent bg, `#6B6B80` icon, hover → `#A0A0B0`

### Form Inputs

- Background: `#1A1225` (or `#0F0A1A` in login context)
- Border: 1px `#2A2035`
- Focus: border `#8B5CF6`
- Border-radius: 8px
- Label: 12px fw500 `#A0A0B0`
- Placeholder: `#6B6B80`

## Enhancement Effects (B-tier)

### 1. Page Entrance Animations

- StatCards: staggered `fadeInUp` with `animation-delay: 0/50/100/150ms`
- Charts: `fadeIn` 300ms ease-out
- Tables: `fadeIn` 200ms
- CSS-only, no JS animation library

```css
@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(12px); }
  to { opacity: 1; transform: translateY(0); }
}
.stat-card { animation: fadeInUp 400ms ease-out both; }
.stat-card:nth-child(1) { animation-delay: 0ms; }
.stat-card:nth-child(2) { animation-delay: 50ms; }
.stat-card:nth-child(3) { animation-delay: 100ms; }
.stat-card:nth-child(4) { animation-delay: 150ms; }
```

### 2. Ambient Gradient Background

- Each page: one `radial-gradient` purple orb (6% opacity) in the content area
- Position varies per page to avoid repetition
- Purely decorative, `pointer-events: none`, `z-index: 0`

### 3. Glassmorphism Consistency

- All card/panel containers use: `backdrop-filter: blur(16px)` + semi-transparent background + top gradient light edge
- Hover: border brightens + subtle lift + purple shadow

### 4. Data Breathing (analytics page only)

- When SSE pushes new data, the updated number gets a brief glow pulse
- CSS animation: `text-shadow` pulse from `0 0 0 transparent` to `0 0 8px rgba(139,92,246,0.4)` and back, 600ms

### 5. Gradient Accents

- Logo icon, user avatar, primary CTA buttons use purple-blue gradient instead of flat color
- Consistent brand signature across the platform

## Pages — Per-Page Specs

### Tenant Pages (8)

| Page | Route | Key Elements |
|------|-------|-------------|
| Dashboard | `/` | 4× glass StatCard (展示量/点击/转化率/收入) + 趋势 AreaChart + 预算 BarChart + 最近campaigns表格 + 低余额警告 |
| Campaigns | `/campaigns` | Filter tabs (全部/运行中/已暂停/草稿) + CampaignCard列表 + 搜索 + 筛选按钮 + "创建广告系列" CTA |
| New Campaign | `/campaigns/new` | 3-step wizard (基本信息+计费→定向→素材), 步骤条带渐变进度, 表单升级 |
| Campaign Detail | `/campaigns/[id]` | Stats区 + campaign信息卡片 + targeting详情 + creatives列表 + bid simulator |
| Reports | `/reports` | Campaign选择器, 卡片化选择 |
| Report Detail | `/reports/[id]` | Stats卡片 + hourly折线图 + geo柱状图 + bid透明度表格 + CSV导出按钮 |
| Analytics | `/analytics` | StatCards + 多线折线图 + 环形图(设备分布) + 渠道分析表格, SSE实时流 + 数据呼吸效果 |
| Billing | `/billing` | 余额大数字卡片 + 充值表单 + 交易记录DataTable |

### Admin Pages (6)

| Page | Route | Key Elements |
|------|-------|-------------|
| Admin Overview | `/admin` | StatCards (系统健康) + 熔断器状态/控制 + pending registrations计数 |
| Agencies | `/admin/agencies` | 广告商列表DataTable + 创建广告商表单 + 充值操作 |
| Users | `/admin/users` | 用户DataTable + 创建/编辑用户表单 |
| Creatives | `/admin/creatives` | 素材审核卡片列表 + approve/reject按钮 |
| Invites | `/admin/invites` | 邀请码DataTable + 创建表单 |
| Audit Log | `/admin/audit` | 审计日志DataTable, 时间线式, 分页 |

## What Does NOT Change

- **API clients:** `lib/api.ts`, `lib/admin-api.ts` — logic untouched, only UI layer changes
- **TypeScript types:** `lib/api-types.ts` — auto-generated, not hand-edited
- **Route structure:** All routes remain identical
- **Auth logic:** JWT + API Key dual-mode, SSE HMAC tokens
- **next.config.ts:** standalone output, port 4000
- **Backend:** Zero backend changes

## DESIGN.md Updates Required

The following DESIGN.md values need updating to reflect approved design decisions:

1. **Sidebar nav inactive color:** `#6B6B80` → `#9898AC` (sidebar-only, other `--text-muted` usage stays `#6B6B80`)
2. **Sidebar nav inactive hover:** `#A0A0B0` → `#C0C0D0`
3. **Sidebar nav font-size:** add explicit `14px` (was implied by Typography section)
4. **Sidebar nav font-weight:** inactive `400`, active `500` (was not explicitly specified)
5. **Sidebar nav icon size:** add explicit `20px`
6. **TopBar:** Add full component pattern section (glassmorphism treatment, search, notifications, avatar)
7. **Enhancement effects:** Add section for glassmorphism, entrance animations, ambient gradients, data breathing
8. **Login page:** Add component pattern for split-layout brand login
9. **Buttons:** Update Primary to gradient instead of flat color
