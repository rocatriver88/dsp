"use client";

export function StatCard({ label, value, className }: { label: string; value: string; className?: string }) {
  return (
    <div className={`rounded-lg bg-white p-5 ${className || ""}`}>
      <p className="text-xs font-medium mb-1 text-gray-500">{label}</p>
      <p className="text-xl font-semibold font-geist tabular-nums">{value}</p>
    </div>
  );
}

export function HeroStatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="col-span-2 rounded-lg bg-white p-6">
      <p className="text-xs font-medium mb-1 text-gray-500">{label}</p>
      <p className="text-4xl font-bold tracking-tight font-geist">{value}</p>
      {sub && <p className="text-xs text-gray-400 mt-1">{sub}</p>}
    </div>
  );
}
