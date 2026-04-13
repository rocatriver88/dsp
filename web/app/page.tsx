"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";
import { StatCard, HeroStatCard } from "./_components/StatCard";
import { StatusBadge } from "./_components/StatusBadge";

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
        <h2 className="text-2xl font-semibold mb-6">概览</h2>
        <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="rounded-lg bg-white p-5 animate-pulse">
              <div className="h-3 w-20 bg-gray-200 rounded mb-3" />
              <div className="h-7 w-16 bg-gray-200 rounded" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg bg-white p-8 text-center">
        <p className="text-sm text-red-600">加载失败: {error}</p>
        <p className="text-xs mt-2 text-gray-500">请确认 API Server 已启动 (port 8181)</p>
        <button onClick={() => window.location.reload()}
          className="mt-4 px-4 py-2 text-sm rounded text-white bg-blue-600 hover:bg-blue-700">
          重试
        </button>
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">概览</h2>

      {isLowBalance && (
        <div className="mb-4 px-4 py-3 rounded-lg bg-yellow-50 border border-yellow-200 text-sm text-yellow-800">
          <span className="font-medium">⚠ 余额不足：</span>
          当前余额 ¥{(balanceCents / 100).toLocaleString()}，
          活跃 Campaign 日预算总计 ¥{(activeDailyBudget / 100).toLocaleString()}。
          请及时<Link href="/billing" className="text-blue-600 underline hover:text-blue-700">充值</Link>以避免投放中断。
        </div>
      )}

      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
        <HeroStatCard label="今日花费" value={`¥${(totalSpent / 100).toLocaleString()}`} sub={`总预算 ¥${(totalBudget / 100).toLocaleString()}`} />
        <StatCard label="活跃 Campaigns" value={String(active.length)} />
        <StatCard label="CTR" value={`${(overview?.ctr || 0).toFixed(2)}%`} />
      </div>
      <div className="grid grid-cols-2 md:grid-cols-4 gap-3 md:gap-4 mb-8">
        <StatCard label="账户余额" value={`¥${((overview?.balance_cents || 0) / 100).toLocaleString()}`} />
        <StatCard label="今日展示" value={String(overview?.today_impressions || 0)} />
        <StatCard label="今日点击" value={String(overview?.today_clicks || 0)} />
        <StatCard label="全部 Campaigns" value={String(campaigns.length)} />
      </div>

      {campaigns.length === 0 ? (
        <div className="rounded-lg bg-white p-12 text-center">
          <p className="text-lg font-medium mb-2">还没有 Campaign</p>
          <p className="text-sm mb-6 text-gray-500">创建第一个 Campaign，开始投放广告</p>
          <Link href="/campaigns/new"
            className="inline-block px-6 py-2.5 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
            创建 Campaign
          </Link>
        </div>
      ) : (
        <div>
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-base font-semibold text-gray-700">最近的 Campaigns</h3>
            <Link href="/campaigns" className="text-sm text-blue-600 px-3 py-2 -mr-3 rounded hover:bg-blue-50">查看全部</Link>
          </div>
          <div className="rounded-lg bg-white overflow-x-auto">
            <table className="w-full text-sm" aria-label="最近的 Campaigns">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">名称</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">状态</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">出价</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">花费</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">预算</th>
                </tr>
              </thead>
              <tbody>
                {campaigns.map((c) => (
                  <tr key={c.id} className="border-t border-gray-100 hover:bg-gray-50">
                    <td className="py-3 px-4">
                      <Link href={`/campaigns/${c.id}`} className="text-blue-600 hover:underline inline-block py-2">{c.name}</Link>
                    </td>
                    <td className="py-3 px-4"><StatusBadge status={c.status} /></td>
                    <td className="py-3 px-4 text-right font-geist tabular-nums">
                      {c.billing_model === "cpc"
                        ? `¥${(c.bid_cpc_cents / 100).toFixed(2)} CPC`
                        : c.billing_model === "ocpm"
                        ? `¥${(c.ocpm_target_cpa_cents / 100).toFixed(2)} oCPM`
                        : `¥${(c.bid_cpm_cents / 100).toFixed(2)} CPM`}
                    </td>
                    <td className="py-3 px-4 text-right font-geist tabular-nums">¥{(c.spent_cents / 100).toLocaleString()}</td>
                    <td className="py-3 px-4 text-right font-geist tabular-nums">¥{(c.budget_total_cents / 100).toLocaleString()}</td>
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

