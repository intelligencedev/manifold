<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="180"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

        <!-- Endpoint Input -->
        <div class="input-field">
            <label :for="`${data.id}-endpoint`" class="input-label">Endpoint:</label>
            <input :id="`${data.id}-endpoint`" type="text" v-model="endpoint" @change="updateNodeData"
                class="input-text" />
        </div>

        <!-- Text Input -->
        <div class="input-field">
            <label :for="`${data.id}-text`" class="input-label">Text:</label>
            <textarea :id="`${data.id}-text`" v-model="text" @change="updateNodeData" class="input-textarea"></textarea>
        </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import { useTextSplitterNode } from '@/composables/useTextSplitterNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'TextSplitter_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'textSplitterNode',
      labelStyle: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '180px'
      },
      inputs: {
        endpoint: 'http://localhost:8080/api/split-text',
        text: '',
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const {
  endpoint,
  text,
  onResize
} = useTextSplitterNode(props, emit)
</script>

<style scoped>
/* Same styles as other tool nodes */
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