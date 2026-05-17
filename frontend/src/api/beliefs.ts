import { apiClient } from "./client";

export async function searchBeliefs(q = "", limit = 50) {
  const { data } = await apiClient.get("/debug/beliefs/search", { params: { q, limit } });
  return data;
}

export async function retractBelief(id: string, reason: string) {
  const { data } = await apiClient.post(`/debug/beliefs/${encodeURIComponent(id)}/retract`, { reason });
  return data;
}
