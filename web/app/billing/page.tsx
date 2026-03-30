"use client";

import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { ErrorState } from "../components/LoadingState";

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
        <div className="flex items-baseline gap-6">
          <div>
            <p className="text-xs font-medium text-gray-500 mb-1">账户余额</p>
            <p className="text-3xl font-semibold">¥{balance !== null ? (balance / 100).toLocaleString() : "—"}</p>
          </div>
          <div>
            <p className="text-xs font-medium text-gray-500 mb-1">计费模式</p>
            <p className="text-lg">{billingType === "prepaid" ? "预付费" : "后付费"}</p>
          </div>
        </div>
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
