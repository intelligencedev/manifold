import { apiClient } from "./client";

export async function fetchTokenMetrics(windowSeconds?: number) {
  const params: Record<string, string> = {};
  if (windowSeconds) params.windowSeconds = String(windowSeconds);
  const { data } = await apiClient.get("/metrics/tokens", { params });
  return data;
}

export async function fetchTraceMetrics(windowSeconds?: number, limit?: number) {
  const params: Record<string, string> = {};
  if (windowSeconds) params.windowSeconds = String(windowSeconds);
  if (limit) params.limit = String(limit);
  const { data } = await apiClient.get("/metrics/traces", { params });
  return data;
}

export async function fetchLogMetrics(windowSeconds?: number, limit?: number) {
  const params: Record<string, string> = {};
  if (windowSeconds) params.windowSeconds = String(windowSeconds);
  if (limit) params.limit = String(limit);
  const { data } = await apiClient.get("/metrics/logs", { params });
  return data;
}

export async function fetchRuns() {
  const { data } = await apiClient.get("/runs");
  return data;
}
