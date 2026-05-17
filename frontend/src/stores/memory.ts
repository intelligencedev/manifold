import { defineStore } from "pinia";
import { ref } from "vue";
import { fetchMemorySessions } from "@/api/memory";
import { searchBeliefs } from "@/api/beliefs";
import { listTransitRecent } from "@/api/transit";

export const useMemoryStore = defineStore("cockpit-memory", () => {
  const sessions = ref<any[]>([]);
  const beliefs = ref<any[]>([]);
  const transit = ref<any[]>([]);

  async function refresh() {
    const [s, b, t] = await Promise.allSettled([
      fetchMemorySessions(),
      searchBeliefs(""),
      listTransitRecent(),
    ]);
    if (s.status === "fulfilled") sessions.value = Array.isArray(s.value) ? s.value : [];
    if (b.status === "fulfilled") beliefs.value = Array.isArray(b.value) ? b.value : [];
    if (t.status === "fulfilled") transit.value = Array.isArray(t.value) ? t.value : [];
  }

  return { sessions, beliefs, transit, refresh };
});
