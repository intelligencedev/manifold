<template>
  <div
    :style="computedContainerStyle"
    class="node-container embeddings-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-endpoint`"
      label="Endpoint"
      v-model="embeddings_endpoint"
      class="mb-2"
    />

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="120"
      :width="200"
      :height="120"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'

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
const vueFlowInstance = useVueFlow()
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  embeddings_endpoint,
  onResize
} = useEmbeddingsNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>