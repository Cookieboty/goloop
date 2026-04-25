"use client";

import { useState, useEffect } from "react";
import { api } from "@/lib/api";
import type { Channel, Account } from "@/lib/types";
import { CHANNEL_TYPES } from "@/lib/types";
import { PageTitle } from "@/components/PageTitle";
import { useDialog } from "@/components/Dialog";

type ChannelWithAccounts = Channel & { accounts: Account[] };

export default function AccountsPage() {
  const dialog = useDialog();
  const [channels, setChannels] = useState<ChannelWithAccounts[]>([]);
  const [loading, setLoading] = useState(true);
  const [showForm, setShowForm] = useState(false);
  const [selectedChannelId, setSelectedChannelId] = useState<number | null>(null);

  useEffect(() => {
    loadAllChannelsWithAccounts();
  }, []);

  async function loadAllChannelsWithAccounts() {
    try {
      setLoading(true);
      const channelData = await api.getChannels();
      
      const channelsWithAccounts = await Promise.all(
        channelData.map(async (channel) => {
          try {
            const accounts = await api.getAccounts(channel.id);
            return { ...channel, accounts };
          } catch (err) {
            return { ...channel, accounts: [] };
          }
        })
      );
      
      setChannels(channelsWithAccounts);
    } catch (err: any) {
      await dialog.alert("加载失败：" + err.message, "错误");
    } finally {
      setLoading(false);
    }
  }

  async function handleDeleteAccount(accountId: number, channelId: number) {
    const confirmed = await dialog.confirm("确定要删除此账号吗？", "确认删除", true);
    if (!confirmed) return;
    try {
      await api.deleteAccount(accountId);
      await loadAllChannelsWithAccounts();
    } catch (err: any) {
      await dialog.alert("删除失败：" + err.message, "错误");
    }
  }

  async function handleToggleAccount(accountId: number, enabled: boolean) {
    try {
      await api.toggleAccount(accountId, enabled);
      await loadAllChannelsWithAccounts();
    } catch (err: any) {
      await dialog.alert("切换状态失败：" + err.message, "错误");
    }
  }

  if (loading) {
    return (
      <div className="p-6 lg:p-8 max-w-7xl mx-auto">
        <PageTitle>账号池管理</PageTitle>
        <div className="flex items-center justify-center mt-12">
          <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-green-500"></div>
        </div>
      </div>
    );
  }

  const totalAccounts = channels.reduce((sum, ch) => sum + (ch.accounts?.length || 0), 0);

  return (
    <div className="p-6 lg:p-8 max-w-7xl mx-auto">
      <div className="flex justify-between items-center mb-8">
        <div>
          <PageTitle>账号池管理</PageTitle>
          <p className="text-gray-400 text-sm mt-2">
            管理所有渠道的 API 账号池 · 共 {totalAccounts} 个账号
          </p>
        </div>
      </div>

      {channels.length === 0 ? (
        <div className="text-center py-16">
          <div className="inline-flex items-center justify-center w-16 h-16 bg-gray-800 rounded-full mb-4">
            <svg className="w-8 h-8 text-gray-600" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
            </svg>
          </div>
          <h3 className="text-lg font-medium text-gray-300 mb-2">暂无渠道</h3>
          <p className="text-gray-500 text-sm">请先创建渠道</p>
        </div>
      ) : (
        <div className="space-y-10">
          {Object.entries(
            channels
              .reduce((bigGroups, channel) => {
                const bigCategory = channel.type.startsWith('gemini') ? 'Gemini' : 'OpenAI';
                if (!bigGroups[bigCategory]) {
                  bigGroups[bigCategory] = {};
                }
                
                const typeInfo = CHANNEL_TYPES.find((t) => t.value === channel.type);
                const typeLabel = typeInfo?.label || channel.type;
                if (!bigGroups[bigCategory][typeLabel]) {
                  bigGroups[bigCategory][typeLabel] = [];
                }
                bigGroups[bigCategory][typeLabel].push(channel);
                return bigGroups;
              }, {} as Record<string, Record<string, ChannelWithAccounts[]>>)
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
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                  </svg>
                </div>
                <h1 className="text-2xl font-bold text-white">{bigCategory}</h1>
                <span className="px-3 py-1 bg-gray-700/50 text-gray-300 text-sm rounded-full font-medium">
                  {Object.values(typeGroups).reduce((sum, channels) => 
                    sum + channels.reduce((acc, ch) => acc + (ch.accounts?.length || 0), 0), 0
                  )} 个账号
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
                          {channelsInType.reduce((sum, ch) => sum + (ch.accounts?.length || 0), 0)} 个账号
                        </span>
                      </div>
                    </div>
                    
                    {/* 渠道及其账号列表 */}
                    <div className="space-y-4">
                      {channelsInType.map((channel) => (
                        <div key={channel.id} className="bg-gray-800/50 rounded-lg border border-gray-700/50 overflow-hidden">
                          {/* 渠道信息头部 */}
                          <div className="bg-gray-800/80 px-4 py-3 border-b border-gray-700/50">
                            <div className="flex items-center justify-between">
                              <div className="flex items-center gap-3">
                                <div className={`w-2 h-2 rounded-full ${
                                  channel.enabled 
                                    ? 'bg-green-400 animate-pulse shadow-lg shadow-green-400/50' 
                                    : 'bg-gray-500'
                                }`}></div>
                                <h3 className="text-sm font-semibold text-white">{channel.name}</h3>
                                <span className={`text-xs ${channel.enabled ? 'text-green-400' : 'text-gray-500'}`}>
                                  {channel.enabled ? '运行中' : '已停用'}
                                </span>
                                <span className="px-2 py-0.5 bg-gray-700/50 text-gray-400 text-xs rounded">
                                  {channel.accounts?.length || 0} 个账号
                                </span>
                              </div>
                              <button
                                onClick={() => {
                                  setSelectedChannelId(channel.id);
                                  setShowForm(true);
                                }}
                                className="px-3 py-1.5 bg-green-500/20 hover:bg-green-500/30 text-green-400 rounded-lg text-xs font-medium transition-all flex items-center gap-1.5"
                              >
                                <svg className="w-3.5 h-3.5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v16m8-8H4" />
                                </svg>
                                添加账号
                              </button>
                            </div>
                          </div>

                          {/* 账号表格 */}
                          {channel.accounts && channel.accounts.length > 0 ? (
                            <div className="overflow-x-auto">
                              <table className="w-full">
                                <thead>
                                  <tr className="bg-gray-800/60">
                                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider w-16">状态</th>
                                    <th className="px-4 py-3 text-left text-xs font-medium text-gray-400 uppercase tracking-wider">API Key</th>
                                    <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-24">权重</th>
                                    <th className="px-4 py-3 text-center text-xs font-medium text-gray-400 uppercase tracking-wider w-32">操作</th>
                                  </tr>
                                </thead>
                                <tbody className="divide-y divide-gray-700/30">
                                  {channel.accounts.map((account) => (
                                    <tr key={account.id} className="hover:bg-gray-800/40 transition-colors">
                                      <td className="px-4 py-3">
                                        <div className={`w-2 h-2 rounded-full ${
                                          account.enabled 
                                            ? 'bg-green-400 animate-pulse shadow-lg shadow-green-400/50' 
                                            : 'bg-gray-500'
                                        }`}></div>
                                      </td>
                                      <td className="px-4 py-3">
                                        <p className="text-sm font-mono text-gray-300 truncate max-w-md">{account.api_key}</p>
                                      </td>
                                      <td className="px-4 py-3 text-center">
                                        <p className="text-sm font-semibold text-gray-300">{account.weight}</p>
                                      </td>
                                      <td className="px-4 py-3">
                                        <div className="flex items-center justify-center gap-2">
                                          <button
                                            onClick={() => handleToggleAccount(account.id, !account.enabled)}
                                            className={`p-1.5 rounded-lg transition-all duration-200 ${
                                              account.enabled
                                                ? 'text-yellow-400 hover:bg-yellow-500/10'
                                                : 'text-green-400 hover:bg-green-500/10'
                                            }`}
                                            title={account.enabled ? '禁用' : '启用'}
                                          >
                                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                              {account.enabled ? (
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
                                            onClick={() => handleDeleteAccount(account.id, channel.id)}
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
                                  ))}
                                </tbody>
                              </table>
                            </div>
                          ) : (
                            <div className="text-center py-8 text-gray-500 text-sm">
                              此渠道暂无账号
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

      {showForm && selectedChannelId && (
        <AccountForm
          channelId={selectedChannelId}
          onClose={() => {
            setShowForm(false);
            setSelectedChannelId(null);
          }}
          onSave={async () => {
            setShowForm(false);
            setSelectedChannelId(null);
            await loadAllChannelsWithAccounts();
          }}
        />
      )}
    </div>
  );
}

function AccountForm({
  channelId,
  onClose,
  onSave,
}: {
  channelId: number;
  onClose: () => void;
  onSave: () => void;
}) {
  const dialog = useDialog();
  const [apiKey, setApiKey] = useState("");
  const [weight, setWeight] = useState(100);
  const [saving, setSaving] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      await api.createAccount(channelId, { api_key: apiKey, weight });
      onSave();
    } catch (err: any) {
      await dialog.alert("创建失败：" + err.message, "错误");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="fixed inset-0 bg-black/70 backdrop-blur-sm flex items-center justify-center z-50 overflow-y-auto p-4">
      <div className="bg-gradient-to-br from-gray-800/95 to-gray-900/95 backdrop-blur-md rounded-2xl border border-gray-700/50 shadow-2xl max-w-lg w-full">
        <div className="sticky top-0 bg-gradient-to-r from-gray-800 to-gray-900 border-b border-gray-700/50 px-8 py-6 flex justify-between items-center backdrop-blur-sm rounded-t-2xl">
          <div>
            <h2 className="text-2xl font-bold text-white mb-1">添加账号</h2>
            <p className="text-sm text-gray-400">为当前渠道添加新的 API 账号</p>
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
              API Key <span className="text-red-400">*</span>
            </label>
            <input
              type="text"
              required
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 font-mono text-sm placeholder-gray-500 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
              placeholder="sk-xxxxx 或其他 API Key"
            />
          </div>

          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              权重 <span className="text-red-400">*</span>
            </label>
            <input
              type="number"
              required
              value={weight}
              onChange={(e) => setWeight(parseInt(e.target.value) || 0)}
              className="w-full px-4 py-2.5 bg-gray-900/50 border border-gray-700 rounded-lg text-gray-200 focus:border-blue-500 focus:ring-2 focus:ring-blue-500/20 transition-all outline-none"
              min="0"
            />
            <p className="text-xs text-gray-400 mt-1.5">
              权重越高，被选中的概率越大
            </p>
          </div>

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
                  添加中...
                </>
              ) : (
                <>
                  <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 13l4 4L19 7" />
                  </svg>
                  添加
                </>
              )}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
