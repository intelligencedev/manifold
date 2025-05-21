<template>
  <BaseNode :id="id" :data="data" :min-height="160" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label font-semibold text-base text-white text-center mb-2">
        {{ data.type }}: {{ tokenCount }}
      </div>
    </template>

    <BaseInput
      :id="`${id}-endpoint`"
      label="Endpoint"
      v-model="endpoint"
      class="mb-2"
    />
    <BaseInput
      :id="`${id}-api-key`"
      label="API Key"
      v-model="api_key"
      class="mb-2"
    />

    <Handle
      style="width:12px;height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
    />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import { onMounted } from 'vue'
import { useTokenCounterNode } from '@/composables/useTokenCounterNode'

/**
 * Define props
 */
const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'TokenCounter_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'TokenCounterNode',
      labelStyle: {
        fontWeight: 'normal',
      },
      // By default, we allow an input handle so this node can receive data
      hasInputs: true,
      hasOutputs: false,
      // Inputs for the node (endpoint, api_key)
      inputs: {
        endpoint: 'http://localhost:32186',
        api_key: '',
      },
      // This node will keep track of the token count
      tokenCount: 0,
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '160px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize'])

const {
  endpoint,
  api_key,
  tokenCount,
  onResize,
  updateInputData,
  run
} = useTokenCounterNode(props, emit)

/**
 * Assign the 'run' method if it doesn't exist yet.
 */
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
  // Initialize by calling update. Important for initial load.
  updateInputData()
})
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>
