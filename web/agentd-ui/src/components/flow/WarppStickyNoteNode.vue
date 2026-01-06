<template>
  <div class="sticky-note-node relative h-full w-full select-none">
    <NodeResizer
      v-if="isDesignMode"
      :min-width="NOTE_MIN_WIDTH"
      :min-height="NOTE_MIN_HEIGHT"
      :handle-style="RESIZER_HANDLE_STYLE"
      :line-style="RESIZER_LINE_STYLE"
      @resize-end="onResizeEnd"
    />
    <div class="sticky-surface" :style="surfaceStyle" />
    <div
      class="sticky-header"
      :class="{ 'pointer-events-none': !isDesignMode }"
    >
      <div class="text-sm font-semibold text-foreground select-none">Note</div>
      <div v-if="isDesignMode" class="flex items-center gap-1 ml-auto">
        <button
          type="button"
          class="note-color-picker"
          :title="showColorPicker ? 'Close color picker' : 'Choose color'"
          @click.stop.prevent="toggleColorPicker"
        >
          <svg
            class="h-3.5 w-3.5"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <circle cx="13.5" cy="6.5" r=".5" fill="currentColor" />
            <circle cx="17.5" cy="10.5" r=".5" fill="currentColor" />
            <circle cx="8.5" cy="7.5" r=".5" fill="currentColor" />
            <circle cx="6.5" cy="12.5" r=".5" fill="currentColor" />
            <path
              d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.555-2.503 5.555-5.554C21.965 6.012 17.461 2 12 2z"
            />
          </svg>
        </button>
        <div v-if="showColorPicker" class="color-picker-popup">
          <button
            v-for="preset in colorPresets"
            :key="preset.value"
            type="button"
            class="color-swatch"
            :class="{ active: noteColor === preset.value }"
            :style="{ backgroundColor: preset.display }"
            :title="preset.label"
            @click.stop.prevent="setColor(preset.value)"
          >
            <svg
              v-if="noteColor === preset.value"
              class="h-3 w-3 text-white drop-shadow"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="3"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <polyline points="20 6 9 17 4 12"></polyline>
            </svg>
          </button>
        </div>
      </div>
    </div>

    <div class="sticky-body" :class="{ 'pointer-events-none': !isDesignMode }">
      <textarea
        v-model="noteText"
        :disabled="!isDesignMode"
        class="sticky-textarea"
        placeholder="Type a note…"
        rows="4"
        @wheel.stop
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  inject,
  ref,
  watch,
  type CSSProperties,
  type Ref,
} from "vue";
import { NodeResizer, type OnResizeEnd } from "@vue-flow/node-resizer";
import { useVueFlow, type NodeProps } from "@vue-flow/core";

import type { StickyNoteNodeData } from "@/types/flow";
import { WARPP_UTILITY_NODE_DIMENSIONS } from "@/constants/warppNodes";

defineOptions({ name: "WarppStickyNoteNode", inheritAttrs: false });

const props = defineProps<NodeProps<StickyNoteNodeData>>();

const { updateNode, updateNodeData } = useVueFlow();

const NOTE_MIN_WIDTH = WARPP_UTILITY_NODE_DIMENSIONS.minWidth;
const NOTE_MIN_HEIGHT = WARPP_UTILITY_NODE_DIMENSIONS.minHeight;
const defaultLabel = "Sticky Note";

const modeRef = inject<Ref<"design" | "run">>(
  "warppMode",
  ref<"design" | "run">("design"),
);
const isDesignMode = computed(() => modeRef.value === "design");

const RESIZER_HANDLE_STYLE = Object.freeze({
  width: "14px",
  height: "14px",
  opacity: "0",
  border: "none",
  background: "transparent",
});
const RESIZER_LINE_STYLE = Object.freeze({ opacity: "0" });

// Reuse group node color presets
const colorPresets = [
  { value: "default", label: "Default", display: "rgba(148, 163, 184, 0.3)" },
  { value: "blue", label: "Blue", display: "rgba(56, 189, 248, 0.25)" },
  { value: "green", label: "Green", display: "rgba(34, 197, 94, 0.25)" },
  { value: "amber", label: "Amber", display: "rgba(251, 191, 36, 0.25)" },
  { value: "rose", label: "Rose", display: "rgba(251, 113, 133, 0.25)" },
  { value: "purple", label: "Purple", display: "rgba(168, 85, 247, 0.25)" },
];

