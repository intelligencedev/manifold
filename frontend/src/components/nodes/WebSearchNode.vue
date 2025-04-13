<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    
    <!-- Query Input -->
    <div class="input-field">
      <label :for="`${data.id}-query`" class="input-label">Query:</label>
      <input 
        :id="`${data.id}-query`" 
        type="text" 
        v-model="query" 
        @change="updateNodeData" 
        class="input-text"
        @mousedown="onInputMouseDown"
        @mouseup="onInputMouseUp"
        @focus="onInputFocus"
        @blur="onInputBlur"
      />
    </div>

    <!-- Result Size Input -->
    <div class="input-field">
      <label :for="`${data.id}-result_size`" class="input-label">Result Size:</label>
      <input
        :id="`${data.id}-result_size`"
        type="number"
        v-model.number="resultSize"
        @change="updateNodeData"
        class="input-number"
        @mousedown="onInputMouseDown"
        @mouseup="onInputMouseUp"
        @focus="onInputFocus"
        @blur="onInputBlur"
      />
    </div>

    <!-- Search Backend Selection -->
    <div class="input-field">
      <label :for="`${data.id}-search_backend`" class="input-label">Search Backend:</label>
      <select
        :id="`${data.id}-search_backend`"
        v-model="searchBackend"
        @change="updateNodeData"
        class="input-select"
        @mousedown="onInputMouseDown"
        @mouseup="onInputMouseUp"
        @focus="onInputFocus"
        @blur="onInputBlur"
      >
        <option value="ddg">DuckDuckGo</option>
        <option value="sxng">SearXNG</option>
      </select>
    </div>

    <!-- SXNG URL Input (Conditional) -->
    <div v-if="searchBackend === 'sxng'" class="input-field">
      <label :for="`${data.id}-sxng_url`" class="input-label">SearXNG URL:</label>
      <input
        :id="`${data.id}-sxng_url`"
        type="text"
        v-model="sxngUrl"
        @change="updateNodeData"
        class="input-text"
        @mousedown="onInputMouseDown"
        @mouseup="onInputMouseUp"
        @focus="onInputFocus"
        @blur="onInputBlur"
      />
    </div>

    <!-- Input Handle (Optional) -->
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasInputs"
      type="target"
      :position="Position.Left"
      id="input"
    />

    <!-- Output Handle -->
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasOutputs"
      type="source"
      :position="Position.Right"
      id="output"
    />
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import { Handle, Position, useVueFlow } from '@vue-flow/core'
import { useWebSearch } from '../../composables/useWebSearch'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'WebSearch_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'WebSearchNode',
      labelStyle: {},
      style: {},
      inputs: {
        query: 'ai news',
        result_size: 1,
        search_backend: 'ddg',
        sxng_url: 'https://searx.be',
      },
      outputs: {
        urls: [],
        result: {
          output: '',
        }
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%',
    }),
  },
})

const emit = defineEmits(['update:data'])

// Get the Vue Flow instance to control node dragging
const { disableNodeDrag, enableNodeDrag } = useVueFlow()

// Use the composable
const {
  query,
  resultSize,
  searchBackend,
  sxngUrl,
  updateNodeData,
  setup
} = useWebSearch(props, emit)

// Initialize the node
onMounted(() => {
  setup()
})

// Disable node dragging when interacting with input fields
const onInputMouseDown = (event) => {
  event.stopPropagation()
  disableNodeDrag(props.id)
}

const onInputMouseUp = () => {
  enableNodeDrag(props.id)
}

const onInputFocus = () => {
  disableNodeDrag(props.id)
}

const onInputBlur = () => {
  enableNodeDrag(props.id)
}
</script>

<style scoped>
.node-container {
  border: 3px solid var(--node-border-color) !important;
  background-color: var(--node-bg-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

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

.input-text,
.input-number,
.input-select {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-number {
  width: 60px;
}

.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}
</style>