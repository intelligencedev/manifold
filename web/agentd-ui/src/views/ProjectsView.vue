<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import DOMPurify from "dompurify";

import { useProjectsStore } from "@/stores/projects";
import { projectFileUrl, projectArchiveUrl } from "@/api/client";
import Panel from "@/components/ui/Panel.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import Pill from "@/components/ui/Pill.vue";
import AppButton from "@/components/ui/AppButton.vue";
import MSegmented from "@/components/ui/MSegmented.vue";
import FileTree from "@/components/FileTree.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import { renderMarkdown } from "@/utils/markdown";
import type { DropdownOption } from "@/types/dropdown";

const fieldClass =
  "halo-focus rounded-md border border-[rgb(var(--line-strong))] bg-surface px-3 py-2 text-sm text-foreground outline-none placeholder:text-faint-foreground";

const store = useProjectsStore();
const newProjectName = ref("");
const newProjectInput = ref<HTMLInputElement | null>(null);
const uploadInput = ref<HTMLInputElement | null>(null);
const treeRef = ref<InstanceType<typeof FileTree> | null>(null);
const splitPaneRef = ref<HTMLElement | null>(null);
const markdownPreviewRef = ref<HTMLElement | null>(null);
const cwd = ref(".");
const selectedFile = ref<string>("");
const editorContent = ref("");
const editorDirty = ref(false);
const editorLoading = ref(false);
const editorSaving = ref(false);
const editorError = ref("");
const previewMode = ref<"raw" | "markdown">("raw");
const leftPaneWidth = ref(50);
const isResizingPanes = ref(false);
const allowedTextExtensions = [
  ".txt",
  ".md",
  ".log",
  ".json",
  ".js",
  ".ts",
  ".go",
  ".py",
  ".java",
  ".c",
  ".cpp",
  ".yml",
  ".yaml",
  ".toml",
  ".ini",
  ".sh",
  ".csv",
];
const isTextFile = computed(() => isTextFilePath(selectedFile.value));
const isMarkdownFile = computed(() =>
  /\.(md|markdown)$/i.test(selectedFile.value),
);
const renderedMarkdown = computed(() =>
  renderMarkdown(editorContent.value, { mermaid: true }),
);
const leftPaneStyle = computed(() => ({
  flexBasis: `${leftPaneWidth.value}%`,
}));
const rightPaneStyle = computed(() => ({
  flexBasis: `${100 - leftPaneWidth.value}%`,
}));
const previewUrl = computed(() => {
  if (!store.currentProjectId || !selectedFile.value) return "";
  return projectFileUrl(store.currentProjectId, selectedFile.value);
});
let mermaidLoad: Promise<typeof import("mermaid").default> | null = null;
let mermaidRenderRun = 0;

function loadMermaid() {
  if (!mermaidLoad) {
    mermaidLoad = import("mermaid").then(({ default: mermaid }) => mermaid);
  }
  return mermaidLoad;
}

function getThemeColor(
  styles: CSSStyleDeclaration,
  name: string,
  fallback: string,
) {
  const value = styles.getPropertyValue(name).trim();
  return value ? `rgb(${value})` : fallback;
}

function initializeMermaid(mermaid: typeof import("mermaid").default) {
  const styles = getComputedStyle(document.documentElement);
  const surface = getThemeColor(styles, "--color-surface", "#ffffff");
  const surfaceMuted = getThemeColor(
    styles,
    "--color-surface-muted",
    "#f6f7f9",
  );
  const foreground = getThemeColor(styles, "--color-foreground", "#111827");
  const border = getThemeColor(styles, "--color-border", "#9ca3af");
  const accent = getThemeColor(styles, "--color-accent", "#2563eb");

  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict",
    theme: "base",
    htmlLabels: false,
    themeVariables: {
      background: surface,
      mainBkg: surfaceMuted,
      nodeBkg: surfaceMuted,
      primaryColor: surfaceMuted,
      primaryTextColor: foreground,
      nodeTextColor: foreground,
      primaryBorderColor: border,
      lineColor: border,
      textColor: foreground,
      titleColor: foreground,
      edgeLabelBackground: surface,
      clusterBkg: surface,
      clusterBorder: border,
      secondaryColor: surface,
      secondaryTextColor: foreground,
      secondaryBorderColor: border,
      tertiaryColor: surface,
      tertiaryTextColor: foreground,
      tertiaryBorderColor: border,
      noteBkgColor: surfaceMuted,
      noteTextColor: foreground,
      noteBorderColor: accent,
      fontFamily:
        "Hanken Grotesk, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, sans-serif",
    },
    flowchart: {
      htmlLabels: false,
    },
  });
}

