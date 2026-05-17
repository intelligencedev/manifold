import { computed } from "vue";

export function useTrustBudget(quota: number, spent: number) {
  const remaining = computed(() => Math.max(quota - spent, 0));
  const ratio = computed(() => (quota > 0 ? spent / quota : 0));
  return { remaining, ratio };
}
