<script setup lang="ts">
import { onMounted, ref, computed, watch } from "vue";
import { useProjectsStore } from "@/stores/projects";
import { projectFileUrl, projectArchiveUrl } from "@/api/client";
import Panel from "@/components/ui/Panel.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import Pill from "@/components/ui/Pill.vue";
import FileTree from "@/components/FileTree.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import type { DropdownOption } from "@/types/dropdown";

const store = useProjectsStore();
const newProjectName = ref("");
const uploadInput = ref<HTMLInputElement | null>(null);
const treeRef = ref<InstanceType<typeof FileTree> | null>(null);
const cwd = ref(".");
const selectedFile = ref<string>("");
const editorContent = ref("");
const editorDirty = ref(false);
const editorLoading = ref(false);
const editorSaving = ref(false);
const editorError = ref("");
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
const previewUrl = computed(() => {
  if (!store.currentProjectId || !selectedFile.value) return "";
  return projectFileUrl(store.currentProjectId, selectedFile.value);
});

onMounted(() => {
  void store.refresh().then(() => store.ensureTree(cwd.value));
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

async function bulkDownload() {
  const ids = Array.from(treeRef.value?.checked ?? new Set<string>());
  if (!ids.length || !store.currentProjectId) return;

  for (const path of ids) {
    const url = projectFileUrl(store.currentProjectId, path);
    const a = document.createElement("a");
    a.href = url;
    a.download = path.split("/").pop() || "download";
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
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col space-y-3">
    <Panel
      title="Projects"
      description="Create projects, upload files, and preview artifacts in one place."
      :padded="true"
    >
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
          <button
            v-if="store.currentProjectId"
            class="inline-flex h-9 items-center justify-center gap-2 rounded-full border border-danger/50 px-3 text-sm font-semibold text-danger transition hover:bg-danger/10"
            @click="() => store.remove(store.currentProjectId)"
          >
            Delete
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
      class="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-2"
    >
      <GlassCard class="flex min-h-0 flex-col p-4 lg:p-6">
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
              :disabled="!(treeRef?.checked && treeRef.checked.size > 0)"
              @click="bulkDownload"
            >
              Download
            </button>
            <button
              class="h-9 rounded-full border border-danger/60 px-3 text-sm text-danger transition hover:bg-danger/10 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!(treeRef?.checked && treeRef.checked.size > 0)"
              @click="bulkDelete"
            >
              Delete
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

      <GlassCard class="flex min-h-0 flex-col p-4 lg:p-6">
        <div
          class="mb-3 flex items-center justify-between text-sm text-faint-foreground"
        >
          <div class="uppercase tracking-wide">Preview</div>
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
                v-model="editorContent"
                class="min-h-[360px] flex-1 resize-none rounded-3 border border-border bg-surface/70 p-3 text-sm text-foreground shadow-inner focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                spellcheck="false"
                @input="editorDirty = true"
              />
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
  </section>
</template>

<style scoped>
/* Use Tailwind utilities with theme tokens; no local component theming */
</style>
