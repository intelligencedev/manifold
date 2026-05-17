import { computed, ref } from "vue";

export function useReplayCursor(total: () => number) {
  const index = ref(0);
  const max = computed(() => Math.max(total() - 1, 0));
  function next() { index.value = Math.min(index.value + 1, max.value); }
  function prev() { index.value = Math.max(index.value - 1, 0); }
  function set(value: number) { index.value = Math.max(0, Math.min(value, max.value)); }
  return { index, max, next, prev, set };
}
