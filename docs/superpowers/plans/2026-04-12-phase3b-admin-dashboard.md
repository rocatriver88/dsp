# Phase 3B: Admin Dashboard Frontend Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build admin dashboard pages under `/admin/*` with token-based auth, separate layout, and pages for agency management, invite codes, creative review, circuit breaker control, and audit log.

**Architecture:** Next.js App Router with a dedicated admin layout (`web/app/admin/layout.tsx`) using its own `AdminTokenGate` auth component (localStorage `dsp_admin_token`). Admin API calls go to the internal port (8182) with `X-Admin-Token` header. Follows existing DESIGN.md patterns (Geist font, blue primary, stat cards, data tables).

**Tech Stack:** Next.js 16, React 19, TypeScript, Tailwind CSS

**IMPORTANT:** Read `DESIGN.md` before implementing any visual component. Read `web/AGENTS.md` — this Next.js version may differ from training data.

---

## File Structure

```
web/lib/
└── admin-api.ts               # Admin API client (X-Admin-Token auth)

web/app/admin/
├── layout.tsx                  # Admin layout with AdminTokenGate + sidebar
├── page.tsx                    # Overview: global stats, circuit breaker status
├── agencies/page.tsx           # Advertiser list with balance, campaigns, top-up
├── creatives/page.tsx          # Creative review queue (approve/reject)
├── invites/page.tsx            # Invite code management (create/list)
└── audit/page.tsx              # Audit log viewer
```

---

### Task 1: Admin API Client

**Files:**
- Create: `web/lib/admin-api.ts`

- [ ] **Step 1: Write admin API client**

```typescript
// web/lib/admin-api.ts
const API_BASE = process.env.NEXT_PUBLIC_ADMIN_API_URL || "http://localhost:8182";

function getAdminToken(): string {
  if (typeof window !== "undefined") {
    return localStorage.getItem("dsp_admin_token") || "";
  }
  return "";
}

async function adminRequest<T>(path: string, options?: RequestInit): Promise<T> {
  const token = getAdminToken();
  const res = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { "X-Admin-Token": token } : {}),
      ...options?.headers,
    },
  });
  if (!res.ok) {
    if (res.status === 401 && typeof window !== "undefined") {
      localStorage.removeItem("dsp_admin_token");
      window.location.reload();
      throw new Error("Admin authentication failed");
    }
    const body = await res.json().catch(() => ({}));
    throw new Error(body.error || `Admin API error: ${res.status}`);
  }
  return res.json();
}

export interface AdminAdvertiser {
  id: number;
  company_name: string;
  contact_email: string;
  api_key: string;
  balance_cents: number;
  billing_type: string;
  created_at: string;
}

export interface InviteCode {
  id: number;
  code: string;
  created_by: string;
  max_uses: number;
  used_count: number;
  expires_at: string | null;
  created_at: string;
}

export interface AdminCreative {
  id: number;
  campaign_id: number;
  name: string;
  ad_type: string;
  format: string;
  size: string;
  ad_markup: string;
  destination_url: string;
  status: string;
  created_at: string;
}

export interface CircuitStatus {
  circuit_breaker: string;
  reason: string;
  global_spend_today_cents: number;
}

export interface AuditEntry {
  id: number;
  advertiser_id: number;
  actor: string;
  action: string;
  resource_type: string;
  resource_id: number;
  details: Record<string, unknown>;
  created_at: string;
}

export interface Registration {
  id: number;
  company_name: string;
  contact_email: string;
  contact_phone: string;
  business_type: string;
  website: string;
  status: string;
  created_at: string;
}

export const adminApi = {
  // Advertisers
  listAdvertisers: () => adminRequest<AdminAdvertiser[]>("/api/v1/admin/advertisers"),
  topUp: (advertiserId: number, amountCents: number, description: string) =>
    adminRequest("/api/v1/admin/topup", {
      method: "POST",
      body: JSON.stringify({ advertiser_id: advertiserId, amount_cents: amountCents, description }),
    }),

  // Registrations
  listRegistrations: () => adminRequest<Registration[]>("/api/v1/admin/registrations"),
  approveRegistration: (id: number) =>
    adminRequest(`/api/v1/admin/registrations/${id}/approve`, { method: "POST" }),
  rejectRegistration: (id: number, reason: string) =>
    adminRequest(`/api/v1/admin/registrations/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),

  // Invite codes
  listInviteCodes: () => adminRequest<InviteCode[]>("/api/v1/admin/invite-codes"),
  createInviteCode: (maxUses: number) =>
    adminRequest<{ code: string }>("/api/v1/admin/invite-codes", {
      method: "POST",
      body: JSON.stringify({ max_uses: maxUses }),
    }),

  // Creatives
  listCreativesForReview: () => adminRequest<AdminCreative[]>("/api/v1/admin/creatives"),
  approveCreative: (id: number) =>
    adminRequest(`/api/v1/admin/creatives/${id}/approve`, { method: "POST" }),
  rejectCreative: (id: number, reason: string) =>
    adminRequest(`/api/v1/admin/creatives/${id}/reject`, {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),

  // Circuit breaker
  getCircuitStatus: () => adminRequest<CircuitStatus>("/api/v1/admin/circuit-status"),
  tripCircuitBreaker: (reason: string) =>
    adminRequest("/api/v1/admin/circuit-break", {
      method: "POST",
      body: JSON.stringify({ reason }),
    }),
  resetCircuitBreaker: () =>
    adminRequest("/api/v1/admin/circuit-reset", { method: "POST" }),

  // System health
  getHealth: () => adminRequest<{ status: string; active_campaigns: number }>("/api/v1/admin/health"),

  // Audit log
  getAuditLog: (limit = 50, offset = 0) =>
    adminRequest<AuditEntry[]>(`/api/v1/admin/audit-log?limit=${limit}&offset=${offset}`),
};
```

- [ ] **Step 2: Commit**

```bash
git add web/lib/admin-api.ts
git commit -m "feat(web): add admin API client"
```

---

### Task 2: Admin Layout + Token Gate

**Files:**
- Create: `web/app/admin/layout.tsx`

- [ ] **Step 1: Write admin layout**

The admin layout has its own token gate (separate from advertiser ApiKeyGate) and its own sidebar navigation. It does NOT use the root layout's ApiKeyGate.

```tsx
// web/app/admin/layout.tsx
"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";

