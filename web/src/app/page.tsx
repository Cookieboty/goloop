"use client";

import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { Channel, APIKey } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";
import Link from "next/link";

export default function OverviewPage() {
  const dialog = useDialog();
  const [channels, setChannels] = useState<Channel[]>([]);
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    try {
      setLoading(true);
      const [channelsData, apiKeysData] = await Promise.all([
        api.getChannels(),
        api.getAPIKeys(),
      ]);
      setChannels(channelsData);
      setAPIKeys(apiKeysData);
    } catch (err: any) {
      console.error("加载失败:", err);
    } finally {
      setLoading(false);
    }
  }

  async function handleReload() {
    try {
      await api.reloadConfig();
      await dialog.alert("配置已重载！", "成功");
      await loadData();
    } catch (err: any) {
      await dialog.alert("重载失败：" + err.message, "错误");
    }
  }

  if (loading) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>系统概览</PageTitle>
        <div className="mt-8 flex items-center justify-center py-12">
          <div className="text-center">
            <div className="inline-block animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500 mb-4"></div>
            <div className="text-gray-400">加载数据中...</div>
          </div>
        </div>
      </div>
    );
  }

  const enabledChannels = channels.filter((ch) => ch.enabled);
  const totalAccounts = channels.reduce(
    (sum, ch) => sum + (ch.accounts?.length || 0),
    0
  );
  const enabledAPIKeys = apiKeys.filter((key) => key.enabled);
  const totalRequests = apiKeys.reduce((sum, key) => sum + key.total_requests, 0);

  return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>系统概览</PageTitle>
          <p className="text-gray-500 text-sm mt-1">实时监控渠道状态和 API 使用情况</p>
        </div>
        <button
          onClick={handleReload}
          className="flex items-center space-x-2 px-5 py-2.5 bg-gradient-to-r from-blue-600 to-blue-700 hover:from-blue-700 hover:to-blue-800 text-white rounded-lg transition-all duration-200 shadow-lg shadow-blue-500/25 font-medium"
        >
          <svg className="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          <span>重载配置</span>
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
        <StatCard
          title="渠道总数"
          value={channels.length}
          subtitle={`${enabledChannels.length} 个启用`}
          icon="🔌"
          link="/channels"
        />
        <StatCard
          title="账号总数"
          value={totalAccounts}
          subtitle="所有渠道账号池"
          icon="👤"
          link="/accounts"
        />
        <StatCard
          title="API Keys"
          value={apiKeys.length}
          subtitle={`${enabledAPIKeys.length} 个启用`}
          icon="🔑"
          link="/api-keys"
        />
        <StatCard
          title="总请求数"
          value={totalRequests}
          subtitle="所有 API Key"
          icon="📊"
          link="/api-keys"
        />
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <div className="bg-gradient-to-br from-gray-900 to-gray-800 border border-gray-700 rounded-xl p-6 shadow-xl">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-xl font-bold text-white flex items-center">
              <span className="text-2xl mr-3">🔌</span>
              渠道概览
            </h2>
            <Link
              href="/channels"
              className="text-blue-400 hover:text-blue-300 text-sm font-medium transition-colors"
            >
              查看全部 →
            </Link>
          </div>
          {channels.length === 0 ? (
            <div className="text-center py-12 px-4">
              <div className="text-5xl mb-4 opacity-20">🔌</div>
              <p className="text-gray-500 mb-4">暂无渠道配置</p>
              <Link
                href="/channels"
                className="inline-flex items-center px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded-lg transition-colors"
              >
                <svg className="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                创建第一个渠道
              </Link>
            </div>
          ) : (
            <div className="space-y-2">
              {channels.slice(0, 5).map((ch) => (
                <div
                  key={ch.id}
                  className="flex justify-between items-center p-3 rounded-lg bg-gray-800/50 border border-gray-700/50 hover:border-gray-600 transition-colors"
                >
                  <div className="flex-1 min-w-0">
                    <div className="font-semibold text-white mb-1">{ch.name}</div>
                    <div className="text-xs text-gray-400 flex items-center space-x-3">
                      <span className="px-2 py-0.5 bg-gray-700 rounded text-gray-300">{ch.type}</span>
                      <span>权重 {ch.weight}</span>
                      <span>{ch.accounts?.length || 0} 个账号</span>
                    </div>
                  </div>
                  <span
                    className={`ml-3 px-3 py-1 text-xs font-medium rounded-full ${
                      ch.enabled
                        ? "bg-green-500/20 text-green-400 border border-green-500/30"
                        : "bg-gray-700 text-gray-400 border border-gray-600"
                    }`}
                  >
                    {ch.enabled ? "● 启用" : "○ 禁用"}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="bg-gradient-to-br from-gray-900 to-gray-800 border border-gray-700 rounded-xl p-6 shadow-xl">
          <div className="flex items-center justify-between mb-6">
            <h2 className="text-xl font-bold text-white flex items-center">
              <span className="text-2xl mr-3">🔑</span>
              API Key 概览
            </h2>
            <Link
              href="/api-keys"
              className="text-blue-400 hover:text-blue-300 text-sm font-medium transition-colors"
            >
              查看全部 →
            </Link>
          </div>
          {apiKeys.length === 0 ? (
            <div className="text-center py-12 px-4">
              <div className="text-5xl mb-4 opacity-20">🔑</div>
              <p className="text-gray-500 mb-4">暂无 API Key</p>
              <Link
                href="/api-keys"
                className="inline-flex items-center px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white text-sm rounded-lg transition-colors"
              >
                <svg className="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                颁发第一个 API Key
              </Link>
            </div>
          ) : (
            <div className="space-y-2">
              {apiKeys.slice(0, 5).map((key) => (
                <div
                  key={key.id}
                  className="flex justify-between items-center p-3 rounded-lg bg-gray-800/50 border border-gray-700/50 hover:border-gray-600 transition-colors"
                >
                  <div className="flex-1 min-w-0">
                    <div className="font-semibold text-white mb-1">{key.name}</div>
                    <div className="text-xs text-gray-400 flex items-center space-x-2">
                      <span className="flex items-center">
                        <span className="w-1.5 h-1.5 bg-blue-400 rounded-full mr-1"></span>
                        {key.total_requests} 请求
                      </span>
                      <span className="text-gray-600">|</span>
                      <span className="flex items-center text-green-400">
                        {key.total_success} 成功
                      </span>
                      <span className="text-gray-600">|</span>
                      <span className="flex items-center text-red-400">
                        {key.total_fail} 失败
                      </span>
                    </div>
                  </div>
                  <span
                    className={`ml-3 px-3 py-1 text-xs font-medium rounded-full ${
                      key.enabled
                        ? "bg-green-500/20 text-green-400 border border-green-500/30"
                        : "bg-gray-700 text-gray-400 border border-gray-600"
                    }`}
                  >
                    {key.enabled ? "● 启用" : "○ 禁用"}
                  </span>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      <div className="mt-8 bg-gradient-to-r from-blue-900/30 to-purple-900/30 border border-blue-700/30 rounded-xl p-6 shadow-lg">
        <div className="flex items-start">
          <div className="text-3xl mr-4">💡</div>
          <div className="flex-1">
            <h3 className="font-bold text-white text-lg mb-3">快速开始</h3>
            <ol className="text-sm space-y-2.5 ml-5 list-decimal text-gray-300">
              <li>
                前往{" "}
                <Link href="/channels" className="text-blue-400 hover:text-blue-300 font-medium underline decoration-dotted">
                  渠道管理
                </Link>{" "}
                创建渠道配置
              </li>
              <li>在渠道中添加 API Key 账号</li>
              <li>
                前往{" "}
                <Link href="/api-keys" className="text-blue-400 hover:text-blue-300 font-medium underline decoration-dotted">
                  API Key 管理
                </Link>{" "}
                颁发客户端密钥
              </li>
              <li>
                使用颁发的{" "}
                <code className="px-2 py-0.5 bg-gray-800 text-blue-400 rounded border border-gray-700 font-mono text-xs">
                  goloop_xxxxx
                </code>{" "}
                密钥调用 API
              </li>
            </ol>
          </div>
        </div>
      </div>
    </div>
  );
}

function StatCard({
  title,
  value,
  subtitle,
  icon,
  link,
}: {
  title: string;
  value: number;
  subtitle: string;
  icon: string;
  link?: string;
}) {
  const content = (
    <div className="group relative bg-gradient-to-br from-gray-900 to-gray-800 border border-gray-700 rounded-xl p-6 hover:border-blue-500/50 transition-all duration-300 hover:shadow-xl hover:shadow-blue-500/10">
      <div className="flex justify-between items-start mb-3">
        <div className="text-gray-400 text-sm font-medium uppercase tracking-wide">{title}</div>
        <div className="text-3xl opacity-80 group-hover:scale-110 transition-transform duration-300">{icon}</div>
      </div>
      <div className="text-4xl font-bold mb-2 bg-gradient-to-r from-white to-gray-300 bg-clip-text text-transparent">
        {value.toLocaleString()}
      </div>
      <div className="text-xs text-gray-500 flex items-center">
        {subtitle}
        {link && (
          <svg className="w-3 h-3 ml-1 opacity-0 group-hover:opacity-100 transition-opacity" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
          </svg>
        )}
      </div>
    </div>
  );

  if (link) {
    return <Link href={link} className="block">{content}</Link>;
  }
  return content;
}
