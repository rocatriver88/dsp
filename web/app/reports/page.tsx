"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { api, Campaign } from "@/lib/api";

export default function ReportsPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api.listCampaigns()
      .then(setCampaigns)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  return (
    <div>
      <h2 className="text-xl font-semibold mb-6">报表</h2>
      <p className="text-sm text-gray-500 mb-6">选择一个 Campaign 查看详细报表和 bid 透明度数据</p>

      {loading ? (
        <div className="animate-pulse space-y-3">
          {[1, 2, 3].map((i) => <div key={i} className="h-14 bg-gray-100 rounded" />)}
        </div>
      ) : campaigns.length === 0 ? (
        <div className="rounded-lg border border-gray-200 bg-white p-12 text-center">
          <p className="text-lg font-medium mb-2">还没有 Campaign</p>
          <p className="text-sm text-gray-500">创建并投放 Campaign 后，这里会显示报表数据</p>
        </div>
      ) : (
        <div className="rounded-lg border border-gray-200 bg-white overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-gray-50">
              <tr>
                <th className="text-left py-3 px-4 font-medium text-gray-500">Campaign</th>
                <th className="text-left py-3 px-4 font-medium text-gray-500">状态</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">CPM (¥)</th>
                <th className="text-center py-3 px-4 font-medium text-gray-500">操作</th>
              </tr>
            </thead>
            <tbody>
              {campaigns.map((c) => (
                <tr key={c.id} className="border-t border-gray-100 hover:bg-gray-50">
                  <td className="py-3 px-4 font-medium">{c.name}</td>
                  <td className="py-3 px-4">
                    <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${
                      c.status === "active" ? "bg-green-50 text-green-700" :
                      c.status === "paused" ? "bg-yellow-50 text-yellow-700" :
                      "bg-gray-100 text-gray-600"
                    }`}>{c.status}</span>
                  </td>
                  <td className="py-3 px-4 text-right font-mono">{(c.bid_cpm_cents / 100).toFixed(2)}</td>
                  <td className="py-3 px-4 text-center">
                    <Link href={`/reports/${c.id}`}
                      className="text-sm px-4 py-1.5 rounded bg-blue-50 text-blue-700 hover:bg-blue-100">
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
