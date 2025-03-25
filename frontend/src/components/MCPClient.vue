<template>
  <div :style="data.style" class="node-container tool-node">
    <div class="node-label">
      <div>MCP Client</div>
    </div>

    <!-- Run button -->
    <button @click="run">Run MCP Client</button>

    <!-- Output handle for VueFlow -->
    <Handle style="width:12px; height:12px" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core';
import { onMounted } from 'vue';

// Define component props.  VERY simplified.
const props = defineProps({
  id: {
    type: String,
    required: false,
    default: 'MCP_Client_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      style: {},
      type: 'MCPClientNode',
      outputs: {}, // Only outputs are needed
      hasOutputs: true,
    })
  }
});

// Emit updated data to VueFlow.
const emit = defineEmits(['update:data']);

// Assign the run() function on mount.
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run;
  }
  updateNodeData(); // Initial update
});

/**
 * EXTREMELY simplified run method.  Hardcoded endpoint.
 */
async function run() {
  try {
    // Clear previous output.
    props.data.outputs.result = '';

    const endpoint = '/v1/tool/list'; // HARDCODED ENDPOINT

    const response = await fetch(endpoint, {
      method: 'GET', // Assuming a GET request for listing tools
    });

    if (!response.ok) {
      const errorMsg = await response.text();
      console.error('Error response from server:', errorMsg);
      props.data.outputs.result = { output: `Error: ${errorMsg}` };
      return { error: errorMsg };
    }

    const result = await response.json();
     const resultStr =  JSON.stringify(result, null, 2);
    props.data.outputs = {
      result: { output: resultStr } // Store the JSON result directly
    };
    updateNodeData();
    return { output: resultStr }; // Return for downstream nodes

  } catch (error) {
    console.error('Error in run():', error);
    props.data.outputs.result = { output: `Error: ${error.message}` };
    return { error: error.message };
  }
}

// Update node data and emit to Vue Flow.
function updateNodeData() {
  emit('update:data', props.data); // Emit the entire data object
}
</script>

<style scoped>
/* Styles (keep only what's necessary) */
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}
</style>