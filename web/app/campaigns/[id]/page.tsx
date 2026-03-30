"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import { api, Campaign, CampaignStats } from "@/lib/api";
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

  useEffect(() => {
    Promise.all([
      api.getCampaign(id),
      api.getCampaignStats(id).catch(() => null),
    ])
      .then(([c, s]) => { setCampaign(c); setStats(s); })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
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

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between">
      <span className="text-gray-500">{label}</span>
      <span className="font-geist tabular-nums">{value}</span>
    </div>
  );
}
