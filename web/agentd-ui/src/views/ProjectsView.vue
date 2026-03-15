<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, computed, watch } from "vue";
import { useProjectsStore } from "@/stores/projects";
import { projectFileUrl, projectArchiveUrl } from "@/api/client";
import Panel from "@/components/ui/Panel.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import Pill from "@/components/ui/Pill.vue";
import FileTree from "@/components/FileTree.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import { renderMarkdown } from "@/utils/markdown";
import type { DropdownOption } from "@/types/dropdown";

const store = useProjectsStore();
const newProjectName = ref("");
const uploadInput = ref<HTMLInputElement | null>(null);
const treeRef = ref<InstanceType<typeof FileTree> | null>(null);
const splitPaneRef = ref<HTMLElement | null>(null);
const cwd = ref(".");
const selectedFile = ref<string>("");
const editorContent = ref("");
const editorDirty = ref(false);
const editorLoading = ref(false);
const editorSaving = ref(false);
const editorError = ref("");
const previewMode = ref<"raw" | "markdown">("raw");
const leftPaneWidth = ref(38);
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
const isMarkdownFile = computed(() => /\.(md|markdown)$/i.test(selectedFile.value));
const renderedMarkdown = computed(() => renderMarkdown(editorContent.value));
const leftPaneStyle = computed(() => ({ flexBasis: `${leftPaneWidth.value}%` }));
const rightPaneStyle = computed(() => ({ flexBasis: `${100 - leftPaneWidth.value}%` }));
const previewUrl = computed(() => {
  if (!store.currentProjectId || !selectedFile.value) return "";
  return projectFileUrl(store.currentProjectId, selectedFile.value);
});

onMounted(() => {
  void store.refresh().then(() => store.ensureTree(cwd.value));
});

