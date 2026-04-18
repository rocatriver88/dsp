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
  if (!details || Object.keys(details).length === 0) return "\u2014";
  try {
    return Object.entries(details)
      .map(([k, v]) => `${k}: ${JSON.stringify(v)}`)
      .join(", ");
  } catch {
    return "\u2014";
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
      adminApi
        .getAuditLog(PAGE_SIZE, pageNum * PAGE_SIZE)
        .then((data) => {
          const rows = Array.isArray(data) ? data : [];
          setEntries(rows);
          setHasMore(rows.length === PAGE_SIZE);
          setError(null);
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
    if (page > 0) {
      setLoading(true);
      setError(null);
      setPage((p) => p - 1);
    }
  }

  function handleNext() {
    if (hasMore) {
      setLoading(true);
      setError(null);
      setPage((p) => p + 1);
    }
  }

  function handleRetry() {
    setLoading(true);
    setError(null);
    load(page);
  }

  return (
    <div className="">
      <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>审计日志</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded text-sm flex items-center justify-between" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>
          <span>{error}</span>
          <button onClick={handleRetry} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {loading ? (
        <div className="glass-card-static p-6 animate-pulse space-y-3">
          {[...Array(6)].map((_, i) => (
            <div key={i} className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
          ))}
        </div>
      ) : entries.length === 0 && page === 0 ? (
        <div className="glass-card-static p-16 text-center">
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无审计记录</p>
        </div>
      ) : (
        <>
          <div className="glass-card-static p-0 overflow-hidden mb-4">
            <table className="w-full text-sm" aria-label="审计日志">
              <thead style={{ background: "var(--bg-card-elevated)" }}>
                <tr>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>时间</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>操作</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>操作人</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>广告主 ID</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>资源</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>详情</th>
                </tr>
              </thead>
              <tbody>
                {entries.map((entry) => (
                  <tr key={entry.id} className="transition-colors" style={{ borderTop: "1px solid var(--border-subtle)" }} onMouseEnter={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }} onMouseLeave={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "transparent"; }}>
                    <td className="py-3 px-4 text-xs tabular-nums whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                      {new Date(entry.created_at).toLocaleString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <span className="text-sm" style={{ color: "var(--text-primary)" }}>{actionLabel(entry.action)}</span>
                      <span className="ml-2 text-xs font-mono" style={{ color: "var(--text-muted)" }}>{entry.action}</span>
                    </td>
                    <td className="py-3 px-4 text-sm" style={{ color: "var(--text-primary)" }}>{entry.actor || "\u2014"}</td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {entry.advertiser_id ? `#${entry.advertiser_id}` : "\u2014"}
                    </td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {entry.resource_type ? (
                        <span>
                          {entry.resource_type}
                          {entry.resource_id ? ` #${entry.resource_id}` : ""}
                        </span>
                      ) : (
                        "\u2014"
                      )}
                    </td>
                    <td className="py-3 px-4 text-xs max-w-xs truncate" style={{ color: "var(--text-muted)" }} title={formatDetails(entry.details)}>
                      {formatDetails(entry.details)}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between">
            <span className="text-xs" style={{ color: "var(--text-muted)" }}>
              第 {page + 1} 页 · 每页 {PAGE_SIZE} 条
            </span>
            <div className="flex items-center gap-2">
              <button
                onClick={handlePrev}
                disabled={page === 0 || loading}
                className="px-3 py-1.5 text-xs font-medium rounded disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                style={{ background: "var(--bg-card)", color: "var(--text-primary)" }}
              >
                上一页
              </button>
              <button
                onClick={handleNext}
                disabled={!hasMore || loading}
                className="px-3 py-1.5 text-xs font-medium rounded disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                style={{ background: "var(--bg-card)", color: "var(--text-primary)" }}
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