const adminNavItems = [
  { href: "/admin", label: "概览", icon: "概" },
  { href: "/admin/agencies", label: "代理商", icon: "商" },
  { href: "/admin/creatives", label: "素材审核", icon: "审" },
  { href: "/admin/invites", label: "邀请码", icon: "邀" },
  { href: "/admin/audit", label: "审计日志", icon: "志" },
];

function AdminSidebar() {
  const pathname = usePathname();

  return (
    <nav className="hidden md:flex w-56 min-h-screen flex-shrink-0 flex-col"
      style={{ background: "#111827" }}>
      <div className="px-5 py-5 border-b border-gray-700">
        <h1 className="text-lg font-semibold text-white tracking-tight">DSP Admin</h1>
        <p className="text-xs mt-0.5 text-gray-400">运营管理后台</p>
      </div>
      <div className="flex-1 py-3">
        {adminNavItems.map((item) => {
          const isActive = pathname === item.href || (item.href !== "/admin" && pathname.startsWith(item.href));
          return (
            <Link key={item.href} href={item.href}
              className={`flex items-center gap-3 px-5 py-2.5 text-sm transition-colors ${isActive ? "bg-blue-600/20 text-blue-400 border-r-2 border-blue-400" : "text-gray-400 hover:bg-gray-800"}`}>
              <span className="text-xs font-bold w-5 h-5 flex items-center justify-center rounded bg-gray-800 text-gray-500">{item.icon}</span>
              {item.label}
            </Link>
          );
        })}
      </div>
      <div className="px-5 py-3 border-t border-gray-700 space-y-2">
        <Link href="/" className="block text-xs text-gray-500 hover:text-gray-300">← 广告主后台</Link>
        <button
          onClick={() => { localStorage.removeItem("dsp_admin_token"); window.location.reload(); }}
          className="text-xs text-gray-500 hover:text-white">
          退出 Admin
        </button>
      </div>
    </nav>
  );
}

