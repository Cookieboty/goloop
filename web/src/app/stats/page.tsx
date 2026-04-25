"use client";

import useSWR from "swr";
import { api } from "@/lib/api";
import type { GlobalStatsResponse } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";

export default function StatsPage() {
  const dialog = useDialog();
  const { data, error, isLoading, mutate } = useSWR<GlobalStatsResponse>(
    "/admin/api/global-stats",
    () => api.getGlobalStats(),
    { refreshInterval: 10000 }
  );

  async function handleResetHealth(channelName: string) {
    const confirmed = await dialog.confirm(
      `确定要将渠道 "${channelName}" 的健康度重置为 100% 吗？`,
      "确认重置"
    );
    if (!confirmed) {
      return;
    }
    try {
      await api.resetChannelHealth(channelName);
      await mutate();
      await dialog.alert("健康度已重置为 100%", "成功");
    } catch (err: any) {
      await dialog.alert("重置失败：" + err.message, "错误");
    }
  }

  if (isLoading) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>统计</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>统计</PageTitle>
        <div className="mt-6 p-4 bg-red-500/10 border border-red-500/20 text-red-400 rounded-lg">
          加载失败：{(error as Error).message}
        </div>
      </div>
    );
  }

  const { type_stats, channel_details } = data || { type_stats: null, channel_details: [] };
  const channels = channel_details || [];

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>统计</PageTitle>
          <p className="text-gray-400 text-sm mt-2">实时监控所有渠道的运行状态和性能指标</p>
        </div>
        <button
          onClick={() => mutate()}
          className="group px-5 py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 text-white rounded-lg hover:from-blue-500 hover:to-blue-600 transition-all duration-300 shadow-lg shadow-blue-500/20 hover:shadow-xl hover:shadow-blue-500/30 flex items-center gap-2 font-medium"
        >
          <svg className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          刷新
        </button>
      </div>

      {type_stats && (
        <>
          {/* 大屏数据 - 汇总统计 */}
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
            {/* Gemini 汇总 */}
            <div className="relative overflow-hidden rounded-lg bg-gradient-to-br from-blue-500/10 to-purple-500/10 backdrop-blur-sm border border-blue-500/20 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gradient-to-br from-blue-500 to-purple-500 rounded-lg flex items-center justify-center">
                    <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                    </svg>
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-white">Gemini</h3>
                    <p className="text-xs text-gray-400">全部请求</p>
                  </div>
                </div>
                <div className="text-right">
                  <p className="text-2xl font-bold text-blue-400">{type_stats.gemini.total_requests.toLocaleString()}</p>
                  <p className="text-xs text-gray-400">
                    成功率 {type_stats.gemini.total_requests > 0 
                      ? ((type_stats.gemini.total_success / type_stats.gemini.total_requests) * 100).toFixed(1)
                      : "0.0"}%
                  </p>
                </div>
              </div>
              <div className="flex items-center justify-between mt-3 pt-3 border-t border-gray-700/50">
                <span className="text-xs text-gray-400">失败 {type_stats.gemini.total_fail.toLocaleString()}</span>
                <span className="text-xs text-cyan-400">延迟 {Math.round(type_stats.gemini.avg_latency_ms)}ms</span>
              </div>
            </div>

            {/* OpenAI 汇总 */}
            <div className="relative overflow-hidden rounded-lg bg-gradient-to-br from-green-500/10 to-emerald-500/10 backdrop-blur-sm border border-green-500/20 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gradient-to-br from-green-500 to-emerald-500 rounded-lg flex items-center justify-center">
                    <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
                    </svg>
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-white">OpenAI</h3>
                    <p className="text-xs text-gray-400">全部请求</p>
                  </div>
                </div>
                <div className="text-right">
                  <p className="text-2xl font-bold text-green-400">{type_stats.openai.total_requests.toLocaleString()}</p>
                  <p className="text-xs text-gray-400">
                    成功率 {type_stats.openai.total_requests > 0 
                      ? ((type_stats.openai.total_success / type_stats.openai.total_requests) * 100).toFixed(1)
                      : "0.0"}%
                  </p>
                </div>
              </div>
              <div className="flex items-center justify-between mt-3 pt-3 border-t border-gray-700/50">
                <span className="text-xs text-gray-400">失败 {type_stats.openai.total_fail.toLocaleString()}</span>
                <span className="text-xs text-cyan-400">延迟 {Math.round(type_stats.openai.avg_latency_ms)}ms</span>
              </div>
            </div>

            {/* 今日统计 */}
            <div className="relative overflow-hidden rounded-lg bg-gradient-to-br from-orange-500/10 to-yellow-500/10 backdrop-blur-sm border border-orange-500/20 p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 bg-gradient-to-br from-orange-500 to-yellow-500 rounded-lg flex items-center justify-center">
                    <svg className="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                    </svg>
                  </div>
                  <div>
                    <h3 className="text-base font-semibold text-white">今日</h3>
                    <p className="text-xs text-gray-400">当天请求</p>
                  </div>
                </div>
                <div className="text-right">
                  <p className="text-2xl font-bold text-orange-400">{type_stats.today.total_requests.toLocaleString()}</p>
                  <p className="text-xs text-gray-400">
                    成功率 {type_stats.today.total_requests > 0 
                      ? ((type_stats.today.total_success / type_stats.today.total_requests) * 100).toFixed(1)
                      : "0.0"}%
                  </p>
                </div>
              </div>
              <div className="flex items-center justify-between mt-3 pt-3 border-t border-gray-700/50">
                <span className="text-xs text-gray-400">失败 {type_stats.today.total_fail.toLocaleString()}</span>
                <span className="text-xs text-cyan-400">延迟 {Math.round(type_stats.today.avg_latency_ms)}ms</span>
              </div>
            </div>
          </div>

          {/* 渠道详细统计 */}
          <div className="mb-6">
            {channels.length === 0 ? (
              <div className="text-center py-16">
                <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
                  <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" />
                  </svg>
                </div>
                <h3 className="text-lg font-medium text-gray-300 mb-2">暂无统计数据</h3>
                <p className="text-gray-500 text-sm">开始使用 API 后将显示统计信息</p>
              </div>
            ) : (
              <div className="space-y-8">
                {Object.entries(
                  channels.reduce((bigGroups, channel) => {
                    // 确定大类 (Gemini 或 OpenAI)
                    const bigCategory = channel.channel_type.includes('gemini') ? 'Gemini' : 'OpenAI';
                    if (!bigGroups[bigCategory]) {
                      bigGroups[bigCategory] = [];
                    }
                    bigGroups[bigCategory].push(channel);
                    return bigGroups;
                  }, {} as Record<string, Array<typeof channels[number]>>)
                ).map(([bigCategory, channelsInGroup]) => (
                  <div key={bigCategory} className="space-y-4">
                    {/* 大类标题 */}
                    <div className="flex items-center gap-3 pb-3 border-b border-gray-700/50">
                      <div className={`w-8 h-8 rounded-lg flex items-center justify-center ${
                        bigCategory === 'Gemini' 
                          ? 'bg-gradient-to-br from-blue-500/20 to-cyan-500/20 border border-blue-500/30' 
                          : 'bg-gradient-to-br from-green-500/20 to-emerald-500/20 border border-green-500/30'
                      }`}>
                        <svg className={`w-5 h-5 ${bigCategory === 'Gemini' ? 'text-blue-400' : 'text-green-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
                        </svg>
                      </div>
                      <h2 className="text-lg font-bold text-white">{bigCategory}</h2>
                      <span className="px-2 py-0.5 bg-gray-700/50 text-gray-300 text-xs rounded-full font-medium">
                        {channelsInGroup.length} 个渠道
                      </span>
                    </div>

                    {/* 渠道列表 */}
                    <div className="bg-gray-800/50 rounded-lg border border-gray-700/50 overflow-hidden">
                      {channelsInGroup.map((channel) => {
                        const healthScore = channel.health_score || 0;
                        const healthStatus = 
                          healthScore >= 0.8 ? { color: 'green', label: '健康' } :
                          healthScore >= 0.5 ? { color: 'yellow', label: '警告' } : 
                          { color: 'red', label: '异常' };
                        const displayName = channel.display_name || channel.channel_name;
                        
                        return (
                          <div
                            key={channel.channel_name}
                            className="group hover:bg-gray-800/80 transition-all duration-200 border-b border-gray-700/50 last:border-b-0"
                          >
                            <div className="flex items-center gap-4 p-4">
                              {/* 健康状态指示 */}
                              <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                                healthStatus.color === 'green' ? 'bg-green-400 animate-pulse shadow-lg shadow-green-400/50' :
                                healthStatus.color === 'yellow' ? 'bg-yellow-400' :
                                'bg-red-400'
                              }`}></div>
                              
                              {/* 渠道名称 */}
                              <div className="flex-shrink-0 w-48">
                                <h3 className="text-sm font-semibold text-white truncate">{displayName}</h3>
                                <div className="flex items-center gap-2 mt-0.5">
                                  <span className="text-xs text-gray-500">{channel.channel_type}</span>
                                  <span className={`text-xs ${
                                    healthStatus.color === 'green' ? 'text-green-400' :
                                    healthStatus.color === 'yellow' ? 'text-yellow-400' :
                                    'text-red-400'
                                  }`}>
                                    {healthStatus.label}
                                  </span>
                                </div>
                              </div>
                              
                              {/* 请求统计 */}
                              <div className="flex-shrink-0 w-28 text-center">
                                <p className="text-xs text-gray-500 mb-0.5">总请求</p>
                                <p className="text-base font-semibold text-gray-300">{channel.total_requests.toLocaleString()}</p>
                              </div>
                              
                              <div className="flex-shrink-0 w-24 text-center">
                                <p className="text-xs text-gray-500 mb-0.5">成功</p>
                                <p className="text-base font-semibold text-green-400">{channel.total_success.toLocaleString()}</p>
                              </div>
                              
                              <div className="flex-shrink-0 w-24 text-center">
                                <p className="text-xs text-gray-500 mb-0.5">失败</p>
                                <p className="text-base font-semibold text-red-400">{channel.total_fail.toLocaleString()}</p>
                              </div>
                              
                              {/* 健康度进度条 */}
                              <div className="flex-1 min-w-[120px]">
                                <div className="flex items-center justify-between mb-1">
                                  <span className="text-xs text-gray-500">健康度</span>
                                  <span className={`text-xs font-semibold ${
                                    healthStatus.color === 'green' ? 'text-green-400' :
                                    healthStatus.color === 'yellow' ? 'text-yellow-400' :
                                    'text-red-400'
                                  }`}>
                                    {(healthScore * 100).toFixed(0)}%
                                  </span>
                                </div>
                                <div className="w-full h-1.5 bg-gray-700/50 rounded-full overflow-hidden">
                                  <div 
                                    className={`h-full rounded-full transition-all ${
                                      healthStatus.color === 'green' ? 'bg-gradient-to-r from-green-400 to-emerald-500' :
                                      healthStatus.color === 'yellow' ? 'bg-gradient-to-r from-yellow-400 to-orange-500' :
                                      'bg-gradient-to-r from-red-400 to-red-500'
                                    }`}
                                    style={{ width: `${healthScore * 100}%` }}
                                  ></div>
                                </div>
                              </div>
                              
                              {/* 成功率进度条 */}
                              <div className="flex-1 min-w-[120px]">
                                <div className="flex items-center justify-between mb-1">
                                  <span className="text-xs text-gray-500">成功率</span>
                                  <span className="text-xs font-semibold text-gray-300">
                                    {channel.success_rate.toFixed(1)}%
                                  </span>
                                </div>
                                <div className="w-full h-1.5 bg-gray-700/50 rounded-full overflow-hidden">
                                  <div 
                                    className="h-full rounded-full transition-all bg-blue-500"
                                    style={{ width: `${channel.success_rate}%` }}
                                  ></div>
                                </div>
                              </div>
                              
                              {/* 平均延迟 */}
                              <div className="flex-shrink-0 w-24 text-center">
                                <p className="text-xs text-gray-500 mb-0.5">延迟</p>
                                <p className="text-sm text-cyan-400 font-medium">{Math.round(channel.avg_latency_ms)}ms</p>
                              </div>
                              
                              {/* 操作按钮 */}
                              <div className="flex-shrink-0">
                                <button
                                  onClick={() => handleResetHealth(channel.channel_name)}
                                  className="p-2 text-purple-400 hover:bg-purple-500/10 rounded-lg transition-all duration-200"
                                  title="还原健康度"
                                >
                                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
                                  </svg>
                                </button>
                              </div>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