onBeforeUnmount(() => {
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
  <section class="flex min-h-0 flex-1 flex-col space-y-3">
    <Panel title="Projects" :padded="true">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex flex-wrap items-center gap-2">
          <label class="sr-only" for="new-project">New project name</label>
          <input
            id="new-project"
            v-model="newProjectName"
            placeholder="New project name"
            class="h-9 w-48 rounded-full border border-white/10 bg-surface/70 px-3 text-sm text-foreground placeholder:text-subtle-foreground transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <button
            class="inline-flex h-9 items-center justify-center gap-2 rounded-full border border-accent/60 bg-accent/90 px-3 text-sm font-semibold text-accent-foreground shadow-[0_8px_30px_rgba(0,0,0,0.25)] transition hover:bg-accent"
            @click="createProject"
          >
            Create
          </button>
          <button
            class="inline-flex h-9 w-9 items-center justify-center rounded-full border border-danger/45 text-danger transition hover:bg-danger/10 disabled:cursor-not-allowed disabled:opacity-50"
            :disabled="!current"
            title="Delete current project"
            aria-label="Delete current project"
            @click="openDeleteProjectDialog"
          >
            <SolarTrashIcon class="h-4 w-4" />
          </button>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <DropdownSelect
            id="project-select"
            v-model="selectedProjectId"
            :options="projectOptions"
            size="sm"
            aria-label="Project"
            title="Current project"
          />
          <button
            v-if="store.currentProjectId"
            class="inline-flex h-9 items-center justify-center gap-2 rounded-full border border-accent/50 px-3 text-sm font-semibold text-accent transition hover:bg-accent/10"
            @click="downloadProject"
            title="Download project as .tar.gz"
          >
            Download
          </button>
        </div>

        <div
          v-if="current"
          class="flex flex-wrap items-center gap-2 text-xs text-faint-foreground md:ml-auto"
        >
          <span
            >Created
            {{ new Date(current.createdAt).toLocaleDateString() }}</span
          >
          <Pill tone="neutral" size="sm">{{ current.files }} files</Pill>
          <Pill tone="neutral" size="sm"
            >{{ (current.sizeBytes / 1024).toFixed(1) }} KB</Pill
          >
        </div>
      </div>
    </Panel>

    <div
      v-if="store.currentProjectId"
      ref="splitPaneRef"
      class="flex min-h-0 flex-1 flex-col gap-3 lg:flex-row lg:gap-0"
    >
      <GlassCard
        class="flex min-h-0 flex-col p-4 lg:min-w-0 lg:shrink-0 lg:p-6"
        :style="leftPaneStyle"
      >
        <div class="mb-4 flex items-center gap-3">
          <button
            class="h-9 rounded-full border border-white/10 px-3 text-sm text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
            @click="() => openDir('.')"
          >
            Root
          </button>
          <div class="truncate text-sm text-faint-foreground">{{ cwd }}</div>
          <div class="ml-auto flex flex-wrap items-center gap-2">
            <button
              class="h-9 rounded-full border border-white/10 bg-surface/70 px-3 text-sm text-foreground transition hover:border-accent/40 hover:text-accent"
              @click="mkdir"
            >
              New Folder
            </button>
            <button
              class="h-9 rounded-full border border-white/10 bg-surface/70 px-3 text-sm text-foreground transition hover:border-accent/40 hover:text-accent"
              @click="createFile"
            >
              New File
            </button>
            <button
              class="h-9 rounded-full border border-white/10 bg-surface/70 px-3 text-sm text-foreground transition hover:border-accent/40 hover:text-accent"
              @click="pickUpload"
            >
              Upload
            </button>
            <button
              class="h-9 rounded-full border border-accent/50 px-3 text-sm text-accent transition hover:bg-accent/10 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!canDeleteSelectedItems"
              @click="bulkDownload"
            >
              Download Selected
            </button>
            <button
              class="h-9 rounded-full border border-danger/60 px-3 text-sm text-danger transition hover:bg-danger/10 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!canDeleteSelectedItems"
              @click="bulkDelete"
            >
              Delete Selected
            </button>
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

      <div class="hidden lg:flex lg:w-4 lg:shrink-0 lg:items-stretch lg:justify-center">
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
        class="flex min-h-0 flex-col p-4 lg:min-w-0 lg:shrink-0 lg:p-6"
        :style="rightPaneStyle"
      >
        <div
          class="mb-3 flex items-center justify-between text-sm text-faint-foreground"
        >
          <div class="flex items-center gap-3">
            <div class="uppercase tracking-wide">Preview</div>
            <div
              v-if="isMarkdownFile"
              class="inline-flex items-center rounded-full border border-white/10 bg-surface/70 p-1"
            >
              <button
                class="rounded-full px-3 py-1 text-xs font-medium transition"
                :class="previewMode === 'raw'
                  ? 'bg-accent/90 text-accent-foreground shadow-[0_6px_20px_rgba(0,0,0,0.2)]'
                  : 'text-subtle-foreground hover:text-foreground'"
                @click="previewMode = 'raw'"
              >
                Raw
              </button>
              <button
                class="rounded-full px-3 py-1 text-xs font-medium transition"
                :class="previewMode === 'markdown'
                  ? 'bg-accent/90 text-accent-foreground shadow-[0_6px_20px_rgba(0,0,0,0.2)]'
                  : 'text-subtle-foreground hover:text-foreground'"
                @click="previewMode = 'markdown'"
              >
                Markdown
              </button>
            </div>
          </div>
          <div
            class="max-w-[70%] truncate text-subtle-foreground"
            v-if="selectedFile"
          >
            {{ selectedFile }}
          </div>
        </div>
        <div class="scrollbar-inset min-h-0 flex-1 overflow-auto">
          <div v-if="!selectedFile" class="p-2 text-subtle-foreground">
            Select a file to preview
          </div>
          <template v-else>
            <div v-if="isTextFile" class="flex h-full flex-col gap-3">
              <div class="flex flex-wrap items-center gap-2">
                <button
                  class="inline-flex h-9 items-center justify-center gap-2 rounded-full border border-accent/50 px-3 text-sm font-semibold text-accent transition hover:bg-accent/10 disabled:cursor-not-allowed disabled:opacity-50"
                  :disabled="editorLoading || editorSaving || !editorDirty"
                  @click="saveEditor"
                >
                  {{ editorSaving ? "Saving..." : "Save" }}
                </button>
                <span
                  v-if="editorLoading"
                  class="text-xs text-subtle-foreground"
                  >Loading...</span
                >
                <span v-if="editorError" class="text-xs text-danger">
                  {{ editorError }}
                </span>
                <span
                  v-else-if="editorDirty"
                  class="text-xs text-subtle-foreground"
                  >Unsaved changes</span
                >
              </div>
              <textarea
                v-if="!isMarkdownFile || previewMode === 'raw'"
                v-model="editorContent"
                class="min-h-[360px] flex-1 resize-none rounded-3 border border-border bg-surface/70 p-3 text-sm text-foreground shadow-inner focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                spellcheck="false"
                @input="editorDirty = true"
              />
              <div
                v-else
                class="project-markdown-surface min-h-[360px] flex-1 rounded-3 border border-border bg-surface/70 shadow-inner"
              >
                <div
                  class="project-markdown scrollbar-inset h-full overflow-auto p-4 text-sm text-foreground"
                  v-html="renderedMarkdown"
                ></div>
              </div>
            </div>
            <div v-else-if="/\.(png|jpe?g|gif|svg|webp)$/i.test(selectedFile)">
              <img
                :src="previewUrl"
                alt="preview"
                class="max-w-full rounded-4 border border-border"
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

    <GlassCard v-else class="p-6 text-subtle-foreground">
      No project selected. Create one to get started.
    </GlassCard>

    <div
      v-if="showDeleteProjectDialog"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 px-4"
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-project-title"
      @keydown.esc.prevent="closeDeleteProjectDialog"
    >
      <div
        class="w-full max-w-md rounded-4 border border-danger/40 bg-surface p-5 shadow-[0_30px_60px_rgba(0,0,0,0.5)]"
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
              class="h-9 w-full rounded-full border border-danger/50 bg-surface/70 px-3 text-sm text-foreground transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-danger/70"
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
            <button
              type="button"
              class="h-9 rounded-full border border-white/15 px-3 text-sm text-subtle-foreground transition hover:border-white/30 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="deleteProjectPending"
              @click="closeDeleteProjectDialog"
            >
              Cancel
            </button>
            <button
              type="submit"
              class="h-9 rounded-full border border-danger/60 bg-danger/10 px-3 text-sm font-semibold text-danger transition hover:bg-danger/20 disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="!canConfirmDeleteProject"
            >
              {{ deleteProjectPending ? "Deleting..." : "Delete Project" }}
            </button>
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
  width: 100%;
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
