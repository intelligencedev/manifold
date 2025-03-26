<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container mermaid-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Input and Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
        
    <!-- Container for Mermaid diagram -->
    <div class="mermaid-container" ref="mermaidContainer"></div>

    <!-- Resizer Component -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :min-width="300" :min-height="300" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from "@vue-flow/core";
import { NodeResizer } from "@vue-flow/node-resizer";
import { useMermaidNode } from "@/composables/useMermaidNode";

// --- PROPS & DEFAULTS ---
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
});
const emit = defineEmits(["update:data", "resize"]);

// Get Vue Flow instance
const vueFlowInstance = useVueFlow();

// Use the composable to manage state and functionality
const {
  // Refs
  mermaidContainer,
  customStyle,
  isHovered,
  
  // Computed properties
  resizeHandleStyle,
  
  // Methods
  onResize
} = useMermaidNode(props, emit, vueFlowInstance);
</script>

<style scoped>
.mermaid-node {
  background-color: #222;
  border: 1px solid #666;
  border-radius: 4px;
  color: #eee;
  display: flex;
  flex-direction: column;
  position: relative;
}

.node-label {
  text-align: center;
  font-size: 16px;
  margin-bottom: 5px;
  padding: 5px;
}

.mermaid-container {
  flex-grow: 1;
  position: relative;
  overflow: hidden;
}
</style>