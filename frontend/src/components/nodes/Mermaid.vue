<template>
  <div
    :style="computedContainerStyle"
    class="node-container mermaid-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-green-400 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <div class="mermaid-container flex-1 bg-zinc-800 rounded p-2 mt-2" ref="mermaidContainer"></div>

    <NodeResizer
      :is-resizable="true"
      :color="'#22c55e'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="300"
      :min-height="300"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { useMermaidNode } from "@/composables/useMermaidNode";

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: "MermaidNode_0",
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: "MermaidNode",
      labelStyle: { fontWeight: "normal" },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        mermaidText: "graph TD\n    A[Connect Input] --> B[To Generate Diagram]",
      },
      outputs: {},
      style: {
        border: "1px solid #666",
        borderRadius: "4px",
        backgroundColor: "#222",
        color: "#eee",
        width: "300px",
        height: "300px",
      },
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
  mermaidContainer,
  onResize
} = useMermaidNode(props, emit, vueFlowInstance)
</script>
