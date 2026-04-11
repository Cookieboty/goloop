"use client";

import { useState } from "react";
import useSWR from "swr";
import { api, fetcher } from "@/lib/api";
import type { StatsResponse } from "@/lib/types";
import { PageTitle, SectionTitle } from "@/components/PageTitle";

function Panel({ children }: { children: React.ReactNode }) {
  return (
    <div
      style={{
        background: "var(--card)",
        border: "1px solid var(--border)",
        borderRadius: 8,
        padding: 20,
        marginBottom: 20,
      }}
    >
      {children}
    </div>
  );
}

function TokenDisplay({ token }: { token: string }) {
  const [copied, setCopied] = useState(false);
  async function copy() {
    await navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div style={{ marginTop: 16 }}>
      <div
        style={{
          padding: "12px",
          background: "var(--bg)",
          borderRadius: 4,
          wordBreak: "break-all",
          fontSize: 11,
          color: "var(--green)",
          lineHeight: 1.6,
          border: "1px solid var(--border)",
          marginBottom: 8,
        }}
      >
        {token}
      </div>
      <button
        onClick={copy}
        style={{
          padding: "5px 12px",
          background: "transparent",
          border: "1px solid var(--border)",
          borderRadius: 4,
          color: copied ? "var(--green)" : "var(--text2)",
          fontSize: 11,
          cursor: "pointer",
        }}
      >
        {copied ? "✓ 已复制" : "复制"}
      </button>
    </div>
  );
}

export default function ToolsPage() {
  const { data: statsData } = useSWR<StatsResponse>("/admin/stats", fetcher);
  const channelNames = statsData ? Object.keys(statsData) : [];

  // Quick token
  const [quickToken, setQuickToken] = useState("");
  const [quickError, setQuickError] = useState("");
  const [quickLoading, setQuickLoading] = useState(false);

  async function handleQuickToken() {
    setQuickLoading(true);
    setQuickToken("");
    setQuickError("");
    try {
      const res = await api.quickToken();
      setQuickToken(res.token);
    } catch (e) {
      setQuickError((e as Error).message);
    } finally {
      setQuickLoading(false);
    }
  }

  // Advanced token
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [subject, setSubject] = useState("user");
  const [channel, setChannel] = useState("");
  const [advToken, setAdvToken] = useState("");
  const [advError, setAdvError] = useState("");
  const [advLoading, setAdvLoading] = useState(false);

  async function handleAdvanced(e: React.FormEvent) {
    e.preventDefault();
    setAdvLoading(true);
    setAdvToken("");
    setAdvError("");
    try {
      const res = await api.issueToken({ subject, channel: channel || undefined });
      setAdvToken(res.token);
    } catch (e) {
      setAdvError((e as Error).message);
    } finally {
      setAdvLoading(false);
    }
  }

  const inputStyle: React.CSSProperties = {
    width: "100%",
    padding: "8px 12px",
    background: "var(--bg)",
    border: "1px solid var(--border)",
    borderRadius: 4,
    color: "var(--text)",
    fontSize: 12,
    outline: "none",
    boxSizing: "border-box",
  };

  return (
    <div>
      <PageTitle>工具</PageTitle>

      {/* Quick token */}
      <Panel>
        <SectionTitle>生成 Token</SectionTitle>
        <p style={{ color: "var(--text3)", fontSize: 12, marginBottom: 16 }}>
          生成用于调用图片生成接口的 JWT Token，账号由 goloop 自动从池中选取。
        </p>
        <button
          onClick={handleQuickToken}
          disabled={quickLoading}
          style={{
            padding: "8px 20px",
            background: quickLoading ? "rgba(88,166,255,0.4)" : "var(--blue)",
            border: "none",
            borderRadius: 6,
            color: "white",
            fontSize: 13,
            fontWeight: "bold",
            cursor: quickLoading ? "not-allowed" : "pointer",
          }}
        >
          {quickLoading ? "生成中…" : "生成 Token"}
        </button>

        {quickToken && <TokenDisplay token={quickToken} />}
        {quickError && (
          <div style={{ marginTop: 12, color: "var(--red)", fontSize: 12 }}>
            错误：{quickError}
          </div>
        )}
      </Panel>

      {/* Advanced */}
      <Panel>
        <div
          style={{ display: "flex", justifyContent: "space-between", alignItems: "center", cursor: "pointer" }}
          onClick={() => setShowAdvanced((v) => !v)}
        >
          <SectionTitle>高级：自定义 Token</SectionTitle>
          <span style={{ color: "var(--text3)", fontSize: 12 }}>{showAdvanced ? "▲ 收起" : "▼ 展开"}</span>
        </div>

        {showAdvanced && (
          <form onSubmit={handleAdvanced} style={{ marginTop: 16 }}>
            <div style={{ marginBottom: 12 }}>
              <label style={{ display: "block", color: "var(--text2)", fontSize: 11, marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
                Subject (sub)
              </label>
              <input style={inputStyle} type="text" value={subject} onChange={(e) => setSubject(e.target.value)} placeholder="user-123" required />
            </div>
            <div style={{ marginBottom: 16 }}>
              <label style={{ display: "block", color: "var(--text2)", fontSize: 11, marginBottom: 4, textTransform: "uppercase", letterSpacing: "0.05em" }}>
                Channel（可选，限定只能用指定渠道）
              </label>
              <select style={inputStyle} value={channel} onChange={(e) => setChannel(e.target.value)}>
                <option value="">不限制（所有渠道）</option>
                {channelNames.map((name) => (
                  <option key={name} value={name}>{name}</option>
                ))}
              </select>
            </div>
            <button
              type="submit"
              disabled={advLoading}
              style={{
                padding: "8px 18px",
                background: advLoading ? "rgba(88,166,255,0.4)" : "var(--blue)",
                border: "none",
                borderRadius: 6,
                color: "white",
                fontSize: 12,
                cursor: advLoading ? "not-allowed" : "pointer",
                fontWeight: "bold",
              }}
            >
              {advLoading ? "生成中…" : "生成 Token"}
            </button>

            {advToken && <TokenDisplay token={advToken} />}
            {advError && (
              <div style={{ marginTop: 12, color: "var(--red)", fontSize: 12 }}>
                错误：{advError}
              </div>
            )}
          </form>
        )}
      </Panel>
    </div>
  );
}
