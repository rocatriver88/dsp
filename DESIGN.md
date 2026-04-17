# Design System — DSP Platform V2

## Product Context
- **What this is:** B2B 程序化广告投放管理平台（DSP）
- **Who it's for:** 中国中小广告代理商的广告运营人员
- **Space/industry:** 广告技术（AdTech），竞品: 巨量引擎、广点通、The Trade Desk
- **Project type:** Web app (dashboard + data visualization + campaign management)
- **Design source:** Figma Make 设计稿 (2026-04-17), screenshots in `docs/design/figma-screens/`

## Aesthetic Direction
- **Direction:** Dark Premium / 深色科技感
- **Theme:** Dark mode only（不提供 light mode）
- **Decoration level:** Moderate — 图标、趋势箭头、渐变色块增强品牌感，但不过度装饰
- **Mood:** 高级、专业、科技感。深紫色调传递"这是一个认真的数据平台"。长时间使用不累眼，数据在深色背景上更醒目。
- **Inspiration:** Linear / Raycast 的品质感，但数据密度更高
- **Anti-patterns:** No light/white backgrounds for main content, no cold gray (use warm dark tones), no flat unstyled tables, no generic blue-only palette

## Typography
- **Display/Hero:** Inter (28-32px, font-weight 700) — 页面标题
- **Body:** Inter (13-14px/1.6, font-weight 400) — 正文、描述
- **UI/Labels:** Inter (11-12px, font-weight 500, uppercase letter-spacing 0.8px) — stat 标签、表头
- **Data/Numbers:** Inter (24-28px, font-weight 700, font-variant-numeric: tabular-nums) — stat 数值
- **Trend indicators:** Inter (13px, font-weight 600) — 涨跌百分比
- **Chinese body:** Inter + system fallback (13-14px) — 中文内容
- **Code:** JetBrains Mono
- **Loading:** Google Fonts (Inter)
- **Scale:** 11px / 12px / 13px / 14px / 16px / 20px / 24px / 28px / 32px

## Color

### Primary Palette
- **Primary accent:** #8B5CF6 (紫色，品牌色，CTA、活跃状态、高亮)
- **Primary hover:** #7C3AED
- **Primary muted:** rgba(139, 92, 246, 0.12) (活跃导航背景、选中态)

### Dark Background Layers
- **Page background:** #0F0A1A (最深层，页面底色)
- **Sidebar background:** #0A0610 (侧边栏，比页面更深)
- **Card background:** #1A1225 (卡片、面板)
- **Card elevated:** #231830 (悬停态、弹窗)
- **Input background:** #1A1225 (输入框、搜索栏)
- **Border:** #2A2035 (卡片边框、分割线)
- **Border subtle:** #1F1730 (更淡的分割)

### Text
- **Text primary:** #FFFFFF (标题、重要数据)
- **Text secondary:** #A0A0B0 (描述、次要信息)
- **Text muted:** #6B6B80 (标签、占位符、禁用态)
- **Text on primary:** #FFFFFF (紫色按钮上的文字)

### Semantic
- **Success:** #22C55E (campaign active, positive metrics, 涨幅)
- **Warning:** #EAB308 (campaign paused, approaching limits)
- **Error:** #EF4444 (failures, overspend, 跌幅)
- **Info:** #3B82F6 (informational badges, 某些图表线)

### Chart Colors (按顺序使用)
1. #8B5CF6 (purple — primary)
2. #3B82F6 (blue)
3. #22C55E (green)
4. #EAB308 (yellow)
5. #EF4444 (red)
6. #EC4899 (pink)

### Stat Card Icon Backgrounds
- Purple: rgba(139, 92, 246, 0.15) with #8B5CF6 icon
- Blue: rgba(59, 130, 246, 0.15) with #3B82F6 icon
- Orange: rgba(249, 115, 22, 0.15) with #F97316 icon
- Green: rgba(34, 197, 94, 0.15) with #22C55E icon

## Spacing
- **Base unit:** 4px
- **Density:** Comfortable
- **Scale:** 2xs(2px) xs(4px) sm(8px) md(16px) lg(24px) xl(32px) 2xl(48px) 3xl(64px)
- **Card padding:** 24px
- **Card gap:** 16px
- **Table row height:** 48px
- **Section gap:** 24-32px
- **Sidebar item padding:** 10px 16px

