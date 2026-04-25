"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { clearAdminKey } from "@/lib/api";

const navItems = [
  { href: "/", label: "概览", icon: "📊" },
  { href: "/channels", label: "渠道管理", icon: "📡" },
  { href: "/api-keys", label: "API Key", icon: "🔑" },
  { href: "/accounts", label: "账号池", icon: "👤" },
  { href: "/error-logs", label: "错误日志", icon: "⚠️" },
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
    <aside className="w-64 bg-gray-900 border-r border-gray-800 flex flex-col h-screen sticky top-0 shadow-xl">
      {/* Logo 区域 */}
      <div className="px-6 py-5 border-b border-gray-800">
        <div className="flex items-center space-x-3">
          <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-600 rounded-lg flex items-center justify-center shadow-lg shadow-blue-500/20">
            <span className="text-xl">🛰</span>
          </div>
          <div>
            <div className="text-white font-bold text-lg tracking-tight">GoLoop</div>
            <div className="text-xs text-gray-500">AI Gateway</div>
          </div>
        </div>
      </div>

      {/* 导航菜单 */}
      <nav className="flex-1 py-4 px-3">
        {navItems.map((item) => {
          const isActive =
            item.href === "/" ? pathname === "/" : pathname.startsWith(item.href);
          return (
            <Link
              key={item.href}
              href={item.href}
              className={`group flex items-center space-x-3 px-4 py-3 rounded-lg mb-1 transition-all duration-200 ${
                isActive
                  ? "bg-gradient-to-r from-blue-600 to-blue-700 text-white shadow-lg shadow-blue-500/30"
                  : "text-gray-400 hover:bg-gray-800 hover:text-white"
              }`}
            >
              <span className="text-xl group-hover:scale-110 transition-transform duration-200">
                {item.icon}
              </span>
              <span className="font-medium text-sm">{item.label}</span>
              {isActive && (
                <span className="ml-auto w-1.5 h-1.5 bg-white rounded-full"></span>
              )}
            </Link>
          );
        })}
      </nav>

      {/* 底部操作 */}
      <div className="border-t border-gray-800">
        <button
          onClick={handleLogout}
          className="w-full px-6 py-4 flex items-center space-x-3 text-gray-500 hover:text-red-400 hover:bg-gray-800/50 transition-all duration-200 group"
        >
          <svg className="w-5 h-5 group-hover:scale-110 transition-transform" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" />
          </svg>
          <span className="text-sm font-medium">退出登录</span>
        </button>
        <div className="px-6 py-3 text-xs text-gray-600 flex items-center justify-between">
          <span>GoLoop v1.0</span>
          <div className="w-2 h-2 bg-green-500 rounded-full animate-pulse"></div>
        </div>
      </div>
    </aside>
  );
}
