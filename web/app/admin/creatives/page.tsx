"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, AdminCreative } from "@/lib/admin-api";

function CreativeCard({
  creative,
  onApprove,
  onReject,
  actionLoading,
}: {
  creative: AdminCreative;
  onApprove: (id: number) => void;
  onReject: (id: number) => void;
  actionLoading: boolean;
}) {
  return (
    <div className="bg-white rounded-lg p-5">
      <div className="flex items-start justify-between gap-4 mb-3">
        <div>
          <p className="text-sm font-semibold text-gray-900">{creative.name}</p>
          <div className="flex items-center gap-3 mt-1">
            <span className="text-xs text-gray-500">Campaign #{creative.campaign_id}</span>
            <span className="text-xs text-gray-400">·</span>
            <span className="text-xs text-gray-500">广告主 #{creative.advertiser_id}</span>
          </div>
        </div>
        <div className="flex items-center gap-2 flex-shrink-0">
          <button
            onClick={() => onApprove(creative.id)}
            disabled={actionLoading}
            className="px-3 py-1.5 text-xs font-medium rounded bg-green-50 text-green-700 hover:bg-green-100 disabled:opacity-50 transition-colors"
          >
            批准
          </button>
          <button
            onClick={() => onReject(creative.id)}
            disabled={actionLoading}
            className="px-3 py-1.5 text-xs font-medium rounded bg-red-50 text-red-700 hover:bg-red-100 disabled:opacity-50 transition-colors"
          >
            拒绝
          </button>
        </div>
      </div>

      <div className="flex items-center gap-4 mb-3">
        <div>
          <p className="text-xs text-gray-400">类型</p>
          <p className="text-xs font-medium text-gray-700 mt-0.5">{creative.ad_type}</p>
        </div>
        {creative.size && (
          <div>
            <p className="text-xs text-gray-400">尺寸</p>
            <p className="text-xs font-medium text-gray-700 mt-0.5">{creative.size}</p>
          </div>
        )}
        {creative.format && (
          <div>
            <p className="text-xs text-gray-400">格式</p>
            <p className="text-xs font-medium text-gray-700 mt-0.5">{creative.format}</p>
          </div>
        )}
        <div>
          <p className="text-xs text-gray-400">提交时间</p>
          <p className="text-xs font-medium text-gray-700 mt-0.5 font-geist tabular-nums">
            {new Date(creative.created_at).toLocaleString("zh-CN")}
          </p>
        </div>
      </div>

      {creative.ad_markup && (
        <div>
          <p className="text-xs text-gray-400 mb-1">广告代码预览</p>
          <pre className="text-xs bg-gray-50 rounded p-3 overflow-x-auto max-h-32 text-gray-700 font-mono whitespace-pre-wrap break-all">
            {creative.ad_markup.slice(0, 500)}{creative.ad_markup.length > 500 ? "…" : ""}
          </pre>
        </div>
      )}

      {creative.destination_url && (
        <div className="mt-2">
          <p className="text-xs text-gray-400">落地页</p>
          <a
            href={creative.destination_url}
            target="_blank"
            rel="noopener noreferrer"
            className="text-xs text-blue-600 hover:underline break-all"
          >
            {creative.destination_url}
          </a>
        </div>
      )}
    </div>
  );
}

export default function CreativesPage() {
  const [creatives, setCreatives] = useState<AdminCreative[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    adminApi
      .listCreativesForReview()
      .then((data) => setCreatives(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleApprove(id: number) {
    setActionLoading(id);
    setActionError(null);
    try {
      await adminApi.approveCreative(id);
      setCreatives((prev) => prev.filter((c) => c.id !== id));
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleReject(id: number) {
    const reason = prompt("拒绝原因:");
    if (reason === null) return;
    setActionLoading(id);
    setActionError(null);
    try {
      await adminApi.rejectCreative(id, reason);
      setCreatives((prev) => prev.filter((c) => c.id !== id));
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    } finally {
      setActionLoading(null);
    }
  }

  return (
    <div className="p-8 max-w-6xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-semibold">素材审核</h2>
        {!loading && creatives.length > 0 && (
          <span className="px-2.5 py-1 text-xs font-medium rounded-full bg-yellow-50 text-yellow-700">
            {creatives.length} 待审核
          </span>
        )}
      </div>

      {error && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {actionError && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm">
          {actionError}
        </div>
      )}

      {loading ? (
        <div className="space-y-3">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="bg-white rounded-lg p-5 animate-pulse">
              <div className="h-4 bg-gray-100 rounded w-1/3 mb-3" />
              <div className="h-3 bg-gray-100 rounded w-1/2 mb-4" />
              <div className="h-20 bg-gray-100 rounded" />
            </div>
          ))}
        </div>
      ) : creatives.length === 0 ? (
        <div className="bg-white rounded-lg p-16 text-center">
          <p className="text-sm text-gray-500">暂无待审核素材</p>
        </div>
      ) : (
        <div className="space-y-3">
          {creatives.map((creative) => (
            <CreativeCard
              key={creative.id}
              creative={creative}
              onApprove={handleApprove}
              onReject={handleReject}
              actionLoading={actionLoading === creative.id}
            />
          ))}
        </div>
      )}
    </div>
  );
}
