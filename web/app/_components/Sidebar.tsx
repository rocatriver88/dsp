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
              style={{ color: isActive ? "var(--primary)" : "var(--text-muted)" }}>
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
              style={{ background: "linear-gradient(135deg, #8B5CF6, #6D28D9)" }}>
              <span className="text-white text-xs font-bold">D</span>
            </div>
            <div>
              <h1 className="text-sm font-semibold" style={{ color: "var(--text-primary)" }}>DSP Platform</h1>
            </div>
          </div>
        </div>
        <div className="flex-1 py-3 px-3" role="list">
          {navItems.map((item) => {
            const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
            return (
              <Link key={item.href} href={item.href} role="listitem"
                className="flex items-center gap-3 px-3 py-2.5 text-sm rounded-lg mb-0.5 transition-colors"
                style={{
                  color: isActive ? "#8B5CF6" : "var(--text-muted)",
                  background: isActive ? "var(--primary-muted)" : "transparent",
                }}>
                <item.icon size={18} />
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4" style={{ borderTop: "1px solid var(--border)" }}>
          <button onClick={() => { logout(); }}
            className="flex items-center gap-2 text-sm transition-colors w-full"
            style={{ color: "var(--text-muted)" }}>
            <LogOut size={16} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}
