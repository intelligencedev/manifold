<template>
  <div
    :style="computedContainerStyle"
    class="node-container save-text-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-filename`"
      label="Filename"
      v-model="filename"
      class="mb-2"
    />

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="80"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
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
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const vueFlowInstance = useVueFlow()
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  filename,
  onResize
} = useSaveTextNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>
