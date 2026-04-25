"use client";

import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { Channel, CreateChannelRequest, CHANNEL_TYPES } from "@/lib/types";
import { CHANNEL_TYPES as channelTypes } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";

export default function ChannelsPage() {
  const dialog = useDialog();
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [editingChannel, setEditingChannel] = useState<Channel | null>(null);

  useEffect(() => {
    loadChannels();
  }, []);

  async function loadChannels() {
    try {
      setLoading(true);
      const data = await api.getChannels();
      setChannels(data);
      setError(null);
    } catch (err: any) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: number) {
    const confirmed = await dialog.confirm(
      "此操作将同时删除关联的账号和模型映射。",
      "确定要删除此渠道吗？",
      true
    );
    if (!confirmed) {
      return;
    }
    try {
      await api.deleteChannel(id);
      await loadChannels();
    } catch (err: any) {
      await dialog.alert("删除失败：" + err.message, "错误");
    }
  }

  async function handleToggle(id: number, enabled: boolean) {
    try {
      await api.toggleChannel(id, enabled);
      await loadChannels();
    } catch (err: any) {
      await dialog.alert("切换状态失败：" + err.message, "错误");
    }
  }

  async function handleReload() {
    try {
      await api.reloadConfig();
      await dialog.alert("配置已重载！", "成功");
      await loadChannels();
    } catch (err: any) {
      await dialog.alert("重载失败：" + err.message, "错误");
    }
  }

  if (loading) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>渠道管理</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>渠道管理</PageTitle>
          <p className="text-gray-400 text-sm mt-2">管理所有 AI 渠道配置和账号</p>
        </div>
        <div className="flex gap-3">
          <button
            onClick={handleReload}
            className="group px-5 py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 text-white rounded-lg hover:from-blue-500 hover:to-blue-600 transition-all duration-300 shadow-lg shadow-blue-500/20 hover:shadow-xl hover:shadow-blue-500/30 flex items-center gap-2 font-medium"
          >
            <svg className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
            重载配置
          </button>
          <button
            onClick={() => {
              setEditingChannel(null);
              setShowForm(true);
            }}
            className="group px-5 py-2.5 bg-gradient-to-r from-green-600 to-emerald-500 text-white rounded-lg hover:from-green-500 hover:to-emerald-600 transition-all duration-300 shadow-lg shadow-green-500/20 hover:shadow-xl hover:shadow-green-500/30 flex items-center gap-2 font-medium"
          >
            <svg className="w-4 h-4 group-hover:scale-110 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
            </svg>
            创建渠道
          </button>
        </div>
      </div>

      {error && (
        <div className="mb-6 p-4 bg-red-500/10 border border-red-500/20 text-red-400 rounded-lg flex items-center gap-3 backdrop-blur-sm">
          <svg className="w-5 h-5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8v4m0 4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
          {error}
        </div>
      )}

      {channels.length === 0 ? (
        <div className="text-center py-16">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
            <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-300 mb-2">暂无渠道</h3>
          <p className="text-gray-500 text-sm">点击"创建渠道"开始添加您的第一个 AI 渠道</p>
        </div>
      ) : (
        <div className="space-y-10">
          {Object.entries(
            channels
              .sort((a, b) => b.weight - a.weight) // 按权重排序
              .reduce((bigGroups, channel) => {
                // 确定大类 (Gemini 或 OpenAI)
                const bigCategory = channel.type.startsWith('gemini') ? 'Gemini' : 'OpenAI';
                if (!bigGroups[bigCategory]) {
                  bigGroups[bigCategory] = {};
                }
                
                // 在大类内按具体类型分组
                const typeInfo = channelTypes.find((t) => t.value === channel.type);
                const typeLabel = typeInfo?.label || channel.type;
                if (!bigGroups[bigCategory][typeLabel]) {
                  bigGroups[bigCategory][typeLabel] = [];
                }
                bigGroups[bigCategory][typeLabel].push(channel);
                return bigGroups;
              }, {} as Record<string, Record<string, Channel[]>>)
          ).map(([bigCategory, typeGroups]) => (
            <div key={bigCategory} className="space-y-6">
              {/* 大类标题 */}
              <div className="flex items-center gap-3 pb-3 border-b border-gray-700/50">
                <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${
                  bigCategory === 'Gemini' 
                    ? 'bg-gradient-to-br from-blue-500/20 to-cyan-500/20 border border-blue-500/30' 
                    : 'bg-gradient-to-br from-green-500/20 to-emerald-500/20 border border-green-500/30'
                }`}>
                  <svg className={`w-6 h-6 ${bigCategory === 'Gemini' ? 'text-blue-400' : 'text-green-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9.663 17h4.673M12 3v1m6.364 1.636l-.707.707M21 12h-1M4 12H3m3.343-5.657l-.707-.707m2.828 9.9a5 5 0 117.072 0l-.548.547A3.374 3.374 0 0014 18.469V19a2 2 0 11-4 0v-.531c0-.895-.356-1.754-.988-2.386l-.548-.547z" />
                  </svg>
                </div>
                <h1 className="text-2xl font-bold text-white">{bigCategory}</h1>
                <span className="px-3 py-1 bg-gray-700/50 text-gray-300 text-sm rounded-full font-medium">
                  {Object.values(typeGroups).reduce((sum, channels) => sum + channels.length, 0)} 个渠道
                </span>
              </div>

              {/* 类型分组 */}
              <div className="space-y-6 pl-4">
                {Object.entries(typeGroups).map(([typeLabel, channelsInType]) => (
                  <div key={typeLabel}>
                    {/* 类型标题 */}
                    <div className="flex items-center gap-3 mb-4">
                      <div className="flex items-center gap-2">
                        <div className={`w-1 h-5 rounded-full ${
                          bigCategory === 'Gemini' 
                            ? 'bg-gradient-to-b from-blue-400 to-cyan-500' 
                            : 'bg-gradient-to-b from-green-400 to-emerald-500'
                        }`}></div>
                        <h2 className="text-lg font-semibold text-white">{typeLabel}</h2>
                        <span className="px-2 py-0.5 bg-gray-700/50 text-gray-400 text-xs rounded-full">
                          {channelsInType.length}
                        </span>
                      </div>
                    </div>
                    
                    {/* 渠道列表 */}
                    <div className="bg-gray-800/50 rounded-lg border border-gray-700/50 overflow-hidden">
                      {channelsInType.map((channel) => (
                        <div
                          key={channel.id}
                          className="group hover:bg-gray-800/80 transition-all duration-200 border-b border-gray-700/50 last:border-b-0"
                        >
                          <div className="flex items-center gap-4 p-4">
                            {/* 状态指示 */}
                            <div className={`w-2 h-2 rounded-full flex-shrink-0 ${
                              channel.enabled 
                                ? 'bg-green-400 animate-pulse shadow-lg shadow-green-400/50' 
                                : 'bg-gray-500'
                            }`}></div>
                            
                            {/* 渠道名称 */}
                            <div className="flex-shrink-0 w-40">
                              <h3 className="text-sm font-semibold text-white truncate">{channel.name}</h3>
                              <span className={`text-xs ${channel.enabled ? 'text-green-400' : 'text-gray-500'}`}>
                                {channel.enabled ? '运行中' : '已停用'}
                              </span>
                            </div>
                            
                            {/* Base URL */}
                            <div className="flex-1 min-w-0">
                              <p className="text-xs text-gray-500 mb-0.5">Base URL</p>
                              <p className="text-sm text-gray-300 truncate font-mono">{channel.base_url}</p>
                            </div>
                            
                            {/* 权重 */}
                            <div className="flex-shrink-0 w-20 text-center">
                              <p className="text-xs text-gray-500 mb-0.5">权重</p>
                              <p className="text-sm font-semibold text-gray-300">{channel.weight}</p>
                            </div>
                            
                            {/* 超时 */}
                            <div className="flex-shrink-0 w-20 text-center">
                              <p className="text-xs text-gray-500 mb-0.5">超时</p>
                              <p className="text-sm text-gray-300">{channel.timeout_seconds}s</p>
                            </div>
                            
                            {/* 账号数 */}
                            <div className="flex-shrink-0 w-20 text-center">
                              <p className="text-xs text-gray-500 mb-0.5">账号</p>
                              <p className="text-sm text-gray-300">{channel.accounts?.length || 0}</p>
                            </div>
                            
                            {/* 映射数 */}
                            <div className="flex-shrink-0 w-20 text-center">
                              <p className="text-xs text-gray-500 mb-0.5">映射</p>
                              <div className="flex items-center justify-center gap-1">
                                <p className="text-sm text-gray-300">{channel.model_mappings?.length || 0}</p>
                                {channel.model_mappings && channel.model_mappings.length > 0 && (
                                  <svg className="w-3 h-3 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                                  </svg>
                                )}
                              </div>
                            </div>
                            
                            {/* 操作按钮 */}
                            <div className="flex items-center gap-2 flex-shrink-0">
                              <button
                                onClick={() => handleToggle(channel.id, !channel.enabled)}
                                className={`p-2 rounded-lg transition-all duration-200 ${
                                  channel.enabled
                                    ? 'text-yellow-400 hover:bg-yellow-500/10'
                                    : 'text-green-400 hover:bg-green-500/10'
                                }`}
                                title={channel.enabled ? '禁用' : '启用'}
                              >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  {channel.enabled ? (
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                                  ) : (
                                    <>
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                                    </>
                                  )}
                                </svg>
                              </button>
                              
                              <button
                                onClick={() => {
                                  setEditingChannel(channel);
                                  setShowForm(true);
                                }}
                                className="p-2 text-blue-400 hover:bg-blue-500/10 rounded-lg transition-all duration-200"
                                title="编辑"
                              >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                </svg>
                              </button>
                              
                              <button
                                onClick={() => handleDelete(channel.id)}
                                className="p-2 text-red-400 hover:bg-red-500/10 rounded-lg transition-all duration-200"
                                title="删除"
                              >
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                </svg>
                              </button>
                            </div>
                          </div>
                          
                          {/* 展开的详细信息 - 轮询配置和模型映射 */}
                          {((channel.type === "gemini_callback" || channel.type === "openai_callback") || 
                            (channel.model_mappings && channel.model_mappings.length > 0)) && (
                            <div className="px-4 pb-4 pl-14 space-y-2">
                              {/* 轮询配置 */}
                              {(channel.type === "gemini_callback" || channel.type === "openai_callback") && (
                                <div className="inline-flex items-center gap-2 px-3 py-1.5 bg-purple-500/10 rounded-lg border border-purple-500/20">
                                  <svg className="w-3.5 h-3.5 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                                  </svg>
                                  <span className="text-xs text-purple-400">
                                    轮询: {channel.initial_interval_seconds}s → {channel.max_interval_seconds}s
                                    {channel.max_wait_time_seconds && ` (${channel.max_wait_time_seconds}s)`}
                                  </span>
                                </div>
                              )}
                              
                              {/* 模型映射 */}
                              {channel.model_mappings && channel.model_mappings.length > 0 && (
                                <div className="flex items-start gap-2">
                                  <svg className="w-3.5 h-3.5 text-blue-400 mt-0.5 flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                                  </svg>
                                  <div className="flex flex-wrap items-center gap-2">
                                    {channel.model_mappings.map((mapping, idx) => (
                                      <div key={idx} className="inline-flex items-center gap-1.5 px-2 py-1 bg-blue-500/10 rounded border border-blue-500/20">
                                        <span className="text-xs text-gray-400 font-mono">{mapping.source_model}</span>
                                        <svg className="w-3 h-3 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                                        </svg>
                                        <span className="text-xs text-blue-400 font-mono">{mapping.target_model}</span>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              )}
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {showForm && (
        <ChannelForm
          channel={editingChannel}
          onClose={() => {
            setShowForm(false);
            setEditingChannel(null);
          }}
          onSave={async () => {
            setShowForm(false);
            setEditingChannel(null);
            await loadChannels();
          }}
        />
      )}
    </div>
  );
}

function ChannelForm({
  channel,
  onClose,
  onSave,
}: {
  channel: Channel | null;
  onClose: () => void;
  onSave: () => void;
}) {
  const dialog = useDialog();
  const [formData, setFormData] = useState<CreateChannelRequest>({
    name: channel?.name || "",
    type: channel?.type || "gemini_original",
    base_url: channel?.base_url || "",
    weight: channel?.weight || 100,
    timeout_seconds: channel?.timeout_seconds || 6000,
    initial_interval_seconds: channel?.initial_interval_seconds ?? 5,
    max_interval_seconds: channel?.max_interval_seconds ?? 10,
    max_wait_time_seconds: channel?.max_wait_time_seconds ?? 12000,
    retry_attempts: channel?.retry_attempts ?? 3,
    probe_model: channel?.probe_model,
    accounts: channel?.accounts?.map((a) => ({ api_key: a.api_key, weight: a.weight })) || [],
    model_mappings:
      channel?.model_mappings?.map((m) => ({
        source_model: m.source_model,
        target_model: m.target_model,
      })) || [],
  });
  const [saving, setSaving] = useState(false);

  const selectedType = channelTypes.find((t) => t.value === formData.type);
  const needsCallbackConfig = formData.type === "gemini_callback" || formData.type === "openai_callback";
  const needsProbeModel = formData.type === "gemini_openai" || formData.type === "openai_original";

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (channel) {
        await api.updateChannel(channel.id, formData);
      } else {
        await api.createChannel(formData);
      }
      onSave();
    } catch (err: any) {
      await dialog.alert("保存失败：" + err.message, "错误");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 overflow-y-auto p-4">
      <div className="bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md rounded-2xl border border-gray-700/50 shadow-2xl max-w-4xl w-full max-h-[90vh] overflow-y-auto">
        {/* 头部 */}
        <div className="sticky top-0 bg-gradient-to-r from-gray-800 to-gray-900 border-b border-gray-700/50 px-8 py-6 flex justify-between items-center backdrop-blur-sm">
          <div>
            <h2 className="text-2xl font-bold text-white mb-1">
              {channel ? "编辑渠道" : "创建渠道"}
            </h2>
            <p className="text-sm text-gray-400">
              {selectedType?.description || "配置 AI 渠道的基本信息和参数"}
            </p>
          </div>
          <button
            onClick={onClose}
            className="p-2 hover:bg-gray-700/50 rounded-lg transition-colors"
          >
            <svg className="w-6 h-6 text-gray-400 hover:text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-8 space-y-6">
          {/* 基本信息 */}
          <div className="space-y-4">
            <div className="flex items-center gap-2 mb-4">
              <div className="w-1 h-6 bg-gradient-to-b from-blue-400 to-purple-500 rounded-full"></div>
              <h3 className="text-lg font-semibold text-white">基本信息</h3>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  渠道名称 <span className="text-red-400">*</span>
                </label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                  className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 placeholder-gray-500 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
                  placeholder="例如: OpenAI 主渠道"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  渠道类型 <span className="text-red-400">*</span>
                </label>
                <select
                  required
                  value={formData.type}
                  onChange={(e) => setFormData({ ...formData, type: e.target.value })}
                  className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
                >
                  {channelTypes.map((t) => (
                    <option key={t.value} value={t.value}>
                      {t.label}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                Base URL <span className="text-red-400">*</span>
              </label>
              <input
                type="url"
                required
                value={formData.base_url}
                onChange={(e) => setFormData({ ...formData, base_url: e.target.value })}
                className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 placeholder-gray-500 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none font-mono text-sm"
                placeholder="https://api.example.com"
              />
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  权重
                </label>
                <input
                  type="number"
                  value={formData.weight}
                  onChange={(e) => setFormData({ ...formData, weight: parseInt(e.target.value) || 0 })}
                  className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
                  min="0"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  超时时间（秒）
                </label>
                <input
                  type="number"
                  value={formData.timeout_seconds}
                  onChange={(e) => setFormData({ ...formData, timeout_seconds: parseInt(e.target.value) || 6000 })}
                  className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
                  min="1"
                />
              </div>
            </div>
          </div>

          {/* 回调配置 */}
          {needsCallbackConfig && (
            <div className="space-y-4 p-5 bg-purple-500/5 border border-purple-500/20 rounded-lg">
              <div className="flex items-center gap-2 mb-2">
                <svg className="w-5 h-5 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 10V3L4 14h7v7l9-11h-7z" />
                </svg>
                <h3 className="text-lg font-semibold text-white">轮询配置</h3>
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">初始间隔（秒）</label>
                  <input
                    type="number"
                    value={formData.initial_interval_seconds || ""}
                    onChange={(e) => setFormData({ ...formData, initial_interval_seconds: e.target.value ? parseInt(e.target.value) : undefined })}
                    className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 transition-all outline-none"
                    min="1"
                    placeholder="默认 5 秒"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">最大间隔（秒）</label>
                  <input
                    type="number"
                    value={formData.max_interval_seconds || ""}
                    onChange={(e) => setFormData({ ...formData, max_interval_seconds: e.target.value ? parseInt(e.target.value) : undefined })}
                    className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 transition-all outline-none"
                    min="1"
                    placeholder="默认 10 秒"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">最长等待（秒）</label>
                  <input
                    type="number"
                    value={formData.max_wait_time_seconds || ""}
                    onChange={(e) => setFormData({ ...formData, max_wait_time_seconds: e.target.value ? parseInt(e.target.value) : undefined })}
                    className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 transition-all outline-none"
                    min="1"
                    placeholder="默认 12000 秒"
                  />
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-300 mb-2">重试次数</label>
                  <input
                    type="number"
                    value={formData.retry_attempts ?? ""}
                    onChange={(e) => setFormData({ ...formData, retry_attempts: e.target.value ? parseInt(e.target.value) : undefined })}
                    className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-purple-500 focus:ring-2 focus:ring-purple-500/20 transition-all outline-none"
                    min="0"
                    placeholder="默认 3 次"
                  />
                </div>
              </div>
            </div>
          )}

          {/* 探测模型 */}
          {needsProbeModel && (
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">
                探测模型
              </label>
              <input
                type="text"
                value={formData.probe_model || ""}
                onChange={(e) => setFormData({ ...formData, probe_model: e.target.value || undefined })}
                className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 placeholder-gray-500 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
                placeholder="用于健康检查的模型名称"
              />
            </div>
          )}

          {/* 账号配置 */}
          <div className="space-y-4 p-5 bg-green-500/5 border border-green-500/20 rounded-lg">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <svg className="w-5 h-5 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
                <h3 className="text-lg font-semibold text-white">账号池配置</h3>
              </div>
              <button
                type="button"
                onClick={() => setFormData({ 
                  ...formData, 
                  accounts: [...formData.accounts, { api_key: "", weight: 100 }] 
                })}
                className="px-3 py-1.5 bg-green-500/20 hover:bg-green-500/30 text-green-400 rounded-lg text-sm font-medium transition-all flex items-center gap-1.5"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                添加账号
              </button>
            </div>
            
            {formData.accounts.length === 0 ? (
              <p className="text-sm text-gray-500 text-center py-4">暂无账号，点击"添加账号"开始配置</p>
            ) : (
              <div className="space-y-3">
                {formData.accounts.map((account, index) => (
                  <div key={index} className="flex gap-3 items-start p-3 bg-gray-900/50 rounded-lg border border-gray-700/50">
                    <div className="flex-1">
                      <label className="block text-xs text-gray-400 mb-1.5">API Key</label>
                      <input
                        type="text"
                        value={account.api_key}
                        onChange={(e) => {
                          const newAccounts = [...formData.accounts];
                          newAccounts[index].api_key = e.target.value;
                          setFormData({ ...formData, accounts: newAccounts });
                        }}
                        className="w-full px-3 py-2 bg-gray-800/50 border border-gray-700 rounded-lg text-gray-200 text-sm focus:border-green-500 focus:ring-1 focus:ring-green-500/20 transition-all outline-none font-mono"
                        placeholder="输入 API Key"
                      />
                    </div>
                    <div className="w-24">
                      <label className="block text-xs text-gray-400 mb-1.5">权重</label>
                      <input
                        type="number"
                        value={account.weight}
                        onChange={(e) => {
                          const newAccounts = [...formData.accounts];
                          newAccounts[index].weight = parseInt(e.target.value) || 100;
                          setFormData({ ...formData, accounts: newAccounts });
                        }}
                        className="w-full px-3 py-2 bg-gray-800/50 border border-gray-700 rounded-lg text-gray-200 text-sm focus:border-green-500 focus:ring-1 focus:ring-green-500/20 transition-all outline-none"
                        min="0"
                      />
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        const newAccounts = formData.accounts.filter((_, i) => i !== index);
                        setFormData({ ...formData, accounts: newAccounts });
                      }}
                      className="mt-6 p-2 text-red-400 hover:bg-red-500/10 rounded-lg transition-all"
                      title="删除账号"
                    >
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 模型映射配置 */}
          <div className="space-y-4 p-5 bg-blue-500/5 border border-blue-500/20 rounded-lg">
            <div className="flex items-center justify-between mb-2">
              <div className="flex items-center gap-2">
                <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 7h12m0 0l-4-4m4 4l-4 4m0 6H4m0 0l4 4m-4-4l4-4" />
                </svg>
                <h3 className="text-lg font-semibold text-white">模型映射配置</h3>
              </div>
              <button
                type="button"
                onClick={() => setFormData({ 
                  ...formData, 
                  model_mappings: [...formData.model_mappings, { source_model: "", target_model: "" }] 
                })}
                className="px-3 py-1.5 bg-blue-500/20 hover:bg-blue-500/30 text-blue-400 rounded-lg text-sm font-medium transition-all flex items-center gap-1.5"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                </svg>
                添加映射
              </button>
            </div>
            
            <p className="text-xs text-gray-400 mb-3">
              将客户端请求的模型名称（源模型）映射到渠道实际使用的模型名称（目标模型）
            </p>
            
            {formData.model_mappings.length === 0 ? (
              <p className="text-sm text-gray-500 text-center py-4">暂无模型映射，点击"添加映射"开始配置</p>
            ) : (
              <div className="space-y-3">
                {formData.model_mappings.map((mapping, index) => (
                  <div key={index} className="flex gap-3 items-start p-3 bg-gray-900/50 rounded-lg border border-gray-700/50">
                    <div className="flex-1">
                      <label className="block text-xs text-gray-400 mb-1.5">源模型（客户端请求）</label>
                      <input
                        type="text"
                        value={mapping.source_model}
                        onChange={(e) => {
                          const newMappings = [...formData.model_mappings];
                          newMappings[index].source_model = e.target.value;
                          setFormData({ ...formData, model_mappings: newMappings });
                        }}
                        className="w-full px-3 py-2 bg-gray-800/50 border border-gray-700 rounded-lg text-gray-200 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500/20 transition-all outline-none font-mono"
                        placeholder="例如：gpt-4"
                      />
                    </div>
                    <div className="flex items-center justify-center mt-6">
                      <svg className="w-5 h-5 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14 5l7 7m0 0l-7 7m7-7H3" />
                      </svg>
                    </div>
                    <div className="flex-1">
                      <label className="block text-xs text-gray-400 mb-1.5">目标模型（实际使用）</label>
                      <input
                        type="text"
                        value={mapping.target_model}
                        onChange={(e) => {
                          const newMappings = [...formData.model_mappings];
                          newMappings[index].target_model = e.target.value;
                          setFormData({ ...formData, model_mappings: newMappings });
                        }}
                        className="w-full px-3 py-2 bg-gray-800/50 border border-gray-700 rounded-lg text-gray-200 text-sm focus:border-blue-500 focus:ring-1 focus:ring-blue-500/20 transition-all outline-none font-mono"
                        placeholder="例如：gemini-pro"
                      />
                    </div>
                    <button
                      type="button"
                      onClick={() => {
                        const newMappings = formData.model_mappings.filter((_, i) => i !== index);
                        setFormData({ ...formData, model_mappings: newMappings });
                      }}
                      className="mt-6 p-2 text-red-400 hover:bg-red-500/10 rounded-lg transition-all"
                      title="删除映射"
                    >
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* 底部操作按钮 */}
          <div className="flex justify-end gap-3 pt-6 border-t border-gray-700/50">
            <button
              type="button"
              onClick={onClose}
              disabled={saving}
              className="px-6 py-2.5 bg-gray-700/50 hover:bg-gray-700 text-gray-300 rounded-lg transition-colors font-medium"
            >
              取消
            </button>
            <button
              type="submit"
              disabled={saving}
              className="px-6 py-2.5 bg-gradient-to-r from-blue-600 to-purple-600 hover:from-blue-500 hover:to-purple-500 text-white rounded-lg transition-all font-medium shadow-lg shadow-blue-500/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
            >
              {saving ? (
                <>
                  <svg className="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  保存中...
                </>
              ) : (
                <>
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                  </svg>
                  {channel ? "更新渠道" : "创建渠道"}
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
