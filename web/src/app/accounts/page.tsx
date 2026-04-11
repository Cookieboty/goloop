"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";
import useSWR from "swr";
import { fetcher } from "@/lib/api";
import type { StatsResponse, ChannelAccountsResponse } from "@/lib/types";
import { AccountTable } from "@/components/AccountTable";
import { PageTitle, SectionTitle } from "@/components/PageTitle";

function AccountsContent() {
  const searchParams = useSearchParams();
  const selectedChannel = searchParams.get("channel") ?? "";

  const { data: statsData } = useSWR<StatsResponse>("/admin/stats", fetcher);
  const channelNames = statsData ? Object.keys(statsData) : [];

  const {
    data: accountsData,
    isLoading,
    error,
    mutate,
  } = useSWR<ChannelAccountsResponse>(
    selectedChannel ? `/admin/channel/${selectedChannel}/accounts` : null,
    fetcher,
    { refreshInterval: 15000 }
  );

  return (
    <div>
      <PageTitle>账号池</PageTitle>

      {/* Channel selector */}
      <div
        style={{
          display: "flex",
          gap: 8,
          marginBottom: 24,
          flexWrap: "wrap",
        }}
      >
        {channelNames.map((name) => {
          const active = name === selectedChannel;
          return (
            <a
              key={name}
              href={`?channel=${name}`}
              style={{
                padding: "6px 14px",
                borderRadius: 6,
                border: `1px solid ${active ? "var(--blue)" : "var(--border)"}`,
                background: active ? "rgba(88,166,255,0.1)" : "transparent",
                color: active ? "var(--blue)" : "var(--text2)",
                fontSize: 12,
                cursor: "pointer",
                textDecoration: "none",
              }}
            >
              {name}
            </a>
          );
        })}
        {channelNames.length === 0 && (
          <p style={{ color: "var(--text3)", fontSize: 12 }}>
            正在加载渠道列表…
          </p>
        )}
      </div>

      {!selectedChannel && (
        <p style={{ color: "var(--text3)" }}>请选择一个渠道查看账号列表</p>
      )}

      {selectedChannel && (
        <section>
          <div
            style={{
              display: "flex",
              justifyContent: "space-between",
              alignItems: "center",
              marginBottom: 16,
            }}
          >
            <SectionTitle>账号列表 — {selectedChannel}</SectionTitle>
            <button
              onClick={() => mutate()}
              style={{
                padding: "5px 12px",
                background: "transparent",
                border: "1px solid var(--border)",
                borderRadius: 4,
                color: "var(--text2)",
                fontSize: 11,
                cursor: "pointer",
              }}
            >
              🔄 刷新
            </button>
          </div>

          {isLoading && (
            <p style={{ color: "var(--text3)" }}>加载中…</p>
          )}
          {error && (
            <p style={{ color: "var(--red)" }}>
              加载失败：{(error as Error).message}
            </p>
          )}
          {accountsData && (
            <AccountTable
              channel={selectedChannel}
              accounts={accountsData.accounts ?? []}
              onRefresh={() => mutate()}
            />
          )}
        </section>
      )}
    </div>
  );
}

export default function AccountsPage() {
  return (
    <Suspense fallback={<p style={{ color: "var(--text3)" }}>加载中…</p>}>
      <AccountsContent />
    </Suspense>
  );
}
