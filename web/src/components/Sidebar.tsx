"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { clearAdminKey } from "@/lib/api";

const navItems = [
  { href: "/", label: "概览", icon: "📊" },
  { href: "/channels", label: "渠道管理", icon: "📡" },
  { href: "/accounts", label: "账号池", icon: "👤" },
  { href: "/tools", label: "工具", icon: "🔧" },
  { href: "/stats", label: "统计", icon: "📈" },
];

export function Sidebar() {
  const pathname = usePathname();
  const router = useRouter();

  function handleLogout() {
    clearAdminKey();
    router.push("/login");
  }

  return (
    <aside
      style={{
        width: 220,
        background: "var(--card)",
        borderRight: "1px solid var(--border)",
        display: "flex",
        flexDirection: "column",
        flexShrink: 0,
        height: "100vh",
        position: "sticky",
        top: 0,
      }}
    >
      <div
        style={{
          padding: "20px",
          fontSize: 16,
          fontWeight: "bold",
          borderBottom: "1px solid var(--border)",
          letterSpacing: "0.02em",
        }}
      >
        🛰 goloop
      </div>

      <nav style={{ flex: 1, padding: "10px 0" }}>
        {navItems.map((item) => {
          const isActive =
            item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              style={{
                display: "block",
                padding: "10px 20px",
                color: isActive ? "var(--text)" : "var(--text2)",
                background: isActive
                  ? "rgba(255,255,255,0.05)"
                  : "transparent",
                borderLeft: isActive
                  ? "2px solid var(--blue)"
                  : "2px solid transparent",
                transition: "all 0.1s",
              }}
            >
              <span style={{ marginRight: 8 }}>{item.icon}</span>
              {item.label}
            </Link>
          );
        })}
      </nav>

      <div
        style={{
          borderTop: "1px solid var(--border)",
        }}
      >
        <button
          onClick={handleLogout}
          style={{
            width: "100%",
            padding: "12px 20px",
            background: "transparent",
            border: "none",
            color: "var(--text3)",
            fontSize: 12,
            cursor: "pointer",
            textAlign: "left",
            display: "flex",
            alignItems: "center",
            gap: 8,
          }}
          onMouseEnter={(e) =>
            ((e.currentTarget as HTMLElement).style.color = "var(--red)")
          }
          onMouseLeave={(e) =>
            ((e.currentTarget as HTMLElement).style.color = "var(--text3)")
          }
        >
          <span>🚪</span>
          退出登录
        </button>
        <div
          style={{
            padding: "8px 20px 16px",
            fontSize: 11,
            color: "var(--text3)",
          }}
        >
          goloop v0.2
        </div>
      </div>
    </aside>
  );
}
