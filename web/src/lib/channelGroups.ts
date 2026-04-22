import type { ChannelStats } from "./types";

export type ChannelEntry = [string, ChannelStats];

export interface ChannelTypeInfo {
  id: string;
  label: string;
  desc: string;
  color: string;
}

export interface ApiProtocol {
  id: string;
  label: string;
  icon: string;
  color: string;
  desc: string;
  types: ChannelTypeInfo[];
}

export const API_PROTOCOLS: ApiProtocol[] = [
  {
    id: "gemini-api",
    label: "Gemini API",
    icon: "🔷",
    color: "var(--blue)",
    desc: "面向 Google Gemini API 的渠道，通过 /v1beta/** 端点提供服务",
    types: [
      {
        id: "gemini",
        label: "Gemini 渠道",
        desc: "Google 原生格式透传",
        color: "var(--blue)",
      },
      {
        id: "kieai",
        label: "KIE.AI 渠道",
        desc: "异步任务模式",
        color: "var(--purple)",
      },
      {
        id: "subrouter",
        label: "Subrouter 渠道",
        desc: "Google → OpenAI 格式转换",
        color: "var(--orange)",
      },
    ],
  },
  {
    id: "openai-api",
    label: "OpenAI API",
    icon: "🎨",
    color: "var(--cyan)",
    desc: "面向 OpenAI 兼容 API 的渠道，通过 /v1/** 端点提供服务",
    types: [
      {
        id: "gpt-image",
        label: "GPT-Image 渠道",
        desc: "OpenAI 完全透传模式",
        color: "var(--cyan)",
      },
    ],
  },
];

const TYPE_BY_ID: Record<string, ChannelTypeInfo> = {};
const PROTOCOL_BY_TYPE: Record<string, ApiProtocol> = {};
for (const p of API_PROTOCOLS) {
  for (const t of p.types) {
    TYPE_BY_ID[t.id] = t;
    PROTOCOL_BY_TYPE[t.id] = p;
  }
}

export function getTypeInfo(typeId: string): ChannelTypeInfo | undefined {
  return TYPE_BY_ID[typeId];
}

export function getProtocolForType(typeId: string): ApiProtocol | undefined {
  return PROTOCOL_BY_TYPE[typeId];
}

export interface ProtocolSummary {
  protocol: ApiProtocol;
  entries: ChannelEntry[];
  byType: Record<string, ChannelEntry[]>;
  totalChannels: number;
  healthyChannels: number;
  totalSuccess: number;
  totalFail: number;
  avgLatencyMs: number;
}

export function groupByProtocol(entries: ChannelEntry[]): ProtocolSummary[] {
  return API_PROTOCOLS.map((protocol) => {
    const protocolEntries: ChannelEntry[] = [];
    const byType: Record<string, ChannelEntry[]> = {};
    for (const t of protocol.types) byType[t.id] = [];

    for (const entry of entries) {
      const type = entry[1].type || "";
      if (byType[type]) {
        byType[type].push(entry);
        protocolEntries.push(entry);
      }
    }

    const totalChannels = protocolEntries.length;
    const healthyChannels = protocolEntries.filter(
      ([, s]) => s.is_healthy
    ).length;
    const totalSuccess = protocolEntries.reduce(
      (sum, [, s]) => sum + s.total_success,
      0
    );
    const totalFail = protocolEntries.reduce(
      (sum, [, s]) => sum + s.total_fail,
      0
    );
    const avgLatencyMs =
      totalChannels > 0
        ? Math.round(
            protocolEntries.reduce((sum, [, s]) => sum + s.avg_latency_ms, 0) /
              totalChannels
          )
        : 0;

    return {
      protocol,
      entries: protocolEntries,
      byType,
      totalChannels,
      healthyChannels,
      totalSuccess,
      totalFail,
      avgLatencyMs,
    };
  });
}

export function getUnknownEntries(entries: ChannelEntry[]): ChannelEntry[] {
  return entries.filter(([, s]) => !PROTOCOL_BY_TYPE[s.type || ""]);
}
