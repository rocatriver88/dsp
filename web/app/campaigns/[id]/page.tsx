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
          <div className="space-y-3">
            {creatives.map((cr) => (
              <CreativeCard key={cr.id} creative={cr} onUpdated={loadCreatives} />
            ))}
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
  const [imageUrl, setImageUrl] = useState("");
  const [uploading, setUploading] = useState(false);
  const [mode, setMode] = useState<"image" | "html">("image");

  const sizes: Record<string, string[]> = {
    banner: ["300x250", "728x90", "320x50", "300x600"],
    splash: ["1080x1920"],
    interstitial: ["640x960", "640x1136"],
    native: [],
  };

  const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

  const handleUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setUploading(true);
    setError(null);
    try {
      const result = await api.uploadFile(file);
      setImageUrl(`${API_BASE}${result.url}`);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "上传失败");
    } finally {
      setUploading(false);
    }
  };

  const handleSubmit = async () => {
    setSubmitting(true);
    setError(null);
    try {
      const formatMap: Record<string, string> = { banner: "banner", native: "native", splash: "banner", interstitial: "banner" };
      const [w, h] = size.split("x");
      let finalMarkup = markup;
      if (mode === "image" && imageUrl) {
        finalMarkup = `<a href="${destUrl}" target="_blank"><img src="${imageUrl}" width="${w}" height="${h}" alt="${name || "ad"}" style="display:block;width:${w}px;height:${h}px;object-fit:cover" /></a>`;
      } else if (!finalMarkup) {
        finalMarkup = `<div style="width:${w}px;height:${h}px;background:#1a1a2e;color:#fff;display:flex;align-items:center;justify-content:center">广告内容</div>`;
      }
      await api.createCreative({
        campaign_id: campaignId,
        name: name || `${adType}-${size}`,
        ad_type: adType,
        format: formatMap[adType] || "banner",
        size,
        ad_markup: finalMarkup,
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

      {/* Mode toggle: image upload vs HTML */}
      <div className="mt-4 flex gap-2 mb-3">
        <button onClick={() => setMode("image")}
          className={`px-3 py-1.5 text-sm rounded-md border ${mode === "image" ? "border-blue-500 bg-blue-50 text-blue-700" : "border-gray-200 text-gray-600"}`}>
          上传图片
        </button>
        <button onClick={() => setMode("html")}
          className={`px-3 py-1.5 text-sm rounded-md border ${mode === "html" ? "border-blue-500 bg-blue-50 text-blue-700" : "border-gray-200 text-gray-600"}`}>
          HTML 代码
        </button>
      </div>

      {mode === "image" ? (
        <div>
          <label className="block text-xs font-medium text-gray-500 mb-1">广告图片</label>
          <div className="border-2 border-dashed border-gray-300 rounded-lg p-4">
            {imageUrl ? (
              <div className="flex items-center gap-4">
                <img src={imageUrl} alt="preview" className="max-h-24 rounded" />
                <div className="flex-1">
                  <p className="text-sm text-green-600 mb-1">上传成功</p>
                  <p className="text-xs text-gray-400 break-all">{imageUrl}</p>
                  <button onClick={() => setImageUrl("")} className="text-xs text-red-500 mt-1">移除</button>
                </div>
              </div>
            ) : (
              <label className="flex flex-col items-center cursor-pointer py-4">
                <span className="text-sm text-gray-500 mb-2">{uploading ? "上传中..." : "点击选择图片或拖拽到此处"}</span>
                <span className="text-xs text-gray-400">支持 JPG, PNG, GIF, WebP, SVG (最大 10MB)</span>
                <input type="file" accept="image/*" onChange={handleUpload} disabled={uploading}
                  className="hidden" />
              </label>
            )}
          </div>
        </div>
      ) : (
        <div>
          <label className="block text-xs font-medium text-gray-500 mb-1">广告代码 (HTML)</label>
          <textarea value={markup} onChange={(e) => setMarkup(e.target.value)} rows={3}
            placeholder="留空将自动生成占位素材" className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-geist tabular-nums" />
        </div>
      )}

      {error && <p className="text-sm text-red-600 mt-2">{error}</p>}
      <div className="mt-4 flex justify-end">
        <button onClick={handleSubmit} disabled={submitting || (mode === "image" && uploading)}
          className="px-6 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300">
          {submitting ? "创建中..." : "添加素材"}
        </button>
      </div>
    </div>
  );
}

function CreativeCard({ creative: cr, onUpdated }: { creative: Creative; onUpdated: () => void }) {
  const [expanded, setExpanded] = useState(false);
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(cr.name);
  const [editMarkup, setEditMarkup] = useState(cr.ad_markup);
  const [editDestUrl, setEditDestUrl] = useState(cr.destination_url);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Try to extract image src from ad_markup for preview
  const imgMatch = cr.ad_markup?.match(/src="([^"]+)"/);
  const previewImgUrl = imgMatch?.[1] || null;

  const handleSave = async () => {
    setSaving(true);
    setError(null);
    try {
      await api.updateCreative(cr.id, {
        name: editName,
        ad_type: cr.ad_type,
        format: cr.format,
        size: cr.size,
        ad_markup: editMarkup,
        destination_url: editDestUrl,
      });
      setEditing(false);
      onUpdated();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "保存失败");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="rounded-lg bg-white overflow-hidden">
      {/* Summary row */}
      <div className="flex items-center px-4 py-3 cursor-pointer hover:bg-gray-50" onClick={() => setExpanded(!expanded)}>
        {previewImgUrl && (
          <img src={previewImgUrl} alt={cr.name} className="w-12 h-12 object-cover rounded mr-3 flex-shrink-0" />
        )}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate">{cr.name || `素材 #${cr.id}`}</p>
          <p className="text-xs text-gray-400">{cr.ad_type} · {cr.size || "—"}</p>
        </div>
        <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full mr-3 ${
          cr.status === "approved" ? "bg-green-50 text-green-700" :
          cr.status === "rejected" ? "bg-red-50 text-red-600" :
          "bg-yellow-50 text-yellow-700"
        }`}>{cr.status}</span>
        <span className="text-gray-400 text-xs">{expanded ? "收起" : "展开"}</span>
      </div>

      {/* Expanded detail */}
      {expanded && (
        <div className="border-t border-gray-100 px-4 py-4">
          {/* Preview */}
          <div className="mb-4">
            <p className="text-xs font-medium text-gray-500 mb-2">素材预览</p>
            {previewImgUrl ? (
              <div className="bg-gray-50 p-3 rounded inline-block">
                <img src={previewImgUrl} alt={cr.name} className="max-w-full max-h-60 rounded" />
              </div>
            ) : cr.ad_markup ? (
              <div className="bg-gray-50 p-3 rounded">
                <div className="text-xs text-gray-400 mb-1">HTML 预览</div>
                <div className="border border-gray-200 rounded overflow-hidden inline-block"
                  dangerouslySetInnerHTML={{ __html: cr.ad_markup }} />
              </div>
            ) : (
              <p className="text-sm text-gray-400">无预览内容</p>
            )}
          </div>

          {/* Info */}
          <div className="grid grid-cols-2 gap-3 text-sm mb-4">
            <div><span className="text-gray-500">落地页:</span> <span className="text-blue-600 break-all">{cr.destination_url}</span></div>
            <div><span className="text-gray-500">格式:</span> <span className="font-geist tabular-nums">{cr.format} · {cr.size}</span></div>
          </div>

          {/* Edit mode */}
          {editing ? (
            <div className="space-y-3 border-t border-gray-100 pt-4">
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">素材名称</label>
                <input type="text" value={editName} onChange={(e) => setEditName(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm" />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">广告代码 (HTML)</label>
                <textarea value={editMarkup} onChange={(e) => setEditMarkup(e.target.value)} rows={4}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-geist tabular-nums" />
              </div>
              <div>
                <label className="block text-xs font-medium text-gray-500 mb-1">落地页 URL</label>
                <input type="text" value={editDestUrl} onChange={(e) => setEditDestUrl(e.target.value)}
                  className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm" />
              </div>
              {error && <p className="text-sm text-red-600">{error}</p>}
              <div className="flex gap-2 justify-end">
                <button onClick={() => setEditing(false)} className="px-4 py-1.5 text-sm rounded-md border border-gray-200 text-gray-600 hover:bg-gray-50">取消</button>
                <button onClick={handleSave} disabled={saving}
                  className="px-4 py-1.5 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300">
                  {saving ? "保存中..." : "保存"}
                </button>
              </div>
            </div>
          ) : (
            <button onClick={() => setEditing(true)}
              className="text-sm text-blue-600 hover:text-blue-700">
              编辑素材
            </button>
          )}
        </div>
      )}
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
