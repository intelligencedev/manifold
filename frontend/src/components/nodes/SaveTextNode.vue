<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Filename Input -->
    <div class="input-field">
      <label :for="`${data.id}-filename`" class="input-label">Filename:</label>
      <input
        :id="`${data.id}-filename`"
        type="text"
        v-model="filename"
        @change="updateNodeData"
        class="input-text"
      />
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
  </div>
</template>

<script setup>
import { watch, onMounted } from 'vue';
import { Handle } from '@vue-flow/core';
import { useSaveTextNode } from '@/composables/useSaveTextNode';

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'SaveText_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'saveTextNode',
      labelStyle: {},
      style: {},
      inputs: {
        filename: 'output.md',
        text: '',
      },
      hasInputs: true,
      hasOutputs: false,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
    }),
  },
});

const emit = defineEmits(['update:data']);

// Use the save text node composable
const { filename, updateNodeData, run } = useSaveTextNode(props, emit);

// Watch for prop changes to update component state
watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);

// Set run function on component mount
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
