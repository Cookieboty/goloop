"use client";

import useSWR from "swr";
import { fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";

function healthColor(score: number) {
  if (score >= 0.8) return "var(--green)";
  if (score >= 0.5) return "var(--orange)";
  return "var(--red)";
}

export default function StatsPage() {
  const { data, error, isLoading, mutate } = useSWR<StatsResponse>(
    "/admin/stats",
    fetcher,
    { refreshInterval: 10000 }
  );

  const channels = data ? Object.entries(data) : [];

  return (
    <div>
      <div
        style={{
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          marginBottom: 24,
        }}
      >
        <PageTitle>统计</PageTitle>
        <button
          onClick={() => mutate()}
          style={{
            padding: "6px 14px",
            background: "transparent",
            border: "1px solid var(--border)",
            borderRadius: 6,
            color: "var(--text2)",
            fontSize: 12,
            cursor: "pointer",
          }}
        >
          🔄 刷新
        </button>
      </div>

      {isLoading && <p style={{ color: "var(--text3)" }}>加载中…</p>}
      {error && (
        <p style={{ color: "var(--red)" }}>
          加载失败：{(error as Error).message}
        </p>
      )}

      {data && (
        <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
          {channels.map(([name, stats]) => {
            const total = stats.total_success + stats.total_fail;
            const successRate =
              total > 0
                ? ((stats.total_success / total) * 100).toFixed(2)
                : "—";
            const barWidth =
              total > 0 ? (stats.total_success / total) * 100 : 0;

            return (
              <div
                key={name}
                style={{
                  background: "var(--card)",
                  border: "1px solid var(--border)",
                  borderRadius: 8,
                  padding: 20,
                }}
              >
                <div
                  style={{
                    display: "flex",
                    justifyContent: "space-between",
                    alignItems: "flex-start",
                    marginBottom: 16,
                  }}
                >
                  <div>
                    <div
                      style={{
                        fontSize: 15,
                        fontWeight: "bold",
                        marginBottom: 4,
                      }}
                    >
                      {name}
                    </div>
                    <div
                      style={{
                        fontSize: 11,
                        color: stats.is_healthy
                          ? "var(--green)"
                          : "var(--red)",
                      }}
                    >
                      {stats.is_healthy ? "● 健康" : "● 异常"}
                    </div>
                  </div>
                  <div style={{ textAlign: "right" }}>
                    <div
                      style={{
                        fontSize: 22,
                        fontWeight: "bold",
                        color: healthColor(stats.health_score),
                      }}
                    >
                      {(stats.health_score * 100).toFixed(1)}%
                    </div>
                    <div style={{ fontSize: 10, color: "var(--text3)" }}>
                      健康分
                    </div>
                  </div>
                </div>

                {/* Success rate bar */}
                <div style={{ marginBottom: 16 }}>
                  <div
                    style={{
                      display: "flex",
                      justifyContent: "space-between",
                      fontSize: 11,
                      color: "var(--text2)",
                      marginBottom: 4,
                    }}
                  >
                    <span>成功率</span>
                    <span style={{ color: "var(--text)" }}>{successRate}%</span>
                  </div>
                  <div
                    style={{
                      height: 4,
                      background: "rgba(255,255,255,0.05)",
                      borderRadius: 2,
                      overflow: "hidden",
                    }}
                  >
                    <div
                      style={{
                        height: "100%",
                        width: `${barWidth}%`,
                        background:
                          barWidth >= 80
                            ? "var(--green)"
                            : barWidth >= 50
                            ? "var(--orange)"
                            : "var(--red)",
                        borderRadius: 2,
                        transition: "width 0.3s ease",
                      }}
                    />
                  </div>
                </div>

                {/* Metrics grid */}
                <div
                  style={{
                    display: "grid",
                    gridTemplateColumns: "repeat(4, 1fr)",
                    gap: 12,
                  }}
                >
                  {[
                    {
                      label: "成功次数",
                      value: stats.total_success.toLocaleString(),
                      color: "var(--green)",
                    },
                    {
                      label: "失败次数",
                      value: stats.total_fail.toLocaleString(),
                      color:
                        stats.total_fail > 0 ? "var(--red)" : "var(--text3)",
                    },
                    {
                      label: "总请求数",
                      value: total.toLocaleString(),
                      color: "var(--text)",
                    },
                    {
                      label: "平均延迟",
                      value: `${stats.avg_latency_ms}ms`,
                      color: "var(--blue)",
                    },
                  ].map((m) => (
                    <div key={m.label}>
                      <div
                        style={{
                          fontSize: 10,
                          color: "var(--text3)",
                          marginBottom: 2,
                          textTransform: "uppercase",
                          letterSpacing: "0.05em",
                        }}
                      >
                        {m.label}
                      </div>
                      <div
                        style={{
                          fontSize: 16,
                          fontWeight: "bold",
                          color: m.color,
                        }}
                      >
                        {m.value}
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            );
          })}

          {channels.length === 0 && (
            <p style={{ color: "var(--text3)" }}>暂无统计数据</p>
          )}
        </div>
      )}
    </div>
  );
}
