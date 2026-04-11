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

function FormGroup({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div style={{ marginBottom: 12 }}>
      <label
        style={{
          display: "block",
          color: "var(--text2)",
          fontSize: 11,
          marginBottom: 4,
          textTransform: "uppercase",
          letterSpacing: "0.05em",
        }}
      >
        {label}
      </label>
      {children}
    </div>
  );
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
};

export default function ToolsPage() {
  const { data: statsData } = useSWR<StatsResponse>("/admin/stats", fetcher);
  const channelNames = statsData ? Object.keys(statsData) : [];

  const [subject, setSubject] = useState("");
  const [apiKey, setApiKey] = useState("");
  const [channel, setChannel] = useState("");
  const [token, setToken] = useState("");
  const [tokenError, setTokenError] = useState("");
  const [issuingToken, setIssuingToken] = useState(false);
  const [copied, setCopied] = useState(false);

  async function handleIssueToken(e: React.FormEvent) {
    e.preventDefault();
    setIssuingToken(true);
    setToken("");
    setTokenError("");
    try {
      const result = await api.issueToken({
        subject,
        api_key: apiKey,
        channel: channel || undefined,
      });
      setToken(result.token);
    } catch (err) {
      setTokenError((err as Error).message);
    } finally {
      setIssuingToken(false);
    }
  }

  async function copyToken() {
    if (!token) return;
    await navigator.clipboard.writeText(token);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div>
      <PageTitle>工具</PageTitle>

      {/* JWT Issue */}
      <Panel>
        <SectionTitle>颁发 JWT Token</SectionTitle>
        <form onSubmit={handleIssueToken}>
          <FormGroup label="Subject (sub)">
            <input
              style={inputStyle}
              type="text"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              placeholder="user-123"
              required
            />
          </FormGroup>
          <FormGroup label="API Key">
            <input
              style={inputStyle}
              type="text"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder="kie_xxxxxxxx"
              required
            />
          </FormGroup>
          <FormGroup label="Channel（可选，空=不限制所有渠道）">
            <select
              style={inputStyle}
              value={channel}
              onChange={(e) => setChannel(e.target.value)}
            >
              <option value="">不限制（超级 Token）</option>
              {channelNames.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
          </FormGroup>

          <button
            type="submit"
            disabled={issuingToken}
            style={{
              padding: "8px 18px",
              background: issuingToken ? "rgba(88,166,255,0.4)" : "var(--blue)",
              border: "none",
              borderRadius: 6,
              color: "white",
              fontSize: 12,
              cursor: issuingToken ? "not-allowed" : "pointer",
              fontWeight: "bold",
            }}
          >
            {issuingToken ? "生成中…" : "生成 Token"}
          </button>
        </form>

        {token && (
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
              onClick={copyToken}
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
        )}

        {tokenError && (
          <div
            style={{
              marginTop: 12,
              padding: "8px 12px",
              background: "rgba(248,81,73,0.1)",
              borderRadius: 4,
              color: "var(--red)",
              fontSize: 12,
            }}
          >
            错误：{tokenError}
          </div>
        )}
      </Panel>

      {/* Bulk operations */}
      <Panel>
        <SectionTitle>批量操作</SectionTitle>
        <p style={{ color: "var(--text3)", fontSize: 12, marginBottom: 12 }}>
          对所有渠道和账号执行批量操作
        </p>
        <button
          onClick={() => window.location.reload()}
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
          🔄 刷新所有状态
        </button>
      </Panel>
    </div>
  );
}
