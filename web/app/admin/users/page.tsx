"use client";

import { useEffect, useState, useCallback } from "react";
import { adminApi, UserResponse } from "@/lib/admin-api";
import PageHeader from "../../_components/PageHeader";

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
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "rgba(0,0,0,0.6)" }}
      role="dialog"
      aria-modal="true"
      aria-label="创建用户"
    >
      <div className="rounded-[14px] p-6 w-full max-w-sm shadow-lg" style={{ background: "var(--bg-card-elevated)" }}>
        <h3 className="text-base font-semibold mb-4" style={{ color: "var(--text-primary)" }}>创建用户</h3>

        <div className="mb-3">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>公司名称 / 姓名</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例: 某某科技"
            autoFocus
            className="w-full px-3 py-2 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          />
        </div>

        <div className="mb-3">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>邮箱</label>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="user@example.com"
            className="w-full px-3 py-2 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          />
        </div>

        <div className="mb-3">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>初始密码</label>
          <input
            type="text"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="至少 8 个字符"
            className="w-full px-3 py-2 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          />
        </div>

        <div className="mb-4">
          <label className="block text-xs font-medium mb-1" style={{ color: "var(--text-secondary)" }}>角色</label>
          <select
            value={role}
            onChange={(e) => setRole(e.target.value)}
            className="w-full px-3 py-2 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-purple-500"
            style={{ background: "var(--bg-input)", borderColor: "var(--border)", color: "var(--text-primary)", border: "1px solid var(--border)" }}
          >
            <option value="advertiser">广告主 (advertiser)</option>
            <option value="platform_admin">管理员 (platform_admin)</option>
          </select>
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
            onClick={handleSubmit}
            disabled={loading || !name || !email || !password}
            className="px-3 py-1.5 text-xs font-medium rounded text-white disabled:cursor-not-allowed transition-colors disabled:opacity-50"
            style={{ background: loading || !name || !email || !password ? "var(--border)" : "var(--primary)" }}
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
      className="fixed inset-0 z-50 flex items-center justify-center"
      style={{ background: "rgba(0,0,0,0.6)" }}
      role="dialog"
      aria-modal="true"
      aria-label="API Key"
    >
      <div className="rounded-[14px] p-6 w-full max-w-sm shadow-lg" style={{ background: "var(--bg-card-elevated)" }}>
        <h3 className="text-base font-semibold mb-2" style={{ color: "var(--text-primary)" }}>用户创建成功</h3>
        <p className="text-xs mb-4" style={{ color: "var(--text-secondary)" }}>
          以下 API Key 仅显示一次，请妥善保存
        </p>
        <div className="rounded-md p-3 mb-4 flex items-center gap-2" style={{ background: "var(--bg-card)" }}>
          <code className="text-xs font-mono break-all flex-1" style={{ color: "var(--text-primary)" }}>{apiKey}</code>
          <button
            onClick={handleCopy}
            className="px-2 py-1 text-xs rounded flex-shrink-0 transition-colors"
            style={{ background: "var(--primary-muted)", color: "var(--primary)" }}
          >
            {copied ? "已复制" : "复制"}
          </button>
        </div>
        <div className="flex justify-end">
          <button
            onClick={onClose}
            className="px-3 py-1.5 text-xs font-medium rounded text-white transition-colors"
            style={{ background: "var(--primary)" }}
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

const statusStyles: Record<string, { text: string; bg: string; color: string }> = {
  active: { text: "正常", bg: "rgba(34,197,94,0.15)", color: "#22C55E" },
  suspended: { text: "已停用", bg: "rgba(239,68,68,0.15)", color: "#EF4444" },
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
    <div className="">
      <PageHeader title="用户管理" action={
        <button onClick={() => setShowCreate(true)} className="btn-primary px-4 py-2 text-sm">
          创建用户
        </button>
      } />

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

      {loading ? (
        <div className="glass-card-static p-6 animate-pulse space-y-3">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-10 rounded" style={{ background: "var(--bg-card-elevated)" }} />
          ))}
        </div>
      ) : users.length === 0 ? (
        <div className="glass-card-static p-12 text-center">
          <p className="text-sm" style={{ color: "var(--text-secondary)" }}>暂无用户</p>
        </div>
      ) : (
        <div className="glass-card-static p-0 overflow-x-auto">
          <table className="w-full text-sm" aria-label="用户列表">
            <thead style={{ background: "var(--bg-card-elevated)" }}>
              <tr>
                <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>姓名</th>
                <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>邮箱</th>
                <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>角色 / 状态</th>
                <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>广告主 ID</th>
                <th className="text-left py-3 px-4 text-xs font-medium" style={{ color: "var(--text-secondary)" }}>最后登录</th>
                <th className="py-3 px-4" />
              </tr>
            </thead>
            <tbody>
              {users.map((u) => {
                const status = statusStyles[u.status] || { text: u.status, bg: "var(--bg-card-elevated)", color: "var(--text-primary)" };
                return (
                  <tr key={u.id} className="transition-colors" style={{ borderTop: "1px solid var(--border-subtle)" }} onMouseEnter={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }} onMouseLeave={(e: React.MouseEvent<HTMLTableRowElement>) => { e.currentTarget.style.background = "transparent"; }}>
                    <td className="py-3 px-4 font-medium max-w-[200px] truncate" style={{ color: "var(--text-primary)" }} title={u.name}>{u.name}</td>
                    <td className="py-3 px-4 text-xs max-w-[240px] truncate" style={{ color: "var(--text-secondary)" }} title={u.email}>{u.email}</td>
                    <td className="py-3 px-4">
                      <div className="flex items-center gap-2 flex-wrap">
                        <span className="inline-block px-2 py-0.5 text-xs font-medium rounded-full whitespace-nowrap"
                          style={u.role === "platform_admin"
                            ? { background: "rgba(139,92,246,0.15)", color: "#8B5CF6" }
                            : { background: "rgba(59,130,246,0.15)", color: "#3B82F6" }}>
                          {roleLabels[u.role] || u.role}
                        </span>
                        <span className="inline-block px-2 py-0.5 text-xs font-medium rounded-full whitespace-nowrap"
                          style={{ background: status.bg, color: status.color }}>
                          {status.text}
                        </span>
                      </div>
                    </td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {u.advertiser_id ?? "-"}
                    </td>
                    <td className="py-3 px-4 text-xs tabular-nums" style={{ color: "var(--text-muted)" }}>
                      {u.last_login_at
                        ? new Date(u.last_login_at).toLocaleString("zh-CN")
                        : "从未登录"}
                    </td>
                    <td className="py-3 px-4">
                      <button
                        onClick={() => handleToggleStatus(u)}
                        disabled={actionLoading === u.id}
                        className="px-3 py-1.5 text-xs font-medium rounded transition-colors disabled:opacity-50"
                        style={
                          u.status === "active"
                            ? { background: "rgba(239,68,68,0.15)", color: "#EF4444" }
                            : { background: "rgba(34,197,94,0.15)", color: "#22C55E" }
                        }
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
