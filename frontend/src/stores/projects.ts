import { defineStore } from "pinia";
import { ref } from "vue";
import { apiClient } from "@/api/client";

export interface CockpitProject {
  id: string;
  name?: string;
  createdAt?: string;
  updatedAt?: string;
  sizeBytes?: number;
  files?: number;
  fileCount?: number;
}

function normalizeProjectsPayload(data: unknown): CockpitProject[] {
  if (Array.isArray(data)) return data as CockpitProject[];
  if (data && typeof data === "object") {
    const projects = (data as { projects?: unknown }).projects;
    if (Array.isArray(projects)) return projects as CockpitProject[];
  }
  return [];
}

export const useProjectsStore = defineStore("cockpit-projects", () => {
  const projects = ref<CockpitProject[]>([]);

  async function refresh() {
    try {
      const { data } = await apiClient.get("/projects");
      projects.value = normalizeProjectsPayload(data);
    } catch {
      projects.value = [];
    }
  }

  return { projects, refresh };
});
