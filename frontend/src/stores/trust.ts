import { defineStore } from "pinia";
import { ref } from "vue";
import { fetchTrustBudgets, refillTrustBudget } from "@/api/trust";

export const useTrustStore = defineStore("cockpit-trust", () => {
  const budgets = ref<any[]>([]);
  async function refresh() { budgets.value = await fetchTrustBudgets(); }
  async function refill(name: string, quota: number) { await refillTrustBudget(name, quota); await refresh(); }
  return { budgets, refresh, refill };
});