## Layout
- **Approach:** Sidebar + Content area
- **Sidebar:** 220px fixed width (desktop), icons with labels
- **Top bar:** Full width, contains search + notifications + avatar
- **Content max-width:** None (fills available space, max ~1200px content)
- **Content padding:** 32px horizontal, 28px vertical
- **Border radius:** sm: 6px, md: 10px, lg: 14px, full: 9999px (pills/badges)
- **Cards:** 1px border (#2A2035), dark background (#1A1225), rounded corners (10-14px)

## Motion
- **Approach:** Subtle and purposeful
- **Easing:** enter: ease-out, exit: ease-in, move: ease-in-out
- **Duration:** micro: 100ms (hover states), short: 150ms (state transitions), medium: 300ms (page transitions)
- **Hover effects:** Cards lift with subtle brightness increase. Buttons brighten.
- **Loading:** Skeleton placeholders with subtle pulse animation on dark background
- **Charts:** Smooth entry animations on data load (recharts default)
- **Rules:** Animations should feel premium, not playful. No bouncing, no overshooting.

## Component Patterns

### Sidebar Navigation
- Dark background (#0A0610), full height
- Logo: "DSP Platform" with icon, top-left
- Nav items: icon (20px) + label, 10px 16px padding
- Active: purple left border accent + rgba(139,92,246,0.12) background + #8B5CF6 text
- Inactive: #6B6B80 text, hover → #A0A0B0
- Bottom: user avatar / settings

### Top Bar
- Search input: dark background, rounded, magnifying glass icon, placeholder "搜索广告系列、受众..."
- Right side: notification bell + user avatar circle (#8B5CF6 gradient)
- Height: ~64px

### Stat Cards
- Dark card background (#1A1225), 1px border, 14px radius
- Left: colored icon circle (40px, with category-specific background)
- Top-right: trend arrow (↑/↓) + percentage change (green for positive, red for negative)
- Value: 24-28px bold white
- Label: 12px muted text below value
- 4-column grid on desktop, responsive

### Campaign Cards (not table rows)
- Full-width dark card, 14px radius
- Header: campaign name (16px bold white) + status badge + type badge
- Sub-header: date range (13px muted)
- Metrics row: 预算 / 已花费 / 展示量 / 点击量 / 点击率 / 转化数 / ROI / 进度
- ROI: green text when positive
- Progress: small circular or bar indicator
- Action buttons: icon buttons row (暂停/复制/编辑/删除/更多), top-right of card
- Hover: subtle brightness lift

### Data Tables (within cards)
- Dark card container, 14px radius
- Header: 11px uppercase muted text, letter-spacing 0.8px
- Rows: 13px, border-top #1F1730
- Numbers: tabular-nums, right-aligned
- Links: #8B5CF6, no underline
- Row hover: subtle background lighten

### Charts (recharts)
- Dark background, no outer border
- Grid lines: very subtle (#1F1730)
- Line charts: gradient fill under curve (purple → transparent)
- Bar charts: solid bars with category colors, rounded top corners
- Donut/pie: category colors with dark center
- Axis labels: 11px muted text
- Tooltip: dark card style with border
- Legend: colored dots + 12px muted text

### Status Badges
- Pill shape (border-radius: 9999px), 11px font-weight 600
- Active/运行中: rgba(34,197,94,0.15) bg, #22C55E text
- Paused/已暂停: rgba(234,179,8,0.15) bg, #EAB308 text
- Draft/草稿: rgba(107,107,128,0.15) bg, #6B6B80 text
- Type badges (搜索广告/展示广告): rgba(139,92,246,0.15) bg, #A78BFA text

### Buttons
- Primary: #8B5CF6 bg, white text, 10px radius, 10px 20px padding, hover → #7C3AED
- Secondary/Ghost: transparent bg, #A0A0B0 text, 1px border #2A2035, hover → border #8B5CF6
- Icon button: 36px circle, transparent bg, #6B6B80 icon, hover → #A0A0B0
- 14px font-weight 600

### Filter Tabs
- Pill-style tab bar with options: 全部 / 运行中 / 已暂停 / 草稿
- Active: #8B5CF6 bg, white text
- Inactive: transparent bg, #A0A0B0 text, hover → card background
- Plus: 筛选 button with filter icon

### Search Input
- Dark background (#1A1225), 1px border #2A2035, 10px radius
- Magnifying glass icon left, 13px placeholder text (#6B6B80)
- Focus: border → #8B5CF6

### Empty States
- Centered text block
- Primary heading (16px medium white)
- Secondary description (14px secondary)
- Primary action button below

### Trend Indicators
- Arrow icon (↑ or ↓) + percentage text
- Positive: #22C55E
- Negative: #EF4444
- Neutral: #6B6B80
- Font: 13px font-weight 600

## Data Visualization

### Chart Library
- **Use:** recharts (React charting library)
- **Theme:** All charts use dark background, matching card style

### Chart Types by Page
- **仪表板:** 面积折线图 (投放趋势) + 柱状图 (预算分配)
- **数据分析:** 多线折线图 (综合表现) + 环形图 (设备分布) + 表格 (渠道分析)
- **受众管理:** 无图表，卡片式数据展示

### Chart Styling
- Background: transparent (inherits card background)
- Grid: #1F1730, dashed
- Axis text: #6B6B80, 11px
- Tooltip: #231830 bg, 1px #2A2035 border, 12px radius
- Area fill: linear gradient from chart color (opacity 0.3) to transparent
- Bar radius: 4px top corners

## Accessibility
- Focus ring: 2px solid #8B5CF6, 2px offset
- Touch targets: 44px minimum (nav links, buttons)
- Skip-to-content link for keyboard navigation
- ARIA landmarks on nav, main content
- Reduced motion: respect prefers-reduced-motion
- Color contrast: all text meets WCAG AA on dark backgrounds (checked: white on #0F0A1A = 15.4:1, #A0A0B0 on #0F0A1A = 7.2:1)
- Status not conveyed by color alone (always pair with text label or icon)

## Responsive Behavior
- **Desktop (≥1280px):** Full sidebar + content area
- **Tablet (768-1279px):** Collapsed sidebar (icons only, 64px) + content area
- **Mobile (<768px):** Bottom tab navigation + full-width content
- Stat cards: 4-col → 2-col → 1-col
- Campaign cards: full-width always
- Charts: maintain aspect ratio, reduce padding

## Decisions Log
| Date | Decision | Rationale |
|------|----------|-----------|
| 2026-03-30 | Initial design system | Created by /design-consultation. Industrial/utilitarian |
| 2026-04-17 | V2 redesign: dark purple theme | User wanted brand-first redesign. Figma Make generated dark theme with #8B5CF6 purple accent. Differentiates from all competitors. |
| 2026-04-17 | Card-based campaigns (not table) | Figma design uses campaign cards with inline metrics instead of table rows. Richer data display per campaign. |
| 2026-04-17 | recharts for visualization | Figma design spec calls for recharts. React-native, supports dark theme well. |
| 2026-04-17 | Inter as sole typeface | Clean, widely available, excellent CJK rendering, tabular-nums support. Replaces Geist + IBM Plex Sans. |
