<template>
  <div
    :style="computedContainerStyle"
    class="node-container base-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="node-header">
      <slot name="header"></slot>
    </div>
    <div class="node-body">
      <slot></slot>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
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
  }
})

const emit = defineEmits(['resize'])

const { isHovered, resizeHandleStyle, computedContainerStyle, onResize } = useNodeBase(props, emit)
</script>

<style scoped>
.node-header {
  cursor: move;
  user-select: none;
}
</style>
