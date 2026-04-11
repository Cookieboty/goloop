"use client";

import { useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import Link from "next/link";

function healthColor(score: number) {
  if (score >= 0.8) return "var(--green)";
  if (score >= 0.5) return "var(--orange)";
  return "var(--red)";
}

export default function ChannelsPage() {
  const { data, error, isLoading, mutate } = useSWR<StatsResponse>(
    "/admin/stats",
    fetcher,
    { refreshInterval: 10000 }
  );

  const [editingChannel, setEditingChannel] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const channels = data ? Object.entries(data) : [];

  async function saveWeight(channel: string, value: string) {
    const weight = parseInt(value, 10);
    if (!weight || weight <= 0) {
      setEditingChannel(null);
      return;
    }
    try {
      await api.updateChannelWeight(channel, weight);
      setFeedback(`渠道 ${channel} 权重已更新为 ${weight}`);
      mutate();
    } catch (e) {
      setFeedback(`更新失败：${(e as Error).message}`);
    }
    setEditingChannel(null);
  }

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
        <PageTitle>渠道管理</PageTitle>
        <button
          onClick={() => mutate()}
          style={{
            padding: "6px 14px",
            background: "transparent",
            border: "1px solid var(--border)",
            borderRadius: 6,
            color: "var(--text2)",
            cursor: "pointer",
            fontSize: 12,
          }}
        >
          🔄 刷新
        </button>
      </div>

      {feedback && (
        <div
          style={{
            padding: "8px 12px",
            borderRadius: 4,
            background: feedback.includes("失败")
              ? "rgba(248,81,73,0.1)"
              : "rgba(46,160,67,0.1)",
            color: feedback.includes("失败") ? "var(--red)" : "var(--green)",
            fontSize: 12,
            marginBottom: 16,
          }}
        >
          {feedback}
        </div>
      )}

      {isLoading && <p style={{ color: "var(--text3)" }}>加载中…</p>}
      {error && (
        <p style={{ color: "var(--red)" }}>
          加载失败：{(error as Error).message}
        </p>
      )}

      {data && (
        <div
          style={{
            background: "var(--card)",
            border: "1px solid var(--border)",
            borderRadius: 8,
            overflow: "hidden",
          }}
        >
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr>
                {["渠道名称", "权重", "状态", "健康分", "成功次数", "失败次数", "平均延迟", "操作"].map(
                  (h) => (
                    <th
                      key={h}
                      style={{
                        textAlign: "left",
                        padding: "10px 16px",
                        color: "var(--text2)",
                        fontSize: 11,
                        borderBottom: "1px solid var(--border)",
                        textTransform: "uppercase",
                        letterSpacing: "0.05em",
                      }}
                    >
                      {h}
                    </th>
                  )
                )}
              </tr>
            </thead>
            <tbody>
              {channels.map(([name, stats]) => (
                <tr
                  key={name}
                  style={{
                    borderBottom: "1px solid var(--border)",
                  }}
                >
                  <td
                    style={{
                      padding: "12px 16px",
                      fontWeight: "bold",
                      fontSize: 13,
                    }}
                  >
                    {name}
                  </td>
                  <td style={{ padding: "12px 16px" }}>
                    {editingChannel === name ? (
                      <input
                        ref={inputRef}
                        type="number"
                        min={1}
                        defaultValue={stats.weight}
                        autoFocus
                        style={{
                          width: 64,
                          padding: "2px 6px",
                          border: "1px solid var(--blue)",
                          borderRadius: 4,
                          background: "var(--card)",
                          color: "var(--text)",
                          fontSize: 12,
                        }}
                        onBlur={(e) => saveWeight(name, e.target.value)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter") e.currentTarget.blur();
                          if (e.key === "Escape") setEditingChannel(null);
                        }}
                      />
                    ) : (
                      <span
                        style={{ cursor: "pointer", color: "var(--blue)" }}
                        onClick={() => {
                          setEditingChannel(name);
                          setTimeout(() => inputRef.current?.select(), 0);
                        }}
                        title="点击修改权重"
                      >
                        {stats.weight}
                      </span>
                    )}
                  </td>
                  <td style={{ padding: "12px 16px" }}>
                    <span
                      style={{
                        color: stats.is_healthy
                          ? "var(--green)"
                          : "var(--red)",
                        fontSize: 12,
                      }}
                    >
                      {stats.is_healthy ? "● 健康" : "● 异常"}
                    </span>
                  </td>
                  <td style={{ padding: "12px 16px" }}>
                    <span
                      style={{
                        color: healthColor(stats.health_score),
                        fontWeight: "bold",
                      }}
                    >
                      {(stats.health_score * 100).toFixed(1)}%
                    </span>
                  </td>
                  <td
                    style={{
                      padding: "12px 16px",
                      color: "var(--green)",
                    }}
                  >
                    {stats.total_success.toLocaleString()}
                  </td>
                  <td
                    style={{
                      padding: "12px 16px",
                      color:
                        stats.total_fail > 0 ? "var(--red)" : "var(--text3)",
                    }}
                  >
                    {stats.total_fail.toLocaleString()}
                  </td>
                  <td
                    style={{ padding: "12px 16px", color: "var(--text2)" }}
                  >
                    {stats.avg_latency_ms}ms
                  </td>
                  <td style={{ padding: "12px 16px" }}>
                    <Link
                      href={`/accounts?channel=${name}`}
                      style={{
                        padding: "4px 10px",
                        background: "transparent",
                        border: "1px solid var(--border)",
                        borderRadius: 4,
                        color: "var(--blue)",
                        fontSize: 11,
                        cursor: "pointer",
                      }}
                    >
                      查看账号
                    </Link>
                  </td>
                </tr>
              ))}
              {channels.length === 0 && (
                <tr>
                  <td
                    colSpan={8}
                    style={{
                      padding: "24px 16px",
                      color: "var(--text3)",
                      textAlign: "center",
                    }}
                  >
                    暂无渠道数据
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
