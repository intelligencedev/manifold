import { defineStore } from "pinia";
import { ref } from "vue";
import { fetchRunTimeline } from "@/api/fleet";

export const useReplayStore = defineStore("cockpit-replay", () => {
  const runId = ref("");
  const status = ref("");
  const events = ref<unknown[]>([]);
  const loading = ref(false);
  const error = ref("");

  async function load(id: string) {
    runId.value = id;
    loading.value = true;
    error.value = "";
    try {
      const data = await fetchRunTimeline(id);
      status.value = data.status ?? "";
      events.value = data.events ?? [];
    } catch (err: unknown) {
      error.value = err instanceof Error ? err.message : "Failed to load timeline";
      events.value = [];
    } finally {
      loading.value = false;
    }
  }

  return { runId, status, events, loading, error, load };
});
