"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";

const ADVERTISER_ID = 1;

export default function OverviewPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.listCampaigns(ADVERTISER_ID)
      .then(setCampaigns)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  const active = campaigns.filter((c) => c.status === "active");
  const totalSpent = campaigns.reduce((sum, c) => sum + c.spent_cents, 0);
  const totalBudget = campaigns.reduce((sum, c) => sum + c.budget_total_cents, 0);

  if (loading) {
    return (
      <div>
        <h2 className="text-xl font-semibold mb-6">概览</h2>
        <div className="grid grid-cols-4 gap-4 mb-8">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="rounded-lg border border-gray-200 bg-white p-5 animate-pulse">
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
      <div className="rounded-lg border border-gray-200 bg-white p-8 text-center">
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
      <h2 className="text-xl font-semibold mb-6">概览</h2>

      <div className="grid grid-cols-4 gap-4 mb-8">
        <StatCard label="活跃 Campaigns" value={String(active.length)} />
        <StatCard label="今日花费" value={`¥${(totalSpent / 100).toLocaleString()}`} />
        <StatCard label="总预算" value={`¥${(totalBudget / 100).toLocaleString()}`} />
        <StatCard label="全部 Campaigns" value={String(campaigns.length)} />
      </div>

      {campaigns.length === 0 ? (
        <div className="rounded-lg border border-gray-200 bg-white p-12 text-center">
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
            <h3 className="text-sm font-medium text-gray-500">最近的 Campaigns</h3>
            <Link href="/campaigns" className="text-sm text-blue-600">查看全部</Link>
          </div>
          <div className="rounded-lg border border-gray-200 bg-white overflow-hidden">
            <table className="w-full text-sm">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">名称</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">状态</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">CPM</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">花费</th>
                  <th className="text-right py-3 px-4 font-medium text-gray-500">预算</th>
                </tr>
              </thead>
              <tbody>
                {campaigns.slice(0, 5).map((c) => (
                  <tr key={c.id} className="border-t border-gray-100 hover:bg-gray-50">
                    <td className="py-3 px-4">
                      <Link href={`/campaigns/${c.id}`} className="text-blue-600 hover:underline">{c.name}</Link>
                    </td>
                    <td className="py-3 px-4"><StatusBadge status={c.status} /></td>
                    <td className="py-3 px-4 text-right font-mono">¥{(c.bid_cpm_cents / 100).toFixed(2)}</td>
                    <td className="py-3 px-4 text-right font-mono">¥{(c.spent_cents / 100).toLocaleString()}</td>
                    <td className="py-3 px-4 text-right font-mono">¥{(c.budget_total_cents / 100).toLocaleString()}</td>
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

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-white p-5">
      <p className="text-xs font-medium mb-1 text-gray-500">{label}</p>
      <p className="text-2xl font-semibold">{value}</p>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const styles: Record<string, string> = {
    draft: "bg-gray-100 text-gray-600",
    active: "bg-green-50 text-green-700",
    paused: "bg-yellow-50 text-yellow-700",
    completed: "bg-blue-50 text-blue-700",
  };
  return (
    <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${styles[status] || styles.draft}`}>
      {status}
    </span>
  );
}