function renderMermaidError(container: HTMLElement, source: string) {
  container.classList.remove("md-mermaid-loading");
  container.classList.add("md-mermaid-error");
  const message = document.createElement("div");
  message.className = "md-mermaid-error-message";
  message.textContent = "Unable to render Mermaid diagram.";
  const pre = document.createElement("pre");
  pre.className = "md-mermaid-source";
  const code = document.createElement("code");
  code.textContent = source;
  pre.appendChild(code);
  container.replaceChildren(message, pre);
}

async function renderMermaidDiagrams() {
  const run = ++mermaidRenderRun;
  if (!isMarkdownFile.value || previewMode.value !== "markdown") return;

  await nextTick();
  const root = markdownPreviewRef.value;
  if (!root || run !== mermaidRenderRun) return;

  const diagrams = Array.from(
    root.querySelectorAll<HTMLElement>('[data-mermaid-diagram="true"]'),
  );
  if (!diagrams.length) return;

  diagrams.forEach((container) => {
    container.classList.add("md-mermaid-loading");
  });

  let mermaid: typeof import("mermaid").default;
  try {
    mermaid = await loadMermaid();
  } catch (error) {
    console.error("failed to load mermaid", error);
    diagrams.forEach((container) => {
      const source =
        container.querySelector(".md-mermaid-source")?.textContent ?? "";
      renderMermaidError(container, source);
    });
    return;
  }

  initializeMermaid(mermaid);

  for (const [index, container] of diagrams.entries()) {
    if (run !== mermaidRenderRun || !root.contains(container)) return;
    const source =
      container.querySelector(".md-mermaid-source")?.textContent ?? "";
    if (!source.trim()) continue;

    try {
      const { svg } = await mermaid.render(
        `project-mermaid-${run}-${index}`,
        source,
      );
      if (run !== mermaidRenderRun || !root.contains(container)) return;
      container.classList.remove("md-mermaid-loading");
      container.classList.add("md-mermaid-rendered");
      container.innerHTML = DOMPurify.sanitize(svg, {
        USE_PROFILES: { svg: true, svgFilters: true },
      });
    } catch (error) {
      console.warn("mermaid render failed", error);
      if (run !== mermaidRenderRun || !root.contains(container)) return;
      renderMermaidError(container, source);
    }
  }
}

onMounted(() => {
  void store.ensureProjects().then(async () => {
    await store.ensureTree(cwd.value);
    void store.refresh({ includeUsage: true });
  });
});

onBeforeUnmount(() => {
  mermaidRenderRun += 1;
  stopPaneResize();
});

const current = computed(
  () => store.projects.find((p) => p.id === store.currentProjectId) || null,
);
const entries = computed(
  () => store.treeByPath[`${store.currentProjectId}:${cwd.value}`] || [],
);

const projectOptions = computed<DropdownOption[]>(() =>
  store.projects.map((p) => ({
    id: p.id,
    label: p.name,
    value: p.id,
  })),
);

const selectedProjectId = computed({
  get: () => store.currentProjectId || "",
  set: (v: string) => {
    void store.setCurrent(v);
  },
});
const selectedCount = computed(() => treeRef.value?.checked?.size ?? 0);
const canDeleteSelectedItems = computed(() => selectedCount.value > 0);

const showDeleteProjectDialog = ref(false);
const deleteProjectTargetId = ref("");
const deleteProjectTargetName = ref("");
const deleteProjectTypedName = ref("");
const deleteProjectAcknowledged = ref(false);
const deleteProjectPending = ref(false);
const deleteProjectError = ref("");
const canConfirmDeleteProject = computed(
  () =>
    !!deleteProjectTargetId.value &&
    deleteProjectTypedName.value.trim() === deleteProjectTargetName.value &&
    deleteProjectAcknowledged.value &&
    !deleteProjectPending.value,
);

