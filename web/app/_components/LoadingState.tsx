"use client";

export function LoadingSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <div className="animate-pulse space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="h-10 bg-gray-100 rounded" />
      ))}
    </div>
  );
}

export function LoadingCards({ count = 4 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="animate-pulse bg-gray-100 rounded-lg h-24" />
      ))}
    </div>
  );
}

export function ErrorState({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="text-center py-12">
      <p className="text-sm text-red-600 mb-3">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-md hover:bg-blue-700"
        >
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
    <div className="rounded-lg bg-white p-12 text-center">
      {heading && <p className="text-base font-medium mb-2">{heading}</p>}
      <p className="text-sm text-gray-500 mb-1">{message}</p>
      {actionLabel && actionHref && (
        <a href={actionHref}
          className="inline-block mt-4 px-6 py-2.5 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
          {actionLabel}
        </a>
      )}
      {actionLabel && onAction && !actionHref && (
        <button onClick={onAction}
          className="mt-4 px-6 py-2.5 text-sm font-medium text-white rounded-md bg-blue-600 hover:bg-blue-700">
          {actionLabel}
        </button>
      )}
    </div>
  );
}
