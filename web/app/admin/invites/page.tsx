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
      <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>邀请码管理</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded text-sm flex items-center justify-between" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {/* Create Section */}
      <div className="glass-card-static p-6 mb-6">
        <h3 className="text-sm font-semibold mb-4" style={{ color: "var(--text-primary)" }}>生成邀请码</h3>

        <div className="flex items-end gap-3">
          <div>
            <label className="block text-xs mb-1" style={{ color: "var(--text-secondary)" }}>最大使用次数</label>
            <input
              type="number"
              min={1}
              value={maxUses}
              onChange={(e) => setMaxUses(Math.max(1, parseInt(e.target.value, 10) || 1))}
              className="w-24 text-sm rounded px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-purple-500"
              style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
            />
          </div>
          <button
            onClick={handleGenerate}
            disabled={generating}
            className="px-4 py-2 text-sm font-medium rounded text-white disabled:cursor-not-allowed transition-colors disabled:opacity-50"
            style={{ background: generating ? "var(--border)" : "var(--primary)" }}
          >
            {generating ? "生成中..." : "生成邀请码"}
          </button>
        </div>

        {genError && (
          <p className="text-sm mt-3" style={{ color: "#EF4444" }}>{genError}</p>
        )}

        {newCode && (
          <div className="mt-4 px-4 py-3 rounded" style={{ background: "rgba(34,197,94,0.15)", border: "1px solid rgba(34,197,94,0.3)" }}>
            <p className="text-xs mb-1" style={{ color: "#22C55E" }}>新邀请码已生成</p>
            <p className="font-mono text-lg font-semibold tracking-widest" style={{ color: "#22C55E" }}>
              {newCode.code}
            </p>
          </div>
        )}
      </div>

      {/* Codes Table */}
      <div>
        <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text-primary)" }}>所有邀请码</h3>
        {loading ? (
          <div className="glass-card-static p-6 animate-pulse space-y-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
            ))}
          </div>
        ) : codes.length === 0 ? (
          <div className="glass-card-static p-12 text-center">
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无邀请码，点击上方按钮生成</p>
          </div>
        ) : (
          <div className="glass-card-static p-0 overflow-hidden">
            <div className="overflow-x-auto">
            <table className="w-full text-sm table-fixed" aria-label="邀请码列表">
              <thead style={{ background: "var(--bg-card-elevated)" }}>
                <tr>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>邀请码</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>状态</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>使用量</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>创建人</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>创建时间</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>过期时间</th>
                </tr>
              </thead>
              <tbody>
                {codes.map((c) => {
                  const exhausted = c.used_count >= c.max_uses;
                  return (
                    <tr key={c.id} className="transition-colors" style={{ borderTop: "1px solid var(--border-subtle)" }} onMouseEnter={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }} onMouseLeave={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "transparent"; }}>
                      <td className="py-3 px-4">
                        <span className="font-mono text-sm" style={{ color: "var(--text-primary)" }}>{c.code}</span>
                      </td>
                      <td className="py-3 px-4">
                        <span
                          className="inline-block px-2 py-0.5 text-xs font-medium rounded-full"
                          style={
                            exhausted
                              ? { background: "var(--bg-card-elevated)", color: "var(--text-muted)" }
                              : { background: "rgba(34,197,94,0.15)", color: "#22C55E" }
                          }
                        >
                          {exhausted ? "已用完" : "可用"}
                        </span>
                      </td>
                      <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                        {c.used_count} / {c.max_uses}
                      </td>
                      <td className="py-3 px-4 text-xs" style={{ color: "var(--text-muted)" }}>
                        {c.created_by || "\u2014"}
                      </td>
                      <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                        {new Date(c.created_at).toLocaleString("zh-CN")}
                      </td>
                      <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                        {c.expires_at ? new Date(c.expires_at).toLocaleString("zh-CN") : "\u2014"}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
