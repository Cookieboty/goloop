import type {
  StatsResponse,
  ChannelAccountsResponse,
  IssueTokenRequest,
  IssueTokenResponse,
  Channel,
  Account,
  ModelMapping,
  APIKey,
  UsageLog,
  CreateChannelRequest,
  CreateAccountRequest,
  CreateMappingRequest,
  CreateAPIKeyRequest,
  APIKeyStatsResponse,
  GlobalStatsResponse,
} from "./types";

const API_BASE = "";
const ADMIN_KEY_STORAGE_KEY = "goloop_admin_key";

// --- Admin key management ---

export function getAdminKey(): string {
  if (typeof window === "undefined") return "";
  return localStorage.getItem(ADMIN_KEY_STORAGE_KEY) ?? "";
}

export function setAdminKey(key: string): void {
  localStorage.setItem(ADMIN_KEY_STORAGE_KEY, key);
}

export function clearAdminKey(): void {
  localStorage.removeItem(ADMIN_KEY_STORAGE_KEY);
}

export function isLoggedIn(): boolean {
  return getAdminKey().length > 0;
}

// --- HTTP request helper ---

export class AuthError extends Error {
  constructor() {
    super("Authentication required");
    this.name = "AuthError";
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const adminKey = getAdminKey();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(adminKey ? { "X-Admin-Key": adminKey } : {}),
  };

  const res = await fetch(API_BASE + path, {
    ...options,
    headers: {
      ...headers,
      ...(options?.headers as Record<string, string> | undefined),
    },
  });

  if (res.status === 401) {
    throw new AuthError();
  }

  if (!res.ok) {
    const text = await res.text();
    throw new Error(`${res.status}: ${text}`);
  }
  return res.json() as Promise<T>;
}

// --- API methods ---

