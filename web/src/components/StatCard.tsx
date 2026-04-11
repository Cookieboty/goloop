interface StatCardProps {
  label: string;
  value: string | number;
  sub?: string;
  valueColor?: string;
}

export function StatCard({ label, value, sub, valueColor }: StatCardProps) {
  return (
    <div
      style={{
        background: "var(--card)",
        border: "1px solid var(--border)",
        borderRadius: 8,
        padding: 16,
      }}
    >
      <div
        style={{ color: "var(--text2)", fontSize: 11, marginBottom: 8, textTransform: "uppercase", letterSpacing: "0.05em" }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: 24,
          fontWeight: "bold",
          color: valueColor ?? "var(--text)",
        }}
      >
        {value}
      </div>
      {sub && (
        <div style={{ color: "var(--text3)", fontSize: 10, marginTop: 4 }}>
          {sub}
        </div>
      )}
    </div>
  );
}
