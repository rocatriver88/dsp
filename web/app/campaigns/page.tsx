"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";
import { LoadingSkeleton, ErrorState, EmptyState } from "../_components/LoadingState";
import { StatusBadge, TypeBadge } from "../_components/StatusBadge";
import { Pause, Play, Copy, Pencil, Trash2, MoreHorizontal, Plus, Filter } from "lucide-react";

type FilterKey = "all" | "active" | "paused" | "draft";

const filterTabs: { key: FilterKey; label: string }[] = [
  { key: "all", label: "全部" },
  { key: "active", label: "运行中" },
  { key: "paused", label: "已暂停" },
  { key: "draft", label: "草稿" },
];

function formatCurrency(cents: number): string {
  return `¥${(cents / 100).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 })}`;
}

function formatBid(c: Campaign): string {
  if (c.billing_model === "cpc") return `¥${(c.bid_cpc_cents / 100).toFixed(2)} CPC`;
  if (c.billing_model === "ocpm") return `¥${(c.ocpm_target_cpa_cents / 100).toFixed(2)} oCPM`;
  return `¥${(c.bid_cpm_cents / 100).toFixed(2)} CPM`;
}

function progressPercent(spent: number, budget: number): number {
  if (budget <= 0) return 0;
  return Math.min(100, Math.round((spent / budget) * 100));
}

function formatDate(d: string | undefined): string {
  if (!d) return "---";
  return d.slice(0, 10);
}

