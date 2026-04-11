"use client";

import useSWR from "swr";
import { fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { StatCard } from "@/components/StatCard";
import { ChannelCard } from "@/components/ChannelCard";
import { PageTitle, SectionTitle } from "@/components/PageTitle";

export default function DashboardPage() {
  const { data, error, isLoading } = useSWR<StatsResponse>(
    "/admin/stats",
    fetcher,
    { refreshInterval: 10000 }
  );

  const channels = data ? Object.entries(data) : [];
  const totalSuccess = channels.reduce(
    (s, [, c]) => s + c.total_success,
    0
  );
  const totalFail = channels.reduce((s, [, c]) => s + c.total_fail, 0);
  const healthyChannels = channels.filter(([, c]) => c.is_healthy).length;
  const avgLatency =
    channels.length > 0
      ? Math.round(
          channels.reduce((s, [, c]) => s + c.avg_latency_ms, 0) /
            channels.length
        )
      : 0;

  return (
    <div>
      <PageTitle>概览</PageTitle>

      {isLoading && (
        <p style={{ color: "var(--text3)" }}>加载中…</p>
      )}
      {error && (
        <p style={{ color: "var(--red)" }}>
          加载失败：{(error as Error).message}
        </p>
      )}

      {data && (
        <>
          {/* Global stat cards */}
          <div
            style={{
              display: "grid",
              gridTemplateColumns: "repeat(4, 1fr)",
              gap: 16,
              marginBottom: 32,
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

          {/* Channel health cards */}
          <section>
            <SectionTitle>渠道健康状态</SectionTitle>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(3, 1fr)",
                gap: 16,
              }}
            >
              {channels.map(([name, stats]) => (
                <ChannelCard key={name} name={name} stats={stats} />
              ))}
              {channels.length === 0 && (
                <p style={{ color: "var(--text3)" }}>暂无渠道数据</p>
              )}
            </div>
          </section>
        </>
      )}
    </div>
  );
}
