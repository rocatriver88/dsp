"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { api, Campaign, CampaignStats, HourlyStats, GeoStats, BidDetail } from "@/lib/api";
import { StatCard } from "../../_components/StatCard";
import { EventBadge } from "../../_components/StatusBadge";

export default function CampaignReportPage() {
  const params = useParams();
  const id = Number(params.id);

  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [hourly, setHourly] = useState<HourlyStats[]>([]);
  const [geo, setGeo] = useState<GeoStats[]>([]);
  const [bids, setBids] = useState<BidDetail[]>([]);
  const [tab, setTab] = useState<"overview" | "bids">("overview");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    Promise.all([
      api.getCampaign(id),
      api.getCampaignStats(id).catch(() => null),
      api.getHourlyStats(id).catch(() => []),
      api.getGeoBreakdown(id).catch(() => []),
      api.getBidTransparency(id).catch(() => []),
    ])
      .then(([c, s, h, g, b]) => {
        setCampaign(c);
        setStats(s);
        setHourly(h);
        setGeo(g);
        setBids(b);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, [id]);

  if (loading) {
    return (
      <div>
        <div className="h-6 w-48 rounded-[14px] animate-pulse mb-6" style={{ background: "var(--bg-card)" }} />
        <div className="grid grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-20 rounded-[14px] animate-pulse" style={{ background: "var(--bg-card)" }} />
          ))}
        </div>
      </div>
    );
  }

  if (error || !campaign) {
    return (
      <div className="rounded-[14px] p-8 text-center" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
        <p className="text-sm" style={{ color: "var(--error)" }}>{error || "Campaign 未找到"}</p>
        <Link href="/reports" className="text-sm mt-4 inline-block" style={{ color: "var(--primary)" }}>返回报表列表</Link>
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <Link href="/reports" style={{ color: "var(--text-muted)" }}>← 报表</Link>
        <h2 className="text-2xl font-semibold" style={{ color: "var(--text-primary)" }}>{campaign.name}</h2>
        <span className="px-2 py-0.5 text-xs font-medium rounded-full"
          style={
            campaign.status === "active" ? { background: "rgba(34,197,94,0.15)", color: "#22C55E" } :
            campaign.status === "paused" ? { background: "rgba(234,179,8,0.15)", color: "#EAB308" } :
            { background: "var(--bg-card-elevated)", color: "var(--text-secondary)" }
          }>{campaign.status}</span>
      </div>

      {/* Stats cards */}
      {stats ? (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-3 mb-6">
          <StatCard label="曝光量" value={stats.impressions.toLocaleString()} />
          <StatCard label="点击量" value={stats.clicks.toLocaleString()} />
          <StatCard label="转化量" value={stats.conversions.toLocaleString()} />
          <StatCard label="CTR" value={`${Math.min(stats.ctr, 100).toFixed(2)}%`} />
          <StatCard label="CVR" value={`${Math.min(stats.cvr || 0, 100).toFixed(2)}%`} />
          <StatCard label="CPA" value={stats.cpa > 0 ? `¥${stats.cpa.toFixed(2)}` : "—"} />
          <StatCard label="Win Rate" value={`${Math.min(stats.win_rate, 100).toFixed(1)}%`} />
          <StatCard label="广告主计费" value={`¥${(stats.spend_cents / 100).toFixed(2)}`} />
          <StatCard label="ADX 成本" value={`¥${((stats.adx_cost_cents || 0) / 100).toFixed(2)}`} />
          <StatCard label="平台利润" value={`¥${((stats.profit_cents || 0) / 100).toFixed(2)}`} />
        </div>
      ) : (
        <div className="rounded-[14px] p-8 text-center mb-6" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>ClickHouse 未连接或暂无数据</p>
          <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>投放广告后数据会自动显示</p>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-4" style={{ borderBottom: "1px solid var(--border)" }}>
        <button onClick={() => setTab("overview")}
          className="px-4 py-2 text-sm font-medium -mb-px"
          style={tab === "overview"
            ? { borderBottom: "2px solid var(--primary)", color: "var(--primary)" }
            : { borderBottom: "2px solid transparent", color: "var(--text-secondary)" }
          }>
          效果概览
        </button>
        <button onClick={() => setTab("bids")}
          className="px-4 py-2 text-sm font-medium -mb-px"
          style={tab === "bids"
            ? { borderBottom: "2px solid var(--primary)", color: "var(--primary)" }
            : { borderBottom: "2px solid transparent", color: "var(--text-secondary)" }
          }>
          Bid 透明度
        </button>
      </div>

      {tab === "overview" ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Hourly chart (text-based for Phase 2) */}
          <div className="rounded-[14px] p-5" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
            <h3 className="text-sm font-medium mb-4" style={{ color: "var(--text-secondary)" }}>今日小时分布</h3>
            {hourly.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无数据</p>
                <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>Campaign 投放后数据会在此显示</p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {hourly.map((h) => {
                  const maxImp = Math.max(...hourly.map((x) => x.impressions), 1);
                  const pct = (h.impressions / maxImp) * 100;
                  return (
                    <div key={h.hour} className="flex items-center gap-2 text-xs">
                      <span className="w-8 text-right tabular-nums" style={{ color: "var(--text-muted)" }}>{String(h.hour).padStart(2, "0")}:00</span>
                      <div className="flex-1 h-5 rounded overflow-hidden" style={{ background: "var(--bg-card-elevated)" }}>
                        <div className="h-full rounded" style={{ width: `${pct}%`, background: "var(--primary)" }} />
                      </div>
                      <span className="w-16 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{h.impressions.toLocaleString()}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Geo breakdown */}
          <div className="rounded-[14px] p-5" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
            <h3 className="text-sm font-medium mb-4" style={{ color: "var(--text-secondary)" }}>地区分布</h3>
            {geo.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无数据</p>
                <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>Campaign 投放后数据会在此显示</p>
              </div>
            ) : (
              <table className="w-full text-sm" aria-label="地区分布">
                <thead>
                  <tr>
                    <th className="text-left py-2 font-medium" style={{ color: "var(--text-secondary)" }}>地区</th>
                    <th className="text-right py-2 font-medium" style={{ color: "var(--text-secondary)" }}>曝光</th>
                    <th className="text-right py-2 font-medium" style={{ color: "var(--text-secondary)" }}>点击</th>
                    <th className="text-right py-2 font-medium" style={{ color: "var(--text-secondary)" }}>花费 (¥)</th>
                  </tr>
                </thead>
                <tbody>
                  {geo.map((g) => (
                    <tr key={g.country} style={{ borderTop: "1px solid var(--border)" }}>
                      <td className="py-2 tabular-nums" style={{ color: "var(--text-primary)" }}>{g.country}</td>
                      <td className="py-2 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{g.impressions.toLocaleString()}</td>
                      <td className="py-2 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{g.clicks.toLocaleString()}</td>
                      <td className="py-2 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>{(g.spend_cents / 100).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      ) : (
        /* Bid Transparency Tab */
        <div className="rounded-[14px] overflow-hidden" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
          <div className="px-5 py-3" style={{ background: "var(--bg-card-elevated)", borderBottom: "1px solid var(--border)" }}>
            <h3 className="text-sm font-medium" style={{ color: "var(--text-primary)" }}>逐笔竞价记录</h3>
            <p className="text-xs mt-0.5" style={{ color: "var(--text-secondary)" }}>每一笔竞价的真实出价和成交价，完全透明</p>
          </div>
          {bids.length === 0 ? (
            <div className="p-12 text-center">
              <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无竞价记录</p>
              <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>Campaign 投放后，每笔竞价都会记录在这里</p>
            </div>
          ) : (
            <table className="w-full text-xs" aria-label="竞价记录">
              <thead style={{ background: "var(--bg-card-elevated)" }}>
                <tr>
                  <th className="text-left py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>时间</th>
                  <th className="text-left py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>Request ID</th>
                  <th className="text-center py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>类型</th>
                  <th className="text-right py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>出价 (¢)</th>
                  <th className="text-right py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>成交价 (¢)</th>
                  <th className="text-center py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>地区</th>
                  <th className="text-center py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>系统</th>
                  <th className="text-left py-2.5 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>备注</th>
                </tr>
              </thead>
              <tbody>
                {bids.map((b, i) => (
                  <tr key={i} style={{
                    borderTop: "1px solid var(--border)",
                    background: b.event_type === "win" ? "rgba(34,197,94,0.06)" :
                      b.event_type === "loss" ? "rgba(239,68,68,0.06)" : undefined
                  }}>
                    <td className="py-2 px-4 tabular-nums" style={{ color: "var(--text-secondary)" }}>
                      {new Date(b.time).toLocaleTimeString("zh-CN")}
                    </td>
                    <td className="py-2 px-4 tabular-nums truncate max-w-[120px]" style={{ color: "var(--text-muted)" }}>
                      {b.request_id}
                    </td>
                    <td className="py-2 px-4 text-center">
                      <EventBadge type={b.event_type} />
                    </td>
                    <td className="py-2 px-4 text-right tabular-nums" style={{ color: "var(--text-primary)" }}>{b.bid_price_cents}</td>
                    <td className="py-2 px-4 text-right tabular-nums" style={{ color: "var(--text-primary)" }}>
                      {b.clear_price_cents > 0 ? b.clear_price_cents : "—"}
                    </td>
                    <td className="py-2 px-4 text-center tabular-nums" style={{ color: "var(--text-secondary)" }}>{b.geo_country || "—"}</td>
                    <td className="py-2 px-4 text-center" style={{ color: "var(--text-secondary)" }}>{b.device_os || "—"}</td>
                    <td className="py-2 px-4" style={{ color: "var(--text-muted)" }}>{b.loss_reason || ""}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  );
}

