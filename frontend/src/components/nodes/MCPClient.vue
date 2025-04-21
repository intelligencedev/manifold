<template>
  <div :style="data.style" class="node-container tool-node">
    <div class="node-label">
      <div>MCP Client</div>
    </div>
    
    <!-- Mode Selection -->
    <div class="mode-selector">
      <label class="radio-label">
        <input type="radio" v-model="mode" value="list" :id="`${data.id}-mode-list`" />
        <span>List Tools</span>
      </label>
      <label class="radio-label">
        <input type="radio" v-model="mode" value="execute" :id="`${data.id}-mode-execute`" />
        <span>Execute Tool</span>
      </label>
    </div>
    
    <!-- LIST MODE -->
    <div v-if="mode === 'list'" class="mode-container">
      <div class="input-field">
        <label :for="`${data.id}-server-list`" class="input-label">Server:</label>
        <select
          :id="`${data.id}-server-list`"
          v-model="selectedServer"
          class="select-input"
          :disabled="isLoadingServers"
        >
          <option value="all">All Servers</option>
          <option v-for="server in servers" :key="server" :value="server">
            {{ server }}
          </option>
        </select>
        <div v-if="isLoadingServers" class="loading-indicator">Loading servers...</div>
        <div v-if="isLoadingToolsList" class="loading-indicator">Loading tools list...</div>
      </div>
      
      <!-- List Mode Info -->
      <div class="info-message">
        <small>
          Select a server to list its tools, or "All Servers" to list tools from all available servers.
          <br>
          The tools list will be sent to the output when this node is run.
        </small>
      </div>
    </div>
    
    <!-- EXECUTE MODE -->
    <div v-if="mode === 'execute'" class="mode-container">
      <!-- Server Selection Dropdown -->
      <div class="input-field">
        <label :for="`${data.id}-server-exec`" class="input-label">Server:</label>
        <select
          :id="`${data.id}-server-exec`"
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
      
      <!-- Schema Button & Display -->
      <div class="input-field" v-if="selectedTool">
        <button 
          @click="toggleToolSchema" 
          class="schema-button"
        >
          {{ showToolSchema ? 'Hide Schema' : 'Show Schema' }}
        </button>
        
        <div v-if="showToolSchema" class="schema-container">
          <h4 class="schema-title">{{ currentToolSchema.name }} Schema</h4>
          <div v-if="currentToolSchema.description" class="schema-description">
            {{ currentToolSchema.description }}
          </div>
          <pre class="schema-content">{{ formattedToolSchema }}</pre>
        </div>
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
      
      <!-- Execute Mode Info -->
      <div class="info-message">
        <small>
          Connect an LLM node that outputs JSON with server, tool, and args properties to execute tools.
        </small>
      </div>
    </div>
    
    <!-- Error Message Display -->
    <div v-if="errorMessage" class="error-message">
      {{ errorMessage }}
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
        mode: 'list',
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
  mode,
  command,
  servers,
  selectedServer,
  toolsForServer,
  selectedTool,
  argsInput,
  isLoadingServers,
  isLoadingTools,
  isLoadingToolsList,
  errorMessage,
  showToolSchema,
  currentToolSchema,
  formattedToolSchema,
  run,
  toggleToolSchema
} = useMCPClient(props, emit, vueFlow)
</script>

<style scoped>
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
  min-width: 280px;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.mode-selector {
  display: flex;
  gap: 10px;
  justify-content: center;
  margin-bottom: 12px;
  padding: 4px 0;
  border-bottom: 1px solid #555;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 5px;
  color: var(--node-text-color);
  cursor: pointer;
  font-size: 13px;
}

.radio-label input[type="radio"] {
  cursor: pointer;
}

.mode-container {
  margin-top: 5px;
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

.schema-button {
  width: 100%;
  padding: 6px;
  color: white;
  background-color: #607d8b;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-weight: bold;
}

.schema-button:disabled {
  background-color: #777;
  cursor: not-allowed;
}

.schema-container {
  background-color: #2a2a2a;
  border: 1px solid #666;
  padding: 8px;
  margin-top: 4px;
  border-radius: 4px;
  max-height: 150px;
  overflow-y: auto;
}

.schema-title {
  margin: 0 0 4px 0;
  font-size: 14px;
  color: #4caf50;
}

.schema-description {
  font-size: 12px;
  color: #bbb;
  margin-bottom: 6px;
}

.schema-content {
  font-size: 11px;
  color: #ddd;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
}

code {
  background-color: #333;
  padding: 2px 4px;
  border-radius: 3px;
  font-family: monospace;
}
</style>
