"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { logout } from "@/lib/api";
import { LayoutDashboard, Megaphone, BarChart3, Activity, Wallet, LogOut } from "lucide-react";
import type { LucideIcon } from "lucide-react";

const navItems: { href: string; label: string; icon: LucideIcon }[] = [
  { href: "/", label: "概览", icon: LayoutDashboard },
  { href: "/campaigns", label: "Campaigns", icon: Megaphone },
  { href: "/reports", label: "报表", icon: BarChart3 },
  { href: "/analytics", label: "实时分析", icon: Activity },
  { href: "/billing", label: "账户", icon: Wallet },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <>
      {/* Mobile top nav */}
      <nav aria-label="移动端导航"
        className="md:hidden flex items-center gap-1 px-3 py-2 overflow-x-auto"
        style={{ background: "var(--sidebar-bg)" }}>
        <span className="text-white font-semibold text-sm mr-2 flex-shrink-0">DSP</span>
        {navItems.map((item) => {
          const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
          return (
            <Link key={item.href} href={item.href}
              className={`flex items-center gap-1 px-3 py-1.5 text-xs rounded-full flex-shrink-0 transition-colors focus:outline-none focus:ring-2 focus:ring-blue-400 ${isActive ? "bg-blue-600 text-white" : "hover:bg-gray-800"}`}
              style={isActive ? {} : { color: "var(--sidebar-text)" }}>
              <item.icon size={14} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      {/* Desktop sidebar */}
      <nav aria-label="主导航"
        className="hidden md:flex w-56 min-h-screen flex-shrink-0 flex-col"
        style={{ background: "var(--sidebar-bg)" }}>
        <div className="px-5 py-5 border-b border-gray-800">
          <h1 className="text-lg font-semibold text-white tracking-tight">DSP Platform</h1>
          <p className="text-xs mt-0.5" style={{ color: "var(--sidebar-text)" }}>广告投放管理平台</p>
        </div>
        <div className="flex-1 py-3" role="list">
          {navItems.map((item) => {
            const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
            return (
              <Link key={item.href} href={item.href} role="listitem"
                className={`flex items-center gap-3 px-5 py-2.5 text-sm transition-colors focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset ${isActive ? "bg-blue-600/20 text-blue-400 border-r-2 border-blue-400" : "hover:bg-gray-800"}`}
                style={isActive ? {} : { color: "var(--sidebar-text)" }}>
                <span className="w-5 h-5 flex items-center justify-center rounded bg-gray-800 text-gray-400"><item.icon size={14} /></span>
                {item.label}
              </Link>
            );
          })}
        </div>
        <div className="px-5 py-4 border-t border-gray-800">
          <button
            onClick={() => { logout(); }}
            className="flex items-center gap-2 text-sm hover:text-white transition-colors w-full"
            style={{ color: "var(--sidebar-text)" }}>
            <LogOut size={14} />
            退出登录
          </button>
        </div>
      </nav>
    </>
  );
}
