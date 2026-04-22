"use client";

import { useRef, useState } from "react";
import useSWR from "swr";
import { api, fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import {
  groupByProtocol,
  getUnknownEntries,
  type ChannelEntry,
  type ChannelTypeInfo,
  type ProtocolSummary,
} from "@/lib/channelGroups";
import Link from "next/link";

function healthColor(score: number) {
  if (score >= 0.8) return "var(--green)";
  if (score >= 0.5) return "var(--orange)";
  return "var(--red)";
}

interface ChannelTableProps {
  channels: ChannelEntry[];
  editingChannel: string | null;
  inputRef: React.RefObject<HTMLInputElement | null>;
  onStartEdit: (name: string) => void;
  onCancelEdit: () => void;
  onSaveWeight: (name: string, value: string) => void;
  onResetHealth: (name: string) => void;
}

function ChannelTable({
  channels,
  editingChannel,
  inputRef,
  onStartEdit,
  onCancelEdit,
  onSaveWeight,
  onResetHealth,
}: ChannelTableProps) {
  return (
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
              "渠道名称",
              "权重",
              "状态",
              "健康分",
              "成功次数",
              "失败次数",
              "平均延迟",
              "操作",
            ].map((h) => (
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
            ))}
          </tr>
        </thead>
        <tbody>
          {channels.map(([name, stats]) => (
            <tr
              key={name}
              style={{ borderBottom: "1px solid var(--border)" }}
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
                    onBlur={(e) => onSaveWeight(name, e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") e.currentTarget.blur();
                      if (e.key === "Escape") onCancelEdit();
                    }}
                  />
                ) : (
                  <span
                    style={{ cursor: "pointer", color: "var(--blue)" }}
                    onClick={() => onStartEdit(name)}
                    title="点击修改权重"
                  >
                    {stats.weight}
                  </span>
                )}
              </td>
              <td style={{ padding: "12px 16px" }}>
                <span
                  style={{
                    color: stats.is_healthy ? "var(--green)" : "var(--red)",
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
              <td style={{ padding: "12px 16px", color: "var(--green)" }}>
                {stats.total_success.toLocaleString()}
              </td>
              <td
                style={{
                  padding: "12px 16px",
                  color: stats.total_fail > 0 ? "var(--red)" : "var(--text3)",
                }}
              >
                {stats.total_fail.toLocaleString()}
              </td>
              <td style={{ padding: "12px 16px", color: "var(--text2)" }}>
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
                    textDecoration: "none",
                  }}
                >
                  查看账号
                </Link>
                {!stats.is_healthy && (
                  <button
                    onClick={() => onResetHealth(name)}
                    style={{
                      marginLeft: 8,
                      padding: "4px 10px",
                      background: "var(--orange)",
                      color: "white",
                      border: "none",
                      borderRadius: 4,
                      fontSize: 11,
                      cursor: "pointer",
                    }}
                  >
                    重置健康
                  </button>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ProtocolHeader({ summary }: { summary: ProtocolSummary }) {
  const { protocol, totalChannels, healthyChannels, totalSuccess, totalFail, avgLatencyMs } =
    summary;
  const unhealthy = totalChannels - healthyChannels;
  return (
    <div
      style={{
        display: "flex",
        alignItems: "stretch",
        gap: 16,
        padding: "14px 18px",
        background: `color-mix(in srgb, ${protocol.color} 8%, var(--card))`,
        border: `1px solid ${protocol.color}`,
        borderRadius: 8,
        marginBottom: 16,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          paddingRight: 16,
          borderRight: "1px solid var(--border)",
          minWidth: 220,
        }}
      >
        <span style={{ fontSize: 22 }}>{protocol.icon}</span>
        <div>
          <div
            style={{
              fontSize: 15,
              fontWeight: "bold",
              color: protocol.color,
            }}
          >
            {protocol.label}
          </div>
          <div style={{ fontSize: 11, color: "var(--text3)", marginTop: 2 }}>
            {protocol.desc}
          </div>
        </div>
      </div>
      <Metric
        label="渠道"
        value={`${totalChannels}`}
        sub={`${healthyChannels} 健康${unhealthy > 0 ? ` / ${unhealthy} 异常` : ""}`}
        subColor={unhealthy > 0 ? "var(--red)" : "var(--green)"}
      />
      <Metric
        label="累计成功"
        value={totalSuccess.toLocaleString()}
        valueColor="var(--green)"
      />
      <Metric
        label="累计失败"
        value={totalFail.toLocaleString()}
        valueColor={totalFail > 0 ? "var(--red)" : "var(--text)"}
      />
      <Metric label="平均延迟" value={`${avgLatencyMs}ms`} />
    </div>
  );
}

function Metric({
  label,
  value,
  sub,
  valueColor,
  subColor,
}: {
  label: string;
  value: string;
  sub?: string;
  valueColor?: string;
  subColor?: string;
}) {
  return (
    <div style={{ display: "flex", flexDirection: "column", justifyContent: "center", minWidth: 100 }}>
      <div
        style={{
          color: "var(--text2)",
          fontSize: 10,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: 18,
          fontWeight: "bold",
          color: valueColor ?? "var(--text)",
          marginTop: 2,
        }}
      >
        {value}
      </div>
      {sub && (
        <div style={{ color: subColor ?? "var(--text3)", fontSize: 10, marginTop: 2 }}>
          {sub}
        </div>
      )}
    </div>
  );
}

function TypeSection({
  type,
  channels,
  children,
}: {
  type: ChannelTypeInfo;
  channels: ChannelEntry[];
  children: React.ReactNode;
}) {
  return (
    <div style={{ marginBottom: 20 }}>
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          gap: 10,
          marginBottom: 8,
        }}
      >
        <span
          style={{
            fontSize: 10,
            padding: "2px 6px",
            borderRadius: 4,
            background: `color-mix(in srgb, ${type.color} 15%, transparent)`,
            color: type.color,
            border: `1px solid ${type.color}`,
            letterSpacing: "0.02em",
          }}
        >
          {type.id}
        </span>
        <h4 style={{ fontSize: 13, fontWeight: "bold", color: "var(--text)" }}>
          {type.label}
        </h4>
        <span style={{ fontSize: 11, color: "var(--text3)" }}>{type.desc}</span>
        <span style={{ fontSize: 11, color: "var(--text3)", marginLeft: "auto" }}>
          {channels.length} 个渠道
        </span>
      </div>
      {children}
    </div>
  );
}

function EmptyType({ type }: { type: ChannelTypeInfo }) {
  return (
    <div
      style={{
        padding: "10px 14px",
        border: "1px dashed var(--border)",
        borderRadius: 6,
        color: "var(--text3)",
        fontSize: 12,
      }}
    >
      暂未配置 {type.label}
    </div>
  );
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

  const channels: ChannelEntry[] = data ? Object.entries(data) : [];
  const summaries = groupByProtocol(channels);
  const unknownEntries = getUnknownEntries(channels);

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

  async function handleResetHealth(channel: string) {
    try {
      await api.resetChannelHealth(channel);
      setFeedback(`渠道 ${channel} 健康状态已重置`);
      mutate();
    } catch (e) {
      setFeedback(`重置失败：${(e as Error).message}`);
    }
  }

  function handleStartEdit(name: string) {
    setEditingChannel(name);
    setTimeout(() => inputRef.current?.select(), 0);
  }

  const tableHandlers = {
    editingChannel,
    inputRef,
    onStartEdit: handleStartEdit,
    onCancelEdit: () => setEditingChannel(null),
    onSaveWeight: saveWeight,
    onResetHealth: handleResetHealth,
  };

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
        <div>
          {summaries.map((summary) => (
            <ProtocolSection
              key={summary.protocol.id}
              summary={summary}
              tableHandlers={tableHandlers}
            />
          ))}

          {unknownEntries.length > 0 && (
            <div style={{ marginBottom: 32 }}>
              <h3
                style={{
                  fontSize: 14,
                  fontWeight: "bold",
                  marginBottom: 8,
                  color: "var(--text3)",
                }}
              >
                ❓ 未分类
                <span style={{ fontSize: 11, marginLeft: 8 }}>
                  （未识别的渠道类型，请检查后端配置）
                </span>
              </h3>
              <ChannelTable channels={unknownEntries} {...tableHandlers} />
            </div>
          )}

          {channels.length === 0 && (
            <p style={{ color: "var(--text3)", textAlign: "center", padding: 32 }}>
              暂无渠道数据
            </p>
          )}
        </div>
      )}
    </div>
  );
}

function ProtocolSection({
  summary,
  tableHandlers,
}: {
  summary: ProtocolSummary;
  tableHandlers: Omit<ChannelTableProps, "channels">;
}) {
  const { protocol, byType, totalChannels } = summary;

  if (totalChannels === 0) {
    return (
      <div style={{ marginBottom: 32 }}>
        <ProtocolHeader summary={summary} />
        <div
          style={{
            padding: "16px",
            border: "1px dashed var(--border)",
            borderRadius: 6,
            color: "var(--text3)",
            fontSize: 12,
            textAlign: "center",
          }}
        >
          暂未配置 {protocol.label} 渠道
        </div>
      </div>
    );
  }

  return (
    <div style={{ marginBottom: 32 }}>
      <ProtocolHeader summary={summary} />
      {protocol.types.map((type) => {
        const channels = byType[type.id];
        return (
          <TypeSection key={type.id} type={type} channels={channels}>
            {channels.length > 0 ? (
              <ChannelTable channels={channels} {...tableHandlers} />
            ) : (
              <EmptyType type={type} />
            )}
          </TypeSection>
        );
      })}
    </div>
  );
}

