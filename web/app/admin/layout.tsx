"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";

const adminNavItems = [
  { href: "/admin", label: "概览", icon: "概" },
  { href: "/admin/agencies", label: "代理商", icon: "商" },
  { href: "/admin/creatives", label: "素材审核", icon: "材" },
  { href: "/admin/invites", label: "邀请码", icon: "邀" },
  { href: "/admin/audit", label: "审计日志", icon: "审" },
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
        <span className="text-white font-semibold text-sm mr-2 flex-shrink-0">Admin</span>
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
              <span className="font-medium">{item.icon}</span>
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
          <h1 className="text-lg font-semibold text-white tracking-tight">DSP Admin</h1>
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
                <span className="text-xs font-bold w-5 h-5 flex items-center justify-center rounded bg-gray-800 text-gray-400">
                  {item.icon}
                </span>
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4 border-t border-gray-800 space-y-2">
          <Link
            href="/"
            className="flex items-center gap-2 text-sm text-gray-400 hover:text-white transition-colors w-full"
          >
            <span className="text-xs">←</span>
            广告主后台
          </Link>
          <button
            onClick={onLogout}
            className="flex items-center gap-2 text-sm text-gray-400 hover:text-white transition-colors w-full"
          >
            <span className="text-xs">退</span>
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
          // Network error — fail closed.  A server outage (or attacker
          // DoSing the health endpoint) must NOT grant admin access.
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
      <div className="min-h-screen w-full flex items-center justify-center bg-gray-50">
        <div className="bg-white rounded-lg p-8 w-full max-w-md shadow-sm">
          <h2 className="text-xl font-semibold mb-2">DSP Admin</h2>
          <p className="text-sm text-gray-500 mb-1">管理员控制台</p>
          <Link
            href="/"
            className="inline-block text-xs text-blue-500 hover:text-blue-600 mb-6"
          >
            ← 广告主后台
          </Link>
          <input
            type="password"
            placeholder="输入管理员 Token"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-md text-sm mb-4 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
            autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter") handleLogin();
            }}
          />
          <button
            onClick={handleLogin}
            disabled={!input || validating}
            className="w-full px-4 py-2 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700 disabled:bg-gray-300 disabled:cursor-not-allowed"
          >
            {validating ? "验证中..." : "登录"}
          </button>
          {error && <p className="text-xs text-red-500 mt-2">{error}</p>}
          <p className="text-xs text-gray-400 mt-3">
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
