// Legacy types (keep for backward compatibility)
export interface ChannelStats {
  type: string;
  weight: number;
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
  channel?: string;
}

export interface IssueTokenResponse {
  token: string;
}

// New database-driven types
export interface Channel {
  id: number;
  name: string;
  type: string;
  base_url: string;
  weight: number;
  timeout_seconds: number;
  initial_interval_seconds?: number;
  max_interval_seconds?: number;
  max_wait_time_seconds?: number;
  retry_attempts?: number;
  probe_model?: string;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  accounts?: Account[];
  model_mappings?: ModelMapping[];
}

export interface Account {
  id: number;
  channel_id: number;
  api_key: string;
  weight: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface ModelMapping {
  id: number;
  channel_id: number;
  source_model: string;
  target_model: string;
  created_at: string;
  updated_at: string;
}

export interface APIKey {
  id: number;
  key: string;
  name: string;
  channel_restriction?: string;
  enabled: boolean;
  expires_at?: string;
  last_used_at?: string;
  total_requests: number;
  total_success: number;
  total_fail: number;
  created_at: string;
  updated_at: string;
}

export interface UsageLog {
  id: number;
  api_key_id: number;
  channel_name: string;
  model: string;
  success: boolean;
  status_code?: number;
  error_message?: string;
  latency_ms?: number;
  request_ip?: string;
  created_at: string;
}

// Request types
export interface CreateChannelRequest {
  name: string;
  type: string;
  base_url: string;
  weight: number;
  timeout_seconds: number;
  initial_interval_seconds?: number;
  max_interval_seconds?: number;
  max_wait_time_seconds?: number;
  retry_attempts?: number;
  probe_model?: string;
  accounts: Array<{
    api_key: string;
    weight: number;
  }>;
  model_mappings: Array<{
    source_model: string;
    target_model: string;
  }>;
}

export interface CreateAccountRequest {
  api_key: string;
  weight: number;
}

export interface CreateMappingRequest {
  source_model: string;
  target_model: string;
}

export interface CreateAPIKeyRequest {
  name: string;
  channel_restriction?: string;
  expires_at?: string;
}

export interface APIKeyStatsResponse {
  [channelName: string]: {
    total_requests: number;
    total_success: number;
    total_fail: number;
    avg_latency_ms: number;
  };
}

// Global Stats
export interface OverviewStats {
  total_requests: number;
  total_success: number;
  total_fail: number;
  avg_latency_ms: number;
}

export interface ChannelTypeStats {
  gemini: OverviewStats;
  openai: OverviewStats;
  today: OverviewStats;
}

export interface ChannelDetailStats {
  channel_name: string;
  channel_type: string;
  display_name: string;
  total_requests: number;
  total_success: number;
  total_fail: number;
  success_rate: number;
  avg_latency_ms: number;
  health_score: number;
  is_healthy: boolean;
}

export interface GlobalStatsResponse {
  type_stats: ChannelTypeStats;
  channel_details: ChannelDetailStats[];
}

// Channel types
export type ChannelType =
  | "gemini_callback"
  | "gemini_openai"
  | "gemini_original"
  | "openai_original"
  | "openai_callback";

export const CHANNEL_TYPES: { value: ChannelType; label: string; description: string }[] = [
  {
    value: "gemini_callback",
    label: "Gemini Callback",
    description: "异步轮询方式的 Gemini API（原 kieai）",
  },
  {
    value: "gemini_openai",
    label: "Gemini OpenAI",
    description: "OpenAI 兼容的 Gemini API（原 subrouter）",
  },
  {
    value: "gemini_original",
    label: "Gemini Original",
    description: "Google 原生 Gemini API（原 gemini）",
  },
  {
    value: "openai_original",
    label: "OpenAI Original",
    description: "OpenAI 原生 API（原 gpt-image）",
  },
  {
    value: "openai_callback",
    label: "OpenAI Callback",
    description: "异步轮询方式的 OpenAI API（新增）",
  },
];
