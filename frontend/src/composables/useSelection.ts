import { ref } from "vue";

export function useSelection<T = string>() {
  const selected = ref<T[]>([]);
  function toggle(item: T) {
    const idx = selected.value.findIndex((value) => value === item);
    if (idx >= 0) {
      selected.value.splice(idx, 1);
      return;
    }
    selected.value.push(item);
  }
  function clear() { selected.value = []; }
  return { selected, toggle, clear };
}
