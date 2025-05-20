<template>
  <BaseNode :id="id" :data="data" :min-height="220" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold">{{ data.type }}</div>
    </template>

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

    <Handle style="width:12px;height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px;height:12px" v-if="data.hasOutputs" type="source" position="right" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseNode from '@/components/base/BaseNode.vue'
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
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '360px',
        height: '220px'
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const {
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
} = useWebSearch(props, emit)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>