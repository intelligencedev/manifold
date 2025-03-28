<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container text-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">
      {{ data.type }}
    </div>

    <!-- Text input area with zoom disabled on mouse enter -->
    <textarea
      v-model="text"
      @change="updateNodeData"
      class="input-textarea"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
    ></textarea>

    <Handle
      style="width:12px; height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />

    <!-- Node resizer for adjusting the node dimensions -->
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="150"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import useTextNode from '../../composables/useTextNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'TextNode_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'TextNode',
      labelStyle: {
        fontWeight: 'bold',
        textAlign: 'center',
        marginBottom: '10px',
        fontSize: '16px'
      },
      style: {
        border: '1px solid var(--node-border-color)',
        borderRadius: '12px',
        backgroundColor: 'var(--node-bg-color)',
        color: 'var(--node-text-color)',
        padding: '10px'
      },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        text: ''
      },
      outputs: {}
    })
  }
})

const emit = defineEmits(['update:data', 'disable-zoom', 'enable-zoom', 'resize'])

// Use the composable to get all the reactive state and methods
const {
  text,
  customStyle,
  isHovered,
  resizeHandleStyle,
  updateNodeData,
  onResize
} = useTextNode(props, emit)
</script>

<style scoped>
.node-container {
  display: flex;
  flex-direction: column;
  position: relative;
  box-sizing: border-box;
  justify-content: center;
  align-items: center;
  padding: 10px;
}

.text-node {
  background-color: var(--node-bg-color);
  border: 1px solid var(--node-border-color);
  border-radius: 4px;
  color: var(--node-text-color);
  width: 100%;
  height: 100%;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

/* Consistent styling for input elements */
.input-textarea {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 10px;
  font-size: 14px;
  width: calc(100% - 20px);
  box-sizing: border-box;
  resize: vertical;
  flex-grow: 1;
  margin: 10px 0;
}
</style>
