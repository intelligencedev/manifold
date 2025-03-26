<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Filename Input -->
    <div class="input-field">
      <label :for="`${data.id}-filepath`" class="input-label">Filepath:</label>
      <input
        :id="`${data.id}-filepath`"
        type="text"
        v-model="filepath"
        @change="updateNodeData"
        class="input-text"
      />
    </div>
    
    <!-- Checkbox to enable/disable updating input from a connected source -->
    <div class="input-field">
        <input
          type="checkbox"
          :id="`${data.id}-update-from-source`"
          v-model="updateFromSource"
          @change="updateNodeData"
        />
        <label :for="`${data.id}-update-from-source`" class="input-label">
          Update Input from Source
        </label>
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { watch, onMounted } from 'vue';
import { Handle } from '@vue-flow/core';
import { useOpenFileNode } from '@/composables/useOpenFileNode';

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'OpenFile_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'openFileNode',
      labelStyle: {},
      style: {},
      inputs: {
        filepath: 'input.txt',
        text: '',
      },
      outputs: {
        result: { output: '' }  // Initialize with empty result
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
      updateFromSource: true,
    }),
  },
});

const emit = defineEmits(['update:data']);

// Use the open file node composable
const { filepath, updateFromSource, updateNodeData, run } = useOpenFileNode(props, emit);

watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run;
  }
});
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

.input-text {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.handle-input,
.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}
</style>