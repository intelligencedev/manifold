<template>
  <div :style="data.style" class="node-container tool-node">
    <div class="node-label">
      <div>MCP Client</div>
    </div>

    <!-- Textarea for JSON configuration -->
    <div class="input-field">
      <label :for="`${data.id}-config`" class="input-label">Configuration:</label>
      <textarea
        :id="`${data.id}-config`"
        v-model="command"
        class="input-text-area"
        rows="5"
      ></textarea>
    </div>

    <!-- Run button to trigger the MCP client execution -->
    <button @click="run">Run MCP Client</button>

    <!-- Input and output handles for VueFlow -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { defineProps, defineEmits } from 'vue'
import { useMCPClient } from '../../composables/useMCPClient'

const props = defineProps({
  id: {
    type: String,
    default: 'MCP_Client_0'
  },
  data: {
    type: Object,
    default: () => ({
      style: {},
      type: 'MCPClientNode',
      inputs: {
        // Default configuration payload as a JSON string
        command: '{"action": "listTools"}'
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleColor: '#777'
    })
  }
})

const emit = defineEmits(['update:data'])
const vueFlow = useVueFlow()

// Import the composable which now contains the run() logic, command computed, etc.
const { command, run } = useMCPClient(props, emit, vueFlow)
</script>

<style scoped>
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

.input-field {
  margin-bottom: 8px;
}

.input-text-area {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  height: auto;
  box-sizing: border-box;
  resize: vertical;
}
</style>
