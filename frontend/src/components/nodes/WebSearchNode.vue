<template>
  <BaseNode :id="id" :data="data" :min-height="160" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>

    <BaseInput
      :id="`${data.id}-query`"
      label="Query"
      v-model="query"
      class="mb-2"
    />

    <BaseInput
      :id="`${data.id}-result_size`"
      label="Result Size"
      type="number"
      v-model.number="resultSize"
      class="mb-2"
    />

    <BaseSelect
      :id="`${data.id}-search_backend`"
      label="Search Backend"
      v-model="searchBackend"
      :options="searchBackendOptions"
      class="mb-2"
    />

    <BaseInput
      v-if="searchBackend === 'sxng'"
      :id="`${data.id}-sxng_url`"
      label="SearXNG URL"
      v-model="sxngUrl"
      class="mb-2"
    />

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import { useWebSearch } from '@/composables/useWebSearch'

const props = defineProps({
  id: { type: String, required: true, default: 'WebSearch_0' },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'WebSearchNode',
      labelStyle: {},
      style: {},
      inputs: {
        query: 'ai news',
        result_size: 1,
        search_backend: 'ddg',
        sxng_url: 'https://searx.be'
      },
      outputs: {
        urls: [],
        result: { output: '' }
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%'
    })
  }
})

const emit = defineEmits(['update:data', 'resize'])

const searchBackendOptions = [
  { value: 'ddg', label: 'DuckDuckGo' },
  { value: 'sxng', label: 'SearXNG' }
]

const {
  query,
  resultSize,
  searchBackend,
  sxngUrl,
  onResize,
  updateNodeData,
  setup
} = useWebSearch(props, emit)

onMounted(() => {
  setup()
})
</script>

<!-- Styling handled by Tailwind and Base components -->
