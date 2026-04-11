"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { setAdminKey, api } from "@/lib/api";

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!password.trim()) return;

    setLoading(true);
    setError("");

    // Temporarily set the key and verify it with a real API call.
    setAdminKey(password.trim());

    const valid = await api.verifyAuth();
    if (valid) {
      router.push("/");
    } else {
      // Wrong password — clear it.
      setAdminKey("");
      setError("密码错误，请重试");
      setLoading(false);
    }
  }

  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        height: "100vh",
        background: "var(--bg)",
      }}
    >
      <div
        style={{
          background: "var(--card)",
          border: "1px solid var(--border)",
          borderRadius: 12,
          padding: "40px 48px",
          width: "100%",
          maxWidth: 400,
        }}
      >
        <div
          style={{
            fontSize: 22,
            fontWeight: "bold",
            marginBottom: 8,
            textAlign: "center",
          }}
        >
          🛰 goloop
        </div>
        <div
          style={{
            fontSize: 12,
            color: "var(--text2)",
            textAlign: "center",
            marginBottom: 32,
          }}
        >
          Admin Console
        </div>

        <form onSubmit={handleSubmit}>
          <div style={{ marginBottom: 16 }}>
            <label
              style={{
                display: "block",
                fontSize: 11,
                color: "var(--text2)",
                marginBottom: 6,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
              }}
            >
              Admin 密码
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoFocus
              required
              placeholder="输入 ADMIN_PASSWORD"
              style={{
                width: "100%",
                padding: "10px 14px",
                background: "var(--bg)",
                border: `1px solid ${error ? "var(--red)" : "var(--border)"}`,
                borderRadius: 6,
                color: "var(--text)",
                fontSize: 13,
                outline: "none",
              }}
            />
          </div>

          {error && (
            <div
              style={{
                padding: "8px 12px",
                background: "rgba(248,81,73,0.1)",
                border: "1px solid rgba(248,81,73,0.3)",
                borderRadius: 4,
                color: "var(--red)",
                fontSize: 12,
                marginBottom: 16,
              }}
            >
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={loading || !password.trim()}
            style={{
              width: "100%",
              padding: "10px",
              background: loading ? "rgba(88,166,255,0.4)" : "var(--blue)",
              border: "none",
              borderRadius: 6,
              color: "white",
              fontSize: 13,
              fontWeight: "bold",
              cursor: loading ? "not-allowed" : "pointer",
            }}
          >
            {loading ? "验证中…" : "登录"}
          </button>
        </form>

        <p
          style={{
            marginTop: 24,
            fontSize: 10,
            color: "var(--text3)",
            textAlign: "center",
            lineHeight: 1.6,
          }}
        >
          密码通过 ADMIN_PASSWORD 环境变量设置
          <br />
          密码保存在浏览器 localStorage 中
        </p>
      </div>
    </div>
  );
}
