"use client";

import { TrendingUp, TrendingDown } from "lucide-react";
import type { LucideIcon } from "lucide-react";

interface StatCardProps {
  label: string;
  value: string;
  trend?: { value: string; positive: boolean };
  icon?: LucideIcon;
  iconColor?: string;
  className?: string;
}

export function StatCard({ label, value, trend, icon: Icon, iconColor = "#8B5CF6", className }: StatCardProps) {
  const iconBg = iconColor + "26";
  return (
    <div className={`rounded-[14px] p-5 ${className || ""}`}
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <div className="flex items-start justify-between mb-3">
        {Icon && (
          <div className="w-10 h-10 rounded-lg flex items-center justify-center"
            style={{ background: iconBg }}>
            <Icon size={20} style={{ color: iconColor }} />
          </div>
        )}
        {trend && (
          <div className="flex items-center gap-1 text-xs font-semibold"
            style={{ color: trend.positive ? "var(--success)" : "var(--error)" }}>
            {trend.positive ? <TrendingUp size={14} /> : <TrendingDown size={14} />}
            {trend.value}
          </div>
        )}
      </div>
      <p className="text-2xl font-bold tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</p>
      <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>{label}</p>
    </div>
  );
}

export function HeroStatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="col-span-2 rounded-[14px] p-6"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <p className="text-xs font-medium mb-2" style={{ color: "var(--text-muted)" }}>{label}</p>
      <p className="text-4xl font-bold tracking-tight tabular-nums" style={{ color: "var(--text-primary)" }}>{value}</p>
      {sub && <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>{sub}</p>}
    </div>
  );
}
