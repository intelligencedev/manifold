<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="300"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

    <BaseInput
      :id="`${data.id}-endpoint`"
      label="Endpoint"
      v-model="endpoint"
      class="mb-2"
      @update:modelValue="updateNodeData"
    />

    <BaseTextarea
      :id="`${data.id}-text`"
      label="Text"
      v-model="text"
      class="mb-2"
      @update:modelValue="updateNodeData"
    />

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
import { useConfigStore } from '@/stores/configStore';
import { getApiEndpoint, API_PATHS } from '@/utils/endpoints';

const configStore = useConfigStore();
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
        endpoint: 'http://localhost:8080/api/split-text', // Will be updated dynamically
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
