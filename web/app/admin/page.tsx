"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, CircuitStatus } from "@/lib/admin-api";

interface Stats {
  agencyCount: number;
  activeCampaigns: number;
  todaySpend: number;
  totalBalance: number;
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-white p-5">
      <p className="text-xs font-medium text-gray-500 mb-2">{label}</p>
      <p className="text-2xl font-semibold font-geist tabular-nums">{value}</p>
    </div>
  );
}

function StatCardSkeleton() {
  return (
    <div className="rounded-lg bg-gray-200 p-5 animate-pulse h-20" />
  );
}

function CircuitBreakerRow({
  circuit,
  onTrip,
  onReset,
}: {
  circuit: CircuitStatus;
  onTrip: (name: string) => void;
  onReset: (name: string) => void;
}) {
  const isOpen = circuit.state === "open";
  const isHalfOpen = circuit.state === "half-open";

  return (
    <div className="flex items-center justify-between py-3 border-b last:border-0">
      <div className="flex items-center gap-3">
        <span
          className={`w-2.5 h-2.5 rounded-full flex-shrink-0 ${
            isOpen
              ? "bg-red-500"
              : isHalfOpen
              ? "bg-yellow-500"
              : "bg-green-500"
          }`}
          aria-label={circuit.state}
        />
        <div>
          <p className="text-sm font-medium text-gray-900">{circuit.name}</p>
          <p className="text-xs text-gray-500">
            {isOpen ? "熔断已触发" : isHalfOpen ? "半开测试中" : "正常"}
            {circuit.failures > 0 && ` · ${circuit.failures} 次失败`}
          </p>
        </div>
      </div>
      <div className="flex items-center gap-2">
        {!isOpen && (
          <button
            onClick={() => onTrip(circuit.name)}
            className="px-3 py-1.5 text-xs font-medium rounded bg-red-50 text-red-700 hover:bg-red-100 transition-colors"
          >
            手动熔断
          </button>
        )}
        {(isOpen || isHalfOpen) && (
          <button
            onClick={() => onReset(circuit.name)}
            className="px-3 py-1.5 text-xs font-medium rounded bg-green-50 text-green-700 hover:bg-green-100 transition-colors"
          >
            重置
          </button>
        )}
      </div>
    </div>
  );
}

export default function AdminOverviewPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [circuits, setCircuits] = useState<CircuitStatus[]>([]);
  const [health, setHealth] = useState<{ status: string; checks: Record<string, string> } | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [circuitError, setCircuitError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      adminApi.listAdvertisers(),
      adminApi.getCircuitStatus(),
      adminApi.getHealth(),
    ])
      .then(([advertisers, circuitData, healthData]) => {
        // Derive stats from available data
        const totalBalance = advertisers.reduce((sum, a) => sum + a.balance_cents, 0);
        setStats({
          agencyCount: advertisers.length,
          activeCampaigns: 0, // not available from advertiser list
          todaySpend: 0,      // not available from overview endpoint
          totalBalance,
        });
        setCircuits(circuitData);
        setHealth(healthData);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleTrip(name: string) {
    setCircuitError(null);
    try {
      await adminApi.tripCircuitBreaker(name);
      const updated = await adminApi.getCircuitStatus();
      setCircuits(updated);
    } catch (e: unknown) {
      setCircuitError(e instanceof Error ? e.message : "操作失败");
    }
  }

  async function handleReset(name: string) {
    setCircuitError(null);
    try {
      await adminApi.resetCircuitBreaker(name);
      const updated = await adminApi.getCircuitStatus();
      setCircuits(updated);
    } catch (e: unknown) {
      setCircuitError(e instanceof Error ? e.message : "操作失败");
    }
  }

  const healthOk = health?.status === "ok" || health?.status === "healthy";

  return (
    <div className="p-8 max-w-6xl">
      <h2 className="text-2xl font-semibold mb-6">概览</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
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
            <StatCard label="代理商数" value={stats.agencyCount.toLocaleString()} />
            <StatCard label="活跃 Campaign" value={stats.activeCampaigns.toLocaleString()} />
            <StatCard label="今日全局花费" value={`¥${(stats.todaySpend / 100).toLocaleString()}`} />
            <StatCard label="平台总余额" value={`¥${(stats.totalBalance / 100).toLocaleString()}`} />
          </>
        )}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Circuit Breakers */}
        <div className="bg-white rounded-lg p-6">
          <div className="flex items-center justify-between mb-4">
            <h3 className="text-sm font-semibold text-gray-700">熔断器状态</h3>
            {circuitError && (
              <span className="text-xs text-red-600">{circuitError}</span>
            )}
          </div>
          {loading ? (
            <div className="animate-pulse space-y-3">
              <div className="h-10 bg-gray-100 rounded" />
              <div className="h-10 bg-gray-100 rounded" />
            </div>
          ) : circuits.length === 0 ? (
            <p className="text-sm text-gray-500 text-center py-6">暂无熔断器数据</p>
          ) : (
            <div>
              {circuits.map((c) => (
                <CircuitBreakerRow
                  key={c.name}
                  circuit={c}
                  onTrip={handleTrip}
                  onReset={handleReset}
                />
              ))}
            </div>
          )}
        </div>

        {/* System Health */}
        <div className="bg-white rounded-lg p-6">
          <h3 className="text-sm font-semibold text-gray-700 mb-4">系统健康</h3>
          {loading ? (
            <div className="animate-pulse space-y-3">
              <div className="h-6 bg-gray-100 rounded w-1/2" />
              <div className="h-10 bg-gray-100 rounded" />
            </div>
          ) : !health ? (
            <p className="text-sm text-gray-500 text-center py-6">健康数据不可用</p>
          ) : (
            <div>
              <div className="flex items-center gap-2 mb-4">
                <span
                  className={`w-2.5 h-2.5 rounded-full ${healthOk ? "bg-green-500" : "bg-red-500"}`}
                />
                <span className={`text-sm font-medium ${healthOk ? "text-green-700" : "text-red-700"}`}>
                  {healthOk ? "所有服务正常" : "存在异常"}
                </span>
              </div>
              {Object.entries(health.checks).length > 0 && (
                <div className="space-y-2">
                  {Object.entries(health.checks).map(([key, val]) => (
                    <div key={key} className="flex items-center justify-between py-1.5 border-b last:border-0">
                      <span className="text-xs text-gray-600">{key}</span>
                      <span
                        className={`text-xs font-medium px-2 py-0.5 rounded-full ${
                          val === "ok" || val === "healthy"
                            ? "bg-green-50 text-green-700"
                            : "bg-red-50 text-red-700"
                        }`}
                      >
                        {val}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
