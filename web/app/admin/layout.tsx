"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { getAccessToken, login, logout } from "@/lib/api";
import { LayoutDashboard, Building2, Users, FileCheck, Ticket, ScrollText, LogOut } from "lucide-react";
import type { LucideIcon } from "lucide-react";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

const adminNavItems: { href: string; label: string; icon: LucideIcon }[] = [
  { href: "/admin", label: "概览", icon: LayoutDashboard },
  { href: "/admin/agencies", label: "代理商", icon: Building2 },
  { href: "/admin/users", label: "用户", icon: Users },
  { href: "/admin/creatives", label: "素材审核", icon: FileCheck },
  { href: "/admin/invites", label: "邀请码", icon: Ticket },
  { href: "/admin/audit", label: "审计日志", icon: ScrollText },
];

function AdminSidebar({ onLogout }: { onLogout: () => void }) {
  const pathname = usePathname();

  return (
    <>
      {/* Mobile top nav */}
      <nav
        aria-label="管理员移动端导航"
        className="md:hidden flex items-center gap-1 px-3 py-2 overflow-x-auto"
        style={{ background: "#111827" }}
      >
        <span className="text-white font-semibold text-sm mr-2 flex-shrink-0">管理后台</span>
        {adminNavItems.map((item) => {
          const isActive =
            pathname === item.href ||
            (item.href !== "/admin" && pathname.startsWith(item.href));
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`flex items-center gap-1 px-3 py-1.5 text-xs rounded-full flex-shrink-0 transition-colors focus:outline-none focus:ring-2 focus:ring-blue-400 ${
                isActive ? "bg-blue-600 text-white" : "text-gray-400 hover:bg-gray-800"
              }`}
            >
              <item.icon size={14} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Desktop sidebar */}
      <nav
        aria-label="管理员导航"
        className="hidden md:flex flex-col flex-shrink-0"
        style={{ width: 224, minHeight: "100vh", background: "#111827" }}
      >
        <div className="px-5 py-5 border-b border-gray-800">
          <h1 className="text-lg font-semibold text-white tracking-tight">DSP 管理后台</h1>
          <p className="text-xs mt-0.5 text-gray-400">管理员控制台</p>
        </div>
        <div className="flex-1 py-3" role="list">
          {adminNavItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== "/admin" && pathname.startsWith(item.href));
            return (
              <Link
                key={item.href}
                href={item.href}
                role="listitem"
                className={`flex items-center gap-3 px-5 py-2.5 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset ${
                  isActive
                    ? "bg-blue-600/20 text-blue-400 border-r-2 border-blue-400"
                    : "text-gray-400 hover:bg-gray-800"
                }`}
              >
                <span className="w-5 h-5 flex items-center justify-center rounded bg-gray-800 text-gray-400">
                  <item.icon size={14} />
                </span>
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4 border-t border-gray-800 space-y-2">
          <button
            onClick={onLogout}
            className="flex items-center gap-2 text-sm text-gray-400 hover:text-white transition-colors w-full"
          >
            <LogOut size={14} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}

function AdminAuthGate({ children }: { children: React.ReactNode }) {
  const [checking, setChecking] = useState(true);
  const [authorized, setAuthorized] = useState(false);
  const [loginEmail, setLoginEmail] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginError, setLoginError] = useState<string | null>(null);
  const [loginLoading, setLoginLoading] = useState(false);

  function checkAuth() {
    const token = getAccessToken();
    if (!token) {
      setChecking(false);
      return;
    }

    fetch(`${API_BASE}/api/v1/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => {
        if (res.status === 401) {
          logout();
          return;
        }
        if (!res.ok) {
          setChecking(false);
          return;
        }
        return res.json();
      })
      .then((data) => {
        if (!data) return;
        if (data.role !== "platform_admin") {
          // Not admin — redirect to tenant dashboard
          window.location.href = "/";
          return;
        }
        setAuthorized(true);
      })
      .catch(() => {
        setChecking(false);
      })
      .finally(() => setChecking(false));
  }

  useEffect(() => { checkAuth(); }, []);

  async function handleAdminLogin() {
    if (!loginEmail || !loginPassword) return;
    setLoginError(null);
    setLoginLoading(true);
    try {
      const result = await login(loginEmail, loginPassword);
      if (result.user?.role !== "platform_admin") {
        setLoginError("该账号不是平台管理员");
        logout();
        return;
      }
      setAuthorized(true);
    } catch (e: unknown) {
      setLoginError(e instanceof Error ? e.message : "登录失败");
    } finally {
      setLoginLoading(false);
    }
  }

  if (checking) return null;

  if (!authorized) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg p-8 w-full max-w-md shadow-sm">
          <h2 className="text-xl font-semibold mb-2">DSP 管理后台</h2>
          <p className="text-sm text-gray-500 mb-6">
            使用管理员账号登录
          </p>
          {loginError && (
            <div className="mb-4 p-3 bg-red-50 border border-red-200 rounded-md text-sm text-red-700">
              {loginError}
            </div>
          )}
          <div className="mb-3">
            <label className="block text-xs font-medium text-gray-500 mb-1">邮箱</label>
            <input
              type="email"
              placeholder="admin@example.com"
              value={loginEmail}
              onChange={(e) => setLoginEmail(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              autoFocus
              onKeyDown={(e) => { if (e.key === "Enter") handleAdminLogin(); }}
            />
          </div>
          <div className="mb-4">
            <label className="block text-xs font-medium text-gray-500 mb-1">密码</label>
            <input
              type="password"
              placeholder="输入密码"
              value={loginPassword}
              onChange={(e) => setLoginPassword(e.target.value)}
              className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
              onKeyDown={(e) => { if (e.key === "Enter") handleAdminLogin(); }}
            />
          </div>
          <button
            onClick={handleAdminLogin}
            disabled={!loginEmail || !loginPassword || loginLoading}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
          >
            {loginLoading ? "登录中..." : "登录"}
          </button>
          <p className="text-xs text-gray-400 mt-4 text-center">
            <a href="/" className="text-blue-500 hover:underline">← 返回广告主登录</a>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen">
      <AdminSidebar onLogout={logout} />
      <main className="flex-1 overflow-auto">{children}</main>
    </div>
  );
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminAuthGate>{children}</AdminAuthGate>;
}
