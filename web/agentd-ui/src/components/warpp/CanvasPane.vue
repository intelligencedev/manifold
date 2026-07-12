<template>
  <div class="warpp-canvas">
    <VueFlow
      :nodes="editor.flowNodes"
      :edges="editor.flowEdges"
      :node-types="nodeTypes"
      fit-view-on-init
      @connect="onConnect"
      @node-drag-stop="onDragStop"
      @node-click="onNodeClick"
      @edge-double-click="onEdgeDoubleClick"
    >
      <Background />
      <MiniMap />
      <Controls />
    </VueFlow>
  </div>
</template>

<script setup lang="ts">
import { markRaw } from "vue";
import {
  VueFlow,
  type Connection,
  type NodeMouseEvent,
  type EdgeMouseEvent,
  type NodeDragEvent,
} from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { MiniMap } from "@vue-flow/minimap";
import { Controls } from "@vue-flow/controls";
import NodeCard from "./NodeCard.vue";
import { assignable } from "@/lib/warppTypes";
import { useWarppEditor } from "@/stores/warppEditor";

const editor = useWarppEditor();
const nodeTypes = { warpp: markRaw(NodeCard) };

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
  const edge = e.edge;
  editor.unwire(edge.target, edge.targetHandle ?? "");
}
</script>

<style scoped>
.warpp-canvas {
  width: 100%;
  height: 100%;
}
</style>
