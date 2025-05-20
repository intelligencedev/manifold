<template>
  <div
    :style="computedContainerStyle"
    class="node-container text-splitter-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-endpoint`"
      label="Endpoint"
      v-model="endpoint"
      class="mb-2"
    />

    <BaseTextarea
      :id="`${data.id}-text`"
      label="Text"
      v-model="text"
      class="mb-2"
    />

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="320"
      :min-height="180"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
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
      style: {},
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
const vueFlowInstance = useVueFlow()
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  endpoint,
  text,
  updateNodeData,
  run,
  onResize
} = useTextSplitterNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>