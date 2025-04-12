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
      @mousedown="onTextareaMouseDown"
      @mouseup="onTextareaMouseUp"
      @focus="onTextareaFocus"
      @blur="onTextareaBlur"
    ></textarea>
    
    <!-- Clear on run checkbox -->
    <div class="node-options">
      <label class="checkbox-label">
        <input type="checkbox" v-model="clearOnRun">
        <span>Clear on run</span>
      </label>
    </div>

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
import { Handle, useVueFlow } from '@vue-flow/core'
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
      outputs: {},
      clearOnRun: false
    })
  }
})

const emit = defineEmits(['update:data', 'disable-zoom', 'enable-zoom', 'resize'])

// Get the Vue Flow instance to control node dragging
const { disableNodeDrag, enableNodeDrag } = useVueFlow()

// Use the composable to get all the reactive state and methods
const {
  text,
  clearOnRun,
  customStyle,
  isHovered,
  resizeHandleStyle,
  updateNodeData,
  onResize
} = useTextNode(props, emit)

// Disable node dragging when interacting with textarea
const onTextareaMouseDown = (event) => {
  event.stopPropagation()
  disableNodeDrag(props.id)
}

const onTextareaMouseUp = () => {
  enableNodeDrag(props.id)
}

const onTextareaFocus = () => {
  disableNodeDrag(props.id)
}

const onTextareaBlur = () => {
  enableNodeDrag(props.id)
}
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

/* Styling for node options */
.node-options {
  display: flex;
  justify-content: flex-start;
  width: calc(100% - 20px);
  margin-bottom: 10px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  color: var(--node-text-color);
  font-size: 12px;
  cursor: pointer;
}

.checkbox-label input {
  margin-right: 5px;
}
</style>
