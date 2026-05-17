import { apiClient } from "./client";

export async function listWorkflows() {
  const { data } = await apiClient.get("/flows/v2/workflows");
  return Array.isArray(data) ? data : (data?.workflows ?? []);
}

export async function listFlowTools() {
  const { data } = await apiClient.get("/flows/v2/tools");
  return Array.isArray(data) ? data : (data?.tools ?? []);
}
