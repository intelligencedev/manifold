import { apiClient } from "./client";

export async function fetchMemorySessions() {
  const { data } = await apiClient.get("/debug/memory/sessions");
  return Array.isArray(data) ? data : [];
}
