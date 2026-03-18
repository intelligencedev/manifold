<template>
  <div
    class="relative w-full text-xs text-muted-foreground overflow-visible"
    :class="rootClasses"
    :style="rootStyle"
  >
    <NodeResizer
      v-if="showResizer"
      :min-width="minWidth"
      :min-height="minHeight"
      :handle-style="RESIZER_HANDLE_STYLE"
      :line-style="RESIZER_LINE_STYLE"
      @resize-end="onResizeEnd"
    />

    <div class="relative h-full w-full">
      <!-- Handles anchored to flip-card for proper centering in all states -->
      <Handle
        type="target"
        :position="Position.Left"
        class="!bg-accent"
        :style="{
          position: 'absolute',
          top: '50%',
          left: '0',
          transform: 'translate(-50%, -50%)',
          width: '14px',
          height: '14px',
          zIndex: 10,
          pointerEvents: 'auto',
        }"
      />
      <Handle
        type="source"
        :position="Position.Right"
        class="!bg-accent"
        :style="{
          position: 'absolute',
          top: '50%',
          right: '0',
          transform: 'translate(50%, -50%)',
          width: '14px',
          height: '14px',
          zIndex: 10,
          pointerEvents: 'auto',
        }"
      />

      <div
        class="node-face h-full w-full flex flex-col overflow-hidden rounded-lg border border-border/60 bg-surface/90 p-3 shadow-lg"
      >
        <div class="flex-1 min-h-0 overflow-y-auto overflow-x-hidden">
          <slot name="front" />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import type { OnResizeEnd } from "@vue-flow/node-resizer";

defineOptions({ name: "FlowBaseNode" });

const props = defineProps<{
  collapsed?: boolean;
  minWidth?: number;
  minHeight?: number;
  minWidthPx?: string;
  minHeightPx?: string;
  showResizer?: boolean;
  rootClass?: string | string[] | Record<string, boolean>;
  selected?: boolean;
}>();

const emit = defineEmits<{
  (event: "resize-end", data: OnResizeEnd): void;
}>();

const RESIZER_HANDLE_STYLE = Object.freeze({
  width: "14px",
  height: "14px",
  opacity: "0",
  border: "none",
  background: "transparent",
});

const RESIZER_LINE_STYLE = Object.freeze({ opacity: "0" });

const rootStyle = computed<CSSProperties>(() => {
  const style: CSSProperties = {};
  if (props.minWidthPx) style.minWidth = props.minWidthPx;
  if (props.minHeightPx) style.minHeight = props.minHeightPx;
  return style;
});

const rootClasses = computed(() => [
  props.rootClass,
  { "is-selected": Boolean(props.selected) },
]);

function onResizeEnd(data: OnResizeEnd) {
  emit("resize-end", data);
}
</script>

<style scoped>
.is-selected .node-face {
  border-color: var(--color-accent, #38bdf8);
}
</style>
