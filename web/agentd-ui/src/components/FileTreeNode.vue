<script setup lang="ts">
import { computed, inject } from "vue";
import type { FileEntry } from "@/api/client";
import { useProjectsStore } from "@/stores/projects";

const props = defineProps<{
  path: string;
  depth: number;
  selected?: string;
  isExpanded: (p: string) => boolean;
  toggle: (p: string) => void | Promise<void>;
  isChecked: (p: string) => boolean;
  toggleCheck: (p: string) => void;
}>();

const emit = defineEmits<{
  (e: "select", path: string): void;
  (e: "open-dir", path: string): void;
  (e: "moved", payload: { from: string; to: string }): void;
}>();

const store = useProjectsStore();
const key = computed(() => `${store.currentProjectId}:${props.path || "."}`);
const list = computed(() => store.treeByPath[key.value] || []);

// Shared drag state from FileTree
type DragKind = "file" | "dir";
const dragging =
  inject<import("vue").Ref<{ path: string; kind: DragKind } | null>>(
    "filetreeDrag",
  );
const dropTargetDir = inject<import("vue").Ref<string | null>>(
  "filetreeDropTargetDir",
);

function select(path: string) {
  emit("select", path);
}
function openDir(path: string) {
  emit("open-dir", path);
}

function getDragData(event: DragEvent) {
  const dt = event.dataTransfer;
  const path = (
    dt?.getData("application/x-project-path") ||
    dt?.getData("text/plain") ||
    ""
  ).trim();
  const kindRaw = (dt?.getData("application/x-project-kind") || "").trim();
  const kind: DragKind = kindRaw === "dir" ? "dir" : "file";
  return { path, kind };
}

function baseName(path: string) {
  const clean = path.replace(/^\.\/+/, "").replace(/\/+$/, "");
  const parts = clean.split("/").filter(Boolean);
  return parts.pop() || clean;
}

function parentPath(path: string) {
  const clean = path.replace(/^\.\/+/, "").replace(/\/+$/, "");
  const idx = clean.lastIndexOf("/");
  if (idx === -1) return ".";
  const parent = clean.slice(0, idx);
  return parent || ".";
}

function normalizeDir(dir: string) {
  if (!dir || dir === ".") return ".";
  const noLeading = dir.replace(/^\.\/+/, "");
  const noTrailing = noLeading.replace(/\/+$/, "");
  return noTrailing || ".";
}

function destinationFor(dir: string, name: string) {
  const normalized = normalizeDir(dir);
  if (!name) return normalized === "." ? "" : normalized;
  if (!normalized || normalized === ".") return name;
  return `${normalized}/${name}`;
}

function canAcceptMove(src: string, dest: string, kind: DragKind) {
  if (!src || !dest) return false;
  if (src === dest) return false;
  if (kind === "dir" && (dest === src || dest.startsWith(`${src}/`))) {
    return false;
  }
  return true;
}

function onDragStart(event: DragEvent, entry: FileEntry) {
  if (!event.dataTransfer) return;
  event.dataTransfer.effectAllowed = "move";
  event.dataTransfer.setData("application/x-project-path", entry.path);
  event.dataTransfer.setData("text/plain", entry.path);
  event.dataTransfer.setData(
    "application/x-project-kind",
    entry.isDir ? "dir" : "file",
  );
  if (dragging)
    dragging.value = { path: entry.path, kind: entry.isDir ? "dir" : "file" };
}

function onDragEnd() {
  if (dragging) dragging.value = null;
}

function onDragOver(event: DragEvent, entry: FileEntry) {
  const d = dragging?.value || getDragData(event);
  if (!d || !d.path) {
    if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
    if (dropTargetDir) dropTargetDir.value = null;
    return;
  }
  const targetDir = entry.isDir ? entry.path || "." : parentPath(entry.path);
  const dest = destinationFor(targetDir, baseName(d.path));
  if (!canAcceptMove(d.path, dest, d.kind)) {
    if (event.dataTransfer) event.dataTransfer.dropEffect = "none";
    if (dropTargetDir) dropTargetDir.value = null;
    return;
  }
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
  if (dropTargetDir) dropTargetDir.value = normalizeDir(targetDir);
}

