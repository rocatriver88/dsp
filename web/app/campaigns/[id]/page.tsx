"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { api, Campaign, CampaignStats, Creative } from "@/lib/api";
import { StatCard } from "../../components/StatCard";
import { StatusBadge } from "../../components/StatusBadge";

export default function CampaignDetailPage() {
  const params = useParams();
  const id = Number(params.id);
  const [campaign, setCampaign] = useState<Campaign | null>(null);
  const [stats, setStats] = useState<CampaignStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [creatives, setCreatives] = useState<Creative[]>([]);
  const [showAddCreative, setShowAddCreative] = useState(false);

  const loadCreatives = () => api.listCreatives(id).then(setCreatives).catch(() => {});

  useEffect(() => {
    Promise.all([
      api.getCampaign(id),
      api.getCampaignStats(id).catch(() => null),
    ])
      .then(([c, s]) => { setCampaign(c); setStats(s); })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
    loadCreatives();
  }, [id]);

  const handleAction = async (action: "start" | "pause") => {
    setActionError(null);
    try {
      if (action === "start") await api.startCampaign(id);
      else await api.pauseCampaign(id);
      const c = await api.getCampaign(id);
      setCampaign(c);
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    }
  };

  if (loading) {
    return <div className="animate-pulse"><div className="h-6 w-48 bg-gray-200 rounded mb-4" /><div className="h-40 bg-gray-100 rounded" /></div>;
  }

  if (error || !campaign) {
    return (
      <div className="rounded-lg bg-white p-8 text-center">
        <p className="text-sm text-red-600">{error || "Campaign 未找到"}</p>
        <Link href="/campaigns" className="text-sm text-blue-600 mt-4 inline-block">返回列表</Link>
      </div>
    );
  }

  const targeting = campaign.targeting as { geo?: string[]; os?: string[]; device?: string[] };

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link href="/campaigns" className="text-gray-400 hover:text-gray-600">← Campaigns</Link>
        <h2 className="text-2xl font-semibold">{campaign.name}</h2>
        <StatusBadge status={campaign.status} />
        {campaign.status === "paused" && campaign.pause_reason && (
          <span className="text-xs text-yellow-700 bg-yellow-50 px-2 py-1 rounded">
            自动暂停: {campaign.pause_reason}
          </span>
        )}
        {campaign.status === "draft" && (
          <button onClick={() => handleAction("start")} className="ml-auto text-sm px-4 py-1.5 rounded bg-green-50 text-green-700 hover:bg-green-100">启动</button>
        )}
        {campaign.status === "active" && (
          <button onClick={() => handleAction("pause")} className="ml-auto text-sm px-4 py-1.5 rounded bg-yellow-50 text-yellow-700 hover:bg-yellow-100">暂停</button>
        )}
        {campaign.status === "paused" && (
          <button onClick={() => handleAction("start")} className="ml-auto text-sm px-4 py-1.5 rounded bg-green-50 text-green-700 hover:bg-green-100">恢复</button>
        )}
      </div>

      {actionError && (
        <div className="mb-4 p-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} className="text-red-500 hover:text-red-700 text-xs ml-4">关闭</button>
        </div>
      )}

      {/* Stats */}
      {stats && (
        <div className="grid grid-cols-2 md:grid-cols-5 gap-4 mb-6">
          <StatCard label="曝光量" value={stats.impressions.toLocaleString()} />
          <StatCard label="点击量" value={stats.clicks.toLocaleString()} />
          <StatCard label="CTR" value={`${Math.min(stats.ctr, 100).toFixed(2)}%`} />
          <StatCard label="Win Rate" value={`${Math.min(stats.win_rate, 100).toFixed(1)}%`} />
          <StatCard label="花费" value={`¥${(stats.spend_cents / 100).toLocaleString()}`} />
        </div>
      )}

      {/* Campaign Info */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <div className="rounded-lg bg-white p-5">
          <h3 className="text-sm font-medium text-gray-500 mb-4">基本信息</h3>
          <div className="space-y-3 text-sm">
            <InfoRow label="计费模式" value={campaign.billing_model || "cpm"} />
            {campaign.bid_cpm_cents > 0 && <InfoRow label="CPM 出价" value={`¥${(campaign.bid_cpm_cents / 100).toFixed(2)}`} />}
            {campaign.bid_cpc_cents > 0 && <InfoRow label="CPC 出价" value={`¥${(campaign.bid_cpc_cents / 100).toFixed(2)}`} />}
            {campaign.ocpm_target_cpa_cents > 0 && <InfoRow label="oCPM 目标CPA" value={`¥${(campaign.ocpm_target_cpa_cents / 100).toFixed(2)}`} />}
            <InfoRow label="总预算" value={`¥${(campaign.budget_total_cents / 100).toLocaleString()}`} />
            <InfoRow label="日预算" value={`¥${(campaign.budget_daily_cents / 100).toLocaleString()}`} />
            <InfoRow label="创建时间" value={new Date(campaign.created_at).toLocaleString("zh-CN")} />
          </div>
        </div>

        <div className="rounded-lg bg-white p-5">
          <h3 className="text-sm font-medium text-gray-500 mb-4">定向设置</h3>
          <div className="space-y-3 text-sm">
            <InfoRow label="地区" value={targeting.geo?.join(", ") || "全部"} />
            <InfoRow label="操作系统" value={targeting.os?.join(", ") || "全部"} />
            <InfoRow label="设备" value={targeting.device?.join(", ") || "全部"} />
          </div>
        </div>
      </div>

      {/* Creatives */}
      <div className="mt-6">
        <div className="flex items-center justify-between mb-3">
          <h3 className="text-sm font-medium text-gray-500">素材 ({creatives.length})</h3>
          <button onClick={() => setShowAddCreative(!showAddCreative)}
            className="text-sm px-3 py-1.5 rounded bg-blue-600 text-white hover:bg-blue-700">
            {showAddCreative ? "取消" : "添加素材"}
          </button>
        </div>

        {showAddCreative && (
          <AddCreativeForm campaignId={id} onCreated={() => { loadCreatives(); setShowAddCreative(false); }} />
        )}

        {creatives.length === 0 ? (
          <div className="rounded-lg bg-white p-8 text-center">
            <p className="text-base font-medium mb-2">暂无素材</p>
            <p className="text-sm text-gray-500">Campaign 需要至少一个素材才能启动投放</p>
          </div>
        ) : (
          <div className="rounded-lg bg-white overflow-hidden">
            <table className="w-full text-sm" aria-label="素材列表">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">名称</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">类型</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">尺寸</th>
                  <th className="text-left py-3 px-4 font-medium text-gray-500">状态</th>
                </tr>
              </thead>
              <tbody>
                {creatives.map((cr) => (
                  <tr key={cr.id} className="border-t border-gray-100">
                    <td className="py-3 px-4">{cr.name || `素材 #${cr.id}`}</td>
                    <td className="py-3 px-4 font-geist tabular-nums">{cr.ad_type}</td>
                    <td className="py-3 px-4 font-geist tabular-nums">{cr.size || "—"}</td>
                    <td className="py-3 px-4">
                      <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${
                        cr.status === "approved" ? "bg-green-50 text-green-700" :
                        cr.status === "rejected" ? "bg-red-50 text-red-600" :
                        "bg-yellow-50 text-yellow-700"
                      }`}>{cr.status}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Link to report */}
      <div className="mt-6">
        <Link href={`/reports/${campaign.id}`}
          className="text-sm px-4 py-2 rounded bg-blue-50 text-blue-700 hover:bg-blue-100">
          查看详细报表 →
        </Link>
      </div>
    </div>
  );
}

function AddCreativeForm({ campaignId, onCreated }: { campaignId: number; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [adType, setAdType] = useState("banner");
  const [size, setSize] = useState("300x250");
  const [markup, setMarkup] = useState("");
  const [destUrl, setDestUrl] = useState("https://example.com/landing");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const sizes: Record<string, string[]> = {
    banner: ["300x250", "728x90", "320x50", "300x600"],
    splash: ["1080x1920"],
    interstitial: ["640x960", "640x1136"],
    native: [],
  };

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const formatMap: Record<string, string> = { banner: "banner", native: "native", splash: "banner", interstitial: "banner" };
      await api.createCreative({
        campaign_id: campaignId,
        name: name || `${adType}-${size}`,
        ad_type: adType,
        format: formatMap[adType] || "banner",
        size,
        ad_markup: markup || `<div style="width:${size.split("x")[0]}px;height:${size.split("x")[1]}px;background:#1a1a2e;color:#fff;display:flex;align-items:center;justify-content:center">广告内容</div>`,
        destination_url: destUrl,
      });
      onCreated();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "创建失败");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="rounded-lg bg-white p-5 mb-4">
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label className="block text-xs font-medium text-gray-500 mb-1">素材名称</label>
          <input type="text" value={name} onChange={(e) => setName(e.target.value)}
            placeholder="例: 横幅素材-01" className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm" />
        </div>
        <div>
          <label className="block text-xs font-medium text-gray-500 mb-1">广告类型</label>
          <div className="flex gap-2">
            {["banner", "native", "splash", "interstitial"].map((t) => (
              <button key={t} onClick={() => { setAdType(t); setSize(sizes[t]?.[0] || ""); }}
                className={`px-3 py-2 text-sm rounded-md border ${adType === t ? "border-blue-500 bg-blue-50 text-blue-700" : "border-gray-200 text-gray-600"}`}>
                {t}
              </button>
            ))}
          </div>
        </div>
        {sizes[adType]?.length > 0 && (
          <div>
            <label className="block text-xs font-medium text-gray-500 mb-1">尺寸</label>
            <div className="flex gap-2 flex-wrap">
              {sizes[adType].map((s) => (
                <button key={s} onClick={() => setSize(s)}
                  className={`px-3 py-1.5 text-sm rounded-md border ${size === s ? "border-blue-500 bg-blue-50 text-blue-700" : "border-gray-200 text-gray-600"}`}>
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}
        <div>
          <label className="block text-xs font-medium text-gray-500 mb-1">落地页 URL</label>
          <input type="text" value={destUrl} onChange={(e) => setDestUrl(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm" />
        </div>
      </div>
      <div className="mt-4">
        <label className="block text-xs font-medium text-gray-500 mb-1">广告代码 (HTML)</label>
        <textarea value={markup} onChange={(e) => setMarkup(e.target.value)} rows={3}
          placeholder="留空将自动生成占位素材" className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-geist tabular-nums" />
      </div>
      {error && <p className="text-sm text-red-600 mt-2">{error}</p>}
      <div className="mt-4 flex justify-end">
        <button onClick={handleSubmit} disabled={submitting}
          className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300">
          {submitting ? "创建中..." : "添加素材"}
        </button>
      </div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span className="text-gray-500">{label}</span>
      <span className="font-geist tabular-nums">{value}</span>
    </div>
  );
}