function AdminTokenGate({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    const t = localStorage.getItem("dsp_admin_token");
    if (t) setToken(t);
    setChecking(false);
  }, []);

  if (checking) return null;

  if (!token) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg p-8 w-full max-w-md">
          <h2 className="text-xl font-semibold mb-2">DSP Admin</h2>
          <p className="text-sm text-gray-500 mb-6">输入管理员 Token 登录运营后台</p>
          <input
            type="password"
            placeholder="admin token"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-blue-500"
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter" && input.length > 0) {
                localStorage.setItem("dsp_admin_token", input.trim());
                setToken(input.trim());
              }
            }}
          />
          <button
            onClick={() => { localStorage.setItem("dsp_admin_token", input.trim()); setToken(input.trim()); }}
            disabled={!input}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300">
            登录
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      <AdminSidebar />
      <main className="flex-1 overflow-auto">
        <div className="max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-6">
          {children}
        </div>
      </main>
    </div>
  );
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminTokenGate>{children}</AdminTokenGate>;
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/layout.tsx
git commit -m "feat(web): add admin layout with token gate and sidebar"
```

---

### Task 3: Admin Overview Page

**Files:**
- Create: `web/app/admin/page.tsx`

- [ ] **Step 1: Write overview page**

```tsx
// web/app/admin/page.tsx
"use client";

import { useEffect, useState } from "react";
import { adminApi, CircuitStatus, AdminAdvertiser } from "@/lib/admin-api";

