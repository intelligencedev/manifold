import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { fetchFleetState, subscribeFleetEvents, type FleetEvent, type FleetState } from "@/api/fleet";

export const useFleetStore = defineStore("cockpit-fleet", () => {
  const state = ref<FleetState | null>(null);
  const events = ref<FleetEvent[]>([]);
  const connected = ref(false);
  let source: EventSource | null = null;

  const runs = computed(() => state.value?.runs ?? []);
  const openRequests = computed(() => state.value?.open_input_requests ?? []);

  async function refresh() {
    state.value = await fetchFleetState();
    events.value = state.value.recent_events ?? [];
  }

  function start() {
    if (source) return;
    source = subscribeFleetEvents((event) => {
      connected.value = true;
      events.value = [...events.value.slice(-199), event];
    });
    source.onerror = () => {
      connected.value = false;
    };
  }

  function stop() {
    source?.close();
    source = null;
    connected.value = false;
  }

  return { state, events, runs, openRequests, connected, refresh, start, stop };
});
