"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { LayoutDashboard, Building2, Users, FileCheck, Ticket, ScrollText, LogOut } from "lucide-react";
import type { LucideIcon } from "lucide-react";

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
              className="flex items-center gap-1 px-3 py-1.5 text-xs rounded-full flex-shrink-0 transition-colors focus:outline-none focus:ring-2 focus:ring-purple-400"
              style={isActive
                ? { background: "var(--primary-muted)", color: "var(--primary)" }
                : { color: "var(--text-muted)" }}
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
        style={{ width: 224, minHeight: "100vh", background: "var(--bg-sidebar)" }}
      >
        <div className="px-5 py-5" style={{ borderBottom: "1px solid var(--border)" }}>
          <h1 className="text-lg font-semibold text-white tracking-tight">DSP Admin</h1>
          <p className="text-xs mt-0.5" style={{ color: "var(--text-muted)" }}>管理员控制台</p>
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
                className="flex items-center gap-3 px-5 py-2.5 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-purple-400 focus:ring-inset"
                style={isActive
                  ? { background: "var(--primary-muted)", color: "var(--primary)", borderLeft: "3px solid var(--primary)" }
                  : { color: "var(--text-muted)", borderLeft: "3px solid transparent" }}
              >
                <span className="w-5 h-5 flex items-center justify-center rounded" style={{ background: "var(--border)" }}>
                  <item.icon size={14} />
                </span>
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4 space-y-2" style={{ borderTop: "1px solid var(--border)" }}>
          <Link
            href="/"
            className="flex items-center gap-2 text-sm transition-colors w-full inline-link"
            style={{ color: "var(--text-muted)" }}
          >
            <span className="text-xs">←</span>
            广告主后台
          </Link>
          <button
            onClick={onLogout}
            className="flex items-center gap-2 text-sm transition-colors w-full"
            style={{ color: "var(--text-muted)" }}
          >
            <LogOut size={14} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}

function AdminTokenGate({ children }: { children: React.ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [input, setInput] = useState("");
  const [checking, setChecking] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [validating, setValidating] = useState(false);

  useEffect(() => {
    const stored = localStorage.getItem("dsp_admin_token");
    if (stored) {
      // Re-validate stored token against server
      fetch(
        `${process.env.NEXT_PUBLIC_ADMIN_API_URL || "http://localhost:8182"}/api/v1/admin/health`,
        { headers: { "X-Admin-Token": stored } }
      )
        .then((res) => {
          if (res.ok) {
            setToken(stored);
          } else {
            localStorage.removeItem("dsp_admin_token");
          }
        })
        .catch(() => {
          localStorage.removeItem("dsp_admin_token");
        })
        .finally(() => setChecking(false));
    } else {
      setChecking(false);
    }
  }, []);

  const handleLogin = async () => {
    if (!input) return;
    setError(null);
    setValidating(true);
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_ADMIN_API_URL || "http://localhost:8182"}/api/v1/admin/health`,
        { headers: { "X-Admin-Token": input.trim() } }
      );
      if (!res.ok) {
        setError("Token 无效或服务不可用");
        return;
      }
      localStorage.setItem("dsp_admin_token", input.trim());
      setToken(input.trim());
    } catch {
      setError("无法连接到管理服务");
    } finally {
      setValidating(false);
    }
  };

  function handleLogout() {
    localStorage.removeItem("dsp_admin_token");
    setToken(null);
    setInput("");
  }

  if (checking) return null;

  if (!token) {
    return (
      <div className="min-h-screen w-full flex items-center justify-center"
        style={{ background: "var(--bg-page)" }}>
        <div className="p-8 w-full max-w-md"
          style={{ background: "var(--bg-card)", border: "1px solid var(--border)", borderRadius: 14 }}>
          <div className="flex items-center gap-3 mb-2">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0"
              style={{ background: "linear-gradient(135deg, #8B5CF6, #6D28D9)" }}>
              <span className="text-white text-xs font-bold">A</span>
            </div>
            <h2 className="text-xl font-semibold" style={{ color: "var(--text-primary)" }}>DSP Admin</h2>
          </div>
          <p className="text-sm mb-1" style={{ color: "var(--text-secondary)" }}>管理员控制台</p>
          <Link
            href="/"
            className="inline-block text-xs mb-6 inline-link"
            style={{ color: "var(--primary)" }}
          >
            ← 广告主后台
          </Link>
          <input
            type="password"
            placeholder="输入管理员 Token"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            className="w-full px-3 py-2 rounded-md text-sm mb-4 focus:outline-none"
            style={{ background: "var(--bg-input)", border: "1px solid var(--border)", color: "var(--text-primary)" }}
            onFocus={(e) => e.currentTarget.style.borderColor = "var(--primary)"}
            onBlur={(e) => e.currentTarget.style.borderColor = "var(--border)"}
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter") handleLogin();
            }}
          />
          <button
            onClick={handleLogin}
            disabled={!input || validating}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md disabled:cursor-not-allowed"
            style={{ background: "var(--primary)", opacity: (!input || validating) ? 0.5 : 1 }}
          >
            {validating ? "验证中..." : "登录"}
          </button>
          {error && <p className="text-xs mt-2" style={{ color: "var(--error)" }}>{error}</p>}
          <p className="text-xs mt-3" style={{ color: "var(--text-muted)" }}>
            管理员 Token 由系统配置，请联系运维获取
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen">
      <AdminSidebar onLogout={handleLogout} />
      <main className="flex-1 overflow-auto">{children}</main>
    </div>
  );
}

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return <AdminTokenGate>{children}</AdminTokenGate>;
}
