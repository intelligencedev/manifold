import { onMounted, onUnmounted } from "vue";

export function useHotkeys(bindings: Record<string, () => void>) {
  const handler = (event: KeyboardEvent) => {
    const key = event.key.toLowerCase();
    const action = bindings[key];
    if (!action) return;
    event.preventDefault();
    action();
  };
  onMounted(() => window.addEventListener("keydown", handler));
  onUnmounted(() => window.removeEventListener("keydown", handler));
}