async function onDrop(event: DragEvent, entry: FileEntry) {
  const d = dragging?.value || getDragData(event);
  if (!d || !d.path) return;
  const targetDir = entry.isDir ? entry.path || "." : parentPath(entry.path);
  const dest = destinationFor(targetDir, baseName(d.path));
  if (!canAcceptMove(d.path, dest, d.kind)) return;
  try {
    await store.movePath(d.path, dest);
    emit("moved", { from: d.path, to: dest });
  } catch (err) {
    console.error("move failed", err);
  } finally {
    if (dragging) dragging.value = null;
    if (dropTargetDir) dropTargetDir.value = null;
  }
}

function onDragLeave() {
  if (dropTargetDir) dropTargetDir.value = null;
}
</script>

<template>
  <ul>
    <template v-for="e in list" :key="e.path">
      <li
        class="group flex items-center gap-2 h-9 pr-2 border-b border-border/70 last:border-b-0 hover:bg-surface-muted cursor-pointer"
        :class="{
          'bg-surface-muted': selected === e.path,
          'ring-2 ring-accent/50 ring-offset-0 bg-accent/10':
            e.isDir && dropTargetDir === normalizeDir(e.path),
          'outline outline-1 outline-accent/50':
            !e.isDir && dropTargetDir === normalizeDir(parentPath(e.path)),
        }"
        :draggable="true"
        @dragstart="onDragStart($event, e)"
        @dragend="onDragEnd"
        @dragover.prevent="onDragOver($event, e)"
        @drop.stop.prevent="onDrop($event, e)"
        @dragleave.prevent="onDragLeave"
      >
        <div
          class="flex items-center shrink-0"
          :style="{ paddingLeft: `${12 + depth * 16}px` }"
        >
          <input
            type="checkbox"
            class="w-5 h-5 mr-2 rounded-3 border-border text-danger focus-visible:outline-none focus-visible:shadow-outline"
            :checked="isChecked(e.path)"
            @click.stop
            @change.stop="() => toggleCheck(e.path)"
            :aria-label="`Select ${e.name}`"
          />
          <button
            v-if="e.isDir"
            class="w-5 h-5 mr-1 rounded-3 text-subtle-foreground hover:bg-surface-muted/70 focus-visible:outline-none focus-visible:shadow-outline"
            :title="isExpanded(e.path) ? 'Collapse' : 'Expand'"
            @click.stop="toggle(e.path)"
          >
            {{ isExpanded(e.path) ? "▾" : "▸" }}
          </button>
          <span v-else class="w-5 h-5 mr-1" />
          <span class="w-5 text-subtle-foreground">{{
            e.isDir ? "📁" : "📄"
          }}</span>
        </div>
        <span
          class="text-foreground truncate flex-1 min-w-0"
          @click.stop="e.isDir ? openDir(e.path) : select(e.path)"
        >
          {{ e.name }}
        </span>
        <span class="ml-auto text-xs text-faint-foreground">{{
          e.isDir ? "" : `${e.sizeBytes} B`
        }}</span>
      </li>
      <li
        v-if="e.isDir && isExpanded(e.path)"
        :key="e.path + '__children'"
        class="border-0 p-0 m-0"
      >
        <FileTreeNode
          :path="e.path"
          :depth="depth + 1"
          :selected="selected"
          :is-expanded="isExpanded"
          :toggle="toggle"
          :is-checked="isChecked"
          :toggle-check="toggleCheck"
          @select="emit('select', $event)"
          @open-dir="emit('open-dir', $event)"
          @moved="emit('moved', $event)"
        />
      </li>
    </template>
  </ul>
</template>