export default function AdminOverviewPage() {
  const [advertisers, setAdvertisers] = useState<AdminAdvertiser[]>([]);
  const [circuit, setCircuit] = useState<CircuitStatus | null>(null);
  const [health, setHealth] = useState<{ status: string; active_campaigns: number } | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    Promise.all([
      adminApi.listAdvertisers().catch(() => []),
      adminApi.getCircuitStatus().catch(() => null),
      adminApi.getHealth().catch(() => null),
    ]).then(([a, c, h]) => {
      setAdvertisers(a as AdminAdvertiser[]);
      setCircuit(c as CircuitStatus | null);
      setHealth(h as { status: string; active_campaigns: number } | null);
    }).finally(() => setLoading(false));
  }, []);

  const totalBalance = advertisers.reduce((s, a) => s + a.balance_cents, 0);
  const globalSpend = circuit?.global_spend_today_cents || 0;

  if (loading) {
    return (
      <div>
        <h2 className="text-2xl font-semibold mb-6">运营概览</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
          {[1, 2, 3, 4].map(i => <div key={i} className="h-24 bg-gray-100 rounded-lg animate-pulse" />)}
        </div>
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">运营概览</h2>

      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
        <div className="rounded-lg bg-white p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">代理商数</p>
          <p className="text-2xl font-semibold font-geist tabular-nums">{advertisers.length}</p>
        </div>
        <div className="rounded-lg bg-white p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">活跃 Campaign</p>
          <p className="text-2xl font-semibold font-geist tabular-nums">{health?.active_campaigns || 0}</p>
        </div>
        <div className="rounded-lg bg-white p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">今日全局花费</p>
          <p className="text-2xl font-semibold font-geist tabular-nums">{(globalSpend / 100).toFixed(2)}</p>
        </div>
        <div className="rounded-lg bg-white p-5">
          <p className="text-xs font-medium text-gray-500 mb-1">平台总余额</p>
          <p className="text-2xl font-semibold font-geist tabular-nums">{(totalBalance / 100).toFixed(2)}</p>
        </div>
      </div>

      {/* Circuit Breaker */}
      <div className="bg-white rounded-lg p-6 mb-6">
        <h3 className="text-sm font-semibold mb-4">熔断器状态</h3>
        <div className="flex items-center gap-4">
          <span className={`inline-block w-3 h-3 rounded-full ${circuit?.circuit_breaker === "open" ? "bg-green-500" : "bg-red-500"}`} />
          <span className="text-sm font-medium">{circuit?.circuit_breaker === "open" ? "正常（竞价开启）" : "已熔断（竞价停止）"}</span>
          {circuit?.reason && <span className="text-xs text-gray-500">原因: {circuit.reason}</span>}
          <div className="ml-auto flex gap-2">
            {circuit?.circuit_breaker === "open" ? (
              <button onClick={() => adminApi.tripCircuitBreaker("admin manual").then(() => window.location.reload())}
                className="px-3 py-1.5 text-xs font-medium bg-red-50 text-red-700 rounded hover:bg-red-100">
                紧急熔断
              </button>
            ) : (
              <button onClick={() => adminApi.resetCircuitBreaker().then(() => window.location.reload())}
                className="px-3 py-1.5 text-xs font-medium bg-green-50 text-green-700 rounded hover:bg-green-100">
                恢复竞价
              </button>
            )}
          </div>
        </div>
      </div>

      {/* System Health */}
      <div className="bg-white rounded-lg p-6">
        <h3 className="text-sm font-semibold mb-4">系统状态</h3>
        <div className="flex items-center gap-2">
          <span className={`w-2 h-2 rounded-full ${health?.status === "ok" ? "bg-green-500" : "bg-red-500"}`} />
          <span className="text-sm">{health?.status === "ok" ? "所有服务正常" : "服务异常"}</span>
        </div>
      </div>
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/page.tsx
git commit -m "feat(web): add admin overview page with circuit breaker control"
```

---

### Task 4: Agencies Management Page

**Files:**
- Create: `web/app/admin/agencies/page.tsx`

- [ ] **Step 1: Write agencies page**

```tsx
// web/app/admin/agencies/page.tsx
"use client";

import { useEffect, useState } from "react";
import { adminApi, AdminAdvertiser, Registration } from "@/lib/admin-api";

export default function AgenciesPage() {
  const [advertisers, setAdvertisers] = useState<AdminAdvertiser[]>([]);
  const [registrations, setRegistrations] = useState<Registration[]>([]);
  const [loading, setLoading] = useState(true);
  const [topUpId, setTopUpId] = useState<number | null>(null);
  const [topUpAmount, setTopUpAmount] = useState("");
  const [topUpDesc, setTopUpDesc] = useState("");

  const load = () => {
    setLoading(true);
    Promise.all([
      adminApi.listAdvertisers().catch(() => []),
      adminApi.listRegistrations().catch(() => []),
    ]).then(([a, r]) => {
      setAdvertisers(a); setRegistrations(r);
    }).finally(() => setLoading(false));
  };

  useEffect(load, []);

  const handleTopUp = async () => {
    if (!topUpId || !topUpAmount) return;
    await adminApi.topUp(topUpId, parseInt(topUpAmount) * 100, topUpDesc || "admin top-up");
    setTopUpId(null); setTopUpAmount(""); setTopUpDesc("");
    load();
  };

  const handleApprove = async (id: number) => {
    await adminApi.approveRegistration(id);
    load();
  };

  const handleReject = async (id: number) => {
    const reason = prompt("拒绝原因:");
    if (reason) { await adminApi.rejectRegistration(id, reason); load(); }
  };

  if (loading) return <div><h2 className="text-2xl font-semibold mb-6">代理商管理</h2><div className="h-40 bg-gray-100 rounded-lg animate-pulse" /></div>;

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">代理商管理</h2>

      {/* Pending Registrations */}
      {registrations.length > 0 && (
        <div className="bg-white rounded-lg p-6 mb-6">
          <h3 className="text-sm font-semibold mb-4">待审核注册 ({registrations.length})</h3>
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead><tr className="text-left text-xs text-gray-500 border-b">
                <th className="pb-2">公司</th><th className="pb-2">邮箱</th><th className="pb-2">业务类型</th><th className="pb-2">申请时间</th><th className="pb-2">操作</th>
              </tr></thead>
              <tbody>
                {registrations.map(r => (
                  <tr key={r.id} className="border-b last:border-0">
                    <td className="py-3 font-medium">{r.company_name}</td>
                    <td className="py-3 text-gray-600">{r.contact_email}</td>
                    <td className="py-3 text-gray-600">{r.business_type || "-"}</td>
                    <td className="py-3 text-gray-500 text-xs">{new Date(r.created_at).toLocaleDateString()}</td>
                    <td className="py-3 flex gap-2">
                      <button onClick={() => handleApprove(r.id)} className="px-2 py-1 text-xs bg-green-50 text-green-700 rounded hover:bg-green-100">通过</button>
                      <button onClick={() => handleReject(r.id)} className="px-2 py-1 text-xs bg-red-50 text-red-700 rounded hover:bg-red-100">拒绝</button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* Advertiser List */}
      <div className="bg-white rounded-lg p-6">
        <h3 className="text-sm font-semibold mb-4">代理商列表 ({advertisers.length})</h3>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead><tr className="text-left text-xs text-gray-500 border-b">
              <th className="pb-2">ID</th><th className="pb-2">公司</th><th className="pb-2">邮箱</th><th className="pb-2">余额 (CNY)</th><th className="pb-2">注册时间</th><th className="pb-2">操作</th>
            </tr></thead>
            <tbody>
              {advertisers.map(a => (
                <tr key={a.id} className="border-b last:border-0">
                  <td className="py-3 font-geist tabular-nums">{a.id}</td>
                  <td className="py-3 font-medium">{a.company_name}</td>
                  <td className="py-3 text-gray-600">{a.contact_email}</td>
                  <td className="py-3 font-geist tabular-nums">{(a.balance_cents / 100).toFixed(2)}</td>
                  <td className="py-3 text-gray-500 text-xs">{new Date(a.created_at).toLocaleDateString()}</td>
                  <td className="py-3">
                    <button onClick={() => setTopUpId(a.id)} className="px-2 py-1 text-xs bg-blue-50 text-blue-700 rounded hover:bg-blue-100">充值</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      {/* Top-up Modal */}
      {topUpId && (
        <div className="fixed inset-0 bg-black/30 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-sm">
            <h3 className="text-sm font-semibold mb-4">手动充值 — 代理商 #{topUpId}</h3>
            <input type="number" placeholder="金额 (元)" value={topUpAmount} onChange={e => setTopUpAmount(e.target.value)}
              className="w-full px-3 py-2 border rounded-md text-sm mb-3" autoFocus />
            <input type="text" placeholder="备注 (选填)" value={topUpDesc} onChange={e => setTopUpDesc(e.target.value)}
              className="w-full px-3 py-2 border rounded-md text-sm mb-4" />
            <div className="flex gap-2">
              <button onClick={handleTopUp} className="flex-1 px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700">确认充值</button>
              <button onClick={() => setTopUpId(null)} className="flex-1 px-4 py-2 text-sm font-medium text-gray-700 bg-gray-100 rounded-md hover:bg-gray-200">取消</button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/app/admin/agencies/page.tsx
git commit -m "feat(web): add agencies management page with registration review and top-up"
```

---

### Task 5: Creative Review + Invite Codes + Audit Pages

**Files:**
- Create: `web/app/admin/creatives/page.tsx`
- Create: `web/app/admin/invites/page.tsx`
- Create: `web/app/admin/audit/page.tsx`

- [ ] **Step 1: Write creative review page**

```tsx
// web/app/admin/creatives/page.tsx
"use client";

import { useEffect, useState } from "react";
import { adminApi, AdminCreative } from "@/lib/admin-api";

export default function CreativeReviewPage() {
  const [creatives, setCreatives] = useState<AdminCreative[]>([]);
  const [loading, setLoading] = useState(true);

  const load = () => {
    setLoading(true);
    adminApi.listCreativesForReview().then(setCreatives).catch(() => setCreatives([])).finally(() => setLoading(false));
  };

  useEffect(load, []);

  const approve = async (id: number) => { await adminApi.approveCreative(id); load(); };
  const reject = async (id: number) => {
    const reason = prompt("拒绝原因:");
    if (reason) { await adminApi.rejectCreative(id, reason); load(); }
  };

  if (loading) return <div><h2 className="text-2xl font-semibold mb-6">素材审核</h2><div className="h-40 bg-gray-100 rounded-lg animate-pulse" /></div>;

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">素材审核</h2>
      {creatives.length === 0 ? (
        <div className="bg-white rounded-lg p-12 text-center">
          <p className="text-gray-500 text-sm">暂无待审核素材</p>
        </div>
      ) : (
        <div className="grid gap-4">
          {creatives.map(c => (
            <div key={c.id} className="bg-white rounded-lg p-5">
              <div className="flex items-start justify-between mb-3">
                <div>
                  <p className="font-medium text-sm">{c.name}</p>
                  <p className="text-xs text-gray-500">Campaign #{c.campaign_id} · {c.ad_type} · {c.size}</p>
                </div>
                <span className="px-2 py-0.5 text-xs font-medium rounded-full bg-yellow-50 text-yellow-700">{c.status}</span>
              </div>
              {c.ad_markup && (
                <div className="border rounded p-2 mb-3 bg-gray-50 overflow-auto max-h-40">
                  <div dangerouslySetInnerHTML={{ __html: c.ad_markup }} />
                </div>
              )}
              <div className="flex gap-2">
                <button onClick={() => approve(c.id)} className="px-3 py-1.5 text-xs font-medium bg-green-50 text-green-700 rounded hover:bg-green-100">通过</button>
                <button onClick={() => reject(c.id)} className="px-3 py-1.5 text-xs font-medium bg-red-50 text-red-700 rounded hover:bg-red-100">拒绝</button>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 2: Write invite codes page**

```tsx
// web/app/admin/invites/page.tsx
"use client";

import { useEffect, useState } from "react";
import { adminApi, InviteCode } from "@/lib/admin-api";

export default function InvitesPage() {
  const [codes, setCodes] = useState<InviteCode[]>([]);
  const [loading, setLoading] = useState(true);
  const [maxUses, setMaxUses] = useState("5");
  const [newCode, setNewCode] = useState("");

  const load = () => {
    setLoading(true);
    adminApi.listInviteCodes().then(setCodes).catch(() => setCodes([])).finally(() => setLoading(false));
  };

  useEffect(load, []);

  const create = async () => {
    const result = await adminApi.createInviteCode(parseInt(maxUses) || 1);
    setNewCode(result.code);
    load();
  };

  if (loading) return <div><h2 className="text-2xl font-semibold mb-6">邀请码管理</h2><div className="h-40 bg-gray-100 rounded-lg animate-pulse" /></div>;

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">邀请码管理</h2>

      {/* Create */}
      <div className="bg-white rounded-lg p-6 mb-6">
        <h3 className="text-sm font-semibold mb-4">生成邀请码</h3>
        <div className="flex gap-3 items-end">
          <div>
            <label className="text-xs text-gray-500 mb-1 block">最大使用次数</label>
            <input type="number" value={maxUses} onChange={e => setMaxUses(e.target.value)}
              className="px-3 py-2 border rounded-md text-sm w-24" />
          </div>
          <button onClick={create} className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700">生成</button>
        </div>
        {newCode && (
          <div className="mt-3 px-3 py-2 bg-green-50 text-green-800 rounded text-sm font-mono">
            新邀请码: {newCode}
          </div>
        )}
      </div>

      {/* List */}
      <div className="bg-white rounded-lg p-6">
        <h3 className="text-sm font-semibold mb-4">已有邀请码 ({codes.length})</h3>
        <table className="w-full text-sm">
          <thead><tr className="text-left text-xs text-gray-500 border-b">
            <th className="pb-2">邀请码</th><th className="pb-2">已用/上限</th><th className="pb-2">创建人</th><th className="pb-2">创建时间</th><th className="pb-2">过期时间</th>
          </tr></thead>
          <tbody>
            {codes.map(c => (
              <tr key={c.id} className="border-b last:border-0">
                <td className="py-3 font-mono text-xs">{c.code}</td>
                <td className="py-3 font-geist tabular-nums">{c.used_count}/{c.max_uses}</td>
                <td className="py-3 text-gray-600">{c.created_by}</td>
                <td className="py-3 text-gray-500 text-xs">{new Date(c.created_at).toLocaleDateString()}</td>
                <td className="py-3 text-gray-500 text-xs">{c.expires_at ? new Date(c.expires_at).toLocaleDateString() : "永不过期"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Write audit log page**

```tsx
// web/app/admin/audit/page.tsx
"use client";

import { useEffect, useState } from "react";
import { adminApi, AuditEntry } from "@/lib/admin-api";

const actionLabels: Record<string, string> = {
  "campaign.create": "创建 Campaign",
  "campaign.update": "更新 Campaign",
  "campaign.start": "启动 Campaign",
  "campaign.pause": "暂停 Campaign",
  "creative.create": "创建素材",
  "creative.approve": "审核通过素材",
  "creative.reject": "拒绝素材",
  "creative.delete": "删除素材",
  "billing.topup": "充值",
  "registration.approve": "通过注册",
  "registration.reject": "拒绝注册",
};

export default function AuditPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [offset, setOffset] = useState(0);
  const limit = 50;

  const load = (off: number) => {
    setLoading(true);
    adminApi.getAuditLog(limit, off).then(setEntries).catch(() => setEntries([])).finally(() => setLoading(false));
  };

  useEffect(() => load(offset), [offset]);

  if (loading) return <div><h2 className="text-2xl font-semibold mb-6">审计日志</h2><div className="h-40 bg-gray-100 rounded-lg animate-pulse" /></div>;

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">审计日志</h2>
      <div className="bg-white rounded-lg p-6">
        {entries.length === 0 ? (
          <p className="text-gray-500 text-sm text-center py-8">暂无记录</p>
        ) : (
          <>
            <table className="w-full text-sm">
              <thead><tr className="text-left text-xs text-gray-500 border-b">
                <th className="pb-2">时间</th><th className="pb-2">操作</th><th className="pb-2">操作人</th><th className="pb-2">广告主</th><th className="pb-2">资源</th><th className="pb-2">详情</th>
              </tr></thead>
              <tbody>
                {entries.map(e => (
                  <tr key={e.id} className="border-b last:border-0">
                    <td className="py-3 text-xs text-gray-500 whitespace-nowrap">{new Date(e.created_at).toLocaleString()}</td>
                    <td className="py-3"><span className="px-2 py-0.5 text-xs rounded-full bg-gray-100 text-gray-700">{actionLabels[e.action] || e.action}</span></td>
                    <td className="py-3 text-gray-600">{e.actor}</td>
                    <td className="py-3 font-geist tabular-nums">{e.advertiser_id || "-"}</td>
                    <td className="py-3 text-xs text-gray-500">{e.resource_type} #{e.resource_id}</td>
                    <td className="py-3 text-xs text-gray-400 max-w-48 truncate">{e.details ? JSON.stringify(e.details) : "-"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            <div className="flex justify-between mt-4">
              <button onClick={() => setOffset(Math.max(0, offset - limit))} disabled={offset === 0}
                className="px-3 py-1 text-xs text-gray-600 bg-gray-100 rounded disabled:opacity-50">上一页</button>
              <button onClick={() => setOffset(offset + limit)} disabled={entries.length < limit}
                className="px-3 py-1 text-xs text-gray-600 bg-gray-100 rounded disabled:opacity-50">下一页</button>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add web/app/admin/creatives/ web/app/admin/invites/ web/app/admin/audit/
git commit -m "feat(web): add creative review, invite codes, and audit log admin pages"
```

---

### Task 6: Smoke Test

- [ ] **Step 1: Verify Next.js build**

Run: `cd /c/Users/Roc/github/dsp/web && npm run build 2>&1 | tail -20`

If build fails, check for TypeScript errors and fix.

- [ ] **Step 2: Verify dev server starts**

Run: `cd /c/Users/Roc/github/dsp/web && timeout 10 npm run dev 2>&1 | head -10`

Should show Next.js starting on port 4000.

- [ ] **Step 3: Commit final state if needed**

```bash
git add -A && git commit -m "chore: Phase 3B frontend smoke test"
```

---

## Task Dependency Graph

```
Task 1 (admin-api.ts) ──────────┐
Task 2 (layout + token gate) ───┤
                                 ├── Task 3 (overview) ──── Task 6 (smoke)
                                 ├── Task 4 (agencies) ────┤
                                 └── Task 5 (3 pages) ─────┘
```

Tasks 1+2 first (shared dependencies), then 3/4/5 in any order, then 6.
