import type { ChannelStats } from "@/lib/types";

interface ChannelCardProps {
  name: string;
  stats: ChannelStats;
}

function StatusDot({ healthy }: { healthy: boolean }) {
  return (
    <span
      style={{
        display: "inline-block",
        width: 8,
        height: 8,
        borderRadius: "50%",
        background: healthy ? "var(--green)" : "var(--red)",
        marginRight: 6,
      }}
    />
  );
}

export function ChannelCard({ name, stats }: ChannelCardProps) {
  const scorePercent = (stats.health_score * 100).toFixed(1);
  const total = stats.total_success + stats.total_fail;
  const successRate =
    total > 0 ? ((stats.total_success / total) * 100).toFixed(1) : "—";

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
        style={{ fontSize: 14, fontWeight: "bold", marginBottom: 10 }}
      >
        {name}
      </div>

      <div
        style={{
          display: "flex",
          alignItems: "center",
          fontSize: 12,
          marginBottom: 6,
          color: stats.is_healthy ? "var(--green)" : "var(--red)",
        }}
      >
        <StatusDot healthy={stats.is_healthy} />
        {stats.is_healthy ? "健康" : "异常"}
      </div>

      <div style={{ fontSize: 11, color: "var(--text2)", lineHeight: 1.8 }}>
        <div>
          健康分:{" "}
          <span style={{ color: "var(--text)" }}>{scorePercent}%</span>
        </div>
        <div>
          成功率:{" "}
          <span style={{ color: "var(--text)" }}>{successRate}%</span>
        </div>
        <div>
          延迟:{" "}
          <span style={{ color: "var(--text)" }}>{stats.avg_latency_ms}ms</span>
        </div>
        <div>
          成功 / 失败:{" "}
          <span style={{ color: "var(--green)" }}>{stats.total_success}</span>
          {" / "}
          <span style={{ color: "var(--red)" }}>{stats.total_fail}</span>
        </div>
      </div>
    </div>
  );
}
