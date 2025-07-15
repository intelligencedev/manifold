<template>
  <div
    ref="containerRef"
    :style="computedContainerStyle"
    class="node-container note-node flex flex-col w-full h-full p-3 rounded-xl border text-gray-900 shadow relative"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div class="flex items-center justify-between w-full mb-2">
      <div class="w-8"><!-- Empty space to balance the layout --></div>
      <div :style="data.labelStyle" class="node-label text-center flex-grow">{{ data.type }}</div>
      <div class="flex items-center">
      <button class="color-cycle-btn mr-1" @click="cycleColor">🎨</button>
      <div class="font-size-controls flex">
        <button @click.prevent="decreaseFontSize" class="px-1">-</button>
        <button @click.prevent="increaseFontSize" class="px-1">+</button>
      </div>
      </div>
    </div>
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
    <div
      class="text-container flex-grow"
      ref="textContainer"
      @scroll="handleScroll"
    >
      <textarea
        v-model="noteText"
        class="w-full h-full resize-none outline-none bg-transparent"
        placeholder="Enter your notes here..."
        :style="{ fontSize: `${currentFontSize}px` }"
        @mouseenter="handleTextareaMouseEnter"
        @mouseleave="handleTextareaMouseLeave"
      ></textarea>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="240"
      :min-height="120"
      :width="width"
      :height="height"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>
<script setup>
import { watch, onMounted } from 'vue'
import { Handle } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import { useNoteNode } from '@/composables/useNoteNode'
import { useDisableNodeDragOnInput } from '@/composables/useDisableNodeDragOnInput'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Note_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'Notes',
      labelStyle: {
        fontWeight: 'normal'
      },
      hasInputs: false,
      hasOutputs: false,
      inputs: {
        note: ''
      },
      outputs: {},
      fontSize: 14, // Initialize fontSize
      style: {
        border: '1px solid #e8c547',
        borderRadius: '12px',
        backgroundColor: '#f7f3d7',
        color: '#333',
        width: '380px',
        height: '120px'
      }
    })
  }
})

const emit = defineEmits([
  'update:data',
  'disable-zoom',
  'enable-zoom',
  'resize'
])


// Use the note node composable
const {
  noteText,
  isHovered,
  resizeHandleStyle,
  computedContainerStyle,
  width,
  height,
  currentFontSize,
  cycleColor,
  increaseFontSize,
  decreaseFontSize,
  handleScroll,
  handleTextareaMouseEnter,
  handleTextareaMouseLeave,
  onResize
} = useNoteNode(props, emit)
// Disable drag when interacting with input fields
const containerRef = useDisableNodeDragOnInput(props.id)

// Watch for changes and emit them upward
watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData })
  },
  { deep: true }
)

</script>
