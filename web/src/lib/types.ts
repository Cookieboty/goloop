export interface ChannelStats {
  is_healthy: boolean;
  health_score: number;
  total_success: number;
  total_fail: number;
  avg_latency_ms: number;
}

export type StatsResponse = Record<string, ChannelStats>;

export interface AccountInfo {
  api_key: string;
  weight: number;
  status: "healthy" | "degraded" | "unhealthy" | "offline";
  usage_count: number;
  health_score: number;
  consecutive_failures: number;
  last_used?: string;
}

export interface ChannelAccountsResponse {
  channel: string;
  accounts: AccountInfo[];
}

export interface IssueTokenRequest {
  subject: string;
  api_key: string;
  channel?: string;
}

export interface IssueTokenResponse {
  token: string;
}

export interface ChannelInfo {
  name: string;
  type: string;
  weight: number;
  health_score: number;
  is_healthy: boolean;
  account_count: number;
  healthy_account_count: number;
}
