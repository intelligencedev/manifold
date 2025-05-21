<template>
  <div
    :style="computedContainerStyle"
    class="node-container tool-node datadog-graph-node flex flex-col w-full h-full p-3 rounded-xl border border-orange-500 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">
      {{ data.type }}
    </div>

    <Handle style="width:12px; height:12px" type="target" position="left" id="input" />

    <div class="graph-container flex-1" ref="graphContainer"></div>

    <Handle style="width:12px; height:12px" type="source" position="right" id="output" />

    <NodeResizer
      :is-resizable="true"
      :color="'#fd7702'"
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
})

const emit = defineEmits(["update:data", "resize", "disable-zoom", "enable-zoom"])
const vueFlowInstance = useVueFlow()
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  graphContainer,
  onResize
} = useDatadogGraphNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>
