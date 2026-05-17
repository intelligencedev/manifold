import { onMounted, onUnmounted } from "vue";
import { useFleetStore } from "@/stores/fleet";

export function useFleetEvents() {
  const fleet = useFleetStore();
  onMounted(async () => {
    await fleet.refresh().catch(() => {});
    fleet.start();
  });
  onUnmounted(() => fleet.stop());
  return { refresh: () => fleet.refresh(), fleet };
}
