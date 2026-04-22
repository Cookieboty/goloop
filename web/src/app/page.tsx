"use client";

import useSWR from "swr";
import { fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { StatCard } from "@/components/StatCard";
import { ChannelCard } from "@/components/ChannelCard";
import { PageTitle, SectionTitle } from "@/components/PageTitle";
import {
  groupByProtocol,
  getUnknownEntries,
  type ChannelEntry,
  type ProtocolSummary,
} from "@/lib/channelGroups";

export default function DashboardPage() {
  const { data, error, isLoading } = useSWR<StatsResponse>(
    "/admin/stats",
    fetcher,
    { refreshInterval: 10000 }
  );

  const channels: ChannelEntry[] = data ? Object.entries(data) : [];
  const totalSuccess = channels.reduce((s, [, c]) => s + c.total_success, 0);
  const totalFail = channels.reduce((s, [, c]) => s + c.total_fail, 0);
  const healthyChannels = channels.filter(([, c]) => c.is_healthy).length;
  const avgLatency =
    channels.length > 0
      ? Math.round(
          channels.reduce((s, [, c]) => s + c.avg_latency_ms, 0) /
            channels.length
        )
      : 0;

  const summaries = groupByProtocol(channels);
  const unknownEntries = getUnknownEntries(channels);

  return (
    <div>
      <PageTitle>概览</PageTitle>

      {isLoading && <p style={{ color: "var(--text3)" }}>加载中…</p>}
      {error && (
        <p style={{ color: "var(--red)" }}>
          加载失败：{(error as Error).message}
        </p>
      )}

      {data && (
        <>
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(4, 1fr)",
              gap: 16,
              marginBottom: 24,
            }}
          >
            <StatCard
              label="渠道总数"
              value={channels.length}
              sub={`${healthyChannels} 健康`}
            />
            <StatCard
              label="累计成功"
              value={totalSuccess.toLocaleString()}
              valueColor="var(--green)"
            />
            <StatCard
              label="累计失败"
              value={totalFail.toLocaleString()}
              valueColor={totalFail > 0 ? "var(--red)" : "var(--text)"}
            />
            <StatCard
              label="平均延迟"
              value={`${avgLatency}ms`}
              sub="所有渠道均值"
            />
          </div>

          <section style={{ marginBottom: 32 }}>
            <SectionTitle>按 API 协议</SectionTitle>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: `repeat(${summaries.length}, 1fr)`,
                gap: 16,
              }}
            >
              {summaries.map((s) => (
                <ProtocolSummaryCard key={s.protocol.id} summary={s} />
              ))}
            </div>
          </section>

          <section>
            <SectionTitle>渠道健康状态</SectionTitle>
            {summaries.map((summary) => (
              <ProtocolChannelsSection key={summary.protocol.id} summary={summary} />
            ))}

            {unknownEntries.length > 0 && (
              <div style={{ marginBottom: 24 }}>
                <h3
                  style={{
                    fontSize: 13,
                    fontWeight: "bold",
                    marginBottom: 10,
                    color: "var(--text3)",
                  }}
                >
                  ❓ 未分类（{unknownEntries.length}）
                </h3>
                <ChannelGrid entries={unknownEntries} />
              </div>
            )}

            {channels.length === 0 && (
              <p style={{ color: "var(--text3)" }}>暂无渠道数据</p>
            )}
          </section>
        </>
      )}
    </div>
  );
}

function ProtocolSummaryCard({ summary }: { summary: ProtocolSummary }) {
  const {
    protocol,
    totalChannels,
    healthyChannels,
    totalSuccess,
    totalFail,
    avgLatencyMs,
  } = summary;
  const unhealthy = totalChannels - healthyChannels;
  const successRate =
    totalSuccess + totalFail > 0
      ? ((totalSuccess / (totalSuccess + totalFail)) * 100).toFixed(1)
      : "—";

  return (
    <div
      style={{
        background: `color-mix(in srgb, ${protocol.color} 8%, var(--card))`,
        border: `1px solid ${protocol.color}`,
        borderRadius: 8,
        padding: 16,
      }}
    >
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 10,
          marginBottom: 12,
        }}
      >
        <span style={{ fontSize: 20 }}>{protocol.icon}</span>
        <div>
          <div
            style={{ fontSize: 14, fontWeight: "bold", color: protocol.color }}
          >
            {protocol.label}
          </div>
          <div style={{ fontSize: 10, color: "var(--text3)", marginTop: 2 }}>
            {protocol.desc}
          </div>
        </div>
      </div>

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(2, 1fr)",
          gap: 8,
          fontSize: 11,
          color: "var(--text2)",
        }}
      >
        <Stat
          label="渠道"
          value={`${totalChannels}`}
          sub={
            totalChannels === 0
              ? "未配置"
              : `${healthyChannels} 健康${
                  unhealthy > 0 ? ` · ${unhealthy} 异常` : ""
                }`
          }
          subColor={unhealthy > 0 ? "var(--red)" : "var(--green)"}
        />
        <Stat
          label="成功率"
          value={successRate === "—" ? "—" : `${successRate}%`}
          sub={`${totalSuccess.toLocaleString()} / ${(
            totalSuccess + totalFail
          ).toLocaleString()}`}
        />
        <Stat
          label="累计失败"
          value={totalFail.toLocaleString()}
          valueColor={totalFail > 0 ? "var(--red)" : "var(--text)"}
        />
        <Stat label="平均延迟" value={`${avgLatencyMs}ms`} />
      </div>
    </div>
  );
}

function Stat({
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
    <div>
      <div
        style={{
          color: "var(--text3)",
          fontSize: 10,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
        }}
      >
        {label}
      </div>
      <div
        style={{
          fontSize: 16,
          fontWeight: "bold",
          color: valueColor ?? "var(--text)",
          marginTop: 2,
        }}
      >
        {value}
      </div>
      {sub && (
        <div
          style={{
            color: subColor ?? "var(--text3)",
            fontSize: 10,
            marginTop: 2,
          }}
        >
          {sub}
        </div>
      )}
    </div>
  );
}

function ProtocolChannelsSection({ summary }: { summary: ProtocolSummary }) {
  const { protocol, entries } = summary;
  if (entries.length === 0) return null;
  return (
    <div style={{ marginBottom: 24 }}>
      <h3
        style={{
          fontSize: 13,
          fontWeight: "bold",
          marginBottom: 10,
          color: "var(--text2)",
          display: "flex",
          alignItems: "center",
          gap: 8,
        }}
      >
        <span style={{ color: protocol.color }}>
          {protocol.icon} {protocol.label}
        </span>
        <span style={{ color: "var(--text3)", fontWeight: "normal", fontSize: 11 }}>
          {entries.length} 个渠道
        </span>
      </h3>
      <ChannelGrid entries={entries} />
    </div>
  );
}

function ChannelGrid({ entries }: { entries: ChannelEntry[] }) {
  return (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(3, 1fr)",
        gap: 16,
      }}
    >
      {entries.map(([name, stats]) => (
        <ChannelCard key={name} name={name} stats={stats} />
      ))}
    </div>
  );
}
