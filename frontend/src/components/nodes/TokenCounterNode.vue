<template>
  <div :style="data.style" class="node-container token-counter-node">
    <!-- Display the node type and the token count -->
    <div :style="data.labelStyle" class="node-label">
      {{ data.type }}: {{ data.tokenCount }}
    </div>

    <!-- Input fields for endpoint and API key -->
    <div class="input-group">
      <label for="endpoint">Endpoint:</label>
      <input
        type="text"
        :id="`${id}-endpoint`"
        v-model="data.inputs.endpoint"
        @change="updateInputData"
        placeholder="Enter endpoint URL"
      />
    </div>
    <div class="input-group">
      <label for="api-key">API Key:</label>
      <input
        type="text"
        :id="`${id}-api-key`"
        v-model="data.inputs.api_key"
        @change="updateInputData"
        placeholder="Enter API key"
      />
    </div>

    <!-- Optional: Input handle if you want to allow connections from other nodes.
         Remove if you truly want zero handles. -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import { onMounted } from 'vue'
import { useTokenCounterNode } from '../../composables/useTokenCounterNode'

/**
 * Define props
 */
const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'TokenCounter_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'TokenCounterNode',
      labelStyle: {
        fontWeight: 'normal',
      },
      // By default, we allow an input handle so this node can receive data
      hasInputs: true,
      hasOutputs: false,
      // Inputs for the node (endpoint, api_key)
      inputs: {
        endpoint: 'http://localhost:32186',
        api_key: '',
      },
      // This node will keep track of the token count
      tokenCount: 0,
    }),
  },
})

// Use the composable
const { updateInputData, run } = useTokenCounterNode(props)

/**
 * Assign the 'run' method if it doesn't exist yet.
 */
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
  // Initialize by calling update. Important for initial load.
  updateInputData()
})
</script>

<style scoped>
.token-counter-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
  --node-label-font-size: 16px;

  border: 1px solid var(--node-border-color);
  background-color: var(--node-bg-color);
  color: var(--node-text-color);
  border-radius: 12px;
  padding: 8px;
}

.node-label {
  font-size: var(--node-label-font-size);
  text-align: center;
  margin-bottom: 4px;
  font-weight: bold;
}

.input-group {
  margin-bottom: 8px; /* Add some spacing between input groups */
}

.input-group label {
  display: block; /* Make labels block-level for better layout */
  margin-bottom: 2px; /* Add spacing between label and input */
  color: #ccc; /* Slightly lighter text color for labels */
}

.input-group input[type="text"] {
  width: calc(100% - 16px); /* Full width, minus padding */
  padding: 4px;
  border: 1px solid #555;
  border-radius: 4px;
  background-color: #333;
  color: #eee;
  box-sizing: border-box; /* Include padding and border in the element's total width and height */
}
</style>