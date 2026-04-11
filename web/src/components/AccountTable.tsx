"use client";

import { useState } from "react";
import type { AccountInfo } from "@/lib/types";
import { Badge } from "./Badge";
import { api } from "@/lib/api";

interface AccountTableProps {
  channel: string;
  accounts: AccountInfo[];
  onRefresh: () => void;
}

function maskKey(key: string) {
  if (key.length <= 8) return key;
  return key.slice(0, 4) + "****" + key.slice(-4);
}

function Btn({
  onClick,
  children,
  danger,
}: {
  onClick: () => void;
  children: React.ReactNode;
  danger?: boolean;
}) {
  return (
    <button
      onClick={onClick}
      style={{
        padding: "3px 8px",
        background: "transparent",
        border: `1px solid ${danger ? "var(--red)" : "var(--border)"}`,
        borderRadius: 4,
        color: danger ? "var(--red)" : "var(--text2)",
        fontSize: 11,
        cursor: "pointer",
        marginRight: 4,
      }}
    >
      {children}
    </button>
  );
}

export function AccountTable({
  channel,
  accounts,
  onRefresh,
}: AccountTableProps) {
  const [loading, setLoading] = useState<string | null>(null);
  const [feedback, setFeedback] = useState<string | null>(null);

  async function act(
    label: string,
    fn: () => Promise<void>
  ) {
    setLoading(label);
    setFeedback(null);
    try {
      await fn();
      setFeedback(`${label} 成功`);
      onRefresh();
    } catch (e) {
      setFeedback(`${label} 失败：${(e as Error).message}`);
    } finally {
      setLoading(null);
    }
  }

  return (
    <div>
      {feedback && (
        <div
          style={{
            padding: "8px 12px",
            borderRadius: 4,
            background: feedback.includes("失败")
              ? "rgba(248,81,73,0.1)"
              : "rgba(46,160,67,0.1)",
            color: feedback.includes("失败")
              ? "var(--red)"
              : "var(--green)",
            fontSize: 12,
            marginBottom: 12,
          }}
        >
          {feedback}
        </div>
      )}

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
              {[
                "API Key",
                "权重",
                "状态",
                "累计请求",
                "健康分",
                "连续失败",
                "操作",
              ].map((h) => (
                <th
                  key={h}
                  style={{
                    textAlign: "left",
                    padding: "10px 12px",
                    color: "var(--text2)",
                    fontSize: 11,
                    borderBottom: "1px solid var(--border)",
                    textTransform: "uppercase",
                    letterSpacing: "0.05em",
                  }}
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {accounts.map((acc) => (
              <tr
                key={acc.api_key}
                style={{ borderBottom: "1px solid var(--border)" }}
              >
                <td
                  style={{
                    padding: "10px 12px",
                    fontFamily: "monospace",
                    fontSize: 12,
                    color: "var(--text2)",
                  }}
                >
                  {maskKey(acc.api_key)}
                </td>
                <td style={{ padding: "10px 12px" }}>{acc.weight}</td>
                <td style={{ padding: "10px 12px" }}>
                  <Badge status={acc.status} />
                </td>
                <td style={{ padding: "10px 12px" }}>
                  {acc.usage_count.toLocaleString()}
                </td>
                <td style={{ padding: "10px 12px" }}>
                  {(acc.health_score * 100).toFixed(0)}%
                </td>
                <td
                  style={{
                    padding: "10px 12px",
                    color:
                      acc.consecutive_failures > 0
                        ? "var(--red)"
                        : "var(--text3)",
                  }}
                >
                  {acc.consecutive_failures}
                </td>
                <td style={{ padding: "10px 12px" }}>
                  <Btn
                    onClick={() =>
                      act(
                        `探测 ${maskKey(acc.api_key)}`,
                        () => api.probeAccount(channel, acc.api_key)
                      )
                    }
                  >
                    {loading === `探测 ${maskKey(acc.api_key)}`
                      ? "…"
                      : "探测"}
                  </Btn>
                  <Btn
                    onClick={() =>
                      act(
                        `重置 ${maskKey(acc.api_key)}`,
                        () => api.resetAccount(channel, acc.api_key)
                      )
                    }
                  >
                    {loading === `重置 ${maskKey(acc.api_key)}`
                      ? "…"
                      : "重置"}
                  </Btn>
                  <Btn
                    danger
                    onClick={() => {
                      if (
                        confirm(
                          `确认下线账号 ${maskKey(acc.api_key)}？此操作不可逆。`
                        )
                      ) {
                        act(
                          `下线 ${maskKey(acc.api_key)}`,
                          () => api.retireAccount(channel, acc.api_key)
                        );
                      }
                    }}
                  >
                    下线
                  </Btn>
                </td>
              </tr>
            ))}
            {accounts.length === 0 && (
              <tr>
                <td
                  colSpan={7}
                  style={{
                    padding: "24px 12px",
                    color: "var(--text3)",
                    textAlign: "center",
                  }}
                >
                  暂无账号数据
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
