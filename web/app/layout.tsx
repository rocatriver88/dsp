import type { Metadata } from "next";
import ApiKeyGate from "./components/ApiKeyGate";
import Sidebar from "./components/Sidebar";
import "./globals.css";

export const metadata: Metadata = {
  title: "DSP Platform",
  description: "Demand-Side Platform — 广告主管理后台",
};

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

        <ApiKeyGate
          sidebar={<Sidebar />}
        >
          {children}
        </ApiKeyGate>
      </body>
    </html>
  );
}
