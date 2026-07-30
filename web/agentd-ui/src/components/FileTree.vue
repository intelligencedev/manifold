<script setup lang="ts">
import { computed, onMounted, provide, ref, watch } from "vue";
import { useProjectsStore } from "@/stores/projects";
import FileTreeNode from "./FileTreeNode.vue";

const props = defineProps<{
  selected?: string;
  cwd?: string;
  rootPath?: string;
}>();

const emit = defineEmits<{
  (e: "select", path: string): void;
  (e: "open-dir", path: string): void;
  (e: "moved", payload: { from: string; to: string }): void;
}>();

const store = useProjectsStore();
const rootPath = computed(() => props.rootPath ?? ".");

// Track expanded folders
const expanded = ref<Set<string>>(new Set([rootPath.value]));
// Track checked items
const checked = ref<Set<string>>(new Set());

// Track current drag item globally for the tree so dragover can read it reliably
type DragKind = "file" | "dir";
const dragging = ref<{ path: string; kind: DragKind } | null>(null);
provide("filetreeDrag", dragging);

// Track which destination directory will receive the drop; used for highlight cues
const dropTargetDir = ref<string | null>(null);
provide("filetreeDropTargetDir", dropTargetDir);

async function ensure(path: string) {
  await store.ensureTree(path || ".");
}

function isExpanded(path: string) {
  return expanded.value.has(path || ".");
}

async function toggle(path: string) {
  const p = path || ".";
  if (expanded.value.has(p)) {
    expanded.value.delete(p);
  } else {
    expanded.value.add(p);
    await ensure(p);
  }
}

function selectFile(path: string) {
  emit("select", path);
}

function openDir(path: string) {
  emit("open-dir", path || ".");
}

function isChecked(path: string) {
  return checked.value.has(path);
}
function toggleCheck(path: string) {
  const next = new Set(checked.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  checked.value = next;
}
function clearChecks() {
  checked.value = new Set();
}
defineExpose({
  isChecked,
  toggleCheck,
  clearChecks,
  checked,
});

// dragData via dataTransfer isn't reliable during dragover in some browsers; prefer shared state
function currentDrag() {
  return dragging.value;
}

function baseName(path: string) {
  const clean = path.replace(/^\.\/+/, "").replace(/\/+$/, "");
  const parts = clean.split("/").filter(Boolean);
  return parts.pop() || clean;
}

function normalizeDir(dir: string) {
  if (!dir || dir === ".") return ".";
  const withoutLeading = dir.replace(/^\.\/+/, "");
  const withoutTrailing = withoutLeading.replace(/\/+$/, "");
  return withoutTrailing || ".";
}

function buildDestination(dir: string, name: string) {
  const normalizedDir = normalizeDir(dir);
  if (!name) return normalizedDir === "." ? "" : normalizedDir;
  if (!normalizedDir || normalizedDir === ".") return name;
  return `${normalizedDir}/${name}`;
}

function canAcceptMove(src: string, dest: string, kind: DragKind) {
  if (!src || !dest) return false;
  if (src === dest) return false;
  if (kind === "dir" && (dest === src || dest.startsWith(`${src}/`))) {
    return false;
  }
  return true;
}

function onRootDragOver(event: DragEvent) {
  const d = currentDrag();
  if (!d) {
    if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
    dropTargetDir.value = null;
    return;
  }
  const dest = buildDestination(rootPath.value, baseName(d.path));
  if (!canAcceptMove(d.path, dest, d.kind)) {
    if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
    dropTargetDir.value = null;
    return;
  }
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
  dropTargetDir.value = normalizeDir(rootPath.value);
}

async function onRootDrop(event: DragEvent) {
  const d = currentDrag();
  if (!d) return;
  const base = baseName(d.path);
  const dest = buildDestination(rootPath.value, base);
  if (!canAcceptMove(d.path, dest, d.kind)) return;
  try {
    await store.movePath(d.path, dest);
    emit("moved", { from: d.path, to: dest });
  } catch (err) {
    console.error("move failed", err);
  } finally {
    dragging.value = null;
    dropTargetDir.value = null;
  }
}

function onRootDragLeave() {
  dropTargetDir.value = null;
}

onMounted(async () => {
  if (store.currentProjectId) {
    await ensure(rootPath.value);
  }
});

watch(
  () => store.currentProjectId,
  async () => {
    expanded.value = new Set([rootPath.value]);
    checked.value.clear();
    if (store.currentProjectId) await ensure(rootPath.value);
  },
);
</script>

<template>
  <div
    class="flex min-h-0 flex-col overflow-hidden rounded-md border border-border/70 bg-surface"
  >
    <div
      class="flex h-8 shrink-0 items-center gap-2 border-b border-border/60 px-3 text-faint-foreground transition-colors"
      :class="{
        'bg-accent/10 ring-1 ring-inset ring-accent/50':
          dropTargetDir === normalizeDir(rootPath),
      }"
      @dragover.prevent="onRootDragOver"
      @drop.prevent="onRootDrop"
      @dragleave.prevent="onRootDragLeave"
    >
      <svg class="h-3.5 w-3.5 text-accent/85" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <path d="M3 7a1 1 0 0 1 1-1h4.6a1 1 0 0 1 .7.3L11 8h8a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z" />
      </svg>
      <span class="font-mono text-[11px] uppercase tracking-[0.14em]">Root</span>
    </div>
    <div
      class="scrollbar-inset min-h-0 flex-1 overflow-auto p-1"
      @dragover.prevent.self="onRootDragOver"
      @drop.prevent.self="onRootDrop"
      @dragleave.prevent="onRootDragLeave"
    >
      <FileTreeNode
        :path="rootPath"
        :depth="0"
        :selected="selected"
        :is-expanded="isExpanded"
        :toggle="toggle"
        :is-checked="isChecked"
        :toggle-check="toggleCheck"
        @select="selectFile"
        @open-dir="openDir"
        @moved="emit('moved', $event)"
      />
    </div>
  </div>
</template>
