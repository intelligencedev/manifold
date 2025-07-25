<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-width="320"
    :min-height="240"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label text-base font-semibold">{{ data.type }}</div>
    </template>
    
    <div class="node-options mode-selector mb-2">
      <label class="select-label flex items-center gap-2">
        <span>Mode:</span>
        <select v-model="mode" @change="updateNodeData" class="mode-select bg-zinc-800 border border-gray-600 rounded px-2 py-1 text-sm">
          <option value="text">Text</option>
          <option value="template">Template</option>
        </select>
      </label>
    </div>

    <BaseTextarea
      v-model="text"
      @change="updateNodeData"
      label="Text"
      class="input-textarea mb-2 flex-1"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
      @mousedown="onTextareaMouseDown"
      @mouseup="onTextareaMouseUp"
      @focus="onTextareaFocus"
      @blur="onTextareaBlur"
      :placeholder="mode === 'template' ? `Paste template + values blocks, e.g.:\n\n--- template:profile ---\nHello {{USER}}, you are {{AGE}} years old.\n--- endtemplate ---\n\n--- values:profile ---\nUSER=Alice\nAGE=23\n--- endvalues ---` : 'Enter text...'"
    />

    <div class="node-options mb-2">
      <label class="checkbox-label flex items-center gap-2">
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
  </BaseNode>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import useTextNode from '@/composables/useTextNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'TextNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'TextNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        text: '',
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
  mode,
  text,
  clearOnRun,
  updateNodeData,
  onTextareaMouseDown,
  onTextareaMouseUp,
  onTextareaFocus,
  onTextareaBlur,
  onResize
} = useTextNode(props, emit, vueFlowInstance)
</script>
