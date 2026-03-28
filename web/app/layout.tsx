import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "DSP Platform",
  description: "Demand-Side Platform — 广告主管理后台",
};

const navItems = [
  { href: "/", label: "概览", icon: "📊" },
  { href: "/campaigns", label: "Campaigns", icon: "📢" },
  { href: "/reports", label: "报表", icon: "📈" },
  { href: "/billing", label: "账户", icon: "💰" },
];

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN" className="h-full">
      <body className="h-full flex flex-col md:flex-row">
        {/* Skip to content link — keyboard accessibility */}
        <a href="#main-content"
          className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:top-2 focus:left-2 focus:px-4 focus:py-2 focus:bg-blue-600 focus:text-white focus:rounded focus:text-sm">
          跳转到主内容
        </a>

        {/* Mobile top nav */}
        <nav aria-label="移动端导航"
          className="md:hidden flex items-center gap-1 px-3 py-2 overflow-x-auto"
          style={{ background: "var(--sidebar-bg)" }}>
          <span className="text-white font-semibold text-sm mr-2 flex-shrink-0">DSP</span>
          {navItems.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="flex items-center gap-1 px-3 py-1.5 text-xs rounded-full flex-shrink-0 transition-colors hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-offset-1 focus:ring-offset-gray-900"
              style={{ color: "var(--sidebar-text)" }}
            >
              <span aria-hidden="true">{item.icon}</span>
              {item.label}
            </Link>
          ))}
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
            {navItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                role="listitem"
                className="flex items-center gap-3 px-5 py-2.5 text-sm transition-colors hover:bg-gray-800 focus:outline-none focus:ring-2 focus:ring-blue-400 focus:ring-inset"
                style={{ color: "var(--sidebar-text)" }}
              >
                <span aria-hidden="true" className="text-base">{item.icon}</span>
                {item.label}
              </Link>
            ))}
          </div>
        </nav>

        {/* Main content */}
        <main id="main-content" className="flex-1 overflow-auto" role="main">
          <div className="max-w-6xl mx-auto px-4 py-4 md:px-8 md:py-6">
            {children}
          </div>
        </main>
      </body>
    </html>
  );
}
