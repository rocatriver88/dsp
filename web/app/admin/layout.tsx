"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";
import { getAccessToken, logout } from "@/lib/api";

const API_BASE = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8181";

const adminNavItems = [
  { href: "/admin", label: "概览", icon: "概" },
  { href: "/admin/agencies", label: "代理商", icon: "商" },
  { href: "/admin/users", label: "用户", icon: "户" },
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

function AdminAuthGate({ children }: { children: React.ReactNode }) {
  const [checking, setChecking] = useState(true);
  const [authorized, setAuthorized] = useState(false);

  useEffect(() => {
    const token = getAccessToken();
    if (!token) {
      // No JWT — redirect to tenant login page
      window.location.href = "/";
      return;
    }

    // Verify JWT and check role via /api/v1/auth/me (on the public port)
    fetch(`${API_BASE}/api/v1/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then((res) => {
        if (res.status === 401) {
          // Token expired or invalid — redirect to login
          logout();
          return;
        }
        if (!res.ok) {
          // Other error — fail closed
          window.location.href = "/";
          return;
        }
        return res.json();
      })
      .then((data) => {
        if (!data) return;
        if (data.role !== "platform_admin") {
          // Not an admin — redirect to tenant dashboard
          window.location.href = "/";
          return;
        }
        setAuthorized(true);
      })
      .catch(() => {
        // Network error — fail closed
        window.location.href = "/";
      })
      .finally(() => setChecking(false));
  }, []);

  if (checking) return null;
  if (!authorized) return null;

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
