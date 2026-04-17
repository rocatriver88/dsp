"use client";

interface GlassCardProps {
  children: React.ReactNode;
  className?: string;
  padding?: string;
  hover?: boolean;
}

export default function GlassCard({ children, className = "", padding = "p-5", hover = true }: GlassCardProps) {
  return (
    <div className={`${hover ? "glass-card" : "glass-card-static"} ${padding} ${className}`}>
      {children}
    </div>
  );
}
