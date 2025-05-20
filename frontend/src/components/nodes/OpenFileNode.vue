<template>
  <div
    :style="computedContainerStyle"
    class="node-container open-file-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-filepath`"
      label="Filepath"
      v-model="filepath"
      class="mb-2"
      @change="updateNodeData"
    />

    <div class="flex items-center gap-2 mb-2">
      <input
        type="checkbox"
        :id="`${data.id}-update-from-source`"
        v-model="updateFromSource"
        @change="updateNodeData"
      />
      <label :for="`${data.id}-update-from-source`" class="input-label text-sm">Update Input from Source</label>
    </div>

    <div v-if="isImage || data.isImage" class="image-preview mb-2">
      <template v-if="data.outputs?.result?.slices">
        <div v-for="(slice, index) in data.outputs.result.slices" :key="index" class="image-container mb-1">
          <img :src="slice.dataUrl" :alt="`Image slice ${index + 1}`" class="rounded border border-gray-700 max-w-full max-h-40" />
        </div>
      </template>
      <template v-else-if="data.outputs?.result?.dataUrl">
        <div class="image-container">
          <img :src="data.outputs.result.dataUrl" alt="Image preview" class="rounded border border-gray-700 max-w-full max-h-40" />
        </div>
      </template>
      <template v-else>
        <div class="empty-image-container text-xs text-gray-400">Image will appear here when loaded</div>
      </template>
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="240"
      :min-height="120"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'
import { useOpenFileNode } from '@/composables/useOpenFileNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'OpenFileNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'OpenFileNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        filepath: '',
        updateFromSource: false,
      },
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '240px',
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
  filepath,
  updateFromSource,
  isImage,
  updateNodeData,
  onResize
} = useOpenFileNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>