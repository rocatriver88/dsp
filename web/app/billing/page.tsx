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
      api.getBalance(1),
      api.getTransactions(1),
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
        <h2 className="text-2xl font-semibold mb-6">账户</h2>
        <div className="animate-pulse"><div className="h-24 bg-gray-100 rounded mb-4" /><div className="h-40 bg-gray-100 rounded" /></div>
      </div>
    );
  }

  if (error) {
    return (
      <div>
        <h2 className="text-2xl font-semibold mb-6">账户</h2>
        <ErrorState message={error} onRetry={load} />
      </div>
    );
  }

  return (
    <div>
      <h2 className="text-2xl font-semibold mb-6">账户</h2>

      {/* Balance card */}
      <div className="rounded-lg bg-white p-6 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-baseline gap-6">
            <div>
              <p className="text-xs font-medium text-gray-500 mb-1">账户余额</p>
              <p className="text-3xl font-semibold font-geist">¥{balance !== null ? (balance / 100).toLocaleString() : "—"}</p>
            </div>
            <div>
              <p className="text-xs font-medium text-gray-500 mb-1">计费模式</p>
              <p className="text-lg">{billingType === "prepaid" ? "预付费" : "后付费"}</p>
            </div>
          </div>
          <button onClick={() => { setShowTopUp(!showTopUp); setTopUpSuccess(false); setTopUpError(null); }}
            className="px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
            {showTopUp ? "取消" : "充值"}
          </button>
        </div>

        {/* Top-up form */}
        {showTopUp && (
          <div className="mt-4 pt-4 border-t border-gray-100">
            {topUpSuccess ? (
              <div className="p-3 rounded bg-green-50 text-green-700 text-sm">
                充值成功！余额已更新。
              </div>
            ) : (
              <div className="flex items-end gap-3">
                <div className="flex-1">
                  <label className="block text-xs font-medium text-gray-500 mb-1">充值金额 (¥)</label>
                  <div className="flex gap-2 mb-2">
                    {[1000, 5000, 10000, 50000].map((amt) => (
                      <button key={amt} onClick={() => setTopUpAmount(String(amt))}
                        className={`px-3 py-1.5 text-sm rounded-md border ${
                          topUpAmount === String(amt) ? "border-blue-500 bg-blue-50 text-blue-700" : "border-gray-200 text-gray-600"
                        }`}>
                        ¥{amt.toLocaleString()}
                      </button>
                    ))}
                  </div>
                  <input type="number" value={topUpAmount} onChange={(e) => setTopUpAmount(e.target.value)}
                    placeholder="输入自定义金额"
                    className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-geist tabular-nums" />
                </div>
                <div className="flex-1">
                  <label className="block text-xs font-medium text-gray-500 mb-1">备注 (可选)</label>
                  <input type="text" value={topUpDesc} onChange={(e) => setTopUpDesc(e.target.value)}
                    placeholder="例: 3月广告预算"
                    className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm" />
                </div>
                <button onClick={async () => {
                  const amt = Number(topUpAmount);
                  if (!amt || amt <= 0) { setTopUpError("请输入有效金额"); return; }
                  setTopUpLoading(true);
                  setTopUpError(null);
                  try {
                    await api.topUp(1, amt * 100, topUpDesc);
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
                  className="px-6 py-2 text-sm font-medium text-white rounded-md bg-green-600 hover:bg-green-700 disabled:bg-gray-300 whitespace-nowrap">
                  {topUpLoading ? "处理中..." : "确认充值"}
                </button>
              </div>
            )}
            {topUpError && <p className="text-sm text-red-600 mt-2">{topUpError}</p>}
          </div>
        )}
      </div>

      {/* Transaction history */}
      <h3 className="text-sm font-medium text-gray-500 mb-3">交易记录</h3>
      {transactions.length === 0 ? (
        <div className="rounded-lg bg-white p-12 text-center">
          <p className="text-base font-medium mb-2">暂无交易记录</p>
          <p className="text-sm text-gray-500">Campaign 开始投放后，花费和充值记录会显示在这里</p>
        </div>
      ) : (
        <div className="rounded-lg bg-white overflow-hidden">
          <table className="w-full text-sm" aria-label="交易记录">
            <thead className="bg-gray-50">
              <tr>
                <th className="text-left py-3 px-4 font-medium text-gray-500">时间</th>
                <th className="text-left py-3 px-4 font-medium text-gray-500">类型</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">金额 (¥)</th>
                <th className="text-right py-3 px-4 font-medium text-gray-500">余额 (¥)</th>
                <th className="text-left py-3 px-4 font-medium text-gray-500">说明</th>
              </tr>
            </thead>
            <tbody>
              {transactions.map((t) => (
                <tr key={t.id} className="border-t border-gray-100">
                  <td className="py-3 px-4 text-gray-500 font-geist tabular-nums text-xs">
                    {new Date(t.created_at).toLocaleString("zh-CN")}
                  </td>
                  <td className="py-3 px-4">
                    <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${
                      t.type === "topup" ? "bg-green-50 text-green-700" :
                      t.type === "spend" ? "bg-red-50 text-red-600" :
                      "bg-gray-100 text-gray-600"
                    }`}>{t.type === "topup" ? "充值" : t.type === "spend" ? "消费" : t.type}</span>
                  </td>
                  <td className={`py-3 px-4 text-right font-geist tabular-nums ${t.amount_cents > 0 ? "text-green-600" : "text-red-600"}`}>
                    {t.amount_cents > 0 ? "+" : ""}{(t.amount_cents / 100).toLocaleString()}
                  </td>
                  <td className="py-3 px-4 text-right font-geist tabular-nums">{(t.balance_after / 100).toLocaleString()}</td>
                  <td className="py-3 px-4 text-gray-500">{t.description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
