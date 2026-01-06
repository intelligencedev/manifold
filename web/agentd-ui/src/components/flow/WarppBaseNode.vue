<template>
  <div
    class="relative w-full flip-root text-xs text-muted-foreground overflow-visible"
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

    <!-- Flip card wrapper with handles anchored for connector alignment -->
    <div class="relative h-full w-full" :class="flipCardClasses">
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

      <!-- Front face -->
      <div
        class="flip-face flip-front rounded-lg border border-border/60 bg-surface/90 p-3 shadow-lg"
      >
        <slot name="front" />
      </div>

      <!-- Back face -->
      <div
        class="flip-face flip-back rounded-lg border border-border/60 bg-surface/90 p-3 shadow-lg"
      >
        <slot name="back" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, type CSSProperties } from "vue";
import { Handle, Position } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import type { OnResizeEnd } from "@vue-flow/node-resizer";

defineOptions({ name: "WarppBaseNode" });

const props = defineProps<{
  collapsed?: boolean;
  minWidth?: number;
  minHeight?: number;
  minWidthPx?: string;
  minHeightPx?: string;
  showResizer?: boolean;
  showBack?: boolean;
  rootClass?: string | string[] | Record<string, boolean>;
  flipCardClass?: string | string[] | Record<string, boolean>;
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

const flipCardClasses = computed(() => [
  "flip-card",
  { "is-flipped": Boolean(props.showBack) },
  props.flipCardClass,
]);

function onResizeEnd(data: OnResizeEnd) {
  emit("resize-end", data);
}
</script>

<style scoped>
.flip-root {
  perspective: 800px;
}
.flip-card {
  display: grid;
  transform-style: preserve-3d;
  transition: transform 200ms ease;
}
.flip-face {
  grid-area: 1 / 1;
  backface-visibility: hidden;
  transform-origin: center;
  transform-style: preserve-3d;
  transition:
    transform 200ms ease,
    opacity 200ms ease;
}
.flip-front {
  transform: rotateX(0deg);
  opacity: 1;
}
.flip-back {
  transform: rotateX(180deg);
  opacity: 0;
  pointer-events: none;
}
.flip-card.is-flipped .flip-front {
  transform: rotateX(180deg);
  opacity: 0;
  pointer-events: none;
}
.flip-card.is-flipped .flip-back {
  transform: rotateX(0deg);
  opacity: 1;
  pointer-events: auto;
}
.is-selected .flip-face {
  border-color: var(--color-accent, #38bdf8);
}
</style>
