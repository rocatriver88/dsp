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
        <div className="h-6 w-48 bg-gray-200 rounded animate-pulse mb-6" />
        <div className="grid grid-cols-4 gap-4">
          {[1, 2, 3, 4].map((i) => (
            <div key={i} className="h-20 bg-gray-100 rounded animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  if (error || !campaign) {
    return (
      <div className="rounded-lg bg-white p-8 text-center">
        <p className="text-sm text-red-600">{error || "Campaign 未找到"}</p>
        <Link href="/reports" className="text-sm text-blue-600 mt-4 inline-block">返回报表列表</Link>
      </div>
    );
  }

  return (
    <div>
      {/* Header */}
      <div className="flex items-center gap-3 mb-6">
        <Link href="/reports" className="text-gray-400 hover:text-gray-600">← 报表</Link>
        <h2 className="text-2xl font-semibold">{campaign.name}</h2>
        <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${
          campaign.status === "active" ? "bg-green-50 text-green-700" :
          campaign.status === "paused" ? "bg-yellow-50 text-yellow-700" :
          "bg-gray-100 text-gray-600"
        }`}>{campaign.status}</span>
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
        <div className="rounded-lg bg-white p-8 text-center mb-6">
          <p className="text-sm text-gray-500">ClickHouse 未连接或暂无数据</p>
          <p className="text-xs text-gray-400 mt-1">投放广告后数据会自动显示</p>
        </div>
      )}

      {/* Tabs */}
      <div className="flex gap-1 mb-4 border-b border-gray-200">
        <button onClick={() => setTab("overview")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px ${
            tab === "overview" ? "border-blue-600 text-blue-600" : "border-transparent text-gray-500 hover:text-gray-700"
          }`}>
          效果概览
        </button>
        <button onClick={() => setTab("bids")}
          className={`px-4 py-2 text-sm font-medium border-b-2 -mb-px ${
            tab === "bids" ? "border-blue-600 text-blue-600" : "border-transparent text-gray-500 hover:text-gray-700"
          }`}>
          Bid 透明度
        </button>
      </div>

      {tab === "overview" ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Hourly chart (text-based for Phase 2) */}
          <div className="rounded-lg bg-white p-5">
            <h3 className="text-sm font-medium text-gray-500 mb-4">今日小时分布</h3>
            {hourly.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-sm text-gray-500">暂无数据</p>
                <p className="text-xs text-gray-400 mt-1">Campaign 投放后数据会在此显示</p>
              </div>
            ) : (
              <div className="space-y-1.5">
                {hourly.map((h) => {
                  const maxImp = Math.max(...hourly.map((x) => x.impressions), 1);
                  const pct = (h.impressions / maxImp) * 100;
                  return (
                    <div key={h.hour} className="flex items-center gap-2 text-xs">
                      <span className="w-8 text-right text-gray-400 font-geist tabular-nums">{String(h.hour).padStart(2, "0")}:00</span>
                      <div className="flex-1 h-5 bg-gray-50 rounded overflow-hidden">
                        <div className="h-full bg-blue-500 rounded" style={{ width: `${pct}%` }} />
                      </div>
                      <span className="w-16 text-right font-geist tabular-nums text-gray-600">{h.impressions.toLocaleString()}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>

          {/* Geo breakdown */}
          <div className="rounded-lg bg-white p-5">
            <h3 className="text-sm font-medium text-gray-500 mb-4">地区分布</h3>
            {geo.length === 0 ? (
              <div className="text-center py-8">
                <p className="text-sm text-gray-500">暂无数据</p>
                <p className="text-xs text-gray-400 mt-1">Campaign 投放后数据会在此显示</p>
              </div>
            ) : (
              <table className="w-full text-sm" aria-label="地区分布">
                <thead>
                  <tr>
                    <th className="text-left py-2 text-gray-500 font-medium">地区</th>
                    <th className="text-right py-2 text-gray-500 font-medium">曝光</th>
                    <th className="text-right py-2 text-gray-500 font-medium">点击</th>
                    <th className="text-right py-2 text-gray-500 font-medium">花费 (¥)</th>
                  </tr>
                </thead>
                <tbody>
                  {geo.map((g) => (
                    <tr key={g.country} className="border-t border-gray-100">
                      <td className="py-2 font-geist tabular-nums">{g.country}</td>
                      <td className="py-2 text-right font-geist tabular-nums">{g.impressions.toLocaleString()}</td>
                      <td className="py-2 text-right font-geist tabular-nums">{g.clicks.toLocaleString()}</td>
                      <td className="py-2 text-right font-geist tabular-nums">{(g.spend_cents / 100).toFixed(2)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </div>
      ) : (
        /* Bid Transparency Tab */
        <div className="rounded-lg bg-white overflow-hidden">
          <div className="px-5 py-3 bg-gray-50 border-b border-gray-200">
            <h3 className="text-sm font-medium">逐笔竞价记录</h3>
            <p className="text-xs text-gray-500 mt-0.5">每一笔竞价的真实出价和成交价，完全透明</p>
          </div>
          {bids.length === 0 ? (
            <div className="p-12 text-center">
              <p className="text-sm text-gray-500">暂无竞价记录</p>
              <p className="text-xs text-gray-400 mt-1">Campaign 投放后，每笔竞价都会记录在这里</p>
            </div>
          ) : (
            <table className="w-full text-xs" aria-label="竞价记录">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-2.5 px-4 font-medium text-gray-500">时间</th>
                  <th className="text-left py-2.5 px-4 font-medium text-gray-500">Request ID</th>
                  <th className="text-center py-2.5 px-4 font-medium text-gray-500">类型</th>
                  <th className="text-right py-2.5 px-4 font-medium text-gray-500">出价 (¢)</th>
                  <th className="text-right py-2.5 px-4 font-medium text-gray-500">成交价 (¢)</th>
                  <th className="text-center py-2.5 px-4 font-medium text-gray-500">地区</th>
                  <th className="text-center py-2.5 px-4 font-medium text-gray-500">系统</th>
                  <th className="text-left py-2.5 px-4 font-medium text-gray-500">备注</th>
                </tr>
              </thead>
              <tbody>
                {bids.map((b, i) => (
                  <tr key={i} className={`border-t border-gray-100 ${
                    b.event_type === "win" ? "bg-green-50/50" :
                    b.event_type === "loss" ? "bg-red-50/30" : ""
                  }`}>
                    <td className="py-2 px-4 font-geist tabular-nums text-gray-500">
                      {new Date(b.time).toLocaleTimeString("zh-CN")}
                    </td>
                    <td className="py-2 px-4 font-geist tabular-nums text-gray-400 truncate max-w-[120px]">
                      {b.request_id}
                    </td>
                    <td className="py-2 px-4 text-center">
                      <EventBadge type={b.event_type} />
                    </td>
                    <td className="py-2 px-4 text-right font-geist tabular-nums">{b.bid_price_cents}</td>
                    <td className="py-2 px-4 text-right font-geist tabular-nums">
                      {b.clear_price_cents > 0 ? b.clear_price_cents : "—"}
                    </td>
                    <td className="py-2 px-4 text-center font-geist tabular-nums">{b.geo_country || "—"}</td>
                    <td className="py-2 px-4 text-center">{b.device_os || "—"}</td>
                    <td className="py-2 px-4 text-gray-400">{b.loss_reason || ""}</td>
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

