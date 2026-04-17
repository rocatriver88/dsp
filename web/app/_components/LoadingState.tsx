"use client";

export function LoadingSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="h-10 rounded-lg animate-pulse"
          style={{ background: "var(--bg-card)" }} />
      ))}
    </div>
  );
}

export function LoadingCards({ count = 4 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="rounded-[14px] h-28 animate-pulse"
          style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }} />
      ))}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="text-center py-12 rounded-[14px]"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      <p className="text-sm mb-3" style={{ color: "var(--error)" }}>{message}</p>
      {onRetry && (
        <button onClick={onRetry}
          className="px-4 py-2 text-sm font-semibold text-white rounded-lg transition-colors"
          style={{ background: "var(--primary)" }}>
          重试
        </button>
      )}
    </div>
  );
}

export function EmptyState({ heading, message, actionLabel, actionHref, onAction }: {
  heading?: string;
  message: string;
  actionLabel?: string;
  actionHref?: string;
  onAction?: () => void;
}) {
  return (
    <div className="rounded-[14px] p-12 text-center"
      style={{ background: "var(--bg-card)", border: "1px solid var(--border)" }}>
      {heading && <p className="text-base font-semibold mb-2" style={{ color: "var(--text-primary)" }}>{heading}</p>}
      <p className="text-sm mb-1" style={{ color: "var(--text-secondary)" }}>{message}</p>
      {actionLabel && actionHref && (
        <a href={actionHref}
          className="inline-block mt-4 px-6 py-2.5 text-sm font-semibold text-white rounded-lg"
          style={{ background: "var(--primary)" }}>
          {actionLabel}
        </a>
      )}
      {actionLabel && onAction && !actionHref && (
        <button onClick={onAction}
          className="mt-4 px-6 py-2.5 text-sm font-semibold text-white rounded-lg"
          style={{ background: "var(--primary)" }}>
          {actionLabel}
        </button>
      )}
    </div>
  );
}