function resetDeleteProjectDialogState() {
  deleteProjectTargetId.value = "";
  deleteProjectTargetName.value = "";
  deleteProjectTypedName.value = "";
  deleteProjectAcknowledged.value = false;
  deleteProjectPending.value = false;
  deleteProjectError.value = "";
}

function openDeleteProjectDialog() {
  if (!current.value?.id) return;
  deleteProjectTargetId.value = current.value.id;
  deleteProjectTargetName.value = current.value.name;
  deleteProjectTypedName.value = "";
  deleteProjectAcknowledged.value = false;
  deleteProjectPending.value = false;
  deleteProjectError.value = "";
  showDeleteProjectDialog.value = true;
}

function closeDeleteProjectDialog() {
  if (deleteProjectPending.value) return;
  showDeleteProjectDialog.value = false;
  resetDeleteProjectDialogState();
}

async function confirmDeleteProject() {
  const projectID = deleteProjectTargetId.value;
  if (!projectID || !canConfirmDeleteProject.value) return;
  deleteProjectPending.value = true;
  deleteProjectError.value = "";
  try {
    await store.remove(projectID);
    showDeleteProjectDialog.value = false;
    resetDeleteProjectDialogState();
  } catch (e) {
    console.error(e);
    deleteProjectError.value = "Failed to delete project.";
  } finally {
    deleteProjectPending.value = false;
  }
}

function pickUpload() {
  uploadInput.value?.click();
}

function isTextFilePath(path: string) {
  if (!path) return false;
  const lower = path.toLowerCase();
  return allowedTextExtensions.some((ext) => lower.endsWith(ext));
}

async function onFiles(e: Event) {
  const input = e.target as HTMLInputElement;
  const files = input.files;
  if (!files || !files.length) return;
  for (const f of Array.from(files)) {
    await store.upload(cwd.value, f);
  }
  input.value = "";
}

async function mkdir() {
  const name = prompt("Folder name?");
  if (!name) return;
  const path = (cwd.value === "." ? "" : cwd.value + "/") + name;
  await store.makeDir(path);
  await store.ensureTree(cwd.value);
}

async function createFile() {
  const name = prompt(
    "New file name? (Text files only, e.g. main.go or notes.txt)",
  );
  if (!name) return;
  if (name.includes("/") || name.includes("\\")) {
    alert("Please provide a file name without path separators.");
    return;
  }
  if (!isTextFilePath(name)) {
    alert(
      `Unsupported file type. Allowed: ${allowedTextExtensions.join(", ")}`,
    );
    return;
  }
  const path = (cwd.value === "." ? "" : cwd.value + "/") + name;
  await store.writeTextFile(path, "");
  await store.ensureTree(cwd.value);
  selectedFile.value = path;
}

function findCheckedEntry(path: string) {
  const projectID = store.currentProjectId;
  if (!projectID) return null;
  const prefix = `${projectID}:`;
  for (const [key, entries] of Object.entries(store.treeByPath)) {
    if (!key.startsWith(prefix)) continue;
    const match = entries.find((entry) => entry.path === path);
    if (match) return match;
  }
  return null;
}

