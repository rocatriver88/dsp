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
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-label="充值"
    >
      <div className="bg-white rounded-lg p-6 w-full max-w-sm shadow-lg">
        <h3 className="text-base font-semibold mb-1">充值</h3>
        <p className="text-xs text-gray-500 mb-4">
          {advertiser.company_name} · 当前余额 ¥{(advertiser.balance_cents / 100).toLocaleString()}
        </p>

        <div className="mb-3">
          <label className="block text-xs font-medium text-gray-500 mb-1">充值金额（元）</label>
          <input
            type="number"
            min="0"
            value={amount}
            onChange={(e) => setAmount(e.target.value)}
            placeholder="例: 1000"
            autoFocus
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm font-geist tabular-nums focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <div className="mb-4">
          <label className="block text-xs font-medium text-gray-500 mb-1">备注（可选）</label>
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="例: 4月预算"
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        {error && <p className="text-sm text-red-600 mb-3">{error}</p>}

        <div className="flex gap-2 justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs font-medium rounded bg-gray-100 text-gray-700 hover:bg-gray-200 transition-colors"
          >
            取消
          </button>
          <button
            onClick={handleConfirm}
            disabled={loading || !amount}
            className="px-3 py-1.5 text-xs font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed transition-colors"
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
    <div className="p-8 max-w-6xl">
      <h2 className="text-2xl font-semibold mb-6">代理商管理</h2>

      {error && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={load} className="text-xs underline ml-4">重试</button>
        </div>
      )}

      {actionError && (
        <div className="mb-4 px-4 py-3 rounded bg-red-50 text-red-700 text-sm">
          {actionError}
        </div>
      )}

      {/* Pending Registrations */}
      {!loading && registrations.length > 0 && (
        <section className="mb-8">
          <h3 className="text-sm font-semibold text-gray-700 mb-3">
            待审核注册
            <span className="ml-2 px-2 py-0.5 text-xs font-medium rounded-full bg-yellow-50 text-yellow-700">
              {registrations.length}
            </span>
          </h3>
          <div className="bg-white rounded-lg overflow-hidden">
            <table className="w-full text-sm" aria-label="待审核注册">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">公司名称</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">邮箱</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">邀请码</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">申请时间</th>
                  <th className="py-3 px-4 border-b border-gray-100" />
                </tr>
              </thead>
              <tbody>
                {registrations.map((reg) => (
                  <tr key={reg.id} className="border-b last:border-0 border-gray-100">
                    <td className="py-3 px-4 font-medium text-gray-900">{reg.company_name}</td>
                    <td className="py-3 px-4 text-gray-600">{reg.contact_email}</td>
                    <td className="py-3 px-4 font-mono text-xs text-gray-500">{reg.invite_code}</td>
                    <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {new Date(reg.created_at).toLocaleString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-2 justify-end">
                        <button
                          onClick={() => handleApprove(reg)}
                          disabled={actionLoading === reg.id}
                          className="px-3 py-1.5 text-xs font-medium rounded bg-green-50 text-green-700 hover:bg-green-100 disabled:opacity-50 transition-colors"
                        >
                          批准
                        </button>
                        <button
                          onClick={() => handleReject(reg)}
                          disabled={actionLoading === reg.id}
                          className="px-3 py-1.5 text-xs font-medium rounded bg-red-50 text-red-700 hover:bg-red-100 disabled:opacity-50 transition-colors"
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
        <h3 className="text-sm font-semibold text-gray-700 mb-3">广告主列表</h3>
        {loading ? (
          <div className="bg-white rounded-lg p-6 animate-pulse space-y-3">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-10 bg-gray-100 rounded" />
            ))}
          </div>
        ) : advertisers.length === 0 ? (
          <div className="bg-white rounded-lg p-12 text-center">
            <p className="text-sm text-gray-500">暂无广告主</p>
          </div>
        ) : (
          <div className="bg-white rounded-lg overflow-hidden">
            <table className="w-full text-sm" aria-label="广告主列表">
              <thead className="bg-gray-50">
                <tr>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">ID</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">公司</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">邮箱</th>
                  <th className="text-right py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">余额 (CNY)</th>
                  <th className="text-left py-3 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">注册时间</th>
                  <th className="py-3 px-4 border-b border-gray-100" />
                </tr>
              </thead>
              <tbody>
                {advertisers.map((adv) => (
                  <tr key={adv.id} className="border-b last:border-0 border-gray-100">
                    <td className="py-3 px-4 text-gray-500 font-geist tabular-nums text-xs">{adv.id}</td>
                    <td className="py-3 px-4 font-medium text-gray-900">{adv.company_name}</td>
                    <td className="py-3 px-4 text-gray-600 text-xs">{adv.contact_email}</td>
                    <td className="py-3 px-4 text-right font-geist tabular-nums">
                      {topUpSuccess === adv.id && (
                        <span className="mr-2 text-xs text-green-600">✓ 已充值</span>
                      )}
                      ¥{(adv.balance_cents / 100).toLocaleString()}
                    </td>
                    <td className="py-3 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {new Date(adv.created_at).toLocaleDateString("zh-CN")}
                    </td>
                    <td className="py-3 px-4">
                      <button
                        onClick={() => { setTopUpTarget(adv); setTopUpSuccess(null); }}
                        className="px-3 py-1.5 text-xs font-medium rounded bg-blue-50 text-blue-700 hover:bg-blue-100 transition-colors"
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
