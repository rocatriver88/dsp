"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";
import { LoadingSkeleton, ErrorState } from "../_components/LoadingState";
import PageHeader from "../_components/PageHeader";

export default function ReportsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(() => {
    api.listCampaigns()
      .then((data) => {
        setCampaigns(data);
        setError(null);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const handleRetry = () => {
    setLoading(true);
    setError(null);
    load();
  };

  return (
    <div>
      <PageHeader title="报表" subtitle="选择一个广告系列查看详细报表和竞价透明度数据" />

      {loading ? (
        <LoadingSkeleton rows={3} />
      ) : error ? (
        <ErrorState message={error} onRetry={handleRetry} />
      ) : campaigns.length === 0 ? (
        <div className="glass-card-static p-12 text-center">
          <p className="text-lg font-medium mb-2" style={{ color: "var(--text-primary)" }}>还没有 Campaign</p>
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>创建并投放 Campaign 后，这里会显示报表数据</p>
        </div>
      ) : (
        <div className="glass-card-static p-0 overflow-hidden">
          <table className="w-full text-sm" aria-label="Campaign 报表列表">
            <thead style={{ background: "var(--bg-card-elevated)" }}>
              <tr>
                <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>Campaign</th>
                <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>状态</th>
                <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>出价 (¥)</th>
                <th className="text-center py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>操作</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((c) => (
                <tr key={c.id} style={{ borderTop: "1px solid var(--border)" }}>
                  <td className="py-3 px-4 font-medium" style={{ color: "var(--text-primary)" }}>{c.name}</td>
                  <td className="py-3 px-4">
                    <span className="inline-block px-2 py-0.5 text-xs font-medium rounded-full"
                      style={
                        c.status === "active" ? { background: "rgba(34,197,94,0.15)", color: "#22C55E" } :
                        c.status === "paused" ? { background: "rgba(234,179,8,0.15)", color: "#EAB308" } :
                        { background: "var(--bg-card-elevated)", color: "var(--text-secondary)" }
                      }>{c.status}</span>
                  </td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-secondary)" }}>
                    {c.billing_model === "cpc"
                      ? `${(c.bid_cpc_cents / 100).toFixed(2)} CPC`
                      : `${(c.bid_cpm_cents / 100).toFixed(2)} CPM`}
                  </td>
                  <td className="py-3 px-4 text-center">
                    <Link href={`/reports/${c.id}`}
                      className="text-sm px-4 py-1.5 rounded-md"
                      style={{ background: "var(--primary-muted)", color: "var(--primary)" }}>
                      查看报表
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
