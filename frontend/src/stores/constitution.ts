import { defineStore } from "pinia";
import { ref } from "vue";
import { activateConstitutionVersion, createConstitutionVersion, fetchConstitutionVersions } from "@/api/constitution";

export const useConstitutionStore = defineStore("cockpit-constitution", () => {
  const versions = ref<any[]>([]);
  async function refresh() { versions.value = await fetchConstitutionVersions(); }
  async function create(body: string) { await createConstitutionVersion(body); await refresh(); }
  async function activate(id: string) { await activateConstitutionVersion(id); await refresh(); }
  return { versions, refresh, create, activate };
});
