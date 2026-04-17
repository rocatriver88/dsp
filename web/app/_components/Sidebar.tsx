"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { logout } from "@/lib/api";
import { LayoutDashboard, Megaphone, BarChart3, Activity, Wallet, LogOut } from "lucide-react";
import type { LucideIcon } from "lucide-react";

const navItems: { href: string; label: string; icon: LucideIcon }[] = [
  { href: "/", label: "仪表板", icon: LayoutDashboard },
  { href: "/campaigns", label: "广告系列", icon: Megaphone },
  { href: "/reports", label: "报表", icon: BarChart3 },
  { href: "/analytics", label: "数据分析", icon: Activity },
  { href: "/billing", label: "账户", icon: Wallet },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <>
      {/* Mobile bottom nav */}
      <nav aria-label="移动端导航"
        className="md:hidden fixed bottom-0 left-0 right-0 z-50 flex items-center justify-around px-2 py-2"
        style={{ background: "var(--bg-sidebar)", borderTop: "1px solid var(--border)" }}>
        {navItems.map((item) => {
          const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
          return (
            <Link key={item.href} href={item.href}
              className="flex flex-col items-center gap-0.5 px-3 py-1.5 text-[10px] rounded-lg transition-colors"
              style={{ color: isActive ? "var(--sidebar-text-active)" : "var(--sidebar-text)" }}>
              <item.icon size={20} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Desktop sidebar */}
      <nav aria-label="主导航"
        className="hidden md:flex flex-shrink-0 flex-col"
        style={{ width: 220, minHeight: "100vh", background: "var(--bg-sidebar)" }}>
        <div className="px-5 py-5" style={{ borderBottom: "1px solid var(--border)" }}>
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg flex items-center justify-center"
              style={{
                background: "linear-gradient(135deg, #8B5CF6, #6D28D9)",
                boxShadow: "0 4px 12px rgba(139,92,246,0.3)",
              }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <h1 className="text-sm font-semibold" style={{ color: "var(--text-primary)" }}>DSP Platform</h1>
          </div>
        </div>

        <div className="flex-1 py-3 px-3" role="list">
          {navItems.map((item) => {
            const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
            return (
              <Link key={item.href} href={item.href} role="listitem"
                className={`flex items-center gap-3 px-4 py-[11px] text-[14px] rounded-lg mb-[3px] transition-colors ${isActive ? "font-medium" : "font-normal"}`}
                style={isActive ? {
                  color: "var(--sidebar-text-active)",
                  background: "var(--primary-muted)",
                  borderLeft: "3px solid var(--primary)",
                  boxShadow: "inset 3px 0 8px -3px rgba(139,92,246,0.4)",
                  paddingLeft: "13px",
                } : {
                  color: "var(--sidebar-text)",
                  background: "transparent",
                }}>
                <item.icon size={20} style={{ opacity: isActive ? 1 : 0.55, flexShrink: 0 }} />
                {item.label}
              </Link>
            );
          })}
        </div>

        <div className="px-5 py-4" style={{ borderTop: "1px solid var(--border)" }}>
          <button onClick={() => { logout(); }}
            className="flex items-center gap-2 text-sm transition-colors w-full"
            style={{ color: "var(--sidebar-text)" }}>
            <LogOut size={16} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}
