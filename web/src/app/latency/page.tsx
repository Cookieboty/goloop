"use client";

import React, { useState, useEffect, useRef, useCallback } from "react";
import { api } from "@/lib/api";
import type { UsageLog, LatencyStats } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { StatCard } from "@/components/StatCard";

type TimeRange = "today" | "1h" | "24h" | "7d" | "custom";

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

export default function LatencyPage() {
  const [logs, setLogs] = useState<UsageLog[]>([]);
  const [stats, setStats] = useState<LatencyStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [totalCount, setTotalCount] = useState(0);
  const [currentPage, setCurrentPage] = useState(1);
  const [timeRange, setTimeRange] = useState<TimeRange>("today");
  const [customStartDate, setCustomStartDate] = useState("");
  const [customEndDate, setCustomEndDate] = useState("");
  const [modelFilter, setModelFilter] = useState("");
  const [channelFilter, setChannelFilter] = useState("");

  const debouncedModel = useDebounce(modelFilter, 500);
  const debouncedChannel = useDebounce(channelFilter, 500);

  const pageSize = 50;

  useEffect(() => {
    loadData();
  }, [currentPage, timeRange, customStartDate, customEndDate, debouncedModel, debouncedChannel]);

  async function loadData() {
    try {
      setLoading(true);
      const { startDate, endDate } = getTimeRangeParams();

      const data = await api.getLatencyLogs({
        limit: pageSize,
        offset: (currentPage - 1) * pageSize,
        model: modelFilter || undefined,
        channel: channelFilter || undefined,
        start_date: startDate,
        end_date: endDate,
      });

      setLogs(data.logs || []);
      setTotalCount(data.total);
      setStats(data.stats);
    } catch (err: any) {
      console.error("Failed to load latency data:", err);
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

  function formatMs(ms?: number): string {
    if (ms == null) return "-";
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(1)}s`;
  }

  function formatBytes(bytes?: number): string {
    if (bytes == null) return "-";
    if (bytes < 1024) return `${bytes}B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
  }

  function formatTime(iso: string): string {
    return new Date(iso).toLocaleString("zh-CN", {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });
  }

  const totalPages = Math.ceil(totalCount / pageSize);

  if (loading && logs.length === 0) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>延迟诊断</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <PageTitle>延迟诊断</PageTitle>

      {/* Stat Cards */}
      {stats && (
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
          <StatCard
            label="总请求 / 成功率"
            value={`${stats.total_requests}`}
            sub={`${stats.success_rate.toFixed(1)}% 成功`}
          />
          <StatCard
            label="平均 TTFB"
            value={formatMs(stats.avg_ttfb_ms)}
            sub="上传 + 上游处理"
          />
          <StatCard
            label="平均 Body 下载"
            value={formatMs(stats.avg_body_read_ms)}
            sub={`平均响应 ${formatBytes(stats.avg_response_bytes)}`}
          />
          <StatCard
            label="P95 总延迟"
            value={formatMs(stats.p95_latency_ms)}
            sub={`平均 ${formatMs(stats.avg_latency_ms)}`}
            valueColor={stats.p95_latency_ms > 60000 ? "var(--red)" : undefined}
          />
        </div>
      )}

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3 mb-6 bg-gray-900 border border-gray-800 rounded-lg p-4">
        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-400">时间</label>
          <select
            value={timeRange}
            onChange={(e) => { setTimeRange(e.target.value as TimeRange); setCurrentPage(1); }}
            className="bg-gray-800 text-white text-sm rounded px-2 py-1 border border-gray-700"
          >
            <option value="today">今天</option>
            <option value="1h">最近 1 小时</option>
            <option value="24h">最近 24 小时</option>
            <option value="7d">最近 7 天</option>
            <option value="custom">自定义</option>
          </select>
        </div>

        {timeRange === "custom" && (
          <>
            <input
              type="datetime-local"
              value={customStartDate}
              onChange={(e) => { setCustomStartDate(e.target.value); setCurrentPage(1); }}
              className="bg-gray-800 text-white text-sm rounded px-2 py-1 border border-gray-700"
            />
            <span className="text-gray-500">-</span>
            <input
              type="datetime-local"
              value={customEndDate}
              onChange={(e) => { setCustomEndDate(e.target.value); setCurrentPage(1); }}
              className="bg-gray-800 text-white text-sm rounded px-2 py-1 border border-gray-700"
            />
          </>
        )}

        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-400">模型</label>
          <input
            type="text"
            placeholder="如 gemini-3-pro-image-preview"
            value={modelFilter}
            onChange={(e) => { setModelFilter(e.target.value); setCurrentPage(1); }}
            className="bg-gray-800 text-white text-sm rounded px-2 py-1 border border-gray-700 w-56"
          />
        </div>

        <div className="flex items-center gap-2">
          <label className="text-xs text-gray-400">渠道</label>
          <input
            type="text"
            placeholder="渠道名"
            value={channelFilter}
            onChange={(e) => { setChannelFilter(e.target.value); setCurrentPage(1); }}
            className="bg-gray-800 text-white text-sm rounded px-2 py-1 border border-gray-700 w-36"
          />
        </div>
      </div>

      {/* Table */}
      <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-gray-800 text-left text-gray-400 text-xs uppercase">
                <th className="px-4 py-3">时间</th>
                <th className="px-4 py-3">模型</th>
                <th className="px-4 py-3">渠道</th>
                <th className="px-4 py-3">状态</th>
                <th className="px-4 py-3 text-center">计入</th>
                <th className="px-4 py-3 text-right">TTFB</th>
                <th className="px-4 py-3 text-right">Body 下载</th>
                <th className="px-4 py-3 text-right">响应大小</th>
                <th className="px-4 py-3 text-right">总延迟</th>
                <th className="px-4 py-3 text-right">差值</th>
              </tr>
            </thead>
            <tbody>
              {logs.map((log) => (
                <tr key={log.id} className="border-b border-gray-800/50 hover:bg-gray-800/30">
                  <td className="px-4 py-2.5 text-gray-300 whitespace-nowrap font-mono text-xs">
                    {formatTime(log.created_at)}
                  </td>
                  <td className="px-4 py-2.5 text-gray-200 whitespace-nowrap text-xs max-w-[200px] truncate" title={log.model}>
                    {log.model}
                  </td>
                  <td className="px-4 py-2.5 text-gray-400 whitespace-nowrap text-xs">
                    {log.channel_name}
                  </td>
                  <td className="px-4 py-2.5">
                    {log.success ? (
                      <span className="inline-block w-2 h-2 rounded-full bg-green-500"></span>
                    ) : (
                      <span className="inline-block w-2 h-2 rounded-full bg-red-500" title={log.error_message}></span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-center text-xs">
                    {log.should_count ? (
                      <span className="text-green-400">✓</span>
                    ) : (
                      <span className="text-gray-600">-</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-xs text-blue-300">
                    {formatMs(log.upstream_ttfb_ms)}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-xs text-purple-300">
                    {formatMs(log.body_read_ms)}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-xs text-gray-400">
                    {formatBytes(log.response_bytes)}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-xs text-yellow-300">
                    {formatMs(log.latency_ms)}
                  </td>
                  <td className="px-4 py-2.5 text-right font-mono text-xs">
                    {(() => {
                      const gap = (log.latency_ms ?? 0) - (log.upstream_ttfb_ms ?? 0) - (log.body_read_ms ?? 0);
                      if (!log.latency_ms || !log.upstream_ttfb_ms) return <span className="text-gray-600">-</span>;
                      return <span className={gap > 5000 ? "text-red-400" : "text-gray-500"}>{formatMs(gap)}</span>;
                    })()}
                  </td>
                </tr>
              ))}
              {logs.length === 0 && (
                <tr>
                  <td colSpan={10} className="px-4 py-8 text-center text-gray-500">
                    暂无数据
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-4">
          <div className="text-sm text-gray-500">
            共 {totalCount} 条，第 {currentPage}/{totalPages} 页
          </div>
          <div className="flex gap-2">
            <button
              onClick={() => setCurrentPage(Math.max(1, currentPage - 1))}
              disabled={currentPage <= 1}
              className="px-3 py-1 text-sm bg-gray-800 text-gray-300 rounded border border-gray-700 disabled:opacity-30 hover:bg-gray-700"
            >
              上一页
            </button>
            <button
              onClick={() => setCurrentPage(Math.min(totalPages, currentPage + 1))}
              disabled={currentPage >= totalPages}
              className="px-3 py-1 text-sm bg-gray-800 text-gray-300 rounded border border-gray-700 disabled:opacity-30 hover:bg-gray-700"
            >
              下一页
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
