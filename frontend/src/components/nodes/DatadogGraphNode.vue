<template>

  <div
    :style="{
      ...data.style,
      width: '100%',
      height: '100%',
      boxSizing: 'border-box',
    }"
    class="node-container tool-node datadog-graph-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">
      {{ data.type }}
    </div>

    <Handle style="width:12px; height:12px" type="target" position="left" id="input" />

    <div class="graph-container" ref="graphContainer"></div>

    <Handle style="width:12px; height:12px" type="source" position="right" id="output"/>

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="120"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { watch, nextTick } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { useDatadogGraphNode } from "@/composables/useDatadogGraphNode";

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    required: true,
    default: () => ({
      type: "DatadogGraphNode",
      style: {
        width: "400px",
        height: "300px",
        backgroundColor: "#1e1e1e",
        border: "2px solid #fd7702",
      },
      inputs: {},
      outputs: {},
    }),
  },
});

// Use the composable to manage state and functionality
const vueFlowInstance = useVueFlow();
const {
  isHovered,
  graphContainer,
  resizeHandleStyle,
  run,
  onResize
} = useDatadogGraphNode(props, vueFlowInstance);

// Watch for data changes
watch(
  () => props.data.inputs,
  (newVal) => {
    console.log("[DatadogGraphNode] data.inputs changed:", newVal);
    if (newVal?.result) {
      nextTick(() => {
        run();
      });
    }
  },
  { deep: true }
);
</script>

<style scoped>
.datadog-graph-node {
  padding: 10px;
  border-radius: 12px;
  background-color: #1e1e1e;
  color: #eee;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.node-label {
  color: #eee;
  font-size: 14px;
  text-align: center;
  margin-bottom: 8px;
}

.graph-container {
  width: 100%;
  height: calc(100% - 30px);
  overflow: hidden;
}

/* Use :deep for styling D3's appended elements */
:deep(svg) {
  width: 100%;
  height: 100%;
}

:deep(.domain),
:deep(.tick line) {
  stroke: #666;
}

:deep(.tick text) {
  fill: #999;
}
</style>
