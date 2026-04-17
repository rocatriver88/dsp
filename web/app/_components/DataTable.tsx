"use client";

interface Column<T> {
  key: string;
  header: string;
  align?: "left" | "right" | "center";
  render: (row: T) => React.ReactNode;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  data: T[];
  keyFn: (row: T) => string | number;
  emptyMessage?: string;
}

export default function DataTable<T>({ columns, data, keyFn, emptyMessage = "暂无数据" }: DataTableProps<T>) {
  return (
    <div className="glass-card-static p-0 overflow-hidden animate-fade-in">
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr style={{ background: "var(--bg-card-elevated)" }}>
              {columns.map((col) => (
                <th key={col.key}
                  className={`py-3 px-4 text-[11px] font-semibold uppercase tracking-wider ${col.align === "right" ? "text-right" : "text-left"}`}
                  style={{ color: "var(--text-muted)" }}>
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.length === 0 ? (
              <tr>
                <td colSpan={columns.length} className="py-12 text-center text-sm"
                  style={{ color: "var(--text-secondary)" }}>
                  {emptyMessage}
                </td>
              </tr>
            ) : data.map((row) => (
              <tr key={keyFn(row)}
                className="transition-colors"
                style={{ borderTop: "1px solid var(--border-subtle)" }}
                onMouseEnter={(e) => { e.currentTarget.style.background = "rgba(255,255,255,0.02)"; }}
                onMouseLeave={(e) => { e.currentTarget.style.background = "transparent"; }}>
                {columns.map((col) => (
                  <td key={col.key}
                    className={`py-3 px-4 ${col.align === "right" ? "text-right tabular-nums" : ""}`}
                    style={{ color: "var(--text-secondary)" }}>
                    {col.render(row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
