<script setup lang="ts">
import { computed, inject } from "vue";
import type { FileEntry } from "@/api/client";
import { useProjectsStore } from "@/stores/projects";
import FileTreeIcon from "./FileTreeIcon.vue";

function formatSize(bytes: number) {
  if (!bytes) return "";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

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
  <ul class="select-none">
    <template v-for="e in list" :key="e.path">
      <li
        class="group relative flex h-7 cursor-pointer items-center rounded-[4px] pr-2 transition-colors"
        :class="{
          'bg-accent/12 text-foreground': selected === e.path,
          'hover:bg-surface-muted/70': selected !== e.path,
          'ring-1 ring-inset ring-accent/60 bg-accent/10':
            e.isDir && dropTargetDir === normalizeDir(e.path),
          'ring-1 ring-inset ring-accent/40':
            !e.isDir && dropTargetDir === normalizeDir(parentPath(e.path)),
        }"
        :draggable="true"
        @dragstart="onDragStart($event, e)"
        @dragend="onDragEnd"
        @dragover.prevent="onDragOver($event, e)"
        @drop.stop.prevent="onDrop($event, e)"
        @dragleave.prevent="onDragLeave"
        @click="e.isDir ? openDir(e.path) : select(e.path)"
      >
        <!-- selection accent bar -->
        <span
          v-if="selected === e.path"
          class="absolute inset-y-0 left-0 w-[2px] rounded-full bg-accent"
        ></span>

        <div
          class="flex min-w-0 flex-1 items-center gap-1.5"
          :style="{ paddingLeft: `${8 + depth * 14}px` }"
        >
          <!-- checkbox: hidden until hover / when checked -->
          <input
            type="checkbox"
            class="h-3.5 w-3.5 shrink-0 rounded-[3px] border-[rgb(var(--line-strong))] bg-surface text-accent opacity-0 transition-opacity focus-visible:opacity-100 focus-visible:outline-none group-hover:opacity-100"
            :class="{ '!opacity-100': isChecked(e.path) }"
            :checked="isChecked(e.path)"
            @click.stop
            @change.stop="() => toggleCheck(e.path)"
            :aria-label="`Select ${e.name}`"
          />

          <!-- chevron (dirs) -->
          <button
            v-if="e.isDir"
            class="grid h-4 w-4 shrink-0 place-items-center rounded-[3px] text-faint-foreground transition-colors hover:text-foreground focus-visible:outline-none"
            :title="isExpanded(e.path) ? 'Collapse' : 'Expand'"
            :aria-expanded="isExpanded(e.path)"
            @click.stop="toggle(e.path)"
          >
            <svg
              class="h-3.5 w-3.5 transition-transform"
              :class="{ 'rotate-90': isExpanded(e.path) }"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2.2"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="m9 6 6 6-6 6" />
            </svg>
          </button>
          <span v-else class="h-4 w-4 shrink-0" />

          <FileTreeIcon
            class="h-4 w-4"
            :name="e.name"
            :is-dir="e.isDir"
            :expanded="e.isDir && isExpanded(e.path)"
          />

          <span
            class="truncate text-[13px] leading-none"
            :class="selected === e.path ? 'text-foreground' : 'text-foreground/90'"
          >
            {{ e.name }}
          </span>
        </div>

        <span
          v-if="!e.isDir"
          class="ml-2 shrink-0 font-mono text-[10px] text-faint-foreground opacity-0 transition-opacity group-hover:opacity-100"
        >
          {{ formatSize(e.sizeBytes) }}
        </span>
      </li>
      <li
        v-if="e.isDir && isExpanded(e.path)"
        :key="e.path + '__children'"
        class="m-0 border-0 p-0"
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
