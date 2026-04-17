"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, UserResponse } from "@/lib/admin-api";

function CreateUserModal({
  onClose,
  onSuccess,
}: {
  onClose: () => void;
  onSuccess: (apiKey?: string) => void;
}) {
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<string>("advertiser");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit() {
    if (!name || !email || !password) {
      setError("请填写所有必填字段");
      return;
    }
    if (password.length < 8) {
      setError("密码至少 8 个字符");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const result = await adminApi.createUser({ name, email, password, role });
      onSuccess(result.api_key);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "创建失败");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-label="创建用户"
    >
      <div className="bg-white rounded-lg p-6 w-full max-w-sm shadow-lg">
        <h3 className="text-base font-semibold mb-4">创建用户</h3>

        <div className="mb-3">
          <label className="block text-xs font-medium text-gray-500 mb-1">公司名称 / 姓名</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例: 某某科技"
            autoFocus
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <div className="mb-3">
          <label className="block text-xs font-medium text-gray-500 mb-1">邮箱</label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="user@example.com"
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <div className="mb-3">
          <label className="block text-xs font-medium text-gray-500 mb-1">初始密码</label>
          <input
            type="text"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="至少 8 个字符"
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
        </div>

        <div className="mb-4">
          <label className="block text-xs font-medium text-gray-500 mb-1">角色</label>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500 bg-white"
          >
            <option value="advertiser">广告主 (advertiser)</option>
            <option value="platform_admin">管理员 (platform_admin)</option>
          </select>
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
            onClick={handleSubmit}
            disabled={loading || !name || !email || !password}
            className="px-3 py-1.5 text-xs font-medium rounded bg-blue-600 text-white hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed transition-colors"
          >
            {loading ? "创建中..." : "创建"}
          </button>
        </div>
      </div>
    </div>
  );
}

function ApiKeyModal({ apiKey, onClose }: { apiKey: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false);

  function handleCopy() {
    navigator.clipboard.writeText(apiKey).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
      role="dialog"
      aria-modal="true"
      aria-label="API Key"
    >
      <div className="bg-white rounded-lg p-6 w-full max-w-sm shadow-lg">
        <h3 className="text-base font-semibold mb-2">用户创建成功</h3>
        <p className="text-xs text-gray-500 mb-4">
          以下 API Key 仅显示一次，请妥善保存
        </p>
        <div className="bg-gray-50 rounded-md p-3 mb-4 flex items-center gap-2">
          <code className="text-xs font-mono text-gray-800 break-all flex-1">{apiKey}</code>
          <button
            onClick={handleCopy}
            className="px-2 py-1 text-xs rounded bg-blue-50 text-blue-700 hover:bg-blue-100 flex-shrink-0 transition-colors"
          >
            {copied ? "已复制" : "复制"}
          </button>
        </div>
        <div className="flex justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs font-medium rounded bg-blue-600 text-white hover:bg-blue-700 transition-colors"
          >
            确定
          </button>
        </div>
      </div>
    </div>
  );
}

const roleLabels: Record<string, string> = {
  platform_admin: "管理员",
  advertiser: "广告主",
};

const statusLabels: Record<string, { text: string; className: string }> = {
  active: { text: "正常", className: "bg-green-50 text-green-700" },
  suspended: { text: "已停用", className: "bg-red-50 text-red-700" },
};

export default function UsersPage() {
  const [users, setUsers] = useState<UserResponse[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newApiKey, setNewApiKey] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<number | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    setError(null);
    adminApi
      .listUsers()
      .then((data) => setUsers(Array.isArray(data) ? data : []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function handleToggleStatus(user: UserResponse) {
    const newStatus = user.status === "active" ? "suspended" : "active";
    setActionLoading(user.id);
    setActionError(null);
    try {
      await adminApi.updateUser(user.id, { status: newStatus });
      setUsers((prev) =>
        prev.map((u) => (u.id === user.id ? { ...u, status: newStatus } : u))
      );
    } catch (e: unknown) {
      setActionError(e instanceof Error ? e.message : "操作失败");
    } finally {
      setActionLoading(null);
    }
  }

  return (
    <div className="p-8 max-w-6xl">
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-semibold">用户管理</h2>
        <button
          onClick={() => setShowCreate(true)}
          className="px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 transition-colors"
        >
          创建用户
        </button>
      </div>

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

      {loading ? (
        <div className="bg-white rounded-lg p-6 animate-pulse space-y-3">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-10 bg-gray-100 rounded" />
          ))}
        </div>
      ) : users.length === 0 ? (
        <div className="bg-white rounded-lg p-12 text-center">
          <p className="text-sm text-gray-500">暂无用户</p>
        </div>
      ) : (
        <div className="bg-white rounded-lg overflow-hidden">
          <table className="w-full text-sm" aria-label="用户列表">
            <thead className="bg-gray-50">
              <tr>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">姓名</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">邮箱</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">角色</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">状态</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">广告主 ID</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">最后登录</th>
                <th className="text-left py-2 px-4 text-xs text-gray-500 font-medium border-b border-gray-100">创建时间</th>
                <th className="py-2 px-4 border-b border-gray-100" />
              </tr>
            </thead>
            <tbody>
              {users.map((u) => {
                const status = statusLabels[u.status] || { text: u.status, className: "bg-gray-50 text-gray-700" };
                return (
                  <tr key={u.id} className="border-b last:border-0 border-gray-100">
                    <td className="py-2 px-4 font-medium text-gray-900">{u.name}</td>
                    <td className="py-2 px-4 text-gray-600 text-xs">{u.email}</td>
                    <td className="py-2 px-4">
                      <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${
                        u.role === "platform_admin" ? "bg-purple-50 text-purple-700" : "bg-blue-50 text-blue-700"
                      }`}>
                        {roleLabels[u.role] || u.role}
                      </span>
                    </td>
                    <td className="py-2 px-4">
                      <span className={`px-2 py-0.5 text-xs font-medium rounded-full ${status.className}`}>
                        {status.text}
                      </span>
                    </td>
                    <td className="py-2 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {u.advertiser_id ?? "-"}
                    </td>
                    <td className="py-2 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {u.last_login_at
                        ? new Date(u.last_login_at).toLocaleString("zh-CN")
                        : "从未登录"}
                    </td>
                    <td className="py-2 px-4 text-xs text-gray-500 font-geist tabular-nums">
                      {new Date(u.created_at).toLocaleDateString("zh-CN")}
                    </td>
                    <td className="py-2 px-4">
                      <button
                        onClick={() => handleToggleStatus(u)}
                        disabled={actionLoading === u.id}
                        className={`px-3 py-1.5 text-xs font-medium rounded transition-colors disabled:opacity-50 ${
                          u.status === "active"
                            ? "bg-red-50 text-red-700 hover:bg-red-100"
                            : "bg-green-50 text-green-700 hover:bg-green-100"
                        }`}
                      >
                        {u.status === "active" ? "停用" : "启用"}
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Create User Modal */}
      {showCreate && (
        <CreateUserModal
          onClose={() => setShowCreate(false)}
          onSuccess={(apiKey) => {
            setShowCreate(false);
            if (apiKey) {
              setNewApiKey(apiKey);
            }
            load();
          }}
        />
      )}

      {/* API Key One-Time Display Modal */}
      {newApiKey && (
        <ApiKeyModal
          apiKey={newApiKey}
          onClose={() => setNewApiKey(null)}
        />
      )}
    </div>
  );
}
