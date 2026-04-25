"use client";

import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { APIKey, CreateAPIKeyRequest, Channel } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";

type GroupedAPIKeys = {
  [category: string]: APIKey[];
};

export default function APIKeysPage() {
  const dialog = useDialog();
  const [apiKeys, setAPIKeys] = useState<APIKey[]>([]);
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [editingKey, setEditingKey] = useState<APIKey | null>(null);
  const [selectedKey, setSelectedKey] = useState<number | null>(null);

  useEffect(() => {
    loadData();
  }, []);

  async function loadData() {
    try {
      setLoading(true);
      const [keysData, channelsData] = await Promise.all([
        api.getAPIKeys(),
        api.getChannels()
      ]);
      setAPIKeys(keysData);
      setChannels(channelsData);
    } catch (err: any) {
      await dialog.alert("加载失败：" + err.message, "错误");
    } finally {
      setLoading(false);
    }
  }

  async function handleDelete(id: number) {
    const confirmed = await dialog.confirm("确定要删除此 API Key 吗？", "确认删除", true);
    if (!confirmed) return;
    try {
      await api.deleteAPIKey(id);
      await loadData();
    } catch (err: any) {
      await dialog.alert("删除失败：" + err.message, "错误");
    }
  }

  async function handleToggle(id: number, enabled: boolean) {
    try {
      await api.toggleAPIKey(id, enabled);
      await loadData();
    } catch (err: any) {
      await dialog.alert("切换状态失败：" + err.message, "错误");
    }
  }

  // 根据渠道限制确定大类
  function getChannelCategory(key: APIKey): string {
    if (!key.channel_restriction) {
      return "全部渠道";
    }
    
    const channel = channels.find(ch => ch.name === key.channel_restriction);
    if (channel) {
      return channel.type.startsWith('gemini') ? 'Gemini' : 'OpenAI';
    }
    
    // 如果找不到渠道，根据名称判断
    const restriction = key.channel_restriction.toLowerCase();
    if (restriction.includes('gemini')) return 'Gemini';
    if (restriction.includes('openai') || restriction.includes('gpt')) return 'OpenAI';
    return "其他";
  }

  if (loading) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>API Key 管理</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
        </div>
      </div>
    );
  }

  // 对 API Keys 进行分组
  const groupedKeys: GroupedAPIKeys = apiKeys.reduce((groups, key) => {
    const category = getChannelCategory(key);
    if (!groups[category]) {
      groups[category] = [];
    }
    groups[category].push(key);
    return groups;
  }, {} as GroupedAPIKeys);

  // 按优先级排序分类：Gemini -> OpenAI -> 全部渠道 -> 其他
  const sortedCategories = Object.keys(groupedKeys).sort((a, b) => {
    const order = { 'Gemini': 0, 'OpenAI': 1, '全部渠道': 2, '其他': 3 };
    return (order[a as keyof typeof order] ?? 999) - (order[b as keyof typeof order] ?? 999);
  });

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>API Key 管理</PageTitle>
          <p className="text-gray-400 text-sm mt-2">
            管理所有 API 访问密钥 · 共 {apiKeys.length} 个
          </p>
        </div>
        <button
          onClick={() => {
            setEditingKey(null);
            setShowForm(true);
          }}
          className="group px-5 py-2.5 bg-gradient-to-r from-green-600 to-emerald-500 text-white rounded-lg hover:from-green-500 hover:to-emerald-600 transition-all duration-300 shadow-lg shadow-green-500/20 hover:shadow-xl hover:shadow-green-500/30 flex items-center gap-2 font-medium"
        >
          <svg className="w-4 h-4 group-hover:scale-110 transition-transform" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
          </svg>
          创建 API Key
        </button>
      </div>

      {apiKeys.length === 0 ? (
        <div className="text-center py-16">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
            <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-300 mb-2">暂无 API Key</h3>
          <p className="text-gray-500 text-sm">点击"创建 API Key"开始添加您的第一个 API Key</p>
        </div>
      ) : (
        <div className="space-y-10">
          {sortedCategories.map((category) => {
            const keysInCategory = groupedKeys[category];
            const getCategoryIcon = () => {
              if (category === 'Gemini') return 'text-blue-400';
              if (category === 'OpenAI') return 'text-green-400';
              if (category === '全部渠道') return 'text-purple-400';
              return 'text-gray-400';
            };
            const getCategoryColor = () => {
              if (category === 'Gemini') return 'from-blue-500/20 to-cyan-500/20 border-blue-500/30';
              if (category === 'OpenAI') return 'from-green-500/20 to-emerald-500/20 border-green-500/30';
              if (category === '全部渠道') return 'from-purple-500/20 to-pink-500/20 border-purple-500/30';
              return 'from-gray-500/20 to-gray-600/20 border-gray-500/30';
            };

            return (
              <div key={category} className="space-y-6">
                {/* 大类标题 */}
                <div className="flex items-center gap-3 pb-3 border-b border-gray-700/50">
                  <div className={`w-10 h-10 rounded-lg flex items-center justify-center bg-gradient-to-br ${getCategoryColor()}`}>
                    <svg className={`w-6 h-6 ${getCategoryIcon()}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z" />
                    </svg>
                  </div>
                  <h1 className="text-2xl font-bold text-white">{category}</h1>
                  <span className="px-3 py-1 bg-gray-700/50 text-gray-300 text-sm rounded-full font-medium">
                    {keysInCategory.length} 个密钥
                  </span>
                </div>

                {/* API Keys 表格 */}
                <div className="bg-gray-800/50 rounded-lg border border-gray-700/50 overflow-hidden">
                  <div className="overflow-x-auto">
                    <table className="w-full">
                      <thead>
                        <tr className="bg-gray-800/80">
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider w-16">状态</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">名称</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">API Key</th>
                          <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">渠道限制</th>
                          <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-24">总请求</th>
                          <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-20">成功</th>
                          <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-20">失败</th>
                          <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-48">操作</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-gray-700/30">
                        {keysInCategory.map((key) => {
                          const isExpired = key.expires_at && new Date(key.expires_at) < new Date();
                          return (
                            <tr key={key.id} className="hover:bg-gray-800/40 transition-colors">
                              <td className="px-4 py-3">
                                <div className={`w-2 h-2 rounded-full ${
                                  isExpired ? 'bg-red-400' :
                                  key.enabled ? 'bg-green-400 animate-pulse shadow-lg shadow-green-400/50' : 'bg-gray-500'
                                }`}></div>
                              </td>
                              <td className="px-4 py-3">
                                <div>
                                  <p className="text-sm font-semibold text-white">{key.name}</p>
                                  <div className="flex items-center gap-2 mt-1">
                                    {key.enabled ? (
                                      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-green-500/10 text-green-400">
                                        启用
                                      </span>
                                    ) : (
                                      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-gray-500/10 text-gray-400">
                                        禁用
                                      </span>
                                    )}
                                    {isExpired && (
                                      <span className="inline-flex items-center px-1.5 py-0.5 rounded text-xs font-medium bg-red-500/10 text-red-400">
                                        已过期
                                      </span>
                                    )}
                                  </div>
                                </div>
                              </td>
                              <td className="px-4 py-3">
                                <p className="text-sm font-mono text-gray-300 truncate max-w-xs">{key.key}</p>
                                {key.last_used_at && (
                                  <p className="text-xs text-gray-500 mt-0.5">
                                    最后使用: {new Date(key.last_used_at).toLocaleString("zh-CN")}
                                  </p>
                                )}
                              </td>
                              <td className="px-4 py-3">
                                {key.channel_restriction ? (
                                  <div>
                                    <p className="text-sm text-gray-300">{key.channel_restriction}</p>
                                    {key.expires_at && (
                                      <p className="text-xs text-gray-500 mt-0.5">
                                        过期: {new Date(key.expires_at).toLocaleDateString("zh-CN")}
                                      </p>
                                    )}
                                  </div>
                                ) : (
                                  <span className="text-sm text-gray-500">-</span>
                                )}
                              </td>
                              <td className="px-4 py-3 text-center">
                                <p className="text-sm text-gray-300 font-medium">{key.total_requests}</p>
                              </td>
                              <td className="px-4 py-3 text-center">
                                <p className="text-sm text-green-400 font-medium">{key.total_success}</p>
                              </td>
                              <td className="px-4 py-3 text-center">
                                <p className="text-sm text-red-400 font-medium">{key.total_fail}</p>
                              </td>
                              <td className="px-4 py-3">
                                <div className="flex items-center justify-center gap-2">
                                  <button
                                    onClick={() => setSelectedKey(key.id)}
                                    className="p-1.5 text-blue-400 hover:bg-blue-500/10 rounded-lg transition-all duration-200"
                                    title="查看日志"
                                  >
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                                    </svg>
                                  </button>
                                  <button
                                    onClick={() => {
                                      setEditingKey(key);
                                      setShowForm(true);
                                    }}
                                    className="p-1.5 text-purple-400 hover:bg-purple-500/10 rounded-lg transition-all duration-200"
                                    title="编辑"
                                  >
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                                    </svg>
                                  </button>
                                  <button
                                    onClick={() => handleToggle(key.id, !key.enabled)}
                                    className={`p-1.5 rounded-lg transition-all duration-200 ${
                                      key.enabled
                                        ? 'text-yellow-400 hover:bg-yellow-500/10'
                                        : 'text-green-400 hover:bg-green-500/10'
                                    }`}
                                    title={key.enabled ? '禁用' : '启用'}
                                  >
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      {key.enabled ? (
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
                                    onClick={() => handleDelete(key.id)}
                                    className="p-1.5 text-red-400 hover:bg-red-500/10 rounded-lg transition-all duration-200"
                                    title="删除"
                                  >
                                    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                                    </svg>
                                  </button>
                                </div>
                              </td>
                            </tr>
                          );
                        })}
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {showForm && (
        <APIKeyForm
          apiKey={editingKey}
          onClose={() => {
            setShowForm(false);
            setEditingKey(null);
          }}
          onSave={async () => {
            setShowForm(false);
            setEditingKey(null);
            await loadData();
          }}
        />
      )}

      {selectedKey && (
        <UsageLogsModal keyId={selectedKey} onClose={() => setSelectedKey(null)} />
      )}
    </div>
  );
}

function APIKeyForm({ 
  apiKey, 
  onClose, 
  onSave 
}: { 
  apiKey: APIKey | null;
  onClose: () => void; 
  onSave: () => void;
}) {
  const dialog = useDialog();
  const [channels, setChannels] = useState<Channel[]>([]);
  const [formData, setFormData] = useState<CreateAPIKeyRequest & { enabled?: boolean }>({
    name: apiKey?.name || "",
    channel_restriction: apiKey?.channel_restriction || undefined,
    expires_at: apiKey?.expires_at || undefined,
    enabled: apiKey?.enabled ?? true,
  });
  const [saving, setSaving] = useState(false);
  const [createdKey, setCreatedKey] = useState<string | null>(null);

  useEffect(() => {
    loadChannels();
  }, []);

  async function loadChannels() {
    try {
      const data = await api.getChannels();
      setChannels(data);
    } catch (err: any) {
      console.error("Failed to load channels:", err);
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      if (apiKey) {
        // 编辑模式
        await api.updateAPIKey(apiKey.id, {
          name: formData.name,
          channel_restriction: formData.channel_restriction,
          expires_at: formData.expires_at,
          enabled: formData.enabled ?? true,
        });
        onSave();
      } else {
        // 创建模式
        const result = await api.createAPIKey(formData);
        setCreatedKey(result.key);
      }
    } catch (err: any) {
      await dialog.alert((apiKey ? "更新" : "创建") + "失败：" + err.message, "错误");
      setSaving(false);
    }
  }

  if (createdKey) {
    return (
      <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50">
        <div className="bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md rounded-2xl border border-gray-700/50 shadow-2xl max-w-lg w-full mx-4">
          <div className="p-8">
            <div className="flex items-center gap-3 mb-4">
              <div className="flex items-center justify-center w-12 h-12 bg-green-500/20 rounded-full border border-green-500/30">
                <svg className="w-6 h-6 text-green-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                </svg>
              </div>
              <h2 className="text-2xl font-bold text-white">API Key 创建成功</h2>
            </div>
            <div className="mb-6">
              <p className="text-sm text-gray-300 mb-3">
                请妥善保存以下 API Key，关闭后将无法再次查看：
              </p>
              <div className="font-mono text-sm bg-gray-900/50 text-gray-200 p-4 rounded-lg break-all border border-gray-700/50">
                {createdKey}
              </div>
            </div>
            <div className="space-y-2">
              <button
                onClick={async () => {
                  navigator.clipboard.writeText(createdKey);
                  await dialog.alert("已复制到剪贴板", "成功");
                }}
                className="w-full px-4 py-2.5 bg-gradient-to-r from-blue-600 to-blue-500 text-white rounded-lg hover:from-blue-500 hover:to-blue-600 transition-all font-medium shadow-lg shadow-blue-500/20 flex items-center justify-center gap-2"
              >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                </svg>
                复制到剪贴板
              </button>
              <button
                onClick={onSave}
                className="w-full px-4 py-2.5 border border-gray-600 rounded-lg hover:bg-gray-700/50 text-gray-300 transition-colors font-medium"
              >
                关闭
              </button>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 overflow-y-auto p-4">
      <div className="bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md rounded-2xl border border-gray-700/50 shadow-2xl max-w-lg w-full">
        <div className="sticky top-0 bg-gradient-to-r from-gray-800 to-gray-900 border-b border-gray-700/50 px-8 py-6 flex justify-between items-center backdrop-blur-sm rounded-t-2xl">
          <div>
            <h2 className="text-2xl font-bold text-white mb-1">
              {apiKey ? "编辑 API Key" : "创建 API Key"}
            </h2>
            <p className="text-sm text-gray-400">
              {apiKey ? "修改 API Key 的配置信息" : "创建新的 API 访问密钥"}
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

        <form onSubmit={handleSubmit} className="p-8 space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              名称 <span className="text-red-400">*</span>
            </label>
            <input
              type="text"
              required
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
              className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 placeholder-gray-500 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
              placeholder="例如: 测试密钥"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">渠道限制（可选）</label>
            <select
              value={formData.channel_restriction || ""}
              onChange={(e) =>
                setFormData({
                  ...formData,
                  channel_restriction: e.target.value || undefined,
                })
              }
              className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
            >
              <option value="">不限制（全部渠道）</option>
              {channels.map((ch) => (
                <option key={ch.id} value={ch.name}>
                  {ch.name} ({ch.type})
                </option>
              ))}
            </select>
            <p className="text-xs text-gray-400 mt-1.5">
              指定此 API Key 只能访问特定渠道
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">过期时间（可选）</label>
            <input
              type="datetime-local"
              value={
                formData.expires_at
                  ? new Date(formData.expires_at).toISOString().slice(0, 16)
                  : ""
              }
              onChange={(e) =>
                setFormData({
                  ...formData,
                  expires_at: e.target.value
                    ? new Date(e.target.value).toISOString()
                    : undefined,
                })
              }
              className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
            />
            <p className="text-xs text-gray-400 mt-1.5">留空表示永不过期</p>
          </div>

          {apiKey && (
            <div>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  checked={formData.enabled}
                  onChange={(e) =>
                    setFormData({
                      ...formData,
                      enabled: e.target.checked,
                    })
                  }
                  className="w-4 h-4 rounded border-gray-700 bg-gray-900/50 text-blue-500 focus:ring-2 focus:ring-blue-500/20"
                />
                <span className="text-sm font-medium text-gray-300">启用此 API Key</span>
              </label>
            </div>
          )}

          <div className="flex justify-end gap-3 pt-6 border-t border-gray-700/50">
            <button
              type="button"
              onClick={onClose}
              className="px-6 py-2.5 bg-gray-700/50 hover:bg-gray-700 text-gray-300 rounded-lg transition-colors font-medium"
              disabled={saving}
            >
              取消
            </button>
            <button
              type="submit"
              className="px-6 py-2.5 bg-gradient-to-r from-green-600 to-emerald-600 hover:from-green-500 hover:to-emerald-500 text-white rounded-lg transition-all font-medium shadow-lg shadow-green-500/20 disabled:opacity-50 disabled:cursor-not-allowed flex items-center gap-2"
              disabled={saving}
            >
              {saving ? (
                <>
                  <svg className="animate-spin h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  创建中...
                </>
              ) : (
                <>
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                  </svg>
                  {apiKey ? "更新" : "创建"}
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

type TimeRange = "today" | "1h" | "24h" | "7d" | "all";

function UsageLogsModal({ keyId, onClose }: { keyId: number; onClose: () => void }) {
  const dialog = useDialog();
  const [logs, setLogs] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [timeRange, setTimeRange] = useState<TimeRange>("today");
  const [selectedLog, setSelectedLog] = useState<any | null>(null);

  useEffect(() => {
    loadLogs();
  }, [keyId, timeRange]);

  async function loadLogs() {
    try {
      setLoading(true);
      const data = await api.getUsageLogs(keyId, 500);
      
      // 根据时间范围筛选
      const now = new Date();
      let filteredLogs = data;
      
      if (timeRange === "today") {
        const todayStart = new Date();
        todayStart.setHours(0, 0, 0, 0);
        filteredLogs = data.filter((log: any) => new Date(log.created_at) >= todayStart);
      } else if (timeRange !== "all") {
        const hoursMap: Record<Exclude<TimeRange, "today" | "all">, number> = {
          "1h": 1,
          "24h": 24,
          "7d": 24 * 7,
        };
        const hours = hoursMap[timeRange];
        const startTime = new Date(now.getTime() - hours * 60 * 60 * 1000);
        filteredLogs = data.filter((log: any) => new Date(log.created_at) >= startTime);
      }
      
      setLogs(filteredLogs);
    } catch (err: any) {
      await dialog.alert("加载日志失败：" + err.message, "错误");
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <div 
        className="fixed inset-0 bg-black/70 backdrop-blur-sm z-50 animate-in fade-in duration-200"
        onClick={onClose}
      >
        <div 
          className="fixed right-0 top-0 h-full w-full max-w-4xl bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md shadow-2xl border-l border-gray-700/50 animate-in slide-in-from-right duration-300 flex flex-col"
          onClick={(e) => e.stopPropagation()}
        >
          {/* Header */}
          <div className="p-6 border-b border-gray-700/50 flex-shrink-0">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-lg bg-blue-500/20 border border-blue-500/30 flex items-center justify-center">
                  <svg className="w-5 h-5 text-blue-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                  </svg>
                </div>
                <div>
                  <h2 className="text-xl font-semibold text-white">使用日志</h2>
                  <p className="text-sm text-gray-400 mt-0.5">
                    共 {logs.length} 条记录
                  </p>
                </div>
              </div>
              <button
                onClick={onClose}
                className="p-2 hover:bg-gray-700/50 rounded-lg transition-all"
              >
                <svg className="w-5 h-5 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            {/* Time Range Filter */}
            <div className="mt-4 flex flex-wrap gap-2">
              <button
                onClick={() => setTimeRange("today")}
                className={`px-3 py-1.5 rounded-lg text-sm transition-all ${
                  timeRange === "today"
                    ? "bg-blue-500 text-white"
                    : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
                }`}
              >
                今天
              </button>
              <button
                onClick={() => setTimeRange("1h")}
                className={`px-3 py-1.5 rounded-lg text-sm transition-all ${
                  timeRange === "1h"
                    ? "bg-blue-500 text-white"
                    : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
                }`}
              >
                最近 1 小时
              </button>
              <button
                onClick={() => setTimeRange("24h")}
                className={`px-3 py-1.5 rounded-lg text-sm transition-all ${
                  timeRange === "24h"
                    ? "bg-blue-500 text-white"
                    : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
                }`}
              >
                最近 24 小时
              </button>
              <button
                onClick={() => setTimeRange("7d")}
                className={`px-3 py-1.5 rounded-lg text-sm transition-all ${
                  timeRange === "7d"
                    ? "bg-blue-500 text-white"
                    : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
                }`}
              >
                最近 7 天
              </button>
              <button
                onClick={() => setTimeRange("all")}
                className={`px-3 py-1.5 rounded-lg text-sm transition-all ${
                  timeRange === "all"
                    ? "bg-blue-500 text-white"
                    : "bg-gray-700/50 text-gray-300 hover:bg-gray-700"
                }`}
              >
                全部
              </button>
            </div>
          </div>

          {/* Content */}
          <div className="flex-1 overflow-hidden">
            {loading ? (
              <div className="flex items-center justify-center h-full">
                <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-500"></div>
              </div>
            ) : logs.length === 0 ? (
              <div className="flex items-center justify-center h-full">
                <div className="text-center">
                  <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
                    <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                    </svg>
                  </div>
                  <h3 className="text-lg font-medium text-gray-300 mb-2">暂无使用记录</h3>
                  <p className="text-gray-500 text-sm">在选定的时间范围内没有日志</p>
                </div>
              </div>
            ) : (
              <div className="h-full overflow-y-auto">
                <table className="w-full">
                  <thead className="sticky top-0 z-10">
                    <tr className="bg-gray-800/95 backdrop-blur-sm">
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">时间</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">渠道</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">模型</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider w-24">状态</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider w-24">延迟</th>
                      <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">IP</th>
                      <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-20">操作</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-700/30 bg-gray-800/50">
                    {logs.map((log) => (
                      <tr 
                        key={log.id} 
                        className="hover:bg-gray-800/40 transition-colors cursor-pointer"
                        onClick={() => setSelectedLog(log)}
                      >
                        <td className="px-4 py-3">
                          <p className="text-sm text-gray-300">
                            {new Date(log.created_at).toLocaleString("zh-CN")}
                          </p>
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-sm text-gray-200">{log.channel_name}</p>
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-sm font-mono text-gray-200">{log.model}</p>
                        </td>
                        <td className="px-4 py-3">
                          {log.success ? (
                            <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-green-500/10 text-green-400">
                              成功
                            </span>
                          ) : (
                            <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-red-500/10 text-red-400">
                              失败
                            </span>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-sm text-gray-300">{log.latency_ms}ms</p>
                        </td>
                        <td className="px-4 py-3">
                          <p className="text-sm font-mono text-gray-300">{log.request_ip || "-"}</p>
                        </td>
                        <td className="px-4 py-3 text-center">
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              setSelectedLog(log);
                            }}
                            className="p-1.5 text-blue-400 hover:bg-blue-500/10 rounded-lg transition-all duration-200"
                            title="查看详情"
                          >
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                            </svg>
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Log Detail Drawer */}
      {selectedLog && (
        <div 
          className="fixed inset-0 bg-black/70 backdrop-blur-sm z-[60] animate-in fade-in duration-200"
          onClick={() => setSelectedLog(null)}
        >
          <div 
            className="fixed right-0 top-0 h-full w-full max-w-2xl bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md shadow-2xl border-l border-gray-700/50 animate-in slide-in-from-right duration-300 flex flex-col"
            onClick={(e) => e.stopPropagation()}
          >
            {/* Header */}
            <div className="p-6 border-b border-gray-700/50 flex-shrink-0">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${
                    selectedLog.success 
                      ? 'bg-green-500/20 border border-green-500/30' 
                      : 'bg-red-500/20 border border-red-500/30'
                  }`}>
                    <svg className={`w-5 h-5 ${selectedLog.success ? 'text-green-400' : 'text-red-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
                      {selectedLog.success ? (
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                      ) : (
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
                      )}
                    </svg>
                  </div>
                  <div>
                    <h2 className="text-xl font-semibold text-white">日志详情</h2>
                    <p className="text-sm text-gray-400 mt-0.5">
                      {new Date(selectedLog.created_at).toLocaleString("zh-CN")}
                    </p>
                  </div>
                </div>
                <button
                  onClick={() => setSelectedLog(null)}
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
                    <p className="text-xs text-gray-500 mb-2">渠道</p>
                    <p className="text-sm text-white font-medium">{selectedLog.channel_name}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">模型</p>
                    <p className="text-sm text-white font-mono">{selectedLog.model}</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">状态</p>
                    {selectedLog.success ? (
                      <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-green-500/10 text-green-400">
                        成功
                      </span>
                    ) : (
                      <span className="inline-flex items-center px-2 py-1 rounded text-xs font-medium bg-red-500/10 text-red-400">
                        失败
                      </span>
                    )}
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">延迟时间</p>
                    <p className="text-sm text-white">{selectedLog.latency_ms}ms</p>
                  </div>
                  <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                    <p className="text-xs text-gray-500 mb-2">请求 IP</p>
                    <p className="text-sm text-white font-mono">{selectedLog.request_ip || "-"}</p>
                  </div>
                  {selectedLog.status_code && (
                    <div className="p-4 rounded-lg bg-gray-800/50 border border-gray-700/50">
                      <p className="text-xs text-gray-500 mb-2">状态码</p>
                      <p className="text-sm text-white font-semibold">{selectedLog.status_code}</p>
                    </div>
                  )}
                </div>

                {/* Error Message */}
                {selectedLog.error_message && (
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <h3 className="text-sm font-semibold text-white">错误信息</h3>
                      <button
                        onClick={async () => {
                          await navigator.clipboard.writeText(selectedLog.error_message);
                          await dialog.alert("已复制到剪贴板", "成功");
                        }}
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
    </>
  );
}
