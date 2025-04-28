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

    <!-- Mode selector dropdown -->
    <div class="node-options mode-selector">
      <label class="select-label">
        <span>Mode:</span>
        <select v-model="mode" @change="updateNodeData" class="mode-select">
          <option value="text">Text</option>
          <option value="template">Template</option>
        </select>
      </label>
    </div>

    <!-- Text input area -->
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
      :placeholder="mode === 'template'
        ? `Paste template + values blocks, e.g.:

--- template:profile ---
Hello {{USER}}, you are {{AGE}} years old.
--- endtemplate ---

--- values:profile ---
USER=Alice
AGE=23
--- endvalues ---`
        : 'Enter text...'"
    ></textarea>

    <!-- Clear on run checkbox -->
    <div class="node-options">
      <label class="checkbox-label">
        <input type="checkbox" v-model="clearOnRun" />
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

    <!-- Node resizer -->
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="380"
      :min-height="380"
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
      clearOnRun: false,
      mode: 'text'
    })
  }
})

const emit = defineEmits(['update:data', 'disable-zoom', 'enable-zoom', 'resize'])

const { disableNodeDrag, enableNodeDrag } = useVueFlow()

const {
  text,
  clearOnRun,
  mode,
  customStyle,
  isHovered,
  resizeHandleStyle,
  updateNodeData,
  onResize
} = useTextNode(props, emit)

// Disable node dragging when interacting with textarea
const onTextareaMouseDown = (e) => {
  e.stopPropagation()
  disableNodeDrag(props.id)
}
const onTextareaMouseUp = () => enableNodeDrag(props.id)
const onTextareaFocus   = () => disableNodeDrag(props.id)
const onTextareaBlur    = () => enableNodeDrag(props.id)
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

.mode-selector {
  margin-bottom: 10px;
  width: calc(100% - 20px);
}

.select-label {
  display: flex;
  align-items: center;
  font-size: 12px;
}

.select-label span { margin-right: 10px; }

.mode-select {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 12px;
  cursor: pointer;
}

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

.node-options {
  display: flex;
  justify-content: flex-start;
  width: calc(100% - 20px);
  margin-bottom: 10px;
}

.checkbox-label {
  display: flex;
  align-items: center;
  font-size: 12px;
  cursor: pointer;
}

.checkbox-label input { margin-right: 5px; }
</style>
