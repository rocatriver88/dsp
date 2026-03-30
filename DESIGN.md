# Design System — DSP Platform

## Product Context
- **What this is:** B2B 程序化广告投放管理平台（DSP）
- **Who it's for:** 中国中小广告代理商的广告运营人员
- **Space/industry:** 广告技术（AdTech），竞品: 巨量引擎、广点通、The Trade Desk
- **Project type:** Web app (dashboard + data tables + campaign management)

## Aesthetic Direction
- **Direction:** Industrial/Utilitarian
- **Decoration level:** Minimal — typography and whitespace do all the work
- **Mood:** 清晰、靠谱、专业。用户每天在这个界面工作数小时，视觉上不能累眼。数据可读性和操作效率优先于装饰。
- **Anti-patterns:** No decorative borders on cards (use background + spacing), no purple gradients, no 3-column icon grids, no centered everything

## Typography
- **Display/Hero:** Geist (48px bold, -0.5px letter-spacing) — 现代感、辨识度高
- **Body:** IBM Plex Sans (15px/1.6) — 中英混排表现优秀，可读性强
- **UI/Labels:** IBM Plex Sans (12-13px medium)
- **Data/Tables:** Geist (14px, font-variant-numeric: tabular-nums) — 数字对齐
- **Code:** JetBrains Mono
- **Loading:** Google Fonts (IBM Plex Sans), CDN for Geist
- **Scale:** 12px / 13px / 14px / 15px / 18px / 20px / 24px / 36px / 48px

## Color
- **Approach:** Restrained — 1 accent + neutrals, color is rare and meaningful
- **Primary:** #2563EB (蓝色，信任+专业，CTA、链接、活跃状态)
- **Primary Hover:** #1D4ED8
- **Sidebar:** #1A1A1A bg, #A1A1AA text, #FFFFFF active text
- **Neutrals:**
  - 50: #F9FAFB (page background)
  - 100: #F3F4F6 (card hover, table header)
  - 200: #E5E7EB (borders)
  - 300: #D1D5DB (disabled)
  - 500: #6B7280 (secondary text)
  - 700: #374151 (strong secondary)
  - 900: #111827 (primary text)
- **Semantic:**
  - Success: #059669 (campaign active, positive metrics)
  - Warning: #D97706 (campaign paused, approaching limits)
  - Error: #DC2626 (failures, overspend)

## Spacing
- **Base unit:** 4px
- **Density:** Comfortable
- **Scale:** 2xs(2px) xs(4px) sm(8px) md(16px) lg(24px) xl(32px) 2xl(48px) 3xl(64px)
- **Card padding:** 20px
- **Card gap:** 12px
- **Table row height:** 44px (touch-friendly)
- **Section gap:** 24-32px

## Layout
- **Approach:** Grid-disciplined
- **Sidebar:** 224px fixed width (desktop), horizontal scroll (mobile)
- **Content max-width:** 1152px (max-w-6xl)
- **Content padding:** 32px horizontal, 24px vertical (desktop); 16px (mobile)
- **Border radius:** sm: 4px, md: 6px, lg: 8px, full: 9999px (pills/badges)
- **Cards:** No border. White background (#FFFFFF) on gray page (#F9FAFB). Differentiation through background contrast and spacing, not lines.

## Motion
- **Approach:** Minimal-functional
- **Easing:** enter: ease-out, exit: ease-in, move: ease-in-out
- **Duration:** micro: 50-100ms (hover), short: 150ms (state transitions), medium: 250ms (page transitions)
- **Rules:** No decorative animations. Transitions only where they help the user understand state changes (active tab, expanding panel, loading skeleton).

## Component Patterns

### Stat Cards
- No border, white background on gray page
- Label: 12px medium, secondary color
- Value: Geist font, bold, primary color
- Sub-text: 12px, muted color
- Hero card: 2-column span, larger value (36px)

### Data Tables
- No outer border, white background with rounded corners (8px)
- Header: neutral-50 background, 13px medium secondary
- Row: 14px, border-top neutral-100
- Numbers: Geist tabular-nums for alignment
- Links: primary blue, no underline

### Status Badges
- Pill shape (border-radius: 9999px)
- Active: green-50 bg, green-700 text
- Paused: yellow-50 bg, yellow-700 text
- Draft: neutral-100 bg, neutral-500 text
- 12px medium weight

### Buttons
- Primary: #2563EB bg, white text, 6px radius, 8px 16px padding
- Ghost: transparent bg, secondary text, 1px border
- 14px medium weight

### Empty States
- Centered text block with icon or illustration
- Primary heading (16px medium)
- Secondary description (14px, secondary color)
- Primary action button below

## Accessibility
- Focus ring: 2px solid #2563EB, 2px offset
- Touch targets: 44px minimum (nav links, buttons)
- Skip-to-content link for keyboard navigation
- ARIA landmarks on nav, main content
- Reduced motion: respect prefers-reduced-motion
- Color contrast: all text meets WCAG AA (4.5:1 for body, 3:1 for large)

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-30 | Initial design system | Created by /design-consultation. Industrial/utilitarian for B2B ad platform |
| 2026-03-30 | Geist for display/data | Modern, excellent tabular-nums, differentiates from Inter/Roboto |
| 2026-03-30 | No-border cards | Cleaner, more professional than bordered cards |
| 2026-03-30 | IBM Plex Sans body | Already in use, proven Chinese/English rendering |
