<template>
  <div :style="data.style" class="node-container tool-node">
    <div class="node-label">
      <div>MCP Client</div>
    </div>
    
    <!-- Server Selection Dropdown -->
    <div class="input-field">
      <label :for="`${data.id}-server`" class="input-label">Server:</label>
      <select
        :id="`${data.id}-server`"
        v-model="selectedServer"
        class="select-input"
        :disabled="isLoadingServers"
      >
        <option value="" disabled>Select a server</option>
        <option v-for="server in servers" :key="server" :value="server">
          {{ server }}
        </option>
      </select>
      <div v-if="isLoadingServers" class="loading-indicator">Loading servers...</div>
    </div>
    
    <!-- Tool Selection Dropdown -->
    <div class="input-field">
      <label :for="`${data.id}-tool`" class="input-label">Tool:</label>
      <select
        :id="`${data.id}-tool`"
        v-model="selectedTool"
        class="select-input"
        :disabled="isLoadingTools || !selectedServer"
      >
        <option value="" disabled>Select a tool</option>
        <option v-for="tool in toolsForServer" :key="tool.name" :value="tool.name">
          {{ tool.name }}
        </option>
      </select>
      <div v-if="isLoadingTools" class="loading-indicator">Loading tools...</div>
    </div>
    
    <!-- Arguments JSON Input -->
    <div class="input-field">
      <label :for="`${data.id}-args`" class="input-label">Arguments (JSON):</label>
      <textarea
        :id="`${data.id}-args`"
        v-model="argsInput"
        class="input-text-area"
        rows="5"
        placeholder="Enter JSON arguments for the selected tool"
      ></textarea>
    </div>
    
    <!-- Error Message Display -->
    <div v-if="errorMessage" class="error-message">
      {{ errorMessage }}
    </div>
    
    <!-- Run Button -->
    <button 
      @click="run"
      :disabled="isLoadingServers || isLoadingTools || !selectedServer || !selectedTool"
      class="run-button"
    >
      Run MCP Tool
    </button>
    
    <!-- Information Message -->
    <div class="info-message">
      <small>
        You can also connect an LLM node that outputs JSON with server, tool, and args properties.
      </small>
    </div>
    
    <!-- Input and Output Handles -->
    <Handle 
      style="width:12px; height:12px" 
      v-if="data.hasInputs" 
      type="target" 
      position="left" 
      id="input" 
    />
    <Handle 
      style="width:12px; height:12px" 
      v-if="data.hasOutputs" 
      type="source" 
      position="right" 
      id="output" 
    />
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
        // We'll store both command and the UI state
        command: '{"server":"","tool":"","args":{}}',
        selectedServer: '',
        selectedTool: '',
        argsInput: '{}'
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

// Import the enhanced composable with all new functionality
const { 
  command,
  servers,
  selectedServer,
  toolsForServer,
  selectedTool,
  argsInput,
  isLoadingServers,
  isLoadingTools,
  errorMessage,
  run 
} = useMCPClient(props, emit, vueFlow)
</script>

<style scoped>
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
  min-width: 250px;
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

.input-label {
  display: block;
  color: var(--node-text-color);
  font-size: 12px;
  margin-bottom: 4px;
}

.input-text-area, .select-input {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  height: auto;
  box-sizing: border-box;
}

.input-text-area {
  resize: vertical;
}

.select-input {
  height: 24px;
}

.error-message {
  color: #ff6b6b;
  font-size: 12px;
  margin: 8px 0;
  padding: 4px;
  background-color: rgba(255, 107, 107, 0.1);
  border-left: 2px solid #ff6b6b;
}

.info-message {
  color: #77bbff;
  font-size: 10px;
  margin: 8px 0;
  text-align: center;
}

.loading-indicator {
  font-size: 10px;
  color: #aaa;
  margin-top: 2px;
}

.run-button {
  width: 100%;
  padding: 6px;
  background-color: #4caf50;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
}

.run-button:disabled {
  background-color: #777;
  cursor: not-allowed;
}
</style>
