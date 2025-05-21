<template>
  <BaseNode :id="id" :data="data" :min-height="80" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

    <BaseInput
      :id="`${data.id}-filename`"
      label="Filename"
      v-model="filename"
      class="mb-2"
    />

    <Handle
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
      style="width:12px;height:12px"
    />
    <Handle
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
      style="width:12px;height:12px"
    />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import { useSaveTextNode } from '@/composables/useSaveTextNode'

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

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

const {
  filename,
  onResize
} = useSaveTextNode(props, emit)
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