const colorMap: Record<string, string> = {
  default: "rgb(var(--color-surface) / 0.85)",
  blue: "rgba(56, 189, 248, 0.28)",
  green: "rgba(34, 197, 94, 0.28)",
  amber: "rgba(251, 191, 36, 0.28)",
  rose: "rgba(251, 113, 133, 0.28)",
  purple: "rgba(168, 85, 247, 0.28)",
};

// Header is static: "Note" per design; no editable label
const showColorPicker = ref(false);
const noteColor = ref<string>("default");
const noteText = ref("");
let suppressCommit = false;

watch(
  () => props.data?.color,
  (next) => {
    noteColor.value = next || "default";
  },
  { immediate: true },
);

watch(
  () => props.data?.note,
  (next) => {
    suppressCommit = true;
    noteText.value = next ?? "";
    suppressCommit = false;
  },
  { immediate: true },
);

const surfaceStyle = computed<CSSProperties>(() => ({
  background: colorMap[noteColor.value] || colorMap.default,
}));

watch(noteText, (next) => {
  if (suppressCommit) return;
  updateNodeData(props.id, {
    ...(props.data ?? { kind: "utility", label: defaultLabel }),
    note: next ?? "",
  });
});

function toggleColorPicker() {
  showColorPicker.value = !showColorPicker.value;
}

function setColor(color: string) {
  noteColor.value = color;
  updateNodeData(props.id, {
    ...(props.data ?? { kind: "utility", label: defaultLabel, note: "" }),
    color,
  });
  showColorPicker.value = false;
}

function onResizeEnd(event: OnResizeEnd) {
  if (!isDesignMode.value) return;
  const widthPx = `${Math.max(Math.round(event.params.width), NOTE_MIN_WIDTH)}px`;
  const heightPx = `${Math.max(Math.round(event.params.height), NOTE_MIN_HEIGHT)}px`;
  updateNode(props.id, (node) => {
    const baseStyle: CSSProperties =
      typeof node.style === "function"
        ? ((node.style(node) as CSSProperties) ?? {})
        : { ...(node.style ?? {}) };
    return {
      style: {
        ...baseStyle,
        width: widthPx,
        height: heightPx,
      },
    };
  });
}
</script>

<style scoped>
.sticky-note-node {
  border-radius: 10px;
}
.sticky-surface {
  position: absolute;
  inset: 0;
  border-radius: 10px;
  background: color-mix(
    in srgb,
    rgb(var(--color-surface) / 1) 80%,
    transparent 20%
  );
  border: 1px dashed rgb(var(--color-border) / 0.65);
}
.sticky-header {
  position: absolute;
  top: 0.35rem;
  left: 0.5rem;
  right: 0.5rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  pointer-events: auto;
}
.sticky-title {
  flex: 1;
  min-width: 0;
  border: 1px solid rgb(var(--color-border) / 0.75);
  border-radius: 0.5rem;
  background: rgb(var(--color-surface) / 0.92);
  padding: 0.25rem 0.5rem;
  font-size: 0.7rem;
  color: rgb(var(--color-foreground));
}
.note-color-picker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.7);
  background: rgb(var(--color-surface-muted) / 0.9);
  padding: 0.25rem;
  color: rgb(var(--color-subtle-foreground));
}
.note-color-picker:hover {
  color: rgb(var(--color-foreground));
  background: rgb(var(--color-surface-muted));
}
.color-picker-popup {
  position: absolute;
  top: 100%;
  right: 0;
  margin-top: 0.25rem;
  display: flex;
  gap: 0.25rem;
  padding: 0.375rem;
  background: rgb(var(--color-surface));
  border: 1px solid rgb(var(--color-border) / 0.7);
  border-radius: 0.5rem;
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.3);
  z-index: 100;
}
.color-swatch {
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 0.375rem;
  border: 2px solid rgb(var(--color-border) / 0.5);
  cursor: pointer;
  transition: all 150ms;
  display: flex;
  align-items: center;
  justify-content: center;
}
.color-swatch:hover {
  border-color: rgb(var(--color-foreground) / 0.6);
  transform: scale(1.1);
}
.color-swatch.active {
  border-color: rgb(var(--color-accent));
  box-shadow: 0 0 0 2px rgb(var(--color-accent) / 0.3);
}
.sticky-body {
  position: absolute;
  top: 2rem;
  left: 0.5rem;
  right: 0.5rem;
  bottom: 0.5rem;
}
.sticky-textarea {
  width: 100%;
  height: 100%;
  resize: none;
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.6);
  background: rgb(var(--color-surface) / 0.85);
  padding: 0.5rem;
  font-size: 0.8rem;
  color: rgb(var(--color-foreground));
  overflow: auto;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