export default function CampaignsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [filter, setFilter] = useState<FilterKey>("all");

  const load = useCallback(() => {
    api.listCampaigns()
      .then((data) => {
        setCampaigns(data);
        setError(null);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleRetry = () => {
    setLoading(true);
    setError(null);
    load();
  };

  const handleAction = async (id: number, action: "start" | "pause") => {
    setActionError(null);
    try {
      if (action === "start") await api.startCampaign(id);
      else await api.pauseCampaign(id);
      handleRetry();
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    }
  };

  const filtered = filter === "all"
    ? campaigns
    : campaigns.filter((c) => c.status === filter);

  return (
    <div>
      {/* Header */}
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-semibold" style={{ color: "var(--text-primary)" }}>
          Campaigns
        </h2>
        <Link
          href="/campaigns/new"
          className="inline-flex items-center gap-1.5 px-4 py-2 text-sm font-semibold text-white rounded-lg transition-colors"
          style={{ background: "var(--primary)" }}
        >
          <Plus size={16} />
          创建广告系列
        </Link>
      </div>

      {/* Filter tabs */}
      <div className="flex items-center gap-2 mb-5">
        {filterTabs.map((t) => (
          <button
            key={t.key}
            onClick={() => setFilter(t.key)}
            className="px-3.5 py-1.5 text-sm font-medium rounded-lg transition-colors"
            style={
              filter === t.key
                ? { background: "var(--primary)", color: "#FFFFFF" }
                : { background: "transparent", color: "var(--text-secondary)" }
            }
          >
            {t.label}
          </button>
        ))}
        <button
          className="ml-auto flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-lg transition-colors"
          style={{ color: "var(--text-secondary)", background: "transparent" }}
        >
          <Filter size={14} />
          筛选
        </button>
      </div>

      {/* Action error banner */}
      {actionError && (
        <div
          className="mb-4 p-3 rounded-lg text-sm flex items-center justify-between"
          style={{ background: "rgba(239,68,68,0.12)", color: "var(--error)", border: "1px solid rgba(239,68,68,0.25)" }}
        >
          <span>{actionError}</span>
          <button
            onClick={() => setActionError(null)}
            className="text-xs ml-4 hover:opacity-70 transition-opacity"
            style={{ color: "var(--error)" }}
          >
            关闭
          </button>
        </div>
      )}

      {/* Content */}
      {loading ? (
        <LoadingSkeleton rows={5} />
      ) : error ? (
        <ErrorState message={error} onRetry={handleRetry} />
      ) : filtered.length === 0 && filter === "all" ? (
        <EmptyState
          heading="还没有 Campaign"
          message="创建第一个 Campaign，开始投放广告"
          actionLabel="创建广告系列"
          actionHref="/campaigns/new"
        />
      ) : filtered.length === 0 ? (
        <div
          className="rounded-[14px] p-12 text-center"
          style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}
        >
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>
            没有{filterTabs.find((t) => t.key === filter)?.label || ""}状态的 Campaign
          </p>
        </div>
      ) : (
        <div className="space-y-3" aria-label="Campaign 列表">
          {filtered.map((c) => {
            const progress = progressPercent(c.spent_cents, c.budget_total_cents);
            const dateRange = c.start_date
              ? `${formatDate(c.start_date)} ~ ${formatDate(c.end_date)}`
              : "---";

            return (
              <div
                key={c.id}
                className="rounded-[14px] p-5 transition-colors"
                style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}
              >
                {/* Card header */}
                <div className="flex items-start justify-between mb-1">
                  <div className="flex items-center gap-2.5 flex-wrap min-w-0">
                    <Link
                      href={`/campaigns/${c.id}`}
                      className="text-[16px] font-bold hover:underline truncate"
                      style={{ color: "var(--text-primary)" }}
                    >
                      {c.name}
                    </Link>
                    <StatusBadge status={c.status} />
                    <TypeBadge type={c.billing_model?.toUpperCase() || "CPM"} />
                  </div>

                  {/* Action buttons */}
                  <div className="flex items-center gap-0.5 flex-shrink-0 ml-3">
                    {c.status === "active" && (
                      <button
                        onClick={() => handleAction(c.id, "pause")}
                        aria-label={`暂停 ${c.name}`}
                        className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                        style={{ color: "var(--text-muted)" }}
                        onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                        onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                      >
                        <Pause size={16} />
                      </button>
                    )}
                    {(c.status === "draft" || c.status === "paused") && (
                      <button
                        onClick={() => handleAction(c.id, "start")}
                        aria-label={c.status === "draft" ? `启动 ${c.name}` : `恢复 ${c.name}`}
                        className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                        style={{ color: "var(--text-muted)" }}
                        onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                        onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                      >
                        <Play size={16} />
                      </button>
                    )}
                    <button
                      aria-label={`复制 ${c.name}`}
                      className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                      style={{ color: "var(--text-muted)" }}
                      onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                      onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                    >
                      <Copy size={16} />
                    </button>
                    <Link
                      href={`/campaigns/${c.id}`}
                      aria-label={`编辑 ${c.name}`}
                      className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                      style={{ color: "var(--text-muted)" }}
                      onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                      onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                    >
                      <Pencil size={16} />
                    </Link>
                    <button
                      aria-label={`删除 ${c.name}`}
                      className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                      style={{ color: "var(--text-muted)" }}
                      onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                      onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                    >
                      <Trash2 size={16} />
                    </button>
                    <button
                      aria-label={`更多操作 ${c.name}`}
                      className="w-8 h-8 flex items-center justify-center rounded-md transition-colors"
                      style={{ color: "var(--text-muted)" }}
                      onMouseEnter={(e) => (e.currentTarget.style.color = "var(--text-secondary)")}
                      onMouseLeave={(e) => (e.currentTarget.style.color = "var(--text-muted)")}
                    >
                      <MoreHorizontal size={16} />
                    </button>
                  </div>
                </div>

                {/* Date range sub-header */}
                <p className="text-xs mb-4" style={{ color: "var(--text-muted)" }}>
                  {dateRange}
                </p>

                {/* Metrics row */}
                <div className="grid grid-cols-4 md:grid-cols-8 gap-4">
                  <MetricCell label="预算" value={formatCurrency(c.budget_total_cents)} />
                  <MetricCell label="已花费" value={formatCurrency(c.spent_cents)} />
                  <MetricCell label="展示量" value="---" />
                  <MetricCell label="点击量" value="---" />
                  <MetricCell label="点击率" value="---" />
                  <MetricCell label="转化数" value="---" />
                  <MetricCell
                    label="ROI"
                    value="---"
                    valueColor="var(--success)"
                  />
                  <div className="flex flex-col gap-1">
                    <span
                      className="text-[11px] font-medium uppercase tracking-wider"
                      style={{ color: "var(--text-muted)" }}
                    >
                      进度
                    </span>
                    <div className="flex items-center gap-2">
                      <span
                        className="text-[14px] font-bold font-geist"
                        style={{ color: "var(--text-primary)", fontVariantNumeric: "tabular-nums" }}
                      >
                        {progress}%
                      </span>
                    </div>
                    <div
                      className="h-1.5 rounded-full w-full mt-0.5"
                      style={{ background: "var(--border)" }}
                    >
                      <div
                        className="h-full rounded-full transition-all"
                        style={{
                          width: `${progress}%`,
                          background: progress >= 90 ? "var(--error)" : "var(--primary)",
                        }}
                      />
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function MetricCell({
  label,
  value,
  valueColor,
}: {
  label: string;
  value: string;
  valueColor?: string;
}) {
  return (
    <div className="flex flex-col gap-1">
      <span
        className="text-[11px] font-medium uppercase tracking-wider"
        style={{ color: "var(--text-muted)" }}
      >
        {label}
      </span>
      <span
        className="text-[14px] font-bold font-geist"
        style={{
          color: valueColor || "var(--text-primary)",
          fontVariantNumeric: "tabular-nums",
        }}
      >
        {value}
      </span>
    </div>
  );
}
