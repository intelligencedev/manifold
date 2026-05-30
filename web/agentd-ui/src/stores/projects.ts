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
  const treeRequests = new Map<string, Promise<FileEntry[]>>();
  let projectsRequest: {
    includeUsage: boolean;
    promise: Promise<ProjectSummary[]>;
  } | null = null;
  let projectsRequestSequence = 0;

  function selectFallbackProject(projectList: ProjectSummary[]) {
    if (
      projectList.length &&
      !projectList.find((p) => p.id === currentProjectId.value)
    ) {
      currentProjectId.value = projectList[0].id;
    }
  }

  async function refresh(options: { includeUsage?: boolean } = {}) {
    const includeUsage = options.includeUsage ?? true;
    loading.value = true;
    error.value = null;
    if (!projectsRequest || (includeUsage && !projectsRequest.includeUsage)) {
      const sequence = projectsRequestSequence + 1;
      projectsRequestSequence = sequence;
      projectsRequest = {
        includeUsage,
        promise: listProjects({ includeUsage })
          .then((projectList) => {
            if (
              sequence === projectsRequestSequence ||
              !projects.value.length
            ) {
              projects.value = projectList;
              selectFallbackProject(projectList);
            }
            return projectList;
          })
          .finally(() => {
            if (sequence === projectsRequestSequence) {
              projectsRequest = null;
            }
          }),
      };
    }

    try {
      return await projectsRequest.promise;
    } catch (e) {
      error.value = "Failed to load projects";
      console.error(e);
      return projects.value;
    } finally {
      loading.value = false;
    }
  }

  async function ensureProjects() {
    if (projects.value.length) return projects.value;
    return refresh({ includeUsage: false });
  }

  async function setCurrent(id: string) {
    currentProjectId.value = id;
    // Persist to backend (triggers MCP session setup when auth is enabled)
    try {
      await setActiveProject(id);
    } catch (e) {
      // Non-fatal - local state is still updated
      console.warn("Failed to persist active project preference:", e);
      return;
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
      return;
    }
  }

  async function ensureTree(path = ".", options: { force?: boolean } = {}) {
    const id = currentProjectId.value;
    if (!id) return [];
    const clean = normalizePath(path);
    const key = `${id}:${clean}`;
    if (!options.force && treeByPath.value[key]) {
      return treeByPath.value[key];
    }
    let request = treeRequests.get(key);
    if (!request) {
      request = listProjectTree(id, clean)
        .then((entries) => {
          treeByPath.value[key] = entries;
          return entries;
        })
        .finally(() => {
          treeRequests.delete(key);
        });
      treeRequests.set(key, request);
    }
    return request;
  }

  async function makeDir(path: string) {
    if (!currentProjectId.value) return;
    await createDir(currentProjectId.value, path);
    await ensureTree(path.split("/").slice(0, -1).join("/") || ".", {
      force: true,
    });
  }

  async function removePath(path: string) {
    if (!currentProjectId.value) return;
    await deletePath(currentProjectId.value, path);
    await ensureTree(path.split("/").slice(0, -1).join("/") || ".", {
      force: true,
    });
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
    await ensureTree(srcParent, { force: true });
    if (destParent !== srcParent) {
      await ensureTree(destParent, { force: true });
    }
  }

  async function upload(path: string, file: File) {
    if (!currentProjectId.value) return;
    await uploadFile(currentProjectId.value, path, file);
    await ensureTree(path || ".", { force: true });
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
    await ensureTree(dir, { force: true });
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
    ensureProjects,
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
