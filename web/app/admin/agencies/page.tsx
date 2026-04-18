"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, AdminAdvertiser, Registration } from "@/lib/admin-api";

function TopUpModal({
  advertiser,
  onClose,
  onSuccess,
}: {
  advertiser: AdminAdvertiser;
  onClose: () => void;
  onSuccess: () => void;
}) {
  const [amount, setAmount] = useState("");
  const [description, setDescription] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleConfirm() {
    const amountNum = Number(amount);
    if (!amountNum || amountNum <= 0) {
      setError("请输入有效金额");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      await adminApi.topUp(advertiser.id, Math.round(amountNum * 100), description || undefined);
      onSuccess();
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "充值失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "rgba(0,0,0,0.6)" }}
      role="dialog"
      aria-modal="true"
      aria-label="充值"
    >
      <div className="rounded-[14px] p-6 w-full max-w-sm shadow-lg" style={{ background: "var(--bg-card-elevated)" }}>
        <h3 className="text-base font-semibold mb-1" style={{ color: "var(--text-primary)" }}>充值</h3>
        <p className="text-xs mb-4" style={{ color: "var(--text-secondary)" }}>
          {advertiser.company_name} · 当前余额 ¥{(advertiser.balance_cents / 100).toLocaleString()}
        </p>

        <div className="mb-3">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>充值金额（元）</label>
          <input
            type="number"
            min="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="例: 1000"
            autoFocus
            className="w-full px-3 py-2 rounded-md text-sm tabular-nums focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          />
        </div>

        <div className="mb-4">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>备注（可选）</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="例: 4月预算"
            className="w-full px-3 py-2 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          />
        </div>

        {error && <p className="text-sm mb-3" style={{ color: "#EF4444" }}>{error}</p>}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs font-medium rounded transition-colors"
            style={{ background: "var(--bg-card)", color: "var(--text-primary)" }}
          >
            取消
          </button>
          <button
            onClick={handleConfirm}
            disabled={loading || !amount}
            className="px-3 py-1.5 text-xs font-medium rounded text-white disabled:cursor-not-allowed transition-colors disabled:opacity-50"
            style={{ background: loading || !amount ? "var(--border)" : "var(--primary)" }}
          >
            {loading ? "处理中..." : "确认充值"}
          </button>
        </div>
      </div>
    </div>
  );
}

export default function AgenciesPage() {
  const [advertisers, setAdvertisers] = useState<AdminAdvertiser[]>([]);
  const [registrations, setRegistrations] = useState<Registration[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [topUpTarget, setTopUpTarget] = useState<AdminAdvertiser | null>(null);
  const [topUpSuccess, setTopUpSuccess] = useState<number | null>(null);
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    Promise.all([
      adminApi.listAdvertisers(),
      adminApi.listRegistrations("pending"),
    ])
      .then(([advs, regs]) => {
        setAdvertisers(Array.isArray(advs) ? advs : []);
        setRegistrations(Array.isArray(regs) ? regs : []);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleApprove(reg: Registration) {
    setActionLoading(reg.id);
    setActionError(null);
    try {
      await adminApi.approveRegistration(reg.id);
      setRegistrations((prev) => prev.filter((r) => r.id !== reg.id));
      load(); // reload to pick up new advertiser
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleReject(reg: Registration) {
    const reason = prompt("拒绝原因:");
    if (reason === null) return; // cancelled
    setActionLoading(reg.id);
    setActionError(null);
    try {
      await adminApi.rejectRegistration(reg.id, reason);
      setRegistrations((prev) => prev.filter((r) => r.id !== reg.id));
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    } finally {
      setActionLoading(null);
    }
  }

  return (
    <div className="">
      <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>广告主管理</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded text-sm flex items-center justify-between" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {actionError && (
        <div className="mb-4 px-4 py-3 rounded text-sm" style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}>
          {actionError}
        </div>
      )}

      {/* Pending Registrations */}
      {!loading && registrations.length > 0 && (
        <section className="mb-8">
          <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text-primary)" }}>
            待审核注册
            <span className="ml-2 px-2 py-0.5 text-xs font-medium rounded-full" style={{ background: "rgba(234,179,8,0.15)", color: "#EAB308" }}>
              {registrations.length}
            </span>
          </h3>
          <div className="glass-card-static p-0 overflow-x-auto">
            <table className="w-full text-sm" aria-label="待审核注册">
              <thead style={{ background: "var(--bg-card-elevated)" }}>
                <tr>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>公司名称</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>邮箱</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>邀请码</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>申请时间</th>
                  <th className="py-3 px-4" />
                </tr>
              </thead>
              <tbody>
                {registrations.map((reg) => (
                  <tr key={reg.id} className="transition-colors" style={{ borderTop: "1px solid var(--border-subtle)" }} onMouseEnter={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }} onMouseLeave={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "transparent"; }}>
                    <td className="py-3 px-4 font-medium max-w-[200px] truncate" style={{ color: "var(--text-primary)" }} title={reg.company_name}>{reg.company_name}</td>
                    <td className="py-3 px-4 max-w-[240px] truncate" style={{ color: "var(--text-secondary)" }} title={reg.contact_email}>{reg.contact_email}</td>
                    <td className="py-3 px-4 font-mono text-xs" style={{ color: "var(--text-muted)" }}>{reg.invite_code}</td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {new Date(reg.created_at).toLocaleString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-2 justify-end">
                        <button
                          onClick={() => handleApprove(reg)}
                          disabled={actionLoading === reg.id}
                          className="px-3 py-1.5 text-xs font-medium rounded disabled:opacity-50 transition-colors"
                          style={{ background: "rgba(34,197,94,0.15)", color: "#22C55E" }}
                        >
                          批准
                        </button>
                        <button
                          onClick={() => handleReject(reg)}
                          disabled={actionLoading === reg.id}
                          className="px-3 py-1.5 text-xs font-medium rounded disabled:opacity-50 transition-colors"
                          style={{ background: "rgba(239,68,68,0.15)", color: "#EF4444" }}
                        >
                          拒绝
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {/* Advertiser List */}
      <section>
        <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text-primary)" }}>广告主列表</h3>
        {loading ? (
          <div className="glass-card-static p-6 animate-pulse space-y-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
            ))}
          </div>
        ) : advertisers.length === 0 ? (
          <div className="glass-card-static p-12 text-center">
            <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无广告主</p>
          </div>
        ) : (
          <div className="glass-card-static p-0 overflow-x-auto">
            <table className="w-full text-sm" aria-label="广告主列表">
              <thead style={{ background: "var(--bg-card-elevated)" }}>
                <tr>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>ID</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>公司</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>邮箱</th>
                  <th className="text-right py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>余额 (CNY)</th>
                  <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>注册时间</th>
                  <th className="py-3 px-4" />
                </tr>
              </thead>
              <tbody>
                {advertisers.map((adv) => (
                  <tr key={adv.id} className="transition-colors" style={{ borderTop: "1px solid var(--border-subtle)" }} onMouseEnter={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }} onMouseLeave={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "transparent"; }}>
                    <td className="py-3 px-4 tabular-nums text-xs" style={{ color: "var(--text-muted)" }}>{adv.id}</td>
                    <td className="py-3 px-4 font-medium max-w-[200px] truncate" style={{ color: "var(--text-primary)" }} title={adv.company_name}>{adv.company_name}</td>
                    <td className="py-3 px-4 text-xs max-w-[240px] truncate" style={{ color: "var(--text-secondary)" }} title={adv.contact_email}>{adv.contact_email}</td>
                    <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-primary)" }}>
                      {topUpSuccess === adv.id && (
                        <span className="mr-2 text-xs" style={{ color: "#22C55E" }}>✓ 已充值</span>
                      )}
                      ¥{(adv.balance_cents / 100).toLocaleString()}
                    </td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {new Date(adv.created_at).toLocaleDateString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <button
                        onClick={() => { setTopUpTarget(adv); setTopUpSuccess(null); }}
                        className="px-3 py-1.5 text-xs font-medium rounded transition-colors"
                        style={{ background: "var(--primary-muted)", color: "var(--primary)" }}
                      >
                        充值
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {/* Top-up Modal */}
      {topUpTarget && (
        <TopUpModal
          advertiser={topUpTarget}
          onClose={() => setTopUpTarget(null)}
          onSuccess={() => {
            setTopUpSuccess(topUpTarget.id);
            setTopUpTarget(null);
            load();
          }}
        />
      )}
    </div>
  );
}
