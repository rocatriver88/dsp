"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, AuditEntry } from "@/lib/admin-api";

const PAGE_SIZE = 50;

const ACTION_LABELS: Record<string, string> = {
  "campaign.create": "创建 Campaign",
  "campaign.update": "更新 Campaign",
  "campaign.delete": "删除 Campaign",
  "campaign.pause": "暂停 Campaign",
  "campaign.resume": "恢复 Campaign",
  "billing.topup": "充值",
  "billing.spend": "消费",
  "creative.approve": "批准素材",
  "creative.reject": "拒绝素材",
  "creative.create": "创建素材",
  "advertiser.create": "创建广告主",
  "advertiser.update": "更新广告主",
  "registration.approve": "批准注册",
  "registration.reject": "拒绝注册",
  "invite.create": "创建邀请码",
  "circuit.trip": "手动熔断",
  "circuit.reset": "重置熔断",
};

function actionLabel(action: string): string {
  return ACTION_LABELS[action] ?? action;
}

function formatDetails(details: Record<string, unknown>): string {
  if (!details || Object.keys(details).length === 0) return "—";
  try {
    return Object.entries(details)
      .map(([k, v]) => `${k}: ${JSON.stringify(v)}`)
      .join(", ");
  } catch {
    return "—";
  }
}

export default function AuditPage() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [page, setPage] = useState(0);
  const [hasMore, setHasMore] = useState(true);

  const load = useCallback(
    (pageNum: number) => {
      setLoading(true);
      setError(null);
      adminApi
        .getAuditLog(PAGE_SIZE, pageNum * PAGE_SIZE)
        .then((data) => {
          const rows = Array.isArray(data) ? data : [];
          setEntries(rows);
          setHasMore(rows.length === PAGE_SIZE);
        })
        .catch((e) => setError(e.message))
        .finally(() => setLoading(false));
    },
    []
  );

  useEffect(() => {
    load(page);
  }, [load, page]);

  function handlePrev() {
    if (page > 0) setPage((p) => p - 1);
  }

  function handleNext() {
    if (hasMore) setPage((p) => p + 1);
  }

  return (
    <div className="p-8 max-w-6xl">
      <h2 className="text-2xl font-semibold mb-6">审计日志</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => load(page)} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {loading ? (
        <div className="bg-white rounded-lg p-6 animate-pulse space-y-3">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="h-10 bg-gray-100 rounded" />
          ))}
        </div>
      ) : entries.length === 0 && page === 0 ? (
        <div className="bg-white rounded-lg p-16 text-center">
          <p className="text-sm text-gray-500">暂无审计记录</p>
        </div>
      ) : (
        <>
          <div className="bg-white rounded-lg overflow-hidden mb-4">
            <table className="w-full text-sm" aria-label="审计日志">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">时间</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">操作</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">操作人</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">广告主 ID</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">资源</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">详情</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id} className="border-b last:border-0 border-gray-100">
                    <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums whitespace-nowrap">
                      {new Date(entry.created_at).toLocaleString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <span className="text-sm text-gray-900">{actionLabel(entry.action)}</span>
                      <span className="ml-2 text-xs text-gray-400 font-mono">{entry.action}</span>
                    </td>
                    <td className="py-3 px-4 text-sm text-gray-700">{entry.actor || "—"}</td>
                    <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {entry.advertiser_id ? `#${entry.advertiser_id}` : "—"}
                    </td>
                    <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {entry.resource_type ? (
                        <span>
                          {entry.resource_type}
                          {entry.resource_id ? ` #${entry.resource_id}` : ""}
                        </span>
                      ) : (
                        "—"
                      )}
                    </td>
                    <td className="py-3 px-4 text-xs text-gray-500 max-w-xs truncate" title={formatDetails(entry.details)}>
                      {formatDetails(entry.details)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between">
            <span className="text-xs text-gray-500">
              第 {page + 1} 页 · 每页 {PAGE_SIZE} 条
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={handlePrev}
                disabled={page === 0 || loading}
                className="px-3 py-1.5 text-xs font-medium rounded bg-gray-100 text-gray-700 hover:bg-gray-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                上一页
              </button>
              <button
                onClick={handleNext}
                disabled={!hasMore || loading}
                className="px-3 py-1.5 text-xs font-medium rounded bg-gray-100 text-gray-700 hover:bg-gray-200 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
              >
                下一页
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
