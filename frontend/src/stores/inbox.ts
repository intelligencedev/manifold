import { defineStore } from "pinia";
import { computed } from "vue";
import { useFleetStore } from "./fleet";

export const useInboxStore = defineStore("cockpit-inbox", () => {
  const fleet = useFleetStore();

  // Open input requests come from fleet state
  const requests = computed(() => fleet.state?.open_input_requests ?? []);

  return { requests };
});