export const api = {
  // Legacy endpoints (kept for backward compatibility)
  getStats(): Promise<StatsResponse> {
    return request<StatsResponse>("/admin/stats");
  },

  getChannelAccounts(channel: string): Promise<ChannelAccountsResponse> {
    return request<ChannelAccountsResponse>(
      `/admin/channel/${channel}/accounts`
    );
  },

  issueToken(body: IssueTokenRequest): Promise<IssueTokenResponse> {
    return request<IssueTokenResponse>("/admin/issue-token", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  quickToken(): Promise<IssueTokenResponse> {
    return request<IssueTokenResponse>("/admin/quick-token", { method: "POST", body: "{}" });
  },

  // ==================== Channel CRUD ====================
  
  getChannels(): Promise<Channel[]> {
    return request<Channel[]>("/admin/api/channels");
  },

  getChannel(id: number): Promise<Channel> {
    return request<Channel>(`/admin/api/channels/${id}`);
  },

  createChannel(body: CreateChannelRequest): Promise<Channel> {
    return request<Channel>("/admin/api/channels", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  updateChannel(id: number, body: CreateChannelRequest): Promise<Channel> {
    return request<Channel>(`/admin/api/channels/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  deleteChannel(id: number): Promise<void> {
    return request<void>(`/admin/api/channels/${id}`, {
      method: "DELETE",
    });
  },

  toggleChannel(id: number, enabled: boolean): Promise<{ enabled: boolean }> {
    return request<{ enabled: boolean }>(`/admin/api/channels/${id}/toggle`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    });
  },

  // ==================== Account CRUD ====================

  getAccounts(channelId: number): Promise<Account[]> {
    return request<Account[]>(`/admin/api/channels/${channelId}/accounts`);
  },

  createAccount(channelId: number, body: CreateAccountRequest): Promise<Account> {
    return request<Account>(`/admin/api/channels/${channelId}/accounts`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  updateAccount(id: number, body: CreateAccountRequest & { enabled: boolean }): Promise<Account> {
    return request<Account>(`/admin/api/accounts/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  deleteAccount(id: number): Promise<void> {
    return request<void>(`/admin/api/accounts/${id}`, {
      method: "DELETE",
    });
  },

  toggleAccount(id: number, enabled: boolean): Promise<{ enabled: boolean }> {
    return request<{ enabled: boolean }>(`/admin/api/accounts/${id}/toggle`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    });
  },

  // ==================== Model Mapping CRUD ====================

  getMappings(channelId: number): Promise<ModelMapping[]> {
    return request<ModelMapping[]>(`/admin/api/channels/${channelId}/mappings`);
  },

  createMapping(channelId: number, body: CreateMappingRequest): Promise<ModelMapping> {
    return request<ModelMapping>(`/admin/api/channels/${channelId}/mappings`, {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  updateMapping(id: number, body: CreateMappingRequest): Promise<ModelMapping> {
    return request<ModelMapping>(`/admin/api/mappings/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  deleteMapping(id: number): Promise<void> {
    return request<void>(`/admin/api/mappings/${id}`, {
      method: "DELETE",
    });
  },

  // ==================== API Key CRUD ====================

  getAPIKeys(): Promise<APIKey[]> {
    return request<APIKey[]>("/admin/api/api-keys");
  },

  getAPIKey(id: number): Promise<APIKey> {
    return request<APIKey>(`/admin/api/api-keys/${id}`);
  },

  createAPIKey(body: CreateAPIKeyRequest): Promise<APIKey> {
    return request<APIKey>("/admin/api/api-keys", {
      method: "POST",
      body: JSON.stringify(body),
    });
  },

  updateAPIKey(id: number, body: CreateAPIKeyRequest & { enabled: boolean }): Promise<APIKey> {
    return request<APIKey>(`/admin/api/api-keys/${id}`, {
      method: "PUT",
      body: JSON.stringify(body),
    });
  },

  deleteAPIKey(id: number): Promise<void> {
    return request<void>(`/admin/api/api-keys/${id}`, {
      method: "DELETE",
    });
  },

  toggleAPIKey(id: number, enabled: boolean): Promise<{ enabled: boolean }> {
    return request<{ enabled: boolean }>(`/admin/api/api-keys/${id}/toggle`, {
      method: "POST",
      body: JSON.stringify({ enabled }),
    });
  },

  // ==================== Usage Logs ====================

  getUsageLogs(apiKeyId: number, limit?: number, offset?: number): Promise<UsageLog[]> {
    const params = new URLSearchParams();
    if (limit) params.append("limit", limit.toString());
    if (offset) params.append("offset", offset.toString());
    const query = params.toString() ? `?${params.toString()}` : "";
    return request<UsageLog[]>(`/admin/api/api-keys/${apiKeyId}/logs${query}`);
  },

  getAPIKeyStats(apiKeyId: number): Promise<APIKeyStatsResponse> {
    return request<APIKeyStatsResponse>(`/admin/api/api-keys/${apiKeyId}/stats`);
  },

  // ==================== Error Logs ====================

  getErrorLogs(params: {
    limit?: number;
    offset?: number;
    start_date?: string;
    end_date?: string;
  }): Promise<{ logs: UsageLog[]; total: number }> {
    const searchParams = new URLSearchParams();
    if (params.limit) searchParams.append("limit", params.limit.toString());
    if (params.offset) searchParams.append("offset", params.offset.toString());
    if (params.start_date) searchParams.append("start_date", params.start_date);
    if (params.end_date) searchParams.append("end_date", params.end_date);
    
    const query = searchParams.toString() ? `?${searchParams.toString()}` : "";
    return request<{ logs: UsageLog[]; total: number }>(`/admin/api/error-logs${query}`);
  },

  // ==================== Global Stats ====================

  getGlobalStats(): Promise<GlobalStatsResponse> {
    return request<GlobalStatsResponse>("/admin/api/global-stats");
  },

  // ==================== Config Reload ====================

  reloadConfig(): Promise<{ message: string }> {
    return request<{ message: string }>("/admin/api/reload", {
      method: "POST",
    });
  },

  // ==================== Channel Health ====================

  resetChannelHealth(channelName: string): Promise<{ status: string; channel: string; message: string }> {
    return request<{ status: string; channel: string; message: string }>("/admin/channel-health", {
      method: "POST",
      body: JSON.stringify({ channel: channelName }),
    });
  },

  /** Verify the current admin key by calling a protected endpoint. */
  async verifyAuth(): Promise<boolean> {
    try {
      await request<Channel[]>("/admin/api/channels");
      return true;
    } catch (e) {
      if (e instanceof AuthError) return false;
      // Network errors etc — assume still logged in, let individual pages handle
      return true;
    }
  },
};

export function fetcher<T>(url: string): Promise<T> {
  return request<T>(url);
}
