<!-- /src/components/WebRetrievalNode.vue -->
<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- URLs Input -->
    <div class="input-field">
      <label :for="`${data.id}-url`" class="input-label">URLs (comma-separated):</label>
      <textarea
        :id="`${data.id}-url`"
        v-model="urls"
        @change="updateNodeData"
        class="input-textarea"
      ></textarea>
    </div>

    <!-- Input Handle (Optional Connection) -->
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

<script setup lang="ts">
import { onMounted } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import { useWebRetrieval } from '../composables/useWebRetrieval'

// ----- Define props & emits -----
const props = defineProps({
  id: {
    type: String,
    required: false,
    default: 'WebRetrieval_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'WebRetrievalNode',
      labelStyle: {},
      style: {},
      inputs: {
        url: 'https://en.wikipedia.org/wiki/Singularity_theory',
      },
      outputs: {},
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

// Use the composable
const { urls, updateNodeData, setup } = useWebRetrieval(props, emit)

// Initialize the node
onMounted(() => {
  setup()
})
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

.input-textarea {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
  min-height: 60px;
}

.handle-input,
.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}
</style>
