import { apiClient } from "./client";

export async function fetchTrustBudgets() {
  const { data } = await apiClient.get("/trust/budgets");
  return data;
}

export async function refillTrustBudget(name: string, quota: number) {
  const { data } = await apiClient.post(`/trust/budgets/${encodeURIComponent(name)}/refill`, { quota });
  return data;
}
