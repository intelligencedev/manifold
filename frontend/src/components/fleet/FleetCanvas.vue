<template>
  <!-- Outer wrapper: flex column so the canvas fills available height -->
  <div ref="wrapperEl" class="relative w-full overflow-hidden rounded-xl border border-border bg-[rgb(7_9_12)]" :style="{ height: canvasHeight + 'px' }">

    <!-- WebGL canvas -->
    <canvas
      v-show="webglAvailable"
      ref="canvasEl"
      class="absolute inset-0 block h-full w-full"
      aria-label="Fleet topology graph"
    />

    <!-- DOM fallback when WebGL unavailable -->
    <div v-if="!webglAvailable" class="flex h-full items-center justify-center text-center">
      <div>
        <div class="mb-2 text-3xl" aria-hidden="true">🚫</div>
        <p class="text-sm text-white/50">WebGL unavailable in this browser.<br />Switch to the list view below.</p>
      </div>
    </div>

    <!-- Controls overlay (top-right) -->
    <div class="pointer-events-none absolute right-3 top-3 flex items-center gap-2">
      <div
        class="pointer-events-auto flex items-center gap-1.5 rounded-full border border-border/60 bg-surface/80 px-3 py-1 text-[10px] text-white/50 backdrop-blur"
      >
        <span class="h-1.5 w-1.5 rounded-full" :class="settled ? 'bg-white/20' : 'bg-amber-400/80 animate-pulse'" />
        {{ settled ? 'stable' : 'settling…' }}
      </div>
    </div>

    <!-- Tooltip (HTML overlay, positioned via JS) -->
    <Transition name="tooltip">
      <div
        v-if="tooltip.visible"
        class="pointer-events-none absolute z-20 max-w-[200px] rounded-lg border border-border bg-surface/95 px-3 py-2 text-xs shadow-xl backdrop-blur"
        :style="{ left: tooltip.x + 'px', top: tooltip.y + 'px', transform: 'translate(-50%, -100%)' }"
      >
        <div class="font-semibold text-white/90">{{ tooltip.node?.label }}</div>
        <div v-if="tooltip.node?.sublabel" class="text-white/50">{{ tooltip.node.sublabel }}</div>
        <div class="mt-1 flex items-center gap-1.5">
          <span
            class="inline-block h-1.5 w-1.5 rounded-full"
            :class="{
              'bg-emerald-400': tooltip.node?.status === 'online' || tooltip.node?.status === 'running',
              'bg-amber-400': tooltip.node?.status === 'paused' || tooltip.node?.status === 'idle',
              'bg-red-400': tooltip.node?.status === 'failed',
              'bg-white/20': tooltip.node?.status === 'completed',
            }"
          />
          <span class="capitalize text-white/50">{{ tooltip.node?.kind }} · {{ tooltip.node?.status }}</span>
        </div>
      </div>
    </Transition>

    <!-- Selected node panel (bottom) -->
    <Transition name="slide-up">
      <div
        v-if="selectedNode"
        class="absolute bottom-3 left-3 right-3 z-20 rounded-lg border border-border/60 bg-surface/90 p-3 backdrop-blur"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="text-xs font-semibold text-white/90">{{ selectedNode.label }}</div>
            <div v-if="selectedNode.sublabel" class="mt-0.5 text-[11px] text-white/50">{{ selectedNode.sublabel }}</div>
          </div>
          <div class="flex items-center gap-2 shrink-0">
            <span
              class="rounded-full px-2 py-0.5 text-[10px] font-medium capitalize"
              :class="{
                'bg-emerald-500/20 text-emerald-300': selectedNode.status === 'online' || selectedNode.status === 'running',
                'bg-amber-500/20 text-amber-300': selectedNode.status === 'paused',
                'bg-red-500/20 text-red-300': selectedNode.status === 'failed',
                'bg-white/10 text-white/50': selectedNode.status === 'completed' || selectedNode.status === 'idle',
              }"
            >{{ selectedNode.status }}</span>
            <button
              class="rounded p-1 text-white/30 hover:text-white"
              @click="selectedNode = null"
              aria-label="Dismiss"
            >✕</button>
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted, reactive, ref, watch } from "vue";
import type { GraphEdge, GraphNode } from "./types";
import { FleetRenderer } from "./fleetRenderer";

const props = withDefaults(
  defineProps<{
    nodes: GraphNode[];
    edges: GraphEdge[];
    height?: number;
  }>(),
  { height: 480 }
);

const canvasHeight = props.height;
const wrapperEl = ref<HTMLDivElement>();
const canvasEl = ref<HTMLCanvasElement>();
const webglAvailable = ref(true);
const settled = ref(false);

const tooltip = reactive<{
  visible: boolean;
  x: number;
  y: number;
  node: GraphNode | null;
}>({ visible: false, x: 0, y: 0, node: null });

const selectedNode = ref<GraphNode | null>(null);

let renderer: FleetRenderer | null = null;
let resizeObserver: ResizeObserver | null = null;
let settleTimer: ReturnType<typeof setInterval> | null = null;

onMounted(() => {
  if (!canvasEl.value || !wrapperEl.value) return;

  webglAvailable.value = FleetRenderer.isWebGLAvailable();
  if (!webglAvailable.value) return;

  const w = wrapperEl.value.clientWidth || 800;
  const h = canvasHeight;

  renderer = new FleetRenderer(canvasEl.value, w, h);

  renderer.onNodeHover = (node, sx, sy) => {
    if (node) {
      // Translate screen coords to be relative to the wrapper element
      const rect = wrapperEl.value?.getBoundingClientRect();
      tooltip.visible = true;
      tooltip.node = node;
      tooltip.x = sx - (rect?.left ?? 0);
      tooltip.y = sy - (rect?.top ?? 0) - 10;
    } else {
      tooltip.visible = false;
    }
  };

  renderer.onNodeClick = (node) => {
    selectedNode.value = selectedNode.value?.id === node.id ? null : node;
  };

  // Initial data push
  renderer.update(props.nodes, props.edges);

  // Responsive resize
  resizeObserver = new ResizeObserver((entries) => {
    const entry = entries[0];
    if (entry && renderer) {
      renderer.resize(entry.contentRect.width, canvasHeight);
    }
  });
  resizeObserver.observe(wrapperEl.value);

  // Poll settle state every second
  settleTimer = setInterval(() => {
    if (renderer) settled.value = (renderer as any).layout.isSettled;
  }, 1000);
});

onUnmounted(() => {
  renderer?.destroy();
  renderer = null;
  resizeObserver?.disconnect();
  if (settleTimer) clearInterval(settleTimer);
});

// React to data changes
watch(
  () => [props.nodes, props.edges] as const,
  ([nodes, edges]) => {
    settled.value = false;
    if (selectedNode.value && !nodes.some((node) => node.id === selectedNode.value?.id)) {
      selectedNode.value = null;
    }
    if (tooltip.node && !nodes.some((node) => node.id === tooltip.node?.id)) {
      tooltip.visible = false;
      tooltip.node = null;
    }
    renderer?.update(nodes, edges);
  },
  { deep: true }
);
</script>

<style scoped>
.tooltip-enter-active,
.tooltip-leave-active {
  transition: opacity 120ms ease, transform 120ms ease;
}
.tooltip-enter-from,
.tooltip-leave-to {
  opacity: 0;
  transform: translate(-50%, calc(-100% + 4px));
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: opacity 150ms ease, transform 150ms ease;
}
.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
