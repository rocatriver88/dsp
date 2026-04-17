"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { getAccessToken, login, logout } from "@/lib/api";
import TopBar from "../_components/TopBar";
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
        style={{ background: "var(--bg-sidebar)" }}
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
              className="flex items-center gap-1 px-3 py-1.5 text-xs rounded-full flex-shrink-0 transition-colors"
              style={isActive
                ? { background: "var(--primary-muted)", color: "var(--sidebar-text-active)" }
                : { color: "var(--sidebar-text)" }}
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
        style={{ width: 220, minHeight: "100vh", background: "var(--bg-sidebar)" }}
      >
        <div className="px-5 py-5" style={{ borderBottom: "1px solid var(--border)" }}>
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center"
              style={{
                background: "linear-gradient(135deg, #8B5CF6, #6D28D9)",
                boxShadow: "0 4px 12px rgba(139,92,246,0.3)",
              }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <div>
              <h1 className="text-sm font-semibold" style={{ color: "var(--text-primary)" }}>DSP 管理后台</h1>
              <p className="text-[11px]" style={{ color: "var(--sidebar-text)" }}>管理员控制台</p>
            </div>
          </div>
        </div>
        <div className="flex-1 py-3 px-3" role="list">
          {adminNavItems.map((item) => {
            const isActive =
              pathname === item.href ||
              (item.href !== "/admin" && pathname.startsWith(item.href));
            return (
              <Link
                key={item.href}
                href={item.href}
                role="listitem"
                className={`flex items-center gap-3 px-4 py-[11px] text-[14px] rounded-lg mb-[3px] transition-colors ${isActive ? "font-medium" : "font-normal"}`}
                style={isActive ? {
                  color: "var(--sidebar-text-active)",
                  background: "var(--primary-muted)",
                  borderLeft: "3px solid var(--primary)",
                  boxShadow: "inset 3px 0 8px -3px rgba(139,92,246,0.4)",
                  paddingLeft: "13px",
                } : {
                  color: "var(--sidebar-text)",
                }}
              >
                <item.icon size={20} style={{ opacity: isActive ? 1 : 0.55, flexShrink: 0 }} />
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4" style={{ borderTop: "1px solid var(--border)" }}>
          <button
            onClick={onLogout}
            className="flex items-center gap-2 text-sm transition-colors w-full"
            style={{ color: "var(--sidebar-text)" }}
          >
            <LogOut size={16} />
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
      <div className="min-h-screen w-full flex items-center justify-center"
        style={{ background: "var(--bg-page)" }}>
        <div className="glass-card p-8 w-full max-w-md">
          <div className="flex items-center gap-3 mb-2">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
              style={{
                background: "linear-gradient(135deg, #8B5CF6, #6D28D9)",
                boxShadow: "0 4px 12px rgba(139,92,246,0.3)",
              }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <h2 className="text-xl font-semibold" style={{ color: "var(--text-primary)" }}>DSP 管理后台</h2>
          </div>
          <p className="text-[13px] mb-6" style={{ color: "var(--text-secondary)" }}>
            使用管理员账号登录
          </p>
          {loginError && (
            <div className="mb-4 p-3 rounded-lg text-sm"
              style={{ background: "rgba(239, 68, 68, 0.1)", border: "1px solid rgba(239, 68, 68, 0.3)", color: "var(--error)" }}>
              {loginError}
            </div>
          )}
          <div className="mb-3">
            <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--text-secondary)" }}>邮箱</label>
            <input
              type="email"
              placeholder="admin@example.com"
              value={loginEmail}
              onChange={(e) => setLoginEmail(e.target.value)}
              className="w-full px-3.5 py-2.5 rounded-lg text-[13px] focus:outline-none transition-colors"
              style={{ background: "#0F0A1A", border: "1px solid var(--border)", color: "var(--text-primary)" }}
              onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
              onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
              autoFocus
              onKeyDown={(e) => { if (e.key === "Enter") handleAdminLogin(); }}
            />
          </div>
          <div className="mb-5">
            <label className="block text-xs font-medium mb-1.5" style={{ color: "var(--text-secondary)" }}>密码</label>
            <input
              type="password"
              placeholder="输入密码"
              value={loginPassword}
              onChange={(e) => setLoginPassword(e.target.value)}
              className="w-full px-3.5 py-2.5 rounded-lg text-[13px] focus:outline-none transition-colors"
              style={{ background: "#0F0A1A", border: "1px solid var(--border)", color: "var(--text-primary)" }}
              onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
              onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
              onKeyDown={(e) => { if (e.key === "Enter") handleAdminLogin(); }}
            />
          </div>
          <button
            onClick={handleAdminLogin}
            disabled={!loginEmail || !loginPassword || loginLoading}
            className="btn-primary w-full px-4 py-2.5 text-sm"
          >
            {loginLoading ? "登录中..." : "登录"}
          </button>
          <p className="text-xs mt-4 text-center">
            <Link href="/" className="inline-link hover:underline" style={{ color: "var(--primary)" }}>← 返回广告主登录</Link>
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen">
      <AdminSidebar onLogout={logout} />
      <div className="flex-1 flex flex-col overflow-hidden">
        <TopBar />
        <main className="flex-1 overflow-auto ambient-glow">
          <div className="relative z-10 max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-7">
            {children}
          </div>
        </main>
      </div>
    </div>
  );
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminAuthGate>{children}</AdminAuthGate>;
}
