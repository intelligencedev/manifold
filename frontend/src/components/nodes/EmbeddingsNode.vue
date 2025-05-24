<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-width="200"
    :min-height="120"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold">{{ data.type }}</div>
    </template>

    <BaseInput
      :id="`${data.id}-endpoint`"
      label="Endpoint"
      v-model="embeddings_endpoint"
      class="mb-2"
    />

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import useEmbeddingsNode from '@/composables/useEmbeddingsNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Embeddings_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'EmbeddingsNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        embeddings_endpoint: 'http://<llama.cpp endpoint only>/v1/embeddings',
      },
      outputs: {
        result: { output: '' },
      },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '120px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const {
  embeddings_endpoint,
  onResize
} = useEmbeddingsNode(props, emit)
</script>
