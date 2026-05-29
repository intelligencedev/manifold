import { apiClient } from "./clientCore";

export interface TokenMetricsRow {
  model: string;
  prompt: number;
  completion: number;
  total: number;
}

export interface TokenMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  models: TokenMetricsRow[];
}

export interface TokenMetricsParams {
  window?: string;
  windowSeconds?: number;
}

export async function fetchTokenMetrics(
  params?: TokenMetricsParams,
): Promise<TokenMetricsResponse> {
  const response = await apiClient.get<TokenMetricsResponse>(
    "/metrics/tokens",
    { params },
  );
  return response.data;
}

export interface MemoryMetricTotals {
  searches: number;
  hits: number;
  avgHitsPerSearch: number;
  evolves: number;
  evolveErrors: number;
  smartMerges: number;
  pruned: number;
}

export interface MemoryLatencyMetrics {
  avgMs?: number;
}

export interface MemorySizeMetric {
  user: string;
  session: string;
  size: number;
}

export interface MemoryReasonMetric {
  reason: string;
  count: number;
}

export interface MemoryResultMetric {
  result: string;
  count: number;
}

export interface MemoryMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  totals: MemoryMetricTotals;
  latency: MemoryLatencyMetrics;
  sizes: MemorySizeMetric[];
  prunedByReason: MemoryReasonMetric[];
  evolvesByResult: MemoryResultMetric[];
  warnings?: string[];
}

export interface MemoryMetricsParams {
  window?: string;
  windowSeconds?: number;
}

export async function fetchMemoryMetrics(
  params?: MemoryMetricsParams,
): Promise<MemoryMetricsResponse> {
  const response = await apiClient.get<MemoryMetricsResponse>(
    "/metrics/memory",
    { params },
  );
  return response.data;
}

export interface TraceMetricRow {
  traceId?: string;
  name: string;
  model?: string;
  status: string;
  durationMillis?: number;
  timestamp: number;
  promptTokens?: number;
  completionTokens?: number;
  totalTokens?: number;
}

export interface TraceMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  traces: TraceMetricRow[];
}

export interface TraceMetricsParams {
  window?: string;
  windowSeconds?: number;
  limit?: number;
}

export async function fetchTraceMetrics(
  params?: TraceMetricsParams,
): Promise<TraceMetricsResponse> {
  const response = await apiClient.get<TraceMetricsResponse>(
    "/metrics/traces",
    { params },
  );
  return response.data;
}

export interface LogMetricsRow {
  id: string;
  timestamp: number;
  level: string;
  message: string;
  service?: string;
  traceId?: string;
  spanId?: string;
  tags?: string[];
}

export interface LogMetricsResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  logs: LogMetricsRow[];
}

export interface LogMetricsParams {
  window?: string;
  windowSeconds?: number;
  limit?: number;
}

export async function fetchLogMetrics(
  params?: LogMetricsParams,
): Promise<LogMetricsResponse> {
  const response = await apiClient.get<LogMetricsResponse>("/metrics/logs", {
    params,
  });
  return response.data;
}

export interface LogDetail {
  id: string;
  timestamp: number;
  level: string;
  message: string;
  service?: string;
  traceId?: string;
  spanId?: string;
  tags?: string[];
  attributes?: Record<string, string>;
  resourceAttributes?: Record<string, string>;
}

export interface LogDetailResponse {
  timestamp: number;
  windowSeconds?: number;
  source?: string;
  log?: LogDetail;
}

export interface LogDetailParams {
  window?: string;
  windowSeconds?: number;
}

export async function fetchLogDetail(
  id: string,
  params?: LogDetailParams,
): Promise<LogDetailResponse> {
  const response = await apiClient.get<LogDetailResponse>(
    "/metrics/logs/detail",
    {
      params: {
        ...params,
        id,
      },
    },
  );
  return response.data;
}
