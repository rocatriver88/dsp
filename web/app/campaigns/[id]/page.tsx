"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { api, Campaign, CampaignStats, Creative } from "@/lib/api";
import { StatCard } from "../../_components/StatCard";
import { StatusBadge } from "../../_components/StatusBadge";

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
    return (
      <div className="animate-pulse">
        <div className="h-6 w-48 rounded mb-4" style={{ background: "var(--bg-card)" }} />
        <div className="h-40 rounded" style={{ background: "var(--bg-card)" }} />
      </div>
    );
  }

  if (error || !campaign) {
    return (
      <div className="rounded-[14px] p-8 text-center" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
        <p className="text-sm" style={{ color: "var(--error)" }}>{error || "Campaign 未找到"}</p>
        <Link href="/campaigns" className="text-sm mt-4 inline-block" style={{ color: "var(--primary)" }}>返回列表</Link>
      </div>
    );
  }

  const targeting = campaign.targeting as { geo?: string[]; os?: string[]; device?: string[] };

  return (
    <div>
      <div className="flex items-center gap-3 mb-6">
        <Link href="/campaigns" className="hover:opacity-80 transition-opacity" style={{ color: "var(--text-muted)" }}>← Campaigns</Link>
        <h2 className="text-2xl font-semibold" style={{ color: "var(--text-primary)" }}>{campaign.name}</h2>
        <StatusBadge status={campaign.status} />
        {campaign.status === "paused" && campaign.pause_reason && (
          <span className="text-xs px-2 py-1 rounded" style={{ color: "var(--warning)", background: "rgba(234, 179, 8, 0.12)" }}>
            自动暂停: {campaign.pause_reason}
          </span>
        )}
        {campaign.status === "draft" && (
          <button onClick={() => handleAction("start")} className="ml-auto text-sm px-4 py-1.5 rounded hover:opacity-80 transition-opacity" style={{ background: "rgba(34, 197, 94, 0.12)", color: "var(--success)" }}>启动</button>
        )}
        {campaign.status === "active" && (
          <button onClick={() => handleAction("pause")} className="ml-auto text-sm px-4 py-1.5 rounded hover:opacity-80 transition-opacity" style={{ background: "rgba(234, 179, 8, 0.12)", color: "var(--warning)" }}>暂停</button>
        )}
        {campaign.status === "paused" && (
          <button onClick={() => handleAction("start")} className="ml-auto text-sm px-4 py-1.5 rounded hover:opacity-80 transition-opacity" style={{ background: "rgba(34, 197, 94, 0.12)", color: "var(--success)" }}>恢复</button>
        )}
      </div>

      {actionError && (
        <div className="mb-4 p-3 rounded-[14px] text-sm flex items-center justify-between" style={{ background: "rgba(239, 68, 68, 0.12)", color: "var(--error)" }}>
          <span>{actionError}</span>
          <button onClick={() => setActionError(null)} className="text-xs ml-4 hover:opacity-80" style={{ color: "var(--error)" }}>关闭</button>
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
        <div className="rounded-[14px] p-5" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
          <h3 className="text-sm font-medium mb-4" style={{ color: "var(--text-muted)" }}>基本信息</h3>
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

        <div className="rounded-[14px] p-5" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
          <h3 className="text-sm font-medium mb-4" style={{ color: "var(--text-muted)" }}>定向设置</h3>
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
          <h3 className="text-sm font-medium" style={{ color: "var(--text-muted)" }}>素材 ({creatives.length})</h3>
          <button onClick={() => setShowAddCreative(!showAddCreative)}
            className="text-sm px-3 py-1.5 rounded text-white hover:opacity-90 transition-opacity"
            style={{ background: "var(--primary)" }}>
            {showAddCreative ? "取消" : "添加素材"}
          </button>
        </div>

        {showAddCreative && (
          <AddCreativeForm campaignId={id} onCreated={() => { loadCreatives(); setShowAddCreative(false); }} />
        )}

        {creatives.length === 0 ? (
          <div className="rounded-[14px] p-8 text-center" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
            <p className="text-base font-medium mb-2" style={{ color: "var(--text-primary)" }}>暂无素材</p>
            <p className="text-sm" style={{ color: "var(--text-muted)" }}>Campaign 需要至少一个素材才能启动投放</p>
          </div>
        ) : (
          <div className="space-y-3">
            {creatives.map((cr) => (
              <CreativeCard key={cr.id} creative={cr} onUpdated={loadCreatives} />
            ))}
          </div>
        )}
      </div>

      {campaign.billing_model === "cpm" && (
        <BidSimulator campaignId={campaign.id} currentBidCPM={campaign.bid_cpm_cents} />
      )}

      {/* Link to report */}
      <div className="mt-6">
        <Link href={`/reports/${campaign.id}`}
          className="text-sm px-4 py-2 rounded hover:opacity-80 transition-opacity"
          style={{ background: "var(--primary-muted)", color: "var(--primary)" }}>
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
    <div className="rounded-[14px] p-5 mb-4" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div>
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>素材名称</label>
          <input type="text" value={name} onChange={(e) => setName(e.target.value)}
            placeholder="例: 横幅素材-01"
            className="w-full px-3 py-2 rounded-md text-sm outline-none focus:ring-2"
            style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
        </div>
        <div>
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>广告类型</label>
          <div className="flex gap-2">
            {["banner", "native", "splash", "interstitial"].map((t) => (
              <button key={t} onClick={() => { setAdType(t); setSize(sizes[t]?.[0] || ""); }}
                className="px-3 py-2 text-sm rounded-md transition-colors"
                style={{
                  background: adType === t ? "var(--primary-muted)" : "var(--bg-card)",
                  border: adType === t ? "1px solid var(--primary)" : "1px solid var(--border)",
                  color: adType === t ? "var(--primary)" : "var(--text-secondary)",
                }}>
                {t}
              </button>
            ))}
          </div>
        </div>
        {sizes[adType]?.length > 0 && (
          <div>
            <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>尺寸</label>
            <div className="flex gap-2 flex-wrap">
              {sizes[adType].map((s) => (
                <button key={s} onClick={() => setSize(s)}
                  className="px-3 py-1.5 text-sm rounded-md transition-colors"
                  style={{
                    background: size === s ? "var(--primary)" : "var(--bg-card)",
                    border: size === s ? "1px solid var(--primary)" : "1px solid var(--border)",
                    color: size === s ? "#fff" : "var(--text-secondary)",
                  }}>
                  {s}
                </button>
              ))}
            </div>
          </div>
        )}
        <div>
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>落地页 URL</label>
          <input type="text" value={destUrl} onChange={(e) => setDestUrl(e.target.value)}
            className="w-full px-3 py-2 rounded-md text-sm outline-none focus:ring-2"
            style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
        </div>
      </div>

      {/* Mode toggle: image upload vs HTML */}
      <div className="mt-4 flex gap-2 mb-3">
        <button onClick={() => setMode("image")}
          className="px-3 py-1.5 text-sm rounded-md transition-colors"
          style={{
            background: mode === "image" ? "var(--primary-muted)" : "var(--bg-card)",
            border: mode === "image" ? "1px solid var(--primary)" : "1px solid var(--border)",
            color: mode === "image" ? "var(--primary)" : "var(--text-secondary)",
          }}>
          上传图片
        </button>
        <button onClick={() => setMode("html")}
          className="px-3 py-1.5 text-sm rounded-md transition-colors"
          style={{
            background: mode === "html" ? "var(--primary-muted)" : "var(--bg-card)",
            border: mode === "html" ? "1px solid var(--primary)" : "1px solid var(--border)",
            color: mode === "html" ? "var(--primary)" : "var(--text-secondary)",
          }}>
          HTML 代码
        </button>
      </div>

      {mode === "image" ? (
        <div>
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>广告图片</label>
          <div className="rounded-lg p-4" style={{ border: "2px dashed var(--border)" }}>
            {imageUrl ? (
              <div className="flex items-center gap-4">
                <img src={imageUrl} alt="preview" className="max-h-24 rounded" />
                <div className="flex-1">
                  <p className="text-sm mb-1" style={{ color: "var(--success)" }}>上传成功</p>
                  <p className="text-xs break-all" style={{ color: "var(--text-muted)" }}>{imageUrl}</p>
                  <button onClick={() => setImageUrl("")} className="text-xs mt-1" style={{ color: "var(--error)" }}>移除</button>
                </div>
              </div>
            ) : (
              <label className="flex flex-col items-center cursor-pointer py-4">
                <span className="text-sm mb-2" style={{ color: "var(--text-muted)" }}>{uploading ? "上传中..." : "点击选择图片或拖拽到此处"}</span>
                <span className="text-xs" style={{ color: "var(--text-muted)" }}>支持 JPG, PNG, GIF, WebP, SVG (最大 10MB)</span>
                <input type="file" accept="image/*" onChange={handleUpload} disabled={uploading}
                  className="hidden" />
              </label>
            )}
          </div>
        </div>
      ) : (
        <div>
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>广告代码 (HTML)</label>
          <textarea value={markup} onChange={(e) => setMarkup(e.target.value)} rows={3}
            placeholder="留空将自动生成占位素材"
            className="w-full px-3 py-2 rounded-md text-sm tabular-nums outline-none focus:ring-2"
            style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
        </div>
      )}

      {error && <p className="text-sm mt-2" style={{ color: "var(--error)" }}>{error}</p>}
      <div className="mt-4 flex justify-end">
        <button onClick={handleSubmit} disabled={submitting || (mode === "image" && uploading)}
          className="px-6 py-2 text-sm font-medium text-white rounded-md disabled:opacity-50 hover:opacity-90 transition-opacity"
          style={{ background: "var(--primary)" }}>
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
    <div className="rounded-[14px] overflow-hidden" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      {/* Summary row */}
      <div className="flex items-center px-4 py-3 cursor-pointer hover:opacity-90 transition-opacity" onClick={() => setExpanded(!expanded)}>
        {previewImgUrl && (
          <img src={previewImgUrl} alt={cr.name} className="w-12 h-12 object-cover rounded mr-3 flex-shrink-0" />
        )}
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium truncate" style={{ color: "var(--text-primary)" }}>{cr.name || `素材 #${cr.id}`}</p>
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>{cr.ad_type} · {cr.size || "—"}</p>
        </div>
        <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full mr-3`}
          style={{
            background: cr.status === "approved" ? "rgba(34, 197, 94, 0.12)" :
              cr.status === "rejected" ? "rgba(239, 68, 68, 0.12)" :
              "rgba(234, 179, 8, 0.12)",
            color: cr.status === "approved" ? "var(--success)" :
              cr.status === "rejected" ? "var(--error)" :
              "var(--warning)",
          }}>{cr.status}</span>
        <span className="text-xs" style={{ color: "var(--text-muted)" }}>{expanded ? "收起" : "展开"}</span>
      </div>

      {/* Expanded detail */}
      {expanded && (
        <div className="px-4 py-4" style={{ borderTop: "1px solid var(--border-subtle)" }}>
          {/* Preview */}
          <div className="mb-4">
            <p className="text-xs font-medium mb-2" style={{ color: "var(--text-muted)" }}>素材预览</p>
            {previewImgUrl ? (
              <div className="p-3 rounded inline-block" style={{ background: "var(--bg-card-elevated)" }}>
                <img src={previewImgUrl} alt={cr.name} className="max-w-full max-h-60 rounded" />
              </div>
            ) : cr.ad_markup ? (
              <div className="p-3 rounded" style={{ background: "var(--bg-card-elevated)" }}>
                <div className="text-xs mb-1" style={{ color: "var(--text-muted)" }}>HTML 预览</div>
                <iframe
                  sandbox=""
                  srcDoc={cr.ad_markup}
                  className="rounded overflow-hidden"
                  style={{ border: "1px solid var(--border)", width: cr.size?.split("x")[0] ? `${Math.min(Number(cr.size.split("x")[0]), 600)}px` : "300px", height: cr.size?.split("x")[1] ? `${Math.min(Number(cr.size.split("x")[1]), 300)}px` : "250px" }}
                  title={`素材预览: ${cr.name}`}
                />
              </div>
            ) : (
              <p className="text-sm" style={{ color: "var(--text-muted)" }}>无预览内容</p>
            )}
          </div>

          {/* Info */}
          <div className="grid grid-cols-2 gap-3 text-sm mb-4">
            <div><span style={{ color: "var(--text-muted)" }}>落地页:</span> <span className="break-all" style={{ color: "var(--primary)" }}>{cr.destination_url}</span></div>
            <div><span style={{ color: "var(--text-muted)" }}>格式:</span> <span className="tabular-nums" style={{ color: "var(--text-secondary)" }}>{cr.format} · {cr.size}</span></div>
          </div>

          {/* Edit mode */}
          {editing ? (
            <div className="space-y-3 pt-4" style={{ borderTop: "1px solid var(--border-subtle)" }}>
              <div>
                <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>素材名称</label>
                <input type="text" value={editName} onChange={(e) => setEditName(e.target.value)}
                  className="w-full px-3 py-2 rounded-md text-sm outline-none focus:ring-2"
                  style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>广告代码 (HTML)</label>
                <textarea value={editMarkup} onChange={(e) => setEditMarkup(e.target.value)} rows={4}
                  className="w-full px-3 py-2 rounded-md text-sm tabular-nums outline-none focus:ring-2"
                  style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>落地页 URL</label>
                <input type="text" value={editDestUrl} onChange={(e) => setEditDestUrl(e.target.value)}
                  className="w-full px-3 py-2 rounded-md text-sm outline-none focus:ring-2"
                  style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)", "--tw-ring-color": "var(--primary)" } as React.CSSProperties} />
              </div>
              {error && <p className="text-sm" style={{ color: "var(--error)" }}>{error}</p>}
              <div className="flex gap-2 justify-end">
                <button onClick={() => setEditing(false)}
                  className="px-4 py-1.5 text-sm rounded-md hover:opacity-80 transition-opacity"
                  style={{ background: "transparent", border: "1px solid var(--border)", color: "var(--text-secondary)" }}>
                  取消
                </button>
                <button onClick={handleSave} disabled={saving}
                  className="px-4 py-1.5 text-sm font-medium text-white rounded-md disabled:opacity-50 hover:opacity-90 transition-opacity"
                  style={{ background: "var(--primary)" }}>
                  {saving ? "保存中..." : "保存"}
                </button>
              </div>
            </div>
          ) : (
            <div className="flex items-center gap-4">
              <button onClick={() => setEditing(true)}
                className="text-sm hover:opacity-80 transition-opacity"
                style={{ color: "var(--primary)" }}>
                编辑素材
              </button>
              <button onClick={async () => {
                if (!confirm("确定删除该素材？")) return;
                try { await api.deleteCreative(cr.id); onUpdated(); }
                catch (e: unknown) { setError(e instanceof Error ? e.message : "删除失败"); }
              }} className="text-sm hover:opacity-80 transition-opacity" style={{ color: "var(--error)" }}>
                删除
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function BidSimulator({ campaignId, currentBidCPM }: { campaignId: number; currentBidCPM: number }) {
  const [simBid, setSimBid] = useState(currentBidCPM);
  const [result, setResult] = useState<{
    current_win_rate: number;
    simulated_win_rate: number;
    simulated_wins: number;
    simulated_spend_cents: number;
    total_bids: number;
  } | null>(null);
  const [loading, setLoading] = useState(false);

  const runSimulation = useCallback(() => {
    if (simBid <= 0) return;
    setLoading(true);
    api.simulateBid(campaignId, simBid)
      .then(setResult)
      .catch(() => setResult(null))
      .finally(() => setLoading(false));
  }, [campaignId, simBid]);

  useEffect(() => {
    const timer = setTimeout(runSimulation, 500);
    return () => clearTimeout(timer);
  }, [runSimulation]);

  const winDelta = result ? result.simulated_win_rate - result.current_win_rate : 0;

  return (
    <div className="rounded-[14px] p-5 mt-6" style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <h3 className="text-sm font-semibold mb-4" style={{ color: "var(--text-primary)" }}>出价模拟器</h3>
      <div className="flex items-center gap-4 mb-4">
        <label className="text-sm flex-shrink-0" style={{ color: "var(--text-muted)" }}>模拟 CPM</label>
        <input
          type="range"
          min={100} max={Math.max(currentBidCPM * 3, 2000)} step={50}
          value={simBid}
          onChange={(e) => setSimBid(Number(e.target.value))}
          className="flex-1"
          style={{ accentColor: "var(--primary)" }}
        />
        <span className="text-sm tabular-nums w-20 text-right" style={{ color: "var(--text-primary)" }}>
          ¥{(simBid / 100).toFixed(2)}
        </span>
      </div>
      {loading ? (
        <p className="text-xs" style={{ color: "var(--text-muted)" }}>计算中...</p>
      ) : result && result.total_bids > 0 ? (
        <div className="grid grid-cols-3 gap-4 text-center">
          <div>
            <p className="text-xs mb-1" style={{ color: "var(--text-muted)" }}>预估胜率</p>
            <p className="text-lg tabular-nums font-semibold" style={{ color: "var(--text-primary)" }}>
              {(result.simulated_win_rate * 100).toFixed(1)}%
            </p>
            <p className="text-xs" style={{ color: winDelta >= 0 ? "var(--success)" : "var(--error)" }}>
              {winDelta >= 0 ? "+" : ""}{(winDelta * 100).toFixed(1)}%
            </p>
          </div>
          <div>
            <p className="text-xs mb-1" style={{ color: "var(--text-muted)" }}>预估曝光</p>
            <p className="text-lg tabular-nums font-semibold" style={{ color: "var(--text-primary)" }}>
              {result.simulated_wins.toLocaleString()}
            </p>
            <p className="text-xs" style={{ color: "var(--text-muted)" }}>/ {result.total_bids.toLocaleString()} 竞价</p>
          </div>
          <div>
            <p className="text-xs mb-1" style={{ color: "var(--text-muted)" }}>预估花费</p>
            <p className="text-lg tabular-nums font-semibold" style={{ color: "var(--text-primary)" }}>
              ¥{(result.simulated_spend_cents / 100).toFixed(2)}
            </p>
            <p className="text-xs" style={{ color: "var(--text-muted)" }}>过去 7 天</p>
          </div>
        </div>
      ) : (
        <p className="text-xs" style={{ color: "var(--text-muted)" }}>暂无历史竞价数据，投放后可使用模拟器</p>
      )}
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span style={{ color: "var(--text-muted)" }}>{label}</span>
      <span className="tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</span>
    </div>
  );
}
