"use client";

import { useEffect, useState, useRef } from "react";
import { LoadingSkeleton, ErrorState, EmptyState } from "../_components/LoadingState";
import { StatCard } from "../_components/StatCard";
import { getAccessToken } from "@/lib/api";
import { Eye, MousePointer, DollarSign, TrendingUp } from "lucide-react";

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
    if (typeof window === "undefined") return;

    // V5.1 P1-1: never put the tenant X-API-Key in URL query. Mint a
    // short-lived HMAC token via POST /analytics/token (header-auth'd),
    // then use the token in ?token= for EventSource. Pre-emptive refresh
    // fires 1 minute before the server-reported expires_at.
    //
    // All state mutations below live inside async continuations
    // (fetchToken's await chain, es.onopen, es.onerror, refresh timer
    // callbacks) so the react-hooks/set-state-in-effect lint rule is
    // satisfied. No synchronous setState at the top of the effect.
    let cancelled = false;
    let es: EventSource | null = null;
    let refreshTimer: ReturnType<typeof setTimeout> | null = null;

    interface TokenResponse {
      token: string;
      expires_at: string; // RFC3339
    }

    async function fetchToken(): Promise<TokenResponse | null> {
      // Support both JWT Bearer token (new) and API Key (legacy/programmatic).
      // JWT takes priority if present. Use getAccessToken() from api.ts
      // instead of raw localStorage to keep token key in one place.
      const accessToken = getAccessToken();
      const apiKey = localStorage.getItem("dsp_api_key") || "";
      const headers: Record<string, string> = {};
      if (accessToken) {
        headers["Authorization"] = `Bearer ${accessToken}`;
      } else if (apiKey) {
        headers["X-API-Key"] = apiKey;
      } else {
        return null;
      }
      const r = await fetch(`${API_BASE}/api/v1/analytics/token`, {
        method: "POST",
        headers,
      });
      if (!r.ok) return null;
      return (await r.json()) as TokenResponse;
    }

    function scheduleRefreshFromExpiry(expiresAt: string) {
      const expiresMs = new Date(expiresAt).getTime();
      const nowMs = Date.now();
      // Refresh 60s before expiry, but never less than 30s from now and
      // never more than 4m30s from now as a defensive clamp.
      const untilExpiry = Math.max(0, expiresMs - nowMs);
      const refreshIn = Math.max(30_000, Math.min(untilExpiry - 60_000, 4 * 60_000 + 30_000));
      refreshTimer = setTimeout(() => {
        if (cancelled) return;
        if (es) {
          es.close();
          es = null;
        }
        connect();
      }, refreshIn);
    }

    async function connect() {
      const tok = await fetchToken();
      if (cancelled) return;
      if (!tok) {
        setConnected(false);
        setError("无法获取 SSE token — API Key 是否有效？");
        // Retry in 5s
        refreshTimer = setTimeout(() => {
          if (!cancelled) connect();
        }, 5000);
        return;
      }
      const url = `${API_BASE}/api/v1/analytics/stream?token=${encodeURIComponent(tok.token)}`;
      es = new EventSource(url);
      eventSourceRef.current = es;

      es.onopen = () => {
        if (cancelled) return;
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
        if (cancelled) return;
        setConnected(false);
        setError("Connection lost. Reconnecting...");
        // EventSource auto-retries the same URL, but our token expires
        // after 5 minutes — on error, close and reconnect with a fresh
        // token after a short backoff so a stale-token retry loop can't
        // spin forever.
        if (es) {
          es.close();
          es = null;
        }
        if (refreshTimer) {
          clearTimeout(refreshTimer);
        }
        refreshTimer = setTimeout(() => {
          if (!cancelled) connect();
        }, 2000);
      };

      scheduleRefreshFromExpiry(tok.expires_at);
    }

    connect();

    return () => {
      cancelled = true;
      if (refreshTimer) {
        clearTimeout(refreshTimer);
        refreshTimer = null;
      }
      if (es) {
        es.close();
        es = null;
      }
      eventSourceRef.current = null;
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
    <main>
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold" style={{ color: "var(--text-primary)" }}>数据分析</h1>
          <p className="text-[13px]" style={{ color: "var(--text-secondary)" }}>实时监控竞价投放数据</p>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <span className={`inline-block w-2 h-2 rounded-full ${connected ? "bg-green-500" : "bg-red-500"}`} />
          <span style={{ color: "var(--text-secondary)" }}>
            {connected ? "实时连接" : "重连中..."}
          </span>
          <span className="ml-2" style={{ color: "var(--text-muted)" }}>
            {new Date(data.timestamp).toLocaleTimeString()}
          </span>
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-6">
        <StatCard label="今日曝光" value={totalImpressions.toLocaleString()} icon={Eye} iconColor="#8B5CF6" stagger={1} />
        <StatCard label="今日点击" value={totalClicks.toLocaleString()} icon={MousePointer} iconColor="#3B82F6" stagger={2} />
        <StatCard label="今日花费" value={`¥${(totalSpend / 100).toFixed(2)}`} icon={DollarSign} iconColor="#F97316" stagger={3} />
        <StatCard label="今日利润" value={`¥${(totalProfit / 100).toFixed(2)}`} icon={TrendingUp} iconColor={totalProfit >= 0 ? "#22C55E" : "#EF4444"} stagger={4} />
      </div>

      {data.campaigns.length === 0 ? (
        <EmptyState heading="暂无活跃 Campaign" message="创建并启动 Campaign 后，实时竞价数据会在这里显示" actionLabel="创建 Campaign" actionHref="/campaigns/new" />
      ) : (
        <div className="glass-card-static p-0 overflow-hidden animate-fade-in">
          <table className="w-full text-sm" aria-label="实时竞价数据">
            <thead>
              <tr className="text-left" style={{ borderBottom: "1px solid var(--border)", color: "var(--text-secondary)" }}>
                <th className="py-3 px-4">Campaign</th>
                <th className="py-3 px-4 text-right">竞价</th>
                <th className="py-3 px-4 text-right">胜出</th>
                <th className="py-3 px-4 text-right">Win Rate</th>
                <th className="py-3 px-4 text-right">曝光</th>
                <th className="py-3 px-4 text-right">点击</th>
                <th className="py-3 px-4 text-right">CTR</th>
                <th className="py-3 px-4 text-right">花费</th>
              </tr>
            </thead>
            <tbody>
              {data.campaigns.map((c) => (
                <tr key={c.campaign_id} style={{ borderBottom: "1px solid var(--border)" }}>
                  <td className="py-3 px-4 font-medium" style={{ color: "var(--text-primary)" }}>{c.name}</td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{c.bids.toLocaleString()}</td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{c.wins.toLocaleString()}</td>
                  <td className="py-3 px-4 text-right tabular-nums">
                    <WinRateBadge rate={c.win_rate} />
                  </td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{c.impressions.toLocaleString()}</td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{c.clicks.toLocaleString()}</td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{c.ctr.toFixed(2)}%</td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>¥{(c.spend_cents / 100).toFixed(2)}</td>
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
  let color = "var(--text-secondary)";
  if (rate > 60) color = "#EAB308"; // overpaying
  if (rate < 20) color = "#EF4444";    // losing too much
  if (rate >= 20 && rate <= 60) color = "#22C55E"; // sweet spot

  return <span style={{ color }}>{rate.toFixed(1)}%</span>;
}
