<template>
  <div ref="wrapper" class="warpp-canvas">
    <VueFlow
      :nodes="editor.flowNodes"
      :edges="editor.flowEdges"
      :node-types="nodeTypes"
      :nodes-draggable="!locked"
      fit-view-on-init
      @connect="onConnect"
      @node-drag-stop="onDragStop"
      @node-click="onNodeClick"
      @edge-double-click="onEdgeDoubleClick"
      @dragover="onDragOver"
      @drop="onDrop"
    >
      <Background
        variant="dots"
        :gap="20"
        :size="1.4"
        color="rgb(var(--color-subtle-foreground) / 0.45)"
      />

      <Panel position="bottom-left">
        <div
          class="flex items-center gap-1 rounded-md border border-border bg-surface p-1"
        >
          <button
            type="button"
            class="warpp-ctl"
            aria-label="Zoom in"
            title="Zoom in"
            @click="onZoomIn"
          >
            <ZoomInIcon class="h-4 w-4" />
          </button>
          <button
            type="button"
            class="warpp-ctl"
            aria-label="Zoom out"
            title="Zoom out"
            @click="onZoomOut"
          >
            <ZoomOutIcon class="h-4 w-4" />
          </button>
          <button
            type="button"
            class="warpp-ctl"
            aria-label="Fit view"
            title="Fit view"
            @click="onFitView"
          >
            <FullScreenIcon class="h-4 w-4" />
          </button>
          <span class="mx-0.5 h-5 w-px bg-border/60" aria-hidden="true"></span>
          <button
            type="button"
            class="warpp-ctl"
            :aria-pressed="locked"
            :aria-label="locked ? 'Unlock node positions' : 'Lock node positions'"
            :title="locked ? 'Unlock node positions' : 'Lock node positions'"
            @click="locked = !locked"
          >
            <LockedIcon v-if="locked" class="h-4 w-4" />
            <UnlockedIcon v-else class="h-4 w-4" />
          </button>
        </div>
      </Panel>

      <MiniMap
        v-if="showMiniMap"
        class="flow-minimap rounded-md border border-border bg-surface p-1"
        :position="'bottom-right'"
        :pannable="true"
        :zoomable="true"
        :width="180"
        :height="120"
        :mask-color="'rgb(var(--color-surface) / 0.85)'"
        :mask-stroke-color="'rgb(var(--color-border) / 0.7)'"
        :mask-stroke-width="1"
        :node-color="miniMapNodeColor"
        :node-border-radius="6"
        :node-stroke-width="1.5"
      />
      <Panel v-if="showMiniMap" position="bottom-right">
        <button
          type="button"
          class="inline-flex h-6 w-6 items-center justify-center rounded-md border border-border bg-surface text-subtle-foreground hover:text-foreground"
          aria-label="Hide minimap"
          title="Hide minimap"
          @click="showMiniMap = false"
        >
          ×
        </button>
      </Panel>
      <Panel v-else position="bottom-right">
        <button
          type="button"
          class="inline-flex items-center justify-center rounded-md border border-border bg-surface p-1.5 text-subtle-foreground hover:text-foreground"
          aria-label="Show minimap"
          title="Show minimap"
          @click="showMiniMap = true"
        >
          <MapShowIcon class="h-5 w-5 -scale-x-100" />
        </button>
      </Panel>
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { markRaw, ref } from "vue";
import {
  VueFlow,
  useVueFlow,
  Panel,
  type Connection,
  type NodeMouseEvent,
  type EdgeMouseEvent,
  type NodeDragEvent,
  type GraphNode,
} from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { MiniMap } from "@vue-flow/minimap";
import NodeCard from "./NodeCard.vue";
import ZoomInIcon from "@/components/icons/ZoomIn.vue";
import ZoomOutIcon from "@/components/icons/ZoomOut.vue";
import FullScreenIcon from "@/components/icons/FullScreen.vue";
import LockedIcon from "@/components/icons/LockedBold.vue";
import UnlockedIcon from "@/components/icons/UnlockedBold.vue";
import MapShowIcon from "@/components/icons/MapShow.vue";
import { assignable, portColor } from "@/lib/warppTypes";
import { useWarppEditor } from "@/stores/warppEditor";

const DRAG_TYPE = "application/warpp-node-type";

const editor = useWarppEditor();
const nodeTypes = { warpp: markRaw(NodeCard) };
const wrapper = ref<HTMLElement | null>(null);
const locked = ref(false);
const showMiniMap = ref(false);

const { project, zoomIn, zoomOut, fitView } = useVueFlow();

function onZoomIn(): void {
  zoomIn();
}

function onZoomOut(): void {
  zoomOut();
}

function onFitView(): void {
  fitView({ padding: 0.15 });
}

function miniMapNodeColor(node: GraphNode): string {
  const type = String(node.data?.node?.type ?? "");
  const cat = type.split(".")[0];
  const colors: Record<string, string> = {
    data: "#c792ea",
    logic: "#ffb46f",
    control: "#62d2c5",
    llm: "#6fb3ff",
    tool: "#8bd17c",
    flow: "#9aa4b2",
  };
  return colors[cat] ?? "#9aa4b2";
}

function portType(
  nodeType: string | undefined,
  port: string,
  dir: "inputs" | "outputs",
): string | undefined {
  if (!nodeType) return undefined;
  const m = editor.manifestByType(nodeType);
  return m?.[dir].find((p) => p.name === port)?.type;
}

function onConnect(conn: Connection): void {
  const src = editor.nodeAtPath(conn.source);
  const dst = editor.nodeAtPath(conn.target);
  const from = portType(src?.type, conn.sourceHandle ?? "", "outputs");
  const to = portType(dst?.type, conn.targetHandle ?? "", "inputs");
  const coercions = editor.catalog?.coercions ?? [];
  if (!from || !to || !assignable(from, to, coercions)) return;
  editor.wire(
    conn.source,
    conn.sourceHandle ?? "",
    conn.target,
    conn.targetHandle ?? "",
  );
}

function onDragStop(e: NodeDragEvent): void {
  editor.setPosition(e.node.id, e.node.position.x, e.node.position.y);
}

function onNodeClick(e: NodeMouseEvent): void {
  editor.selectedPath = e.node.id;
}

function onEdgeDoubleClick(e: EdgeMouseEvent): void {
  editor.unwire(e.edge.target, e.edge.targetHandle ?? "");
}

function onDragOver(event: DragEvent): void {
  if (!event.dataTransfer?.types.includes(DRAG_TYPE)) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = "copy";
}

function onDrop(event: DragEvent): void {
  if (!event.dataTransfer?.types.includes(DRAG_TYPE)) return;
  event.preventDefault();
  const type = event.dataTransfer.getData(DRAG_TYPE);
  if (!type || !wrapper.value) return;
  const bounds = wrapper.value.getBoundingClientRect();
  const position = project({
    x: event.clientX - bounds.left,
    y: event.clientY - bounds.top,
  });
  // Drop into a selected Map node if one is active; otherwise root scope.
  const parent =
    editor.selectedPath &&
    editor.nodeAtPath(editor.selectedPath)?.type === "control.map"
      ? editor.selectedPath
      : undefined;
  const path = editor.addNode(type, position, parent);
  if (path) editor.selectedPath = path;
}
</script>

<style scoped>
.warpp-canvas {
  width: 100%;
  height: 100%;
}
.warpp-ctl {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 0.25rem;
  padding: 0.5rem;
  color: rgb(var(--color-subtle-foreground));
}
.warpp-ctl:hover {
  background: rgb(var(--color-surface-muted) / 0.8);
  color: rgb(var(--color-foreground));
}
</style>
