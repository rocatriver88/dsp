"use client";

import { useEffect, useState, useRef } from "react";
import { LoadingSkeleton, ErrorState, EmptyState } from "../components/LoadingState";
import { StatCard } from "../components/StatCard";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

interface CampaignLive {
  campaign_id: number;
  name: string;
  impressions: number;
  clicks: number;
  wins: number;
  bids: number;
  win_rate: number;
  ctr: number;
  spend_cents: number;
  profit_cents: number;
}

interface AnalyticsData {
  timestamp: string;
  campaigns: CampaignLive[];
}

export default function AnalyticsPage() {
  const [data, setData] = useState<AnalyticsData | null>(null);
  const [connected, setConnected] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const apiKey = typeof window !== "undefined" ? localStorage.getItem("dsp_api_key") || "" : "";

    // Use SSE for real-time updates
    const url = `${API_BASE}/api/v1/analytics/stream?api_key=${encodeURIComponent(apiKey)}`;
    const es = new EventSource(url);
    eventSourceRef.current = es;

    es.onopen = () => {
      setConnected(true);
      setError(null);
    };

    es.onmessage = (event) => {
      try {
        const parsed = JSON.parse(event.data) as AnalyticsData;
        setData(parsed);
      } catch {
        // ignore parse errors
      }
    };

    es.onerror = () => {
      setConnected(false);
      setError("Connection lost. Reconnecting...");
      // EventSource auto-reconnects
    };

    return () => {
      es.close();
    };
  }, []);

  if (error && !data) {
    return <ErrorState message={error} onRetry={() => window.location.reload()} />;
  }

  if (!data) {
    return <LoadingSkeleton />;
  }

  const totalImpressions = data.campaigns.reduce((s, c) => s + c.impressions, 0);
  const totalClicks = data.campaigns.reduce((s, c) => s + c.clicks, 0);
  const totalSpend = data.campaigns.reduce((s, c) => s + c.spend_cents, 0);
  const totalProfit = data.campaigns.reduce((s, c) => s + c.profit_cents, 0);

  return (
    <main className="p-6 max-w-7xl mx-auto">
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold">实时竞价分析</h1>
        <div className="flex items-center gap-2 text-sm">
          <span className={`inline-block w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
          <span className="text-gray-500">
            {connected ? "实时连接" : "重连中..."}
          </span>
          <span className="text-gray-400 ml-2">
            {new Date(data.timestamp).toLocaleTimeString()}
          </span>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="今日曝光" value={totalImpressions.toLocaleString()} />
        <StatCard label="今日点击" value={totalClicks.toLocaleString()} />
        <StatCard label="今日花费" value={`¥${(totalSpend / 100).toFixed(2)}`} />
        <StatCard label="今日利润" value={`¥${(totalProfit / 100).toFixed(2)}`} className={totalProfit >= 0 ? "text-green-600" : "text-red-600"} />
      </div>

      {data.campaigns.length === 0 ? (
        <EmptyState heading="暂无活跃 Campaign" message="创建并启动 Campaign 后，实时竞价数据会在这里显示" actionLabel="创建 Campaign" actionHref="/campaigns/new" />
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm" aria-label="实时竞价数据">
            <thead>
              <tr className="border-b text-left text-gray-500">
                <th className="py-3 pr-4">Campaign</th>
                <th className="py-3 pr-4 text-right">竞价</th>
                <th className="py-3 pr-4 text-right">胜出</th>
                <th className="py-3 pr-4 text-right">Win Rate</th>
                <th className="py-3 pr-4 text-right">曝光</th>
                <th className="py-3 pr-4 text-right">点击</th>
                <th className="py-3 pr-4 text-right">CTR</th>
                <th className="py-3 pr-4 text-right">花费</th>
              </tr>
            </thead>
            <tbody>
              {data.campaigns.map((c) => (
                <tr key={c.campaign_id} className="border-b hover:bg-gray-50">
                  <td className="py-3 pr-4 font-medium">{c.name}</td>
                  <td className="py-3 pr-4 text-right tabular-nums">{c.bids.toLocaleString()}</td>
                  <td className="py-3 pr-4 text-right tabular-nums">{c.wins.toLocaleString()}</td>
                  <td className="py-3 pr-4 text-right tabular-nums">
                    <WinRateBadge rate={c.win_rate} />
                  </td>
                  <td className="py-3 pr-4 text-right tabular-nums">{c.impressions.toLocaleString()}</td>
                  <td className="py-3 pr-4 text-right tabular-nums">{c.clicks.toLocaleString()}</td>
                  <td className="py-3 pr-4 text-right tabular-nums">{c.ctr.toFixed(2)}%</td>
                  <td className="py-3 pr-4 text-right tabular-nums">¥{(c.spend_cents / 100).toFixed(2)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}

function WinRateBadge({ rate }: { rate: number }) {
  let color = "text-gray-600";
  if (rate > 60) color = "text-orange-600"; // overpaying
  if (rate < 20) color = "text-red-600";    // losing too much
  if (rate >= 20 && rate <= 60) color = "text-green-600"; // sweet spot

  return <span className={color}>{rate.toFixed(1)}%</span>;
}
