"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { ErrorState } from "../_components/LoadingState";

interface Transaction {
  id: number;
  type: string;
  amount_cents: number;
  balance_after: number;
  description: string;
  created_at: string;
}

export default function BillingPage() {
  const [balance, setBalance] = useState<number | null>(null);
  const [billingType, setBillingType] = useState("");
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showTopUp, setShowTopUp] = useState(false);
  const [topUpAmount, setTopUpAmount] = useState("");
  const [topUpDesc, setTopUpDesc] = useState("");
  const [topUpLoading, setTopUpLoading] = useState(false);
  const [topUpError, setTopUpError] = useState<string | null>(null);
  const [topUpSuccess, setTopUpSuccess] = useState(false);

  const load = () => {
    setLoading(true);
    setError(null);
    Promise.all([
      api.getBalance(),
      api.getTransactions(),
    ])
      .then(([b, t]) => {
        setBalance(b.balance_cents);
        setBillingType(b.billing_type);
        setTransactions(Array.isArray(t) ? t : []);
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  if (loading) {
    return (
      <div>
        <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>账户</h2>
        <div className="animate-pulse"><div className="h-24 rounded-[14px] mb-4" style={{ background: "var(--bg-card)" }} /><div className="h-40 rounded-[14px]" style={{ background: "var(--bg-card)" }} /></div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <h2 className="text-2xl font-semibold mb-6" style={{ color: "var(--text-primary)" }}>账户</h2>
        <ErrorState message={error} onRetry={load} />
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-2xl font-bold mb-1" style={{ color: "var(--text-primary)" }}>账户</h2>
      <p className="text-[13px] mb-6" style={{ color: "var(--text-secondary)" }}>管理账户余额和查看交易记录</p>

      {/* Balance card */}
      <div className="glass-card-static p-6 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-baseline gap-6">
            <div>
              <p className="text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>账户余额</p>
              <p className="text-3xl font-semibold tabular-nums" style={{ color: "var(--text-primary)" }}>¥{balance !== null ? (balance / 100).toLocaleString() : "—"}</p>
            </div>
            <div>
              <p className="text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>计费模式</p>
              <p className="text-lg" style={{ color: "var(--text-primary)" }}>{billingType === "prepaid" ? "预付费" : "后付费"}</p>
            </div>
          </div>
          <button onClick={() => { setShowTopUp(!showTopUp); setTopUpSuccess(false); setTopUpError(null); }}
            className="btn-primary px-4 py-2 text-sm">
            {showTopUp ? "取消" : "充值"}
          </button>
        </div>

        {/* Top-up form */}
        {showTopUp && (
          <div className="mt-4 pt-4" style={{ borderTop: "1px solid var(--border)" }}>
            {topUpSuccess ? (
              <div className="p-3 rounded-md text-sm" style={{ background: "rgba(34,197,94,0.15)", color: "#22C55E" }}>
                充值成功！余额已更新。
              </div>
            ) : (
              <div className="flex items-end gap-3">
                <div className="flex-1">
                  <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>充值金额 (¥)</label>
                  <div className="flex gap-2 mb-2">
                    {[1000, 5000, 10000, 50000].map((amt) => (
                      <button key={amt} onClick={() => setTopUpAmount(String(amt))}
                        className="px-3 py-1.5 text-sm rounded-md"
                        style={topUpAmount === String(amt)
                          ? { border: "1px solid var(--primary)", background: "var(--primary-muted)", color: "var(--primary)" }
                          : { border: "1px solid var(--border)", color: "var(--text-secondary)" }
                        }>
                        ¥{amt.toLocaleString()}
                      </button>
                    ))}
                  </div>
                  <input type="number" value={topUpAmount} onChange={(e) => setTopUpAmount(e.target.value)}
                    placeholder="输入自定义金额"
                    className="w-full px-3 py-2 rounded-md text-sm tabular-nums"
                    style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }} />
                </div>
                <div className="flex-1">
                  <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>备注 (可选)</label>
                  <input type="text" value={topUpDesc} onChange={(e) => setTopUpDesc(e.target.value)}
                    placeholder="例: 3月广告预算"
                    className="w-full px-3 py-2 rounded-md text-sm"
                    style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }} />
                </div>
                <button onClick={async () => {
                  const amt = Number(topUpAmount);
                  if (!amt || amt <= 0) { setTopUpError("请输入有效金额"); return; }
                  setTopUpLoading(true);
                  setTopUpError(null);
                  try {
                    await api.topUp(amt * 100, topUpDesc);
                    setTopUpSuccess(true);
                    setTopUpAmount("");
                    setTopUpDesc("");
                    load();
                  } catch (e: unknown) {
                    setTopUpError(e instanceof Error ? e.message : "充值失败");
                  } finally {
                    setTopUpLoading(false);
                  }
                }} disabled={topUpLoading || !topUpAmount}
                  className="px-6 py-2 text-sm font-medium text-white rounded-md whitespace-nowrap disabled:opacity-50"
                  style={{ background: "var(--success)" }}>
                  {topUpLoading ? "处理中..." : "确认充值"}
                </button>
              </div>
            )}
            {topUpError && <p className="text-sm mt-2" style={{ color: "var(--error)" }}>{topUpError}</p>}
          </div>
        )}
      </div>

      {/* Transaction history */}
      <h3 className="text-sm font-medium mb-3" style={{ color: "var(--text-secondary)" }}>交易记录</h3>
      {transactions.length === 0 ? (
        <div className="glass-card-static p-12 text-center">
          <p className="text-base font-medium mb-2" style={{ color: "var(--text-primary)" }}>暂无交易记录</p>
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>Campaign 开始投放后，花费和充值记录会显示在这里</p>
        </div>
      ) : (
        <div className="glass-card-static p-0 overflow-hidden">
          <table className="w-full text-sm" aria-label="交易记录">
            <thead style={{ background: "var(--bg-card-elevated)" }}>
              <tr>
                <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>时间</th>
                <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>类型</th>
                <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>金额 (¥)</th>
                <th className="text-right py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>余额 (¥)</th>
                <th className="text-left py-3 px-4 font-medium" style={{ color: "var(--text-secondary)" }}>说明</th>
              </tr>
            </thead>
            <tbody>
              {transactions.map((t) => (
                <tr key={t.id} style={{ borderTop: "1px solid var(--border)" }}>
                  <td className="py-3 px-4 tabular-nums text-xs" style={{ color: "var(--text-secondary)" }}>
                    {new Date(t.created_at).toLocaleString("zh-CN")}
                  </td>
                  <td className="py-3 px-4">
                    <span className="inline-block px-2 py-0.5 text-xs font-medium rounded-full"
                      style={
                        t.type === "topup" ? { background: "rgba(34,197,94,0.15)", color: "#22C55E" } :
                        t.type === "spend" ? { background: "rgba(239,68,68,0.15)", color: "#EF4444" } :
                        { background: "var(--bg-card-elevated)", color: "var(--text-secondary)" }
                      }>{t.type === "topup" ? "充值" : t.type === "spend" ? "消费" : t.type}</span>
                  </td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: t.amount_cents > 0 ? "#22C55E" : "#EF4444" }}>
                    {t.amount_cents > 0 ? "+" : ""}{(t.amount_cents / 100).toLocaleString()}
                  </td>
                  <td className="py-3 px-4 text-right tabular-nums" style={{ color: "var(--text-primary)" }}>{(t.balance_after / 100).toLocaleString()}</td>
                  <td className="py-3 px-4" style={{ color: "var(--text-secondary)" }}>{t.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
