"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, InviteCode } from "@/lib/admin-api";

export default function InvitesPage() {
  const [codes, setCodes] = useState<InviteCode[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [generating, setGenerating] = useState(false);
  const [genError, setGenError] = useState<string | null>(null);
  const [newCode, setNewCode] = useState<{ code: string } | null>(null);
  const [maxUses, setMaxUses] = useState<number>(1);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    adminApi
      .listInviteCodes()
      .then((data) => setCodes(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleGenerate() {
    setGenerating(true);
    setGenError(null);
    setNewCode(null);
    try {
      const result = await adminApi.createInviteCode(maxUses);
      setNewCode(result);
      load();
    } catch (e: unknown) {
      setGenError(e instanceof Error ? e.message : "生成失败");
    } finally {
      setGenerating(false);
    }
  }

  return (
    <div className="p-8 max-w-6xl">
      <h2 className="text-2xl font-semibold mb-6">邀请码管理</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {/* Create Section */}
      <div className="bg-white rounded-lg p-6 mb-6">
        <h3 className="text-sm font-semibold text-gray-700 mb-4">生成邀请码</h3>

        <div className="flex items-end gap-3">
          <div>
            <label className="block text-xs text-gray-500 mb-1">最大使用次数</label>
            <input
              type="number"
              min={1}
              value={maxUses}
              onChange={(e) => setMaxUses(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="w-24 text-sm border border-gray-200 rounded px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-blue-300"
            />
          </div>
          <button
            onClick={handleGenerate}
            disabled={generating}
            className="px-4 py-2 text-sm font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed transition-colors"
          >
            {generating ? "生成中..." : "生成邀请码"}
          </button>
        </div>

        {genError && (
          <p className="text-sm text-red-600 mt-3">{genError}</p>
        )}

        {newCode && (
          <div className="mt-4 px-4 py-3 rounded bg-green-50 border border-green-200">
            <p className="text-xs text-green-700 mb-1">新邀请码已生成</p>
            <p className="font-mono text-lg font-semibold text-green-800 tracking-widest">
              {newCode.code}
            </p>
          </div>
        )}
      </div>

      {/* Codes Table */}
      <div>
        <h3 className="text-sm font-semibold text-gray-700 mb-3">所有邀请码</h3>
        {loading ? (
          <div className="bg-white rounded-lg p-6 animate-pulse space-y-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-10 bg-gray-100 rounded" />
            ))}
          </div>
        ) : codes.length === 0 ? (
          <div className="bg-white rounded-lg p-12 text-center">
            <p className="text-sm text-gray-500">暂无邀请码，点击上方按钮生成</p>
          </div>
        ) : (
          <div className="bg-white rounded-lg overflow-hidden">
            <table className="w-full text-sm" aria-label="邀请码列表">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">邀请码</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">状态</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">使用量</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">创建人</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">创建时间</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">过期时间</th>
                </tr>
              </thead>
              <tbody>
                {codes.map((c) => {
                  const exhausted = c.used_count >= c.max_uses;
                  return (
                    <tr key={c.id} className="border-b last:border-0 border-gray-100">
                      <td className="py-3 px-4">
                        <span className="font-mono text-sm text-gray-900">{c.code}</span>
                      </td>
                      <td className="py-3 px-4">
                        <span
                          className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${
                            exhausted
                              ? "bg-gray-100 text-gray-500"
                              : "bg-green-50 text-green-700"
                          }`}
                        >
                          {exhausted ? "已用完" : "可用"}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                        {c.used_count} / {c.max_uses}
                      </td>
                      <td className="py-3 px-4 text-xs text-gray-500">
                        {c.created_by || "—"}
                      </td>
                      <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                        {new Date(c.created_at).toLocaleString("zh-CN")}
                      </td>
                      <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                        {c.expires_at ? new Date(c.expires_at).toLocaleString("zh-CN") : "—"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}
