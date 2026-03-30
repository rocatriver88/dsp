"use client";

const styles: Record<string, string> = {
  draft: "bg-gray-100 text-gray-600",
  active: "bg-green-50 text-green-700",
  paused: "bg-yellow-50 text-yellow-700",
  completed: "bg-blue-50 text-blue-700",
};

export function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`inline-block px-2 py-0.5 text-xs font-medium rounded-full ${styles[status] || styles.draft}`}>
      {status}
    </span>
  );
}

const eventStyles: Record<string, string> = {
  bid: "bg-blue-50 text-blue-700",
  win: "bg-green-50 text-green-700",
  loss: "bg-red-50 text-red-600",
  impression: "bg-purple-50 text-purple-700",
  click: "bg-orange-50 text-orange-700",
};

export function EventBadge({ type }: { type: string }) {
  return (
    <span className={`inline-block px-1.5 py-0.5 text-xs font-medium rounded ${eventStyles[type] || "bg-gray-100 text-gray-600"}`}>
      {type}
    </span>
  );
}
