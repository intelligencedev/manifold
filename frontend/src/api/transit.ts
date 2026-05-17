import { apiClient } from "./client";

export async function listTransitRecent(limit = 50) {
  const { data } = await apiClient.get("/transit/recent", { params: { limit } });
  return data;
}

export async function searchTransit(q: string) {
  const { data } = await apiClient.get("/transit/search", { params: { query: q } });
  return data;
}
