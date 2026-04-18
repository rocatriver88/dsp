"use client";

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  action?: React.ReactNode;
}

export default function PageHeader({ title, subtitle, action }: PageHeaderProps) {
  return (
    <div className="flex items-start justify-between mb-6">
      <div>
        <h2 className="text-2xl font-bold" style={{ color: "var(--text-primary)" }}>{title}</h2>
        {subtitle && <p className="text-[13px] mt-1" style={{ color: "var(--text-secondary)" }}>{subtitle}</p>}
      </div>
      {action && <div className="flex-shrink-0 ml-4">{action}</div>}
    </div>
  );
}
