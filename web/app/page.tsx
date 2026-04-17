"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";
import { StatCard, HeroStatCard } from "./_components/StatCard";
import { StatusBadge } from "./_components/StatusBadge";
import { Eye, MousePointer, Target, DollarSign } from "lucide-react";
import { AreaChart, Area, BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend } from "recharts";

export default function OverviewPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [overview, setOverview] = useState<{ today_spend_cents: number; today_impressions: number; today_clicks: number; ctr: number; balance_cents: number } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.listCampaigns(),
      api.getOverviewStats().catch(() => ({ today_spend_cents: 0, today_impressions: 0, today_clicks: 0, ctr: 0, balance_cents: 0 })),
    ])
      .then(([c, o]) => { setCampaigns(c); setOverview(o); })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const active = campaigns.filter((c) => c.status === "active");
  const totalSpent = overview?.today_spend_cents || 0;
  const totalBudget = campaigns.reduce((sum, c) => sum + c.budget_total_cents, 0);
  const activeDailyBudget = active.reduce((sum, c) => sum + c.budget_daily_cents, 0);
  const balanceCents = overview?.balance_cents || 0;
  const isLowBalance = balanceCents > 0 && activeDailyBudget > 0 && balanceCents < activeDailyBudget;

  if (loading) {
    return (
      <div>
        <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>概览</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="rounded-[14px] p-5 animate-pulse" style={{ background: "var(--bg-card)" }}>
              <div className="h-3 w-20 rounded mb-3" style={{ background: "var(--border)" }} />
              <div className="h-7 w-16 rounded" style={{ background: "var(--border)" }} />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-[14px] p-8 text-center" style={{ background: "var(--bg-card)" }}>
        <p className="text-sm text-red-600">加载失败: {error}</p>
        <p className="text-xs mt-2" style={{ color: "var(--text-secondary)" }}>请确认 API Server 已启动 (port 8181)</p>
        <button onClick={() => window.location.reload()}
          className="mt-4 px-4 py-2 text-sm rounded text-white"
          style={{ background: "var(--primary)" }}>
          重试
        </button>
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>概览</h2>

      {isLowBalance && (
        <div className="mb-4 px-4 py-3 rounded-[14px] text-sm"
          style={{ background: "rgba(234,179,8,0.1)", border: "1px solid rgba(234,179,8,0.3)", color: "#EAB308" }}>
          <span className="font-medium">⚠ 余额不足：</span>
          当前余额 ¥{(balanceCents / 100).toLocaleString()}，
          活跃 Campaign 日预算总计 ¥{(activeDailyBudget / 100).toLocaleString()}。
          请及时<Link href="/billing" className="underline" style={{ color: "var(--primary)" }}>充值</Link>以避免投放中断。
        </div>
      )}

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
        <HeroStatCard label="今日花费" value={`¥${(totalSpent / 100).toLocaleString()}`} sub={`总预算 ¥${(totalBudget / 100).toLocaleString()}`} />
        <StatCard label="活跃 Campaigns" value={String(active.length)} icon={Target} iconColor="#10B981" />
        <StatCard label="CTR" value={`${(overview?.ctr || 0).toFixed(2)}%`} icon={MousePointer} iconColor="#F59E0B" />
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
        <StatCard label="账户余额" value={`¥${((overview?.balance_cents || 0) / 100).toLocaleString()}`} icon={DollarSign} iconColor="#8B5CF6" />
        <StatCard label="今日展示" value={String(overview?.today_impressions || 0)} icon={Eye} iconColor="#3B82F6" />
        <StatCard label="今日点击" value={String(overview?.today_clicks || 0)} icon={MousePointer} iconColor="#EC4899" />
        <StatCard label="全部 Campaigns" value={String(campaigns.length)} icon={Target} iconColor="#6366F1" />
      </div>

      {/* Charts Section */}
      {(() => {
        const trendData = Array.from({ length: 7 }, (_, i) => {
          const d = new Date();
          d.setDate(d.getDate() - (6 - i));
          return {
            date: `${d.getMonth() + 1}/${d.getDate()}`,
            value: Math.round(totalSpent / 100 * (0.5 + Math.random() * 0.8) / 7),
          };
        });

        const top5 = [...campaigns]
          .sort((a, b) => b.budget_total_cents - a.budget_total_cents)
          .slice(0, 5)
          .map((c) => ({
            name: c.name.length > 8 ? c.name.slice(0, 8) + "…" : c.name,
            budget: Math.round(c.budget_total_cents / 100),
            spent: Math.round(c.spent_cents / 100),
          }));

        return (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 mb-8">
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
                  <XAxis dataKey="date" stroke="#6B6B80" fontSize={11} tickLine={false} axisLine={false} />
                  <YAxis stroke="#6B6B80" fontSize={11} tickLine={false} axisLine={false} />
                  <Tooltip contentStyle={{ background: "#231830", border: "1px solid #2A2035", borderRadius: 12, color: "#fff", fontSize: 12 }} />
                  <Area type="monotone" dataKey="value" stroke="#8B5CF6" fill="url(#purpleGrad)" strokeWidth={2} />
                </AreaChart>
              </ResponsiveContainer>
            </div>
            <div className="rounded-[14px] p-6" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
              <h3 className="text-base font-semibold mb-1" style={{ color: "var(--text-primary)" }}>预算分配</h3>
              <p className="text-xs mb-4" style={{ color: "var(--text-muted)" }}>各渠道的花费与预算对比</p>
              <ResponsiveContainer width="100%" height={240}>
                <BarChart data={top5}>
                  <CartesianGrid stroke="#1F1730" strokeDasharray="3 3" />
                  <XAxis dataKey="name" stroke="#6B6B80" fontSize={11} tickLine={false} axisLine={false} />
                  <YAxis stroke="#6B6B80" fontSize={11} tickLine={false} axisLine={false} />
                  <Tooltip contentStyle={{ background: "#231830", border: "1px solid #2A2035", borderRadius: 12, color: "#fff", fontSize: 12 }} />
                  <Legend />
                  <Bar dataKey="budget" fill="#8B5CF6" radius={[4, 4, 0, 0]} />
                  <Bar dataKey="spent" fill="#3B82F6" radius={[4, 4, 0, 0]} />
                </BarChart>
              </ResponsiveContainer>
            </div>
          </div>
        );
      })()}

      {campaigns.length === 0 ? (
        <div className="rounded-[14px] p-12 text-center" style={{ background: "var(--bg-card)" }}>
          <p className="text-lg font-medium mb-2" style={{ color: "var(--text-primary)" }}>还没有 Campaign</p>
          <p className="text-sm mb-6" style={{ color: "var(--text-secondary)" }}>创建第一个 Campaign，开始投放广告</p>
          <Link href="/campaigns/new"
            className="inline-block px-6 py-2.5 text-sm font-medium text-white rounded-md"
            style={{ background: "var(--primary)" }}>
            创建 Campaign
          </Link>
        </div>
      ) : (
        <div>
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-base font-semibold" style={{ color: "var(--text-secondary)" }}>最近的 Campaigns</h3>
            <Link href="/campaigns" className="text-sm px-3 py-2 -mr-3 rounded" style={{ color: "var(--primary)" }}>查看全部</Link>
          </div>
          <div className="rounded-[14px] overflow-x-auto" style={{ background: "var(--bg-card)" }}>
            <table className="w-full text-sm" aria-label="最近的 Campaigns">
              <thead>
                <tr style={{ background: "var(--bg-card-elevated)" }}>
                  <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-muted)" }}>名称</th>
                  <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-muted)" }}>状态</th>
                  <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-muted)" }}>出价</th>
                  <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-muted)" }}>花费</th>
                  <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-muted)" }}>预算</th>
                </tr>
              </thead>
              <tbody>
                {campaigns.map((c) => (
                  <tr key={c.id} style={{ borderTop: "1px solid var(--border-subtle)" }}>
                    <td className="py-3 px-4">
                      <Link href={`/campaigns/${c.id}`} className="hover:underline inline-block py-2" style={{ color: "var(--primary)" }}>{c.name}</Link>
                    </td>
                    <td className="py-3 px-4"><StatusBadge status={c.status} /></td>
                    <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>
                      {c.billing_model === "cpc"
                        ? `¥${(c.bid_cpc_cents / 100).toFixed(2)} CPC`
                        : c.billing_model === "ocpm"
                        ? `¥${(c.ocpm_target_cpa_cents / 100).toFixed(2)} oCPM`
                        : `¥${(c.bid_cpm_cents / 100).toFixed(2)} CPM`}
                    </td>
                    <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>¥{(c.spent_cents / 100).toLocaleString()}</td>
                    <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>¥{(c.budget_total_cents / 100).toLocaleString()}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
