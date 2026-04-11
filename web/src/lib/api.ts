import type {
  StatsResponse,
  ChannelAccountsResponse,
  IssueTokenRequest,
  IssueTokenResponse,
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

  resetAccount(channel: string, accountId: string): Promise<void> {
    return request<void>(
      `/admin/channel/${channel}/accounts/${accountId}/reset`,
      { method: "POST" }
    );
  },

  retireAccount(channel: string, accountId: string): Promise<void> {
    return request<void>(
      `/admin/channel/${channel}/accounts/${accountId}/retire`,
      { method: "POST" }
    );
  },

  probeAccount(channel: string, accountId: string): Promise<void> {
    return request<void>(
      `/admin/channel/${channel}/accounts/${accountId}/probe`,
      { method: "POST" }
    );
  },

  updateAccountWeight(channel: string, accountId: string, weight: number): Promise<void> {
    return request<void>(
      `/admin/channel/${channel}/accounts/${accountId}/weight`,
      { method: "POST", body: JSON.stringify({ weight }) }
    );
  },

  updateChannelWeight(channel: string, weight: number): Promise<void> {
    return request<void>("/admin/channel-weight", {
      method: "POST",
      body: JSON.stringify({ channel, weight }),
    });
  },

  /** Verify the current admin key by calling a protected endpoint. */
  async verifyAuth(): Promise<boolean> {
    try {
      await request<unknown>("/admin/stats");
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
