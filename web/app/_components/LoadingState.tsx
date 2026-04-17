"use client";

export function LoadingSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="h-10 rounded-[14px] animate-pulse"
          style={{ background: "var(--glass-bg)" }} />
      ))}
    </div>
  );
}

export function LoadingCards({ count = 4 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="glass-card-static h-28 animate-pulse" />
      ))}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="glass-card-static text-center py-12 px-6">
      <p className="text-sm mb-3" style={{ color: "var(--error)" }}>{message}</p>
      {onRetry && (
        <button onClick={onRetry}
          className="btn-primary px-4 py-2 text-sm">
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
    <div className="glass-card-static p-12 text-center">
      {heading && <p className="text-base font-semibold mb-2" style={{ color: "var(--text-primary)" }}>{heading}</p>}
      <p className="text-sm mb-1" style={{ color: "var(--text-secondary)" }}>{message}</p>
      {actionLabel && actionHref && (
        <a href={actionHref}
          className="btn-primary inline-block mt-4 px-6 py-2.5 text-sm">
          {actionLabel}
        </a>
      )}
      {actionLabel && onAction && !actionHref && (
        <button onClick={onAction}
          className="btn-primary mt-4 px-6 py-2.5 text-sm">
          {actionLabel}
        </button>
      )}
    </div>
  );
}
