"use client";

const styles: Record<string, { bg: string; text: string }> = {
  draft: { bg: "rgba(107,107,128,0.15)", text: "#6B6B80" },
  active: { bg: "rgba(34,197,94,0.15)", text: "#22C55E" },
  paused: { bg: "rgba(234,179,8,0.15)", text: "#EAB308" },
  completed: { bg: "rgba(59,130,246,0.15)", text: "#3B82F6" },
};

const zhLabels: Record<string, string> = {
  draft: "草稿",
  active: "运行中",
  paused: "已暂停",
  completed: "已完成",
};

export function StatusBadge({ status }: { status: string }) {
  const s = styles[status] || styles.draft;
  return (
    <span className="inline-block px-2.5 py-0.5 text-[11px] font-semibold rounded-full"
      style={{ background: s.bg, color: s.text }}>
      {zhLabels[status] || status}
    </span>
  );
}

const eventStyles: Record<string, { bg: string; text: string }> = {
  bid: { bg: "rgba(59,130,246,0.15)", text: "#3B82F6" },
  win: { bg: "rgba(34,197,94,0.15)", text: "#22C55E" },
  loss: { bg: "rgba(239,68,68,0.15)", text: "#EF4444" },
  impression: { bg: "rgba(139,92,246,0.15)", text: "#8B5CF6" },
  click: { bg: "rgba(249,115,22,0.15)", text: "#F97316" },
};

export function EventBadge({ type }: { type: string }) {
  const s = eventStyles[type] || { bg: "rgba(107,107,128,0.15)", text: "#6B6B80" };
  return (
    <span className="inline-block px-1.5 py-0.5 text-[11px] font-medium rounded"
      style={{ background: s.bg, color: s.text }}>
      {type}
    </span>
  );
}

export function TypeBadge({ type }: { type: string }) {
  return (
    <span className="inline-block px-2 py-0.5 text-[11px] font-medium rounded-full"
      style={{ background: "rgba(139,92,246,0.15)", color: "#A78BFA" }}>
      {type}
    </span>
  );
}
