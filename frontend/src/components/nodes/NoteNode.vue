<template>
  <div
    :style="computedContainerStyle"
    class="node-container note-node flex flex-col w-full h-full p-3 rounded-xl border border-yellow-400 bg-yellow-100 text-gray-900 shadow relative"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="absolute top-2 right-2 flex gap-2 z-10">
      <button class="color-cycle-btn px-2 py-1 rounded bg-yellow-300 hover:bg-yellow-400 text-xs" @click="cycleColor" title="Change color">🎨</button>
      <div class="font-size-controls flex gap-1">
        <button class="px-2 py-1 rounded bg-yellow-300 hover:bg-yellow-400 text-xs" @click.prevent="decreaseFontSize" title="Decrease font size">-</button>
        <button class="px-2 py-1 rounded bg-yellow-300 hover:bg-yellow-400 text-xs" @click.prevent="increaseFontSize" title="Increase font size">+</button>
      </div>
    </div>
    <div :style="data.labelStyle" class="node-label text-base font-semibold mb-2">{{ data.type }}</div>
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
    <div
      class="flex-1 text-container mt-2"
      ref="textContainer"
      @scroll="handleScroll"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
    >
      <textarea
        v-model="noteText"
        class="note-textarea w-full h-full p-2 rounded border border-yellow-300 bg-yellow-50 text-gray-900 resize-none"
        placeholder="Enter your notes here..."
        :style="{ fontSize: `${currentFontSize}px` }"
        @mousedown="onTextareaMouseDown"
        @mouseup="onTextareaMouseUp"
        @focus="onTextareaFocus"
        @blur="onTextareaBlur"
      ></textarea>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#e8c547'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="120"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>
<script setup>
import { Handle } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import { useNoteNode } from '@/composables/useNoteNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Note_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'Notes',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: false,
      hasOutputs: false,
      inputs: { note: '' },
      outputs: {},
      style: {
        border: '1px solid #e8c547',
        borderRadius: '12px',
        backgroundColor: '#f7f3d7',
        color: '#333',
        width: '200px',
        height: '120px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])
const {
  isHovered,
  customStyle,
  resizeHandleStyle,
  computedContainerStyle,
  noteText,
  currentFontSize,
  currentColor,
  cycleColor,
  decreaseFontSize,
  increaseFontSize,
  handleScroll,
  onTextareaMouseDown,
  onTextareaMouseUp,
  onTextareaFocus,
  onTextareaBlur,
  onResize
} = useNoteNode(props, emit)
</script>

<style scoped>
/* Remove legacy styles in favor of Tailwind */
</style>