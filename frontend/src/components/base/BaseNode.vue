<template>
  <div
    ref="containerRef"
    :style="computedContainerStyle"
    class="node-container bg-zinc-900 flex flex-col"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="node-header text-white text-center mb-2 text-lg">
      <slot name="header"></slot>
    </div>
    <div class="flex-1 flex flex-col min-h-0 overflow-visible">
      <slot></slot>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :min-width="minWidth"
      :min-height="minHeight"
      :width="width"
      :height="height"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { NodeResizer } from '@vue-flow/node-resizer'
import { useNodeBase } from '@/composables/useNodeBase'
import { useDisableNodeDragOnInput } from '@/composables/useDisableNodeDragOnInput'

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    default: () => ({ style: {} })
  },
  minWidth: {
    type: Number,
    default: 352
  },
  minHeight: {
    type: Number,
    default: 0
  }
})

const emit = defineEmits(['resize'])

const { isHovered, resizeHandleStyle, computedContainerStyle, width, height, onResize } = useNodeBase(props, emit)
const containerRef = useDisableNodeDragOnInput(props.id)
</script>

<style scoped>

.node-container {
  background-color: oklch(21% 0.006 285.885) !important;
  border: 2px solid oklch(44.6% 0.043 257.281) !important;
  padding: 15px;
  border-radius: 8px;
  overflow: visible !important;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}
</style>
