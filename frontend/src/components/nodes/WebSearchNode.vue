<template>
  <div
    :style="computedContainerStyle"
    class="flex flex-col w-full h-full p-3 rounded-xl border border-gray-600 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-query`"
      label="Query"
      v-model="query"
      class="mb-2"
      @mousedown="onInputMouseDown"
      @mouseup="onInputMouseUp"
      @focus="onInputFocus"
      @blur="onInputBlur"
    />

    <BaseInput
      :id="`${data.id}-result_size`"
      label="Result Size"
      type="number"
      v-model.number="resultSize"
      class="mb-2"
      @mousedown="onInputMouseDown"
      @mouseup="onInputMouseUp"
      @focus="onInputFocus"
      @blur="onInputBlur"
    />

    <BaseSelect
      :id="`${data.id}-search_backend`"
      label="Search Backend"
      v-model="searchBackend"
      :options="searchBackendOptions"
      class="mb-2"
      @mousedown="onInputMouseDown"
      @mouseup="onInputMouseUp"
      @focus="onInputFocus"
      @blur="onInputBlur"
    />

    <BaseInput
      v-if="searchBackend === 'sxng'"
      :id="`${data.id}-sxng_url`"
      label="SearXNG URL"
      v-model="sxngUrl"
      class="mb-2"
      @mousedown="onInputMouseDown"
      @mouseup="onInputMouseUp"
      @focus="onInputFocus"
      @blur="onInputBlur"
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
import BaseSelect from '@/components/base/BaseSelect.vue'
import { useWebSearch } from '../../composables/useWebSearch'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'WebSearchNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'WebSearchNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        query: '',
        resultSize: 5,
        searchBackend: 'ddg',
        sxngUrl: '',
      },
      outputs: {},
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
  query,
  resultSize,
  searchBackend,
  sxngUrl,
  searchBackendOptions,
  onInputMouseDown,
  onInputMouseUp,
  onInputFocus,
  onInputBlur,
  onResize
} = useWebSearch(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>