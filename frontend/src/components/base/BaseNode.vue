<template>
  <div
    :style="computedContainerStyle"
    class="node-container bg-neutral-900"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="node-header">
      <slot name="header"></slot>
    </div>
    <div>
      <slot></slot>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :min-width="352"
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

const props = defineProps({
  id: {
    type: String,
    required: true,
  },
  data: {
    type: Object,
    default: () => ({ style: {} })
  },
  minHeight: {
    type: Number,
    default: 0
  }
})

const emit = defineEmits(['resize'])

const { isHovered, resizeHandleStyle, computedContainerStyle, width, height, onResize } = useNodeBase(props, emit)
</script>

<style scoped>
.node-header {
  cursor: move;
  user-select: none;
}

.node-container {
  background-color: oklch(26.9% 0 0) !important;
  border: 3px solid var(--node-border-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}
</style>
