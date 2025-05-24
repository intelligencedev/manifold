<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-width="320" 
    :min-height="220" 
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}
      </div>
    </template>

    <!-- Mode Selection Dropdown -->
    <BaseDropdown label="Mode" v-model="mode" :options="modeOptions" />

    <!-- Conditional Input Field for JumpToNode -->
    <div v-if="mode === 'JumpToNode'">
      <BaseInput label="Target Node ID" :id="`${data.id}-targetNodeId`" type="text" v-model="targetNodeId" />
    </div>

    <!-- Conditional Input Field for ForEachDelimited -->
    <div v-if="mode === 'ForEachDelimited'">
      <BaseInput label="Delimiter" :id="`${data.id}-delimiter`" type="text" v-model="delimiter" placeholder="e.g. ," />
    </div>

    <!-- Conditional Input Field for Wait -->
    <div v-if="mode === 'Wait'">
      <BaseInput label="Wait Time (seconds)" :id="`${data.id}-waitTime`" type="number" v-model="waitTime" min="1" />
    </div>

    <!-- Conditional Input Field for Combine -->
    <div v-if="mode === 'Combine'">
      <label class="block mb-1 text-sm text-gray-200">Combine Mode:</label>
      <div class="flex gap-4 mb-2">
        <label v-for="option in combineModeOptions" :key="option.value" class="flex items-center text-sm text-gray-200">
          <input type="radio" v-model="combineMode" :value="option.value" class="mr-1 focus:ring-blue-500 h-4 w-4 text-blue-600 border-gray-300" />
          <span>{{ option.label }}</span>
        </label>
      </div>
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseDropdown from '@/components/base/BaseDropdown.vue'
import { useFlowControl } from '@/composables/useFlowControl'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'FlowControl_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'FlowControl',
      labelStyle: { fontWeight: 'normal' }, // Kept for now, can be moved to BaseNode or removed if not dynamic
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        mode: 'RunAllChildren',
        targetNodeId: '',
        delimiter: '',
        waitTime: 5,
        combineMode: 'newline',
      },
      outputs: {},
      style: {}, // BaseNode handles styling
    }),
  },
})

const emit = defineEmits(['resize', 'disable-zoom', 'enable-zoom'])

const {
  mode,
  targetNodeId,
  delimiter,
  waitTime,
  combineMode,
  modeOptions,
  combineModeOptions,
  isHovered, // from useNodeBase via useFlowControl
  resizeHandleStyle, // from useNodeBase via useFlowControl
  computedContainerStyle, // from useNodeBase via useFlowControl
  handleTextareaMouseEnter,
  handleTextareaMouseLeave,
  run, // Core run function
  onResize // from useNodeBase via useFlowControl
} = useFlowControl(props, emit)

// Ensure run function is assigned to node data (already handled in composable)
// Watchers for props.data.inputs and props.data.style also handled in composable if needed

</script>

<style scoped>
/* Scoped styles are removed as TailwindCSS is used via base components */
/* Any specific overrides or very component-specific styles can remain if necessary */
</style>
