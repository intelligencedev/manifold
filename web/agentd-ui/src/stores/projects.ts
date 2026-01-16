import { defineStore } from "pinia";
import { ref } from "vue";
import type { ProjectSummary, FileEntry } from "@/api/client";
import {
  listProjects,
  createProject,
  deleteProject,
  listProjectTree,
  uploadFile,
  deletePath,
  createDir,
  moveProjectPath,
  fetchProjectFileText,
  saveProjectFileText,
  setActiveProject,
  getUserPreferences,
} from "@/api/client";

export const useProjectsStore = defineStore("projects", () => {
  const projects = ref<ProjectSummary[]>([]);
  const loading = ref(false);
  const error = ref<string | null>(null);
  const currentProjectId = ref<string>("");
  const treeByPath = ref<Record<string, FileEntry[]>>({});

  async function refresh() {
    loading.value = true;
    error.value = null;
    try {
      projects.value = await listProjects();
      if (
        projects.value.length &&
        !projects.value.find((p) => p.id === currentProjectId.value)
      ) {
        currentProjectId.value = projects.value[0].id;
      }
    } catch (e) {
      error.value = "Failed to load projects";
      console.error(e);
    } finally {
      loading.value = false;
    }
  }

  async function setCurrent(id: string) {
    currentProjectId.value = id;
    // Persist to backend (triggers MCP session setup when auth is enabled)
    try {
      await setActiveProject(id);
    } catch (e) {
      // Non-fatal - local state is still updated
      console.warn("Failed to persist active project preference:", e);
    }
  }

  // Initialize from user preferences (called on app mount)
  async function initFromPreferences() {
    try {
      const prefs = await getUserPreferences();
      if (prefs?.activeProjectId) {
        // Only set if the project exists in our list
        const exists = projects.value.find(
          (p) => p.id === prefs.activeProjectId,
        );
        if (exists) {
          currentProjectId.value = prefs.activeProjectId;
        }
      }
    } catch (e) {
      // Non-fatal - will use default project selection
      console.warn("Failed to load user preferences:", e);
    }
  }

  async function ensureTree(path = ".") {
    const id = currentProjectId.value;
    if (!id) return;
    const key = `${id}:${path}`;
    treeByPath.value[key] = await listProjectTree(id, path);
  }

  async function makeDir(path: string) {
    if (!currentProjectId.value) return;
    await createDir(currentProjectId.value, path);
    await ensureTree(path.split("/").slice(0, -1).join("/") || ".");
  }

  async function removePath(path: string) {
    if (!currentProjectId.value) return;
    await deletePath(currentProjectId.value, path);
    await ensureTree(path.split("/").slice(0, -1).join("/") || ".");
  }

  function normalizePath(path: string) {
    const trimmed = path.trim();
    if (!trimmed || trimmed === ".") return ".";
    const withoutLeading = trimmed.replace(/^\.?\/+/, "");
    const collapsed = withoutLeading.replace(/\/{2,}/g, "/");
    const withoutTrailing = collapsed.replace(/\/+$/, "");
    return withoutTrailing || ".";
  }

  function invalidateCachedSubtree(projectID: string, prefix: string) {
    if (!prefix) return;
    const cleanPrefix = normalizePath(prefix);
    const keyPrefix = `${projectID}:${cleanPrefix}`;
    for (const key of Object.keys(treeByPath.value)) {
      if (key === keyPrefix || key.startsWith(`${keyPrefix}/`)) {
        delete treeByPath.value[key];
      }
    }
  }

  function parentPath(path: string) {
    const clean = normalizePath(path);
    if (clean === "." || clean === "") return ".";
    const idx = clean.lastIndexOf("/");
    if (idx === -1) return ".";
    const parent = clean.slice(0, idx);
    return parent || ".";
  }

  function fileName(path: string) {
    const clean = normalizePath(path);
    if (clean === "." || clean === "") return "";
    const idx = clean.lastIndexOf("/");
    if (idx === -1) return clean;
    return clean.slice(idx + 1);
  }

  async function movePath(from: string, to: string) {
    if (!currentProjectId.value) return;
    const projectID = currentProjectId.value;
    const src = normalizePath(from);
    const dest = normalizePath(to);
    if (!src || src === "." || !dest || dest === ".") return;
    if (src === dest) return;
    await moveProjectPath(projectID, src, dest);
    const srcParent = parentPath(src);
    const destParent = parentPath(dest);
    invalidateCachedSubtree(projectID, src);
    await ensureTree(srcParent);
    if (destParent !== srcParent) {
      await ensureTree(destParent);
    }
  }

  async function upload(path: string, file: File) {
    if (!currentProjectId.value) return;
    await uploadFile(currentProjectId.value, path, file);
    await ensureTree(path || ".");
  }

  async function readTextFile(path: string) {
    if (!currentProjectId.value) return "";
    const clean = normalizePath(path);
    return fetchProjectFileText(currentProjectId.value, clean);
  }

  async function writeTextFile(path: string, content: string) {
    if (!currentProjectId.value) return;
    const clean = normalizePath(path);
    const name = fileName(clean);
    if (!name) return;
    const dir = parentPath(clean);
    await saveProjectFileText(currentProjectId.value, dir, name, content);
    await ensureTree(dir);
  }

  async function create(name: string) {
    const p = await createProject(name);
    projects.value = [p, ...projects.value];
    // Set as current and persist preference
    await setCurrent(p.id);
  }

  async function remove(id: string) {
    await deleteProject(id);
    projects.value = projects.value.filter((p) => p.id !== id);
    if (currentProjectId.value === id) {
      const nextProject = projects.value[0]?.id || "";
      if (nextProject) {
        await setCurrent(nextProject);
      } else {
        currentProjectId.value = "";
      }
    }
  }

  return {
    // state
    projects,
    loading,
    error,
    currentProjectId,
    treeByPath,
    // actions
    refresh,
    setCurrent,
    ensureTree,
    makeDir,
    removePath,
    movePath,
    upload,
    readTextFile,
    writeTextFile,
    create,
    remove,
    initFromPreferences,
  };
});
