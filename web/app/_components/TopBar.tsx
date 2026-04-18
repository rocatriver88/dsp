"use client";

import { Search, Bell } from "lucide-react";

export default function TopBar() {
  return (
    <div className="h-16 flex items-center justify-between px-8"
      style={{
        background: "rgba(15, 10, 26, 0.6)",
        backdropFilter: "blur(12px)",
        WebkitBackdropFilter: "blur(12px)",
        borderBottom: "1px solid var(--border)",
      }}>
      {/* Search */}
      <div className="flex items-center gap-2 px-4 py-2 rounded-[10px] max-w-xs flex-1 text-[13px]"
        style={{ background: "var(--bg-card)", border: "1px solid var(--border)", color: "var(--text-muted)" }}>
        <Search size={14} />
        <span>搜索广告系列、受众...</span>
      </div>

      {/* Right side */}
      <div className="flex items-center gap-4">
        {/* Notification bell */}
        <button className="relative w-9 h-9 rounded-full flex items-center justify-center transition-colors"
          style={{ color: "var(--text-muted)" }}
          aria-label="通知">
          <Bell size={18} />
          <span className="absolute top-1.5 right-2 w-1.5 h-1.5 rounded-full"
            style={{ background: "var(--error)" }} />
        </button>

        {/* Avatar */}
        <div className="w-8 h-8 rounded-full flex items-center justify-center text-xs font-semibold cursor-pointer"
          style={{
            background: "linear-gradient(135deg, #8B5CF6, #3B82F6)",
            boxShadow: "0 0 0 2px var(--bg-page), 0 0 0 3px rgba(139,92,246,0.3)",
          }}>
          U
        </div>
      </div>
    </div>
  );
}
