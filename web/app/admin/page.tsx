"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, CircuitStatus, SystemHealth } from "@/lib/admin-api";

interface Stats {
  agencyCount: number;
  activeCampaigns: number;
  todaySpend: number;
  totalBalance: number;
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="glass-card-static p-5">
      <p className="text-xs font-medium mb-2" style={{ color: "var(--text-secondary)" }}>{label}</p>
      <p className="text-2xl font-semibold tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</p>
    </div>
  );
}

function StatCardSkeleton() {
  return (
    <div className="glass-card-static p-5 animate-pulse h-20" />
  );
}

export default function AdminOverviewPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [circuit, setCircuit] = useState<CircuitStatus | null>(null);
  const [health, setHealth] = useState<SystemHealth | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [circuitError, setCircuitError] = useState<string | null>(null);
  const [circuitReason, setCircuitReason] = useState("");

  const load = useCallback(() => {
    Promise.all([
      adminApi.listAdvertisers(),
      adminApi.getCircuitStatus(),
      adminApi.getHealth(),
    ])
      .then(([advertisers, circuitData, healthData]) => {
        // Derive stats from available data
        const totalBalance = advertisers.reduce((sum, a) => sum + a.balance_cents, 0);
        const activeCampaigns = advertisers.reduce((sum, a) => sum + (a.active_campaigns ?? 0), 0);
        setStats({
          agencyCount: advertisers.length,
          activeCampaigns,
          todaySpend: circuitData.global_spend_today_cents,
          totalBalance,
        });
        setCircuit(circuitData);
        setHealth(healthData);
        setError(null);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  function handleRetry() {
    setLoading(true);
    setError(null);
    load();
  }

  async function handleTrip() {
    setCircuitError(null);
    const reason = circuitReason.trim() || "manual trip";
    try {
      await adminApi.tripCircuitBreaker(reason);
      const updated = await adminApi.getCircuitStatus();
      setCircuit(updated);
      setCircuitReason("");
    } catch (e: unknown) {
      setCircuitError(e instanceof Error ? e.message : "操作失败");
    }
  }

  async function handleReset() {
    setCircuitError(null);
    try {
      await adminApi.resetCircuitBreaker();
      const updated = await adminApi.getCircuitStatus();
      setCircuit(updated);
    } catch (e: unknown) {
      setCircuitError(e instanceof Error ? e.message : "操作失败");
    }
  }

  // V5.2A: Standard CB terminology — "open" means breaker is open (tripped/failing).
  const isTripped = circuit?.circuit_breaker === "open";
  const healthOk = health?.status === "ok" || health?.status === "healthy";

  return (
    <div className="">
      <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>概览</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded text-sm flex items-center justify-between" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>
          <span>{error}</span>
          <button onClick={handleRetry} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {/* Stat Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-3 mb-6">
        {loading || !stats ? (
          <>
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
            <StatCardSkeleton />
          </>
        ) : (
          <>
            <StatCard label="广告主数" value={stats.agencyCount.toLocaleString()} />
            <StatCard label="活跃 Campaign" value={stats.activeCampaigns.toLocaleString()} />
            <StatCard label="今日全局花费" value={`¥${(stats.todaySpend / 100).toLocaleString()}`} />
            <StatCard label="平台总余额" value={`¥${(stats.totalBalance / 100).toLocaleString()}`} />
          </>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Circuit Breaker */}
        <div className="glass-card-static p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-semibold" style={{ color: "var(--text-primary)" }}>熔断器状态</h3>
            {circuitError && (
              <span className="text-xs" style={{ color: "#EF4444" }}>{circuitError}</span>
            )}
          </div>
          {loading ? (
            <div className="animate-pulse space-y-3">
              <div className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
              <div className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
            </div>
          ) : !circuit ? (
            <p className="text-sm text-center py-6" style={{ color: "var(--text-secondary)" }}>暂无熔断器数据</p>
          ) : (
            <div>
              <div className="flex items-center justify-between py-3" style={{ borderBottom: "1px solid var(--border)" }}>
                <div className="flex items-center gap-3">
                  <span
                    className="w-2.5 h-2.5 rounded-full flex-shrink-0"
                    style={{ background: isTripped ? "#EF4444" : "#22C55E" }}
                    aria-label={circuit.circuit_breaker}
                  />
                  <div>
                    <p className="text-sm font-medium" style={{ color: "var(--text-primary)" }}>
                      {isTripped ? "熔断已触发" : "正常运行"}
                    </p>
                    {circuit.reason && (
                      <p className="text-xs mt-0.5" style={{ color: "var(--text-secondary)" }}>{circuit.reason}</p>
                    )}
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {isTripped ? (
                    <button
                      onClick={handleReset}
                      className="px-3 py-1.5 text-xs font-medium rounded transition-colors"
                      style={{ background: "rgba(34,197,94,0.15)", color: "#22C55E" }}
                    >
                      重置
                    </button>
                  ) : (
                    <button
                      onClick={handleTrip}
                      className="px-3 py-1.5 text-xs font-medium rounded transition-colors"
                      style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}
                    >
                      手动熔断
                    </button>
                  )}
                </div>
              </div>
              {!isTripped && (
                <div className="mt-3 flex items-center gap-2">
                  <input
                    type="text"
                    value={circuitReason}
                    onChange={(e) => setCircuitReason(e.target.value)}
                    placeholder="熔断原因（可选）"
                    className="flex-1 text-xs rounded px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-red-300"
                    style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
                  />
                </div>
              )}
              <div className="mt-3 text-xs" style={{ color: "var(--text-secondary)" }}>
                今日全局花费：¥{(circuit.global_spend_today_cents / 100).toLocaleString()}
              </div>
            </div>
          )}
        </div>

        {/* System Health */}
        <div className="glass-card-static p-6">
          <h3 className="text-sm font-semibold mb-4" style={{ color: "var(--text-primary)" }}>系统健康</h3>
          {loading ? (
            <div className="animate-pulse space-y-3">
              <div className="h-6 rounded w-1/2" style={{ background: "var(--bg-card-elevated)" }} />
              <div className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
            </div>
          ) : !health ? (
            <p className="text-sm text-center py-6" style={{ color: "var(--text-secondary)" }}>健康数据不可用</p>
          ) : (
            <div>
              <div className="flex items-center gap-2 mb-4">
                <span
                  className="w-2.5 h-2.5 rounded-full"
                  style={{ background: healthOk ? "#22C55E" : "#EF4444" }}
                />
                <span className="text-sm font-medium" style={{ color: healthOk ? "#22C55E" : "#EF4444" }}>
                  {healthOk ? "所有服务正常" : "存在异常"}
                </span>
              </div>
              <div className="space-y-2">
                {[
                  { key: "redis", label: "Redis" },
                  { key: "clickhouse", label: "ClickHouse" },
                ].map(({ key, label }) => {
                  const val = health[key as keyof SystemHealth] as string;
                  const ok = val === "ok" || val === "healthy";
                  return (
                    <div key={key} className="flex items-center justify-between py-1.5" style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                      <span className="text-xs" style={{ color: "var(--text-secondary)" }}>{label}</span>
                      <span
                        className="text-xs font-medium px-2 py-0.5 rounded-full"
                        style={
                          ok
                            ? { background: "rgba(34,197,94,0.15)", color: "#22C55E" }
                            : { background: "rgba(239,68,68,0.15)", color: "#EF4444" }
                        }
                      >
                        {val}
                      </span>
                    </div>
                  );
                })}
                <div className="flex items-center justify-between py-1.5" style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                  <span className="text-xs" style={{ color: "var(--text-secondary)" }}>活跃 Campaign</span>
                  <span className="text-xs font-medium tabular-nums" style={{ color: "var(--text-primary)" }}>{health.active_campaigns}</span>
                </div>
                <div className="flex items-center justify-between py-1.5">
                  <span className="text-xs" style={{ color: "var(--text-secondary)" }}>待审注册</span>
                  <span className="text-xs font-medium tabular-nums" style={{ color: "var(--text-primary)" }}>{health.pending_registrations}</span>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