async function bulkDownload() {
  const projectID = store.currentProjectId;
  const ids = Array.from(treeRef.value?.checked ?? new Set<string>());
  if (!ids.length || !projectID) return;

  for (const path of ids) {
    const entry = findCheckedEntry(path);
    const isDir = entry?.isDir ?? false;
    const url = isDir
      ? projectArchiveUrl(projectID, path)
      : projectFileUrl(projectID, path);
    const baseName = path.split("/").pop() || (isDir ? "folder" : "download");
    const a = document.createElement("a");
    a.href = url;
    a.download = isDir ? `${baseName}.tar.gz` : baseName;
    a.style.display = "none";
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    // Small delay between downloads to avoid browser blocking
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
}

async function bulkDelete() {
  const ids = Array.from(treeRef.value?.checked ?? new Set<string>());
  if (!ids.length) return;
  if (!confirm(`Delete ${ids.length} item(s)? This cannot be undone.`)) return;
  for (const p of ids) {
    await store.removePath(p);
    if (selectedFile.value === p) selectedFile.value = "";
  }
  treeRef.value?.clearChecks();
  await store.ensureTree(cwd.value);
}

async function openDir(path: string) {
  cwd.value = path || ".";
  await store.ensureTree(cwd.value);
  selectedFile.value = "";
}

function downloadProject() {
  if (!store.currentProjectId) return;
  const url = projectArchiveUrl(store.currentProjectId);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${current.value?.name || "project"}.tar.gz`;
  a.style.display = "none";
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
}

async function createProject() {
  const name = newProjectName.value.trim();
  if (!name) return;
  await store.create(name);
  newProjectName.value = "";
  cwd.value = ".";
  await store.ensureTree(".");
}

function focusNewProject() {
  newProjectInput.value?.focus();
}

function openFile(path: string) {
  selectedFile.value = path;
}

async function loadEditorFile(path: string) {
  editorError.value = "";
  editorDirty.value = false;
  editorContent.value = "";
  if (!path || !isTextFilePath(path)) return;
  editorLoading.value = true;
  try {
    editorContent.value = await store.readTextFile(path);
  } catch (e) {
    console.error(e);
    editorError.value = "Failed to load file.";
  } finally {
    editorLoading.value = false;
  }
}

function onEditorKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") {
    event.preventDefault();
    if (editorDirty.value && !editorSaving.value && !editorLoading.value) {
      void saveEditor();
    }
  }
}

async function saveEditor() {
  if (!selectedFile.value || !isTextFilePath(selectedFile.value)) return;
  editorSaving.value = true;
  editorError.value = "";
  try {
    await store.writeTextFile(selectedFile.value, editorContent.value);
    editorDirty.value = false;
  } catch (e) {
    console.error(e);
    editorError.value = "Failed to save file.";
  } finally {
    editorSaving.value = false;
  }
}

watch(
  () => store.currentProjectId,
  () => {
    cwd.value = ".";
    selectedFile.value = "";
    editorContent.value = "";
    editorDirty.value = false;
    editorError.value = "";
    void store.ensureTree(".");
  },
);

watch(
  () => selectedFile.value,
  (path) => {
    previewMode.value = "raw";
    void loadEditorFile(path);
  },
);

watch(
  [renderedMarkdown, previewMode],
  () => {
    void renderMermaidDiagrams();
  },
  { flush: "post" },
);

function rebasePath(current: string, from: string, to: string) {
  if (!current || current === ".") return current;
  if (current === from) return to;
  if (current.startsWith(`${from}/`)) {
    const suffix = current.slice(from.length + 1);
    return suffix ? `${to}/${suffix}` : to;
  }
  return current;
}

function onMoved(payload: { from: string; to: string }) {
  const nextSelected = rebasePath(selectedFile.value, payload.from, payload.to);
  if (nextSelected !== selectedFile.value) {
    selectedFile.value = nextSelected;
  }
  const nextCwd = rebasePath(cwd.value, payload.from, payload.to);
  if (nextCwd !== cwd.value) {
    cwd.value = nextCwd;
  }
  // Ensure current directory reflects latest tree after a move.
  void store.ensureTree(cwd.value);
}

function updatePaneWidth(clientX: number) {
  const container = splitPaneRef.value;
  if (!container) return;
  const bounds = container.getBoundingClientRect();
  if (!bounds.width) return;
  const nextPercent = ((clientX - bounds.left) / bounds.width) * 100;
  leftPaneWidth.value = Math.min(70, Math.max(25, nextPercent));
}

function onPaneResize(event: PointerEvent) {
  updatePaneWidth(event.clientX);
}

function stopPaneResize() {
  if (!isResizingPanes.value) return;
  isResizingPanes.value = false;
  window.removeEventListener("pointermove", onPaneResize);
  window.removeEventListener("pointerup", stopPaneResize);
  document.body.classList.remove("projects-resizing");
}

function startPaneResize(event: PointerEvent) {
  if (window.innerWidth < 1024) return;
  isResizingPanes.value = true;
  updatePaneWidth(event.clientX);
  window.addEventListener("pointermove", onPaneResize);
  window.addEventListener("pointerup", stopPaneResize);
  document.body.classList.add("projects-resizing");
}
</script>

<template>
  <section class="flex h-full min-h-0 flex-col gap-3 overflow-hidden">
    <Panel
      flat
      :padded="false"
      class="halo-hairline-b shrink-0 pb-3"
    >
      <div class="flex flex-wrap items-center gap-3">
        <!-- Current project + its actions -->
        <div class="flex items-center gap-2">
          <DropdownSelect
            id="project-select"
            v-model="selectedProjectId"
            :options="projectOptions"
            size="sm"
            aria-label="Project"
            title="Current project"
          />
          <AppButton
            v-if="store.currentProjectId"
            variant="neutral"
            size="sm"
            title="Download project as .tar.gz"
            aria-label="Download project"
            @click="downloadProject"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M12 3v11m0 0 4-4m-4 4-4-4" />
              <path d="M4 17v2a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-2" />
            </svg>
          </AppButton>
          <AppButton
            variant="danger"
            size="sm"
            :disabled="!current"
            title="Delete current project"
            aria-label="Delete current project"
            @click="openDeleteProjectDialog"
          >
            <SolarTrashIcon class="h-4 w-4" />
          </AppButton>
        </div>

        <!-- Project meta -->
        <div
          v-if="current"
          class="flex items-center gap-2 text-xs text-faint-foreground"
        >
          <span>Created {{ new Date(current.createdAt).toLocaleDateString() }}</span>
          <Pill v-if="current.usageLoaded" tone="neutral" size="sm">{{ current.files }} files</Pill>
          <Pill v-if="current.usageLoaded" tone="neutral" size="sm">{{ (current.sizeBytes / 1024).toFixed(1) }} KB</Pill>
        </div>

        <!-- New project -->
        <div class="ml-auto flex items-center gap-2">
          <label class="sr-only" for="new-project">New project name</label>
          <input
            id="new-project"
            ref="newProjectInput"
            v-model="newProjectName"
            placeholder="New project name"
            :class="[fieldClass, 'h-9 w-44']"
            @keydown.enter="createProject"
          />
          <AppButton
            variant="accent"
            size="sm"
            :disabled="!newProjectName.trim()"
            title="Create project"
            aria-label="Create project"
            @click="createProject"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" aria-hidden="true">
              <path d="M12 5v14M5 12h14" />
            </svg>
          </AppButton>
        </div>
      </div>
    </Panel>

    <div
      v-if="store.currentProjectId"
      ref="splitPaneRef"
      class="flex min-h-0 flex-1 flex-row items-stretch gap-0 overflow-hidden"
    >
      <GlassCard
        flat
        class="flex h-full min-h-0 min-w-0 shrink-0 flex-col p-6 pr-6"
        :style="leftPaneStyle"
      >
        <div class="mb-4 flex items-center gap-2">
          <AppButton
            variant="ghost"
            size="sm"
            title="Go to project root"
            aria-label="Go to project root"
            @click="() => openDir('.')"
          >
            <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M3 11.5 12 4l9 7.5" />
              <path d="M5 10v9a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1v-9" />
            </svg>
          </AppButton>
          <div class="min-w-0 flex-1 truncate text-sm text-faint-foreground">{{ cwd }}</div>

          <div class="flex items-center gap-2">
            <!-- Selection actions (contextual) -->
            <template v-if="canDeleteSelectedItems">
              <span class="whitespace-nowrap text-xs text-subtle-foreground">{{ selectedCount }} selected</span>
              <AppButton
                variant="neutral"
                size="sm"
                title="Download selected"
                aria-label="Download selected"
                @click="bulkDownload"
              >
                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                  <path d="M12 3v11m0 0 4-4m-4 4-4-4" />
                  <path d="M4 17v2a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-2" />
                </svg>
              </AppButton>
              <AppButton
                variant="danger"
                size="sm"
                title="Delete selected"
                aria-label="Delete selected"
                @click="bulkDelete"
              >
                <SolarTrashIcon class="h-4 w-4" />
              </AppButton>
              <div class="mx-1 h-5 w-px bg-border"></div>
            </template>

            <!-- Create / upload -->
            <AppButton variant="neutral" size="sm" title="New folder" aria-label="New folder" @click="mkdir">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M3 7a1 1 0 0 1 1-1h5l2 2h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />
                <path d="M12 11v5M9.5 13.5h5" />
              </svg>
            </AppButton>
            <AppButton variant="neutral" size="sm" title="New file" aria-label="New file" @click="createFile">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M14 4H7a1 1 0 0 0-1 1v14a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V8z" />
                <path d="M14 4v4h4" />
                <path d="M12 12v5M9.5 14.5h5" />
              </svg>
            </AppButton>
            <AppButton variant="neutral" size="sm" title="Upload files" aria-label="Upload files" @click="pickUpload">
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M12 16V5m0 0 4 4m-4-4-4 4" />
                <path d="M4 17v2a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-2" />
              </svg>
            </AppButton>
            <input
              ref="uploadInput"
              type="file"
              multiple
              class="sr-only"
              @change="onFiles"
            />
          </div>
        </div>
        <div class="scrollbar-inset min-h-0 flex-1 overflow-auto">
          <FileTree
            ref="treeRef"
            :selected="selectedFile"
            :root-path="cwd"
            @select="openFile"
            @open-dir="openDir"
            @moved="onMoved"
          />
        </div>
      </GlassCard>

      <div class="flex w-4 shrink-0 self-stretch items-stretch justify-center">
        <button
          type="button"
          class="projects-splitter"
          :class="{ 'projects-splitter-active': isResizingPanes }"
          aria-label="Resize file tree and preview panes"
          title="Drag to resize panes"
          @pointerdown.prevent="startPaneResize"
        >
          <span class="projects-splitter-handle"></span>
        </button>
      </div>

      <GlassCard
        flat
        class="flex h-full min-h-0 min-w-0 shrink-0 flex-col p-6 pl-6"
        :style="rightPaneStyle"
      >
        <div class="mb-3 flex items-center justify-between gap-3">
          <div class="flex min-w-0 items-center gap-2">
            <svg class="h-4 w-4 shrink-0 text-faint-foreground" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M14 3H7a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V7z" />
              <path d="M14 3v4h4" />
            </svg>
            <span class="truncate text-sm text-foreground">{{ selectedFile || "No file selected" }}</span>
            <span v-if="editorError" class="shrink-0 text-xs text-danger">{{ editorError }}</span>
            <span v-else-if="editorLoading" class="shrink-0 text-xs text-subtle-foreground">Loading…</span>
            <span v-else-if="editorDirty" class="shrink-0 text-xs text-warning">• Unsaved</span>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <MSegmented
              v-if="isMarkdownFile"
              v-model="previewMode"
              :options="[
                { value: 'raw', label: 'Raw' },
                { value: 'markdown', label: 'Markdown' },
              ]"
            />
            <AppButton
              v-if="isTextFile"
              variant="accent"
              size="sm"
              :loading="editorSaving"
              :disabled="editorLoading || editorSaving || !editorDirty"
              title="Save (⌘S)"
              aria-label="Save file"
              @click="saveEditor"
            >
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M5 4h11l3 3v13a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z" />
                <path d="M8 4v5h7" />
                <path d="M8 21v-6h8v6" />
              </svg>
            </AppButton>
          </div>
        </div>
        <div class="scrollbar-inset min-h-0 flex-1 overflow-auto">
          <div v-if="!selectedFile" class="p-2 text-subtle-foreground">
            Select a file to preview
          </div>
          <template v-else>
            <div v-if="isTextFile" class="flex h-full flex-col">
              <textarea
                v-if="!isMarkdownFile || previewMode === 'raw'"
                v-model="editorContent"
                class="min-h-[360px] flex-1 resize-none rounded-md border border-border bg-surface p-3 text-sm text-foreground shadow-inner focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                spellcheck="false"
                @input="editorDirty = true"
                @keydown="onEditorKeydown"
              />
              <div
                v-else
                class="project-markdown-surface min-h-[360px] flex-1 rounded-md border border-border bg-surface shadow-inner"
              >
                <div
                  ref="markdownPreviewRef"
                  class="project-markdown scrollbar-inset h-full overflow-auto p-4 text-sm text-foreground"
                  v-html="renderedMarkdown"
                ></div>
              </div>
            </div>
            <div v-else-if="/\.(png|jpe?g|gif|svg|webp)$/i.test(selectedFile)">
              <img
                :src="previewUrl"
                alt="preview"
                class="max-w-full rounded-md border border-border"
              />
            </div>
            <div v-else class="text-sm text-subtle-foreground">
              Preview not available.
              <a
                :href="previewUrl"
                target="_blank"
                class="text-accent hover:underline"
                >Open</a
              >
            </div>
          </template>
        </div>
      </GlassCard>
    </div>

    <div
      v-else
      class="flex min-h-0 flex-1 flex-col items-center justify-center gap-4 text-center"
    >
      <div class="flex h-14 w-14 items-center justify-center rounded-xl border border-[rgb(var(--line-strong))] bg-surface-muted text-accent">
        <svg class="h-7 w-7" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M3 7a1 1 0 0 1 1-1h5l2 2h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />
        </svg>
      </div>
      <div>
        <p class="text-sm font-semibold">No project selected</p>
        <p class="mx-auto mt-1 max-w-sm text-sm text-subtle-foreground">
          Projects are isolated workspaces where agents read and write files. Pick one from the selector, or name a new one and create it.
        </p>
      </div>
      <AppButton variant="accent" size="sm" @click="focusNewProject">New project</AppButton>
    </div>

    <div
      v-if="showDeleteProjectDialog"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-project-title"
      @keydown.esc.prevent="closeDeleteProjectDialog"
    >
      <div
        class="w-full max-w-md rounded-lg border border-danger/40 bg-surface p-5"
      >
        <h2
          id="delete-project-title"
          class="text-base font-semibold text-danger"
        >
          Delete Project
        </h2>
        <p class="mt-2 text-sm text-subtle-foreground">
          This will permanently remove
          <span class="font-semibold text-foreground">{{
            deleteProjectTargetName
          }}</span>
          and all files in it.
        </p>
        <form class="mt-4 space-y-3" @submit.prevent="confirmDeleteProject">
          <div class="space-y-1">
            <label
              for="delete-project-confirm"
              class="text-xs font-medium uppercase tracking-wide text-faint-foreground"
            >
              Type project name to confirm
            </label>
            <input
              id="delete-project-confirm"
              v-model="deleteProjectTypedName"
              type="text"
              autocomplete="off"
              spellcheck="false"
              class="h-9 w-full rounded-md border border-danger/50 bg-surface px-3 text-sm text-foreground outline-none transition focus-visible:ring-2 focus-visible:ring-danger/70"
              placeholder="Project name"
            />
          </div>
          <label class="flex items-start gap-2 text-xs text-subtle-foreground">
            <input
              v-model="deleteProjectAcknowledged"
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-border bg-surface"
            />
            <span>I understand this action cannot be undone.</span>
          </label>
          <p v-if="deleteProjectError" class="text-xs text-danger">
            {{ deleteProjectError }}
          </p>
          <div class="flex items-center justify-end gap-2">
            <AppButton
              type="button"
              variant="ghost"
              size="sm"
              :disabled="deleteProjectPending"
              @click="closeDeleteProjectDialog"
            >
              Cancel
            </AppButton>
            <AppButton
              type="submit"
              variant="danger"
              size="sm"
              :loading="deleteProjectPending"
              :disabled="!canConfirmDeleteProject"
            >
              Delete Project
            </AppButton>
          </div>
        </form>
      </div>
    </div>
  </section>
</template>

<style scoped>
.project-markdown-surface {
  overflow: hidden;
}

.projects-splitter {
  position: relative;
  display: block;
  height: 100%;
  width: 100%;
  padding: 0;
  border: 0;
  appearance: none;
  cursor: col-resize;
  background: transparent;
}

.projects-splitter::before {
  content: "";
  position: absolute;
  top: 0.75rem;
  bottom: 0.75rem;
  left: 50%;
  width: 1px;
  transform: translateX(-50%);
  background: rgb(var(--color-border));
  transition: background-color 150ms ease;
}

.projects-splitter:hover::before,
.projects-splitter-active::before {
  background: rgb(var(--color-accent));
}

.projects-splitter-handle {
  position: absolute;
  top: 50%;
  left: 50%;
  height: 3.5rem;
  width: 0.4rem;
  transform: translate(-50%, -50%);
  border-radius: 9999px;
  background: rgb(var(--color-surface-muted));
  border: 1px solid rgb(var(--color-border));
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
}

.project-markdown {
  overflow-wrap: anywhere;
  word-break: break-word;
}

.project-markdown:deep(p) {
  margin: 0 0 0.85rem;
}

.project-markdown:deep(p:last-child) {
  margin-bottom: 0;
}

.project-markdown:deep(h1),
.project-markdown:deep(h2),
.project-markdown:deep(h3),
.project-markdown:deep(h4),
.project-markdown:deep(h5),
.project-markdown:deep(h6) {
  margin: 1.25rem 0 0.6rem;
  font-weight: 600;
  line-height: 1.2;
}

.project-markdown:deep(h1) {
  font-size: 1.5rem;
}

.project-markdown:deep(h2) {
  font-size: 1.25rem;
}

.project-markdown:deep(h3) {
  font-size: 1.125rem;
}

.project-markdown:deep(ul),
.project-markdown:deep(ol) {
  margin: 0 0 0.85rem;
  padding-left: 1.35rem;
}

.project-markdown:deep(li) {
  margin: 0.25rem 0;
}

.project-markdown:deep(a) {
  color: rgb(var(--color-accent));
  text-decoration: underline;
}

.project-markdown:deep(blockquote) {
  margin: 0 0 0.85rem;
  border-left: 3px solid rgb(var(--color-border));
  padding-left: 0.85rem;
  color: rgb(var(--color-subtle-foreground));
}

.project-markdown:deep(pre) {
  margin: 0 0 0.85rem;
  overflow-x: auto;
}

.project-markdown:deep(.md-mermaid) {
  margin: 0 0 1rem;
  overflow-x: auto;
  border: 1px solid rgb(var(--color-border));
  border-radius: 0.75rem;
  background: rgb(var(--color-surface) / 0.76);
  padding: 1rem;
}

.project-markdown:deep(.md-mermaid-source) {
  margin: 0;
}

.project-markdown:deep(.md-mermaid-loading) {
  color: rgb(var(--color-subtle-foreground));
}

.project-markdown:deep(.md-mermaid-rendered svg) {
  display: block;
  max-width: 100%;
  height: auto;
  margin: 0 auto;
  color: rgb(var(--color-foreground));
}

.project-markdown:deep(.md-mermaid-rendered svg text),
.project-markdown:deep(.md-mermaid-rendered svg .label),
.project-markdown:deep(.md-mermaid-rendered svg .nodeLabel),
.project-markdown:deep(.md-mermaid-rendered svg .edgeLabel) {
  color: rgb(var(--color-foreground)) !important;
  fill: rgb(var(--color-foreground)) !important;
}

.project-markdown:deep(.md-mermaid-error) {
  border-color: rgb(var(--color-danger) / 0.45);
}

.project-markdown:deep(.md-mermaid-error-message) {
  margin-bottom: 0.75rem;
  color: rgb(var(--color-danger));
  font-weight: 600;
}

.project-markdown:deep(code) {
  font-size: 0.875em;
}

.project-markdown:deep(img) {
  max-width: 100%;
  border-radius: 0.75rem;
}

.project-markdown:deep(table) {
  width: 100%;
  border-collapse: collapse;
  margin: 0 0 0.85rem;
}

.project-markdown:deep(th),
.project-markdown:deep(td) {
  border: 1px solid rgb(var(--color-border));
  padding: 0.5rem 0.65rem;
  text-align: left;
  vertical-align: top;
}

:global(body.projects-resizing) {
  cursor: col-resize;
  user-select: none;
}
</style>
