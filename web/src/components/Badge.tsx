type Status = "healthy" | "degraded" | "unhealthy" | "offline";

const statusConfig: Record<
  Status,
  { label: string; bg: string; color: string }
> = {
  healthy: {
    label: "● healthy",
    bg: "rgba(46,160,67,0.15)",
    color: "var(--green)",
  },
  degraded: {
    label: "● degraded",
    bg: "rgba(210,153,34,0.15)",
    color: "var(--orange)",
  },
  unhealthy: {
    label: "● unhealthy",
    bg: "rgba(248,81,73,0.15)",
    color: "var(--red)",
  },
  offline: {
    label: "○ offline",
    bg: "rgba(110,118,129,0.15)",
    color: "var(--text3)",
  },
};

export function Badge({ status }: { status: Status }) {
  const cfg = statusConfig[status] ?? statusConfig.offline;
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        borderRadius: 12,
        fontSize: 10,
        background: cfg.bg,
        color: cfg.color,
        fontWeight: 500,
      }}
    >
      {cfg.label}
    </span>
  );
}
