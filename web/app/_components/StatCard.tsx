"use client";

import { TrendingUp, TrendingDown } from "lucide-react";
import type { LucideIcon } from "lucide-react";

interface StatCardProps {
  label: string;
  value: string;
  trend?: { value: string; positive: boolean };
  icon?: LucideIcon;
  iconColor?: string;
  stagger?: number;
  className?: string;
}

export function StatCard({ label, value, trend, icon: Icon, iconColor = "#8B5CF6", stagger = 1, className }: StatCardProps) {
  const iconBg = iconColor + "26"; // ~15% opacity
  return (
    <div className={`glass-card p-5 animate-fade-in-up stagger-${stagger} ${className || ""}`}>
      <div className="flex items-start justify-between mb-3">
        {Icon && (
          <div className="w-9 h-9 rounded-[10px] flex items-center justify-center"
            style={{ background: iconBg }}>
            <Icon size={18} style={{ color: iconColor }} />
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
