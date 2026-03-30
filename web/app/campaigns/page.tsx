"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";
import { LoadingSkeleton, ErrorState } from "../components/LoadingState";
import { StatusBadge } from "../components/StatusBadge";

export default function CampaignsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = () => {
    setLoading(true);
    setError(null);
    api.listCampaigns()
      .then(setCampaigns)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  const handleAction = async (id: number, action: "start" | "pause") => {
    setActionError(null);
    try {
      if (action === "start") await api.startCampaign(id);
      else await api.pauseCampaign(id);
      load();
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    }
  };

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-semibold">Campaigns</h2>
        <Link href="/campaigns/new"
          className="px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
          创建 Campaign
        </Link>
      </div>

      {actionError && (
        <div className="mb-4 p-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} className="text-red-500 hover:text-red-700 text-xs ml-4">关闭</button>
        </div>
      )}

      {loading ? (
        <LoadingSkeleton rows={5} />
      ) : error ? (
        <ErrorState message={error} onRetry={load} />
      ) : campaigns.length === 0 ? (
        <div className="rounded-lg bg-white p-12 text-center">
          <p className="text-lg font-medium mb-2">还没有 Campaign</p>
          <p className="text-sm mb-6 text-gray-500">创建第一个 Campaign，开始投放广告</p>
          <Link href="/campaigns/new"
            className="inline-block px-6 py-2.5 text-sm font-medium text-white rounded-md bg-blue-600">
            创建 Campaign
          </Link>
        </div>
      ) : (
        <div className="rounded-lg bg-white overflow-hidden">
          <table className="w-full text-sm" role="table" aria-label="Campaign 列表">
            <thead className="bg-gray-50">
              <tr>
                <th className="text-left py-3 px-4 font-medium text-gray-500">ID</th>
                <th className="text-left py-3 px-4 font-medium text-gray-500">名称</th>
                <th className="text-left py-3 px-4 font-medium text-gray-500">状态</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">出价</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">日预算 (¥)</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">已花费 (¥)</th>
                <th className="text-center py-3 px-4 font-medium text-gray-500">操作</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((c) => (
                <tr key={c.id} className="border-t border-gray-100 hover:bg-gray-50">
                  <td className="py-3 px-4 font-geist tabular-nums text-gray-400">{c.id}</td>
                  <td className="py-3 px-4">
                    <Link href={`/campaigns/${c.id}`} className="text-blue-600 hover:underline">{c.name}</Link>
                  </td>
                  <td className="py-3 px-4">
                    <StatusBadge status={c.status} />
                  </td>
                  <td className="py-3 px-4 text-right font-geist tabular-nums">
                    {c.billing_model === "cpc"
                      ? `¥${(c.bid_cpc_cents / 100).toFixed(2)} CPC`
                      : c.billing_model === "ocpm"
                      ? `¥${(c.ocpm_target_cpa_cents / 100).toFixed(2)} oCPM`
                      : `¥${(c.bid_cpm_cents / 100).toFixed(2)} CPM`}
                  </td>
                  <td className="py-3 px-4 text-right font-geist tabular-nums">{(c.budget_daily_cents / 100).toLocaleString()}</td>
                  <td className="py-3 px-4 text-right font-geist tabular-nums">{(c.spent_cents / 100).toLocaleString()}</td>
                  <td className="py-3 px-4 text-center">
                    {c.status === "draft" && (
                      <button onClick={() => handleAction(c.id, "start")}
                        aria-label={`启动 ${c.name}`}
                        className="text-xs px-3 py-1 rounded bg-green-50 text-green-700 hover:bg-green-100">
                        启动
                      </button>
                    )}
                    {c.status === "active" && (
                      <button onClick={() => handleAction(c.id, "pause")}
                        aria-label={`暂停 ${c.name}`}
                        className="text-xs px-3 py-1 rounded bg-yellow-50 text-yellow-700 hover:bg-yellow-100">
                        暂停
                      </button>
                    )}
                    {c.status === "paused" && (
                      <button onClick={() => handleAction(c.id, "start")}
                        aria-label={`恢复 ${c.name}`}
                        className="text-xs px-3 py-1 rounded bg-green-50 text-green-700 hover:bg-green-100">
                        恢复
                      </button>
                    )}
                    {c.status === "completed" && (
                      <span className="text-xs text-gray-400">已完成</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

