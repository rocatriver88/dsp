import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";

export const metadata: Metadata = {
  title: "DSP Platform",
  description: "Demand-Side Platform",
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
      <body className="h-full flex">
        {/* Sidebar */}
        <nav className="w-56 min-h-screen flex-shrink-0 flex flex-col"
          style={{ background: "var(--sidebar-bg)" }}>
          <div className="px-5 py-5 border-b border-gray-800">
            <h1 className="text-lg font-semibold text-white tracking-tight">DSP Platform</h1>
            <p className="text-xs mt-0.5" style={{ color: "var(--sidebar-text)" }}>Demand-Side Platform</p>
          </div>
          <div className="flex-1 py-3">
            {navItems.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className="flex items-center gap-3 px-5 py-2.5 text-sm transition-colors hover:bg-gray-800"
                style={{ color: "var(--sidebar-text)" }}
              >
                <span className="text-base">{item.icon}</span>
                {item.label}
              </Link>
            ))}
          </div>
          <div className="px-5 py-4 border-t border-gray-800">
            <p className="text-xs" style={{ color: "var(--sidebar-text)" }}>Phase 1 MVP</p>
          </div>
        </nav>

        {/* Main content */}
        <main className="flex-1 overflow-auto">
          <div className="max-w-6xl mx-auto px-8 py-6">
            {children}
          </div>
        </main>
      </body>
    </html>
  );
}
