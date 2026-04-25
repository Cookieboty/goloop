"use client";

import React, { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { UsageLog, APIKey } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";

type TimeRange = "today" | "1h" | "24h" | "7d" | "custom";

export default function ErrorLogsPage() {
  const dialog = useDialog();
  const [logs, setLogs] = useState<UsageLog[]>([]);
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [totalCount, setTotalCount] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [timeRange, setTimeRange] = useState<TimeRange>("today");
  const [customStartDate, setCustomStartDate] = useState("");
  const [customEndDate, setCustomEndDate] = useState("");
  const [selectedApiKeys, setSelectedApiKeys] = useState<number[]>([]);
  const [isFilterDrawerOpen, setIsFilterDrawerOpen] = useState(false);
  const [selectedLog, setSelectedLog] = useState<UsageLog | null>(null);
  const [isLogDrawerOpen, setIsLogDrawerOpen] = useState(false);

  const pageSize = 50;

  useEffect(() => {
    loadData();
  }, [currentPage, timeRange, customStartDate, customEndDate, selectedApiKeys]);

  async function loadData() {
    try {
      setLoading(true);
      const { startDate, endDate } = getTimeRangeParams();
      
      const [errorLogsData, apiKeysData] = await Promise.all([
        api.getErrorLogs({
          limit: pageSize,
          offset: (currentPage - 1) * pageSize,
          start_date: startDate,
          end_date: endDate,
        }),
        api.getAPIKeys()
      ]);
      
      // 如果选择了特定的 API Keys，则过滤结果
      let filteredLogs = errorLogsData.logs;
      let filteredTotal = errorLogsData.total;
      
      if (selectedApiKeys.length > 0) {
        filteredLogs = errorLogsData.logs.filter(log => 
          selectedApiKeys.includes(log.api_key_id)
        );
        // 注意：这是前端过滤，总数可能不准确。理想情况下应该在后端过滤
        filteredTotal = filteredLogs.length;
      }
      
      setLogs(filteredLogs);
      setTotalCount(filteredTotal);
      setAPIKeys(apiKeysData);
    } catch (err: any) {
      await dialog.alert("加载失败：" + err.message, "错误");
    } finally {
      setLoading(false);
    }
  }

  function getTimeRangeParams() {
    const now = new Date();
    let startDate: string | undefined;
    let endDate: string | undefined = now.toISOString();

    if (timeRange === "custom") {
      startDate = customStartDate ? new Date(customStartDate).toISOString() : undefined;
      endDate = customEndDate ? new Date(customEndDate).toISOString() : undefined;
    } else if (timeRange === "today") {
      // 今天从0点开始
      const todayStart = new Date();
      todayStart.setHours(0, 0, 0, 0);
      startDate = todayStart.toISOString();
    } else {
      const hoursMap: Record<Exclude<TimeRange, "custom" | "today">, number> = {
        "1h": 1,
        "24h": 24,
        "7d": 24 * 7,
      };
      const hours = hoursMap[timeRange];
      const start = new Date(now.getTime() - hours * 60 * 60 * 1000);
      startDate = start.toISOString();
    }

    return { startDate, endDate };
  }

  function getAPIKeyName(apiKeyId: number): string {
    const apiKey = apiKeys.find(k => k.id === apiKeyId);
    return apiKey?.name || `API Key #${apiKeyId}`;
  }

  function openLogDrawer(log: UsageLog) {
    setSelectedLog(log);
    setIsLogDrawerOpen(true);
  }

  function closeLogDrawer() {
    setIsLogDrawerOpen(false);
    setSelectedLog(null);
  }

  function toggleApiKey(apiKeyId: number) {
    setSelectedApiKeys(prev => {
      if (prev.includes(apiKeyId)) {
        return prev.filter(id => id !== apiKeyId);
      } else {
        return [...prev, apiKeyId];
      }
    });
  }

  function clearApiKeyFilter() {
    setSelectedApiKeys([]);
    setCurrentPage(1);
  }

  function getStatusCodeColor(statusCode?: number) {
    if (!statusCode) return "text-gray-400";
    if (statusCode >= 500) return "text-red-400";
    if (statusCode >= 400) return "text-yellow-400";
    return "text-gray-400";
  }

  async function copyErrorMessage(message: string) {
    try {
      await navigator.clipboard.writeText(message);
      await dialog.alert("已复制到剪贴板", "成功");
    } catch (err) {
      console.error("Failed to copy:", err);
    }
  }

  const totalPages = Math.ceil(totalCount / pageSize);

  if (loading && logs.length === 0) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>错误日志</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-red-500"></div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>错误日志</PageTitle>
          <p className="text-gray-400 text-sm mt-2">
            查看所有失败的 API 请求记录 · 共 {totalCount} 条错误
          </p>
        </div>
      </div>

      {/* Filters Bar */}
      <div className="mb-6 flex items-center gap-3">
        <button
          onClick={() => setIsFilterDrawerOpen(true)}
          className="flex items-center gap-2 px-4 py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 hover:from-blue-500 hover:to-blue-600 text-white rounded-lg transition-all shadow-lg shadow-blue-500/20"
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M3 4a1 1 0 011-1h16a1 1 0 011 1v2.586a1 1 0 01-.293.707l-6.414 6.414a1 1 0 00-.293.707V17l-4 4v-6.586a1 1 0 00-.293-.707L3.293 7.293A1 1 0 013 6.586V4z" />
          </svg>
          筛选 API Key
          {selectedApiKeys.length > 0 && (
            <span className="px-2 py-0.5 bg-white/20 rounded-full text-xs font-semibold">
              {selectedApiKeys.length}
            </span>
          )}
        </button>
        
        {selectedApiKeys.length > 0 && (
          <button
            onClick={clearApiKeyFilter}
            className="px-3 py-2 text-sm text-gray-400 hover:text-white hover:bg-gray-800 rounded-lg transition-all"
          >
            清除筛选
          </button>
        )}
      </div>

      {/* Time Range Selector */}
      <div className="mb-6 p-5 rounded-xl bg-gradient-to-br from-gray-800/90 to-gray-900/90 backdrop-blur-sm border border-gray-700/50">
        <div className="flex items-center gap-2 mb-4">
          <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          <h2 className="text-lg font-semibold text-white">时间范围</h2>
        </div>
        
        <div className="flex flex-wrap gap-3 mb-4">
          <button
            onClick={() => {
              setTimeRange("today");
              setCurrentPage(1);
            }}
            className={`px-4 py-2 rounded-lg transition-all ${
              timeRange === "today"
                ? "bg-blue-500 text-white"
                : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
            }`}
          >
            今天
          </button>
          <button
            onClick={() => {
              setTimeRange("1h");
              setCurrentPage(1);
            }}
            className={`px-4 py-2 rounded-lg transition-all ${
              timeRange === "1h"
                ? "bg-blue-500 text-white"
                : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
            }`}
          >
            最近 1 小时
          </button>
          <button
            onClick={() => {
              setTimeRange("24h");
              setCurrentPage(1);
            }}
            className={`px-4 py-2 rounded-lg transition-all ${
              timeRange === "24h"
                ? "bg-blue-500 text-white"
                : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
            }`}
          >
            最近 24 小时
          </button>
          <button
            onClick={() => {
              setTimeRange("7d");
              setCurrentPage(1);
            }}
            className={`px-4 py-2 rounded-lg transition-all ${
              timeRange === "7d"
                ? "bg-blue-500 text-white"
                : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
            }`}
          >
            最近 7 天
          </button>
          <button
            onClick={() => setTimeRange("custom")}
            className={`px-4 py-2 rounded-lg transition-all ${
              timeRange === "custom"
                ? "bg-blue-500 text-white"
                : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
            }`}
          >
            自定义
          </button>
        </div>

        {timeRange === "custom" && (
          <div className="flex gap-4 items-end">
            <div className="flex-1">
              <label className="block text-sm text-gray-400 mb-2">开始时间</label>
              <input
                type="datetime-local"
                value={customStartDate}
                onChange={(e) => setCustomStartDate(e.target.value)}
                className="w-full px-4 py-2 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
              />
            </div>
            <div className="flex-1">
              <label className="block text-sm text-gray-400 mb-2">结束时间</label>
              <input
                type="datetime-local"
                value={customEndDate}
                onChange={(e) => setCustomEndDate(e.target.value)}
                className="w-full px-4 py-2 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
              />
            </div>
            <button
              onClick={() => {
                setCurrentPage(1);
                loadData();
              }}
              className="px-6 py-2 bg-blue-500 hover:bg-blue-600 text-white rounded-lg transition-all"
            >
              查询
            </button>
          </div>
        )}
      </div>

      {/* Error Logs Table */}
      {logs.length === 0 ? (
        <div className="text-center py-16 bg-gray-800/30 rounded-xl border-2 border-dashed border-gray-700/50">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
            <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-300 mb-2">暂无错误日志</h3>
          <p className="text-gray-500 text-sm">在选定的时间范围内没有错误记录</p>
        </div>
      ) : (
        <div className="space-y-4">
          <div className="bg-gray-800/50 rounded-lg border border-gray-700/50 overflow-hidden">
            <div className="overflow-x-auto max-h-[calc(100vh-400px)]">
              <table className="w-full">
                <thead className="sticky top-0 z-10">
                  <tr className="bg-gray-800/95 backdrop-blur-sm">
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider w-16">状态</th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">时间</th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">API Key</th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">渠道</th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">模型</th>
                    <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-24">状态码</th>
                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">错误信息</th>
                    <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-20">操作</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-700/30 bg-gray-800/50">
                  {logs.map((log) => (
                    <tr 
                      key={log.id} 
                      className="hover:bg-gray-800/40 transition-colors cursor-pointer"
                      onClick={() => openLogDrawer(log)}
                    >
                      <td className="px-4 py-3">
                        <div className="w-2 h-2 rounded-full bg-red-400"></div>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm text-gray-300">
                          {new Date(log.created_at).toLocaleString("zh-CN")}
                        </p>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm text-gray-300">{getAPIKeyName(log.api_key_id)}</p>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm text-gray-300">{log.channel_name}</p>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm font-mono text-gray-300">{log.model}</p>
                      </td>
                      <td className="px-4 py-3 text-center">
                        <span className={`text-sm font-semibold ${getStatusCodeColor(log.status_code)}`}>
                          {log.status_code || "-"}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <p className="text-sm text-gray-300 truncate max-w-md">
                          {log.error_message || "未知错误"}
                        </p>
                      </td>
                      <td className="px-4 py-3 text-center">
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            openLogDrawer(log);
                          }}
                          className="p-1.5 text-blue-400 hover:bg-blue-500/10 rounded-lg transition-all duration-200"
                          title="查看详情"
                        >
                          <svg 
                            className="w-4 h-4"
                            fill="none" 
                            stroke="currentColor" 
                            viewBox="0 0 24 24"
                          >
                            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                          </svg>
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex items-center justify-between px-4 py-3 bg-gray-800/50 rounded-lg border border-gray-700/50">
              <div className="text-sm text-gray-400">
                显示 {(currentPage - 1) * pageSize + 1} - {Math.min(currentPage * pageSize, totalCount)} 条，共 {totalCount} 条
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => setCurrentPage(p => Math.max(1, p - 1))}
                  disabled={currentPage === 1}
                  className="px-3 py-1 bg-gray-700/50 text-gray-300 rounded hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                >
                  上一页
                </button>
                <span className="text-sm text-gray-300">
                  第 {currentPage} / {totalPages} 页
                </span>
                <button
                  onClick={() => setCurrentPage(p => Math.min(totalPages, p + 1))}
                  disabled={currentPage === totalPages}
                  className="px-3 py-1 bg-gray-700/50 text-gray-300 rounded hover:bg-gray-700 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                >
                  下一页
                </button>
              </div>
            </div>
          )}
        </div>
      )}

      {/* API Key Filter Drawer */}
      {isFilterDrawerOpen && (
        <div 
          className="fixed inset-0 bg-black/70 backdrop-blur-sm z-50 animate-in fade-in duration-200"
          onClick={() => setIsFilterDrawerOpen(false)}
        >
          <div 
            className="fixed right-0 top-0 h-full w-full max-w-md bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md shadow-2xl border-l border-gray-700/50 animate-in slide-in-from-right duration-300"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="p-6 border-b border-gray-700/50">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-blue-500/20 border border-blue-500/30 flex items-center justify-center">
                    <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                    </svg>
                  </div>
                  <div>
                    <h2 className="text-xl font-semibold text-white">筛选 API Key</h2>
                    <p className="text-sm text-gray-400 mt-0.5">
                      选择要查看的 API Key
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => setIsFilterDrawerOpen(false)}
                  className="p-2 hover:bg-gray-700/50 rounded-lg transition-all"
                >
                  <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>

            {/* Content */}
            <div className="p-6 overflow-y-auto h-[calc(100%-180px)]">
              <div className="space-y-2">
                {apiKeys.length === 0 ? (
                  <div className="text-center py-8">
                    <p className="text-gray-400">暂无 API Key</p>
                  </div>
                ) : (
                  apiKeys.map((apiKey) => (
                    <label
                      key={apiKey.id}
                      className="flex items-center gap-3 p-4 rounded-lg bg-gray-800/50 hover:bg-gray-800 border border-gray-700/50 cursor-pointer transition-all group"
                    >
                      <input
                        type="checkbox"
                        checked={selectedApiKeys.includes(apiKey.id)}
                        onChange={() => toggleApiKey(apiKey.id)}
                        className="w-5 h-5 rounded border-gray-600 text-blue-500 focus:ring-2 focus:ring-blue-500 focus:ring-offset-0 bg-gray-700"
                      />
                      <div className="flex-1 min-w-0">
                        <p className="text-white font-medium group-hover:text-blue-400 transition-colors">
                          {apiKey.name}
                        </p>
                        <p className="text-sm text-gray-400 font-mono truncate mt-1">
                          {apiKey.key}
                        </p>
                      </div>
                      {selectedApiKeys.includes(apiKey.id) && (
                        <svg className="w-5 h-5 text-blue-400 flex-shrink-0" fill="currentColor" viewBox="0 0 20 20">
                          <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.707-9.293a1 1 0 00-1.414-1.414L9 10.586 7.707 9.293a1 1 0 00-1.414 1.414l2 2a1 1 0 001.414 0l4-4z" clipRule="evenodd" />
                        </svg>
                      )}
                    </label>
                  ))
                )}
              </div>
            </div>

            {/* Footer */}
            <div className="absolute bottom-0 left-0 right-0 p-6 border-t border-gray-700/50 bg-gray-900/95 backdrop-blur-sm">
              <div className="flex gap-3">
                <button
                  onClick={clearApiKeyFilter}
                  className="flex-1 px-4 py-2.5 bg-gray-700/50 hover:bg-gray-700 text-gray-300 rounded-lg transition-all font-medium"
                >
                  清除选择
                </button>
                <button
                  onClick={() => {
                    setIsFilterDrawerOpen(false);
                    setCurrentPage(1);
                  }}
                  className="flex-1 px-4 py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 hover:from-blue-500 hover:to-blue-600 text-white rounded-lg transition-all font-medium shadow-lg shadow-blue-500/20"
                >
                  应用筛选
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Log Detail Drawer */}
      {isLogDrawerOpen && selectedLog && (
        <div 
          className="fixed inset-0 bg-black/70 backdrop-blur-sm z-50 animate-in fade-in duration-200"
          onClick={closeLogDrawer}
        >
          <div 
            className="fixed right-0 top-0 h-full w-full max-w-2xl bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md shadow-2xl border-l border-gray-700/50 animate-in slide-in-from-right duration-300 flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="p-6 border-b border-gray-700/50 flex-shrink-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-lg bg-red-500/20 border border-red-500/30 flex items-center justify-center">
                    <svg className="w-5 h-5 text-red-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                    </svg>
                  </div>
                  <div>
                    <h2 className="text-xl font-semibold text-white">错误详情</h2>
                    <p className="text-sm text-gray-400 mt-0.5">
                      {new Date(selectedLog.created_at).toLocaleString("zh-CN")}
                    </p>
                  </div>
                </div>
                <button
                  onClick={closeLogDrawer}
                  className="p-2 hover:bg-gray-700/50 rounded-lg transition-all"
                >
                  <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-y-auto p-6">
              <div className="space-y-6">
                {/* Basic Info */}
                <div className="grid grid-cols-2 gap-4">
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">API Key</p>
                    <p className="text-sm text-white font-medium">{getAPIKeyName(selectedLog.api_key_id)}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">渠道</p>
                    <p className="text-sm text-white font-medium">{selectedLog.channel_name}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">模型</p>
                    <p className="text-sm text-white font-mono">{selectedLog.model}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">状态码</p>
                    <p className={`text-sm font-semibold ${getStatusCodeColor(selectedLog.status_code)}`}>
                      {selectedLog.status_code || "-"}
                    </p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">请求 IP</p>
                    <p className="text-sm text-white font-mono">{selectedLog.request_ip || "-"}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">延迟时间</p>
                    <p className="text-sm text-white">{selectedLog.latency_ms ? `${selectedLog.latency_ms}ms` : "-"}</p>
                  </div>
                </div>

                {/* Error Message */}
                {selectedLog.error_message && (
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <h3 className="text-sm font-semibold text-white">错误信息</h3>
                      <button
                        onClick={() => copyErrorMessage(selectedLog.error_message!)}
                        className="px-3 py-1.5 text-xs text-blue-400 hover:text-blue-300 hover:bg-blue-500/10 rounded-lg transition-all flex items-center gap-1.5"
                      >
                        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                        </svg>
                        复制
                      </button>
                    </div>
                    <div className="p-4 bg-gray-950/80 rounded-lg border border-red-500/20">
                      <p className="text-sm text-red-300 font-mono whitespace-pre-wrap break-all leading-relaxed">
                        {selectedLog.error_message}
                      </p>
                    </div>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
