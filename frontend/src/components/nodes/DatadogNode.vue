<template>
  <div
    :style="computedContainerStyle"
    class="node-container datadog-node tool-node flex flex-col w-full h-full p-3 rounded-xl border border-blue-400 bg-zinc-900 text-gray-100 shadow"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>

    <BaseInput
      :id="`${data.id}-api_key`"
      label="API Key"
      v-model="apiKey"
      :type="showApiKey ? 'text' : 'password'"
      class="mb-2"
    >
      <template #suffix>
        <BaseTogglePassword v-model="showApiKey" />
      </template>
    </BaseInput>

    <BaseInput
      :id="`${data.id}-app_key`"
      label="Application Key"
      v-model="appKey"
      :type="showAppKey ? 'text' : 'password'"
      class="mb-2"
    >
      <template #suffix>
        <BaseTogglePassword v-model="showAppKey" />
      </template>
    </BaseInput>

    <BaseSelect
      :id="`${data.id}-site`"
      label="Datadog Site"
      v-model="site"
      :options="siteOptions"
      class="mb-2"
    />

    <BaseSelect
      :id="`${data.id}-operation`"
      label="Operation"
      v-model="operation"
      :options="operationOptions"
      class="mb-2"
    />

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <NodeResizer
      :is-resizable="true"
      :color="'#60a5fa'"
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
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import { useDatadogNode } from '@/composables/useDatadogNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'DatadogNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'DatadogNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        apiKey: '',
        appKey: '',
        site: 'datadoghq.com',
        operation: 'getLogs',
      },
      outputs: {},
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '180px',
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
  apiKey,
  appKey,
  site,
  operation,
  showApiKey,
  showAppKey,
  siteOptions,
  operationOptions,
  onResize
} = useDatadogNode(props, emit, vueFlowInstance)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>