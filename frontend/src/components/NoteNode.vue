<template>
  <div
    :style="{
      ...data.style,
      ...customStyle,
      '--node-bg-color': currentColor,
      width: '100%',
      height: '100%',
      boxSizing: 'border-box',
      position: 'relative'
    }"
    class="node-container note-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <!-- Button to cycle sticky note colors -->
    <button class="color-cycle-btn" @click="cycleColor">🎨</button>
    
    <!-- Font size controls -->
    <div class="font-size-controls">
      <button @click.prevent="decreaseFontSize">-</button>
      <button @click.prevent="increaseFontSize">+</button>
    </div>

    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <div
      class="text-container"
      ref="textContainer"
      @scroll="handleScroll"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
    >
      <textarea
        v-model="noteText"
        class="note-textarea"
        placeholder="Enter your notes here..."
        :style="{ fontSize: `${currentFontSize}px` }"
      ></textarea>
    </div>
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
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
import { ref, computed, watch, nextTick, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

async function run() {
  return
}

onMounted(() => {
  props.data.run = run
})

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
      style: {
        border: '1px solid #e8c547',
        borderRadius: '12px',
        backgroundColor: '#f7f3d7',
        color: '#333',
        width: '200px',
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

const noteText = computed({
  get: () => props.data.inputs.note,
  set: (value) => {
    props.data.inputs.note = value
  }
})

const isHovered = ref(false)
const customStyle = ref({})

// Show/hide the resize handles when hovering
const resizeHandleStyle = computed(() => ({
visibility: isHovered.value ? 'visible' : 'hidden',
width: '12px',
height: '12px',
}))

// Font size control
const currentFontSize = ref(14); // Default font size
const minFontSize = 10;
const maxFontSize = 24;
const fontSizeStep = 2;

const increaseFontSize = () => {
  currentFontSize.value = Math.min(currentFontSize.value + fontSizeStep, maxFontSize);
};

const decreaseFontSize = () => {
  currentFontSize.value = Math.max(currentFontSize.value - fontSizeStep, minFontSize);
};

// References to DOM elements
const textContainer = ref(null)

// Auto-scroll control
const isAutoScrollEnabled = ref(true)

// Access zoom functions from VueFlow
const { zoomIn, zoomOut } = useVueFlow()

// Disable zoom when interacting with the text container
const disableZoom = () => {
  zoomIn(0)
  zoomOut(0)
}

// Enable zoom when not interacting
const enableZoom = () => {
  zoomIn(1)
  zoomOut(1)
}

// Function to scroll to the bottom of the text container
const scrollToBottom = () => {
  nextTick(() => {
    if (textContainer.value) {
      textContainer.value.scrollTop = textContainer.value.scrollHeight
    }
  })
}

// Handle scroll events to toggle auto-scroll
const handleScroll = () => {
  if (textContainer.value) {
    const { scrollTop, scrollHeight, clientHeight } = textContainer.value
    if (scrollTop + clientHeight < scrollHeight) {
      isAutoScrollEnabled.value = false
    } else {
      isAutoScrollEnabled.value = true
    }
  }
}

// Handle resize events
const onResize = (event) => {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
}

// Watch for changes and emit them upward
watch(
  () => props.data,
  (newData) => {
    emit('update:data', { id: props.id, data: newData })
    if (isAutoScrollEnabled.value) {
      scrollToBottom()
    }
  },
  { deep: true }
)

// Define five pastel colors suitable for a sticky note background.
const pastelColors = ['#FFB3BA', '#FFDFBA', '#FFFFBA', '#BAFFC9', '#BAE1FF']
const currentColorIndex = ref(0)
const currentColor = computed(() => pastelColors[currentColorIndex.value])

// Add computed properties for scroll bar colors based on current background color
const scrollbarTrackColor = computed(() => currentColor.value)
const scrollbarBorderColor = computed(() => currentColor.value)

const cycleColor = () => {
  currentColorIndex.value = (currentColorIndex.value + 1) % pastelColors.length
  // Add this line to update the data style with the new color
  props.data.style.backgroundColor = currentColor.value
}
</script>

<style scoped>
.node-container {
  background-color: var(--node-bg-color, #f7f3d7) !important;
  padding: 4px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

.note-node {
  /* Remove the hardcoded color override */
  /* --node-bg-color: #e8c547 !important; */
  --node-text-color: #333 !important;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  font-family: 'Arial', sans-serif;
}

.node-label {
  font-family: 'Roboto', sans-serif;
  font-size: 14px;
  font-weight: bolder;
  text-align: center;
  color: #333;
  user-select: none;
  pointer-events: none;
  box-sizing: border-box;
}

.text-container {
  flex-grow: 1;
  padding: 2px;
  width: 100%;
  height: 100%;
  min-height: 0;
  text-align: left;
  box-sizing: border-box;
  overflow-y: hidden;
}

.note-textarea {
  width: 100%;
  height: 100%;
  background-color: transparent;
  border: none;
  color: #333;
  /* font-size is now dynamic via inline style */
  font-weight: 500;
  resize: none;
  outline: none;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  line-height: 1.5;
  box-sizing: border-box;
  white-space: pre-wrap;
  word-wrap: break-word;
  overflow-wrap: break-word;
  overflow-y: auto;
  letter-spacing: 0.01em;
  padding: 8px 6px;
  text-rendering: optimizeLegibility;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}

.note-textarea::placeholder {
  color: rgba(0,0,0,0.35);
  font-style: italic;
}

.note-textarea::-webkit-scrollbar {
  width: 4px; /* Even thinner scrollbar for more circular appearance */
}

.note-textarea::-webkit-scrollbar-track {
  background: transparent; /* Keep the background transparent */
  border-radius: 12px;
  margin: 8px 0; /* Add some margin to create space between thumb and edges */
}

.note-textarea::-webkit-scrollbar-thumb {
  background-color: rgba(82, 82, 91, 0.4); /* Neutral dark slate with transparency */
  border-radius: 10px; /* Rounded corners for circle effect */
  border: none; /* Remove border */
  min-height: 30px; /* Ensure minimum size for better visibility */
}

.note-textarea::-webkit-scrollbar-thumb:hover {
  background-color: rgba(82, 82, 91, 0.6); /* More opaque on hover */
}

/* Styles for the color cycle button */
.color-cycle-btn {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 50%;
  background-color: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  padding: 0;
  font-size: 16px;
  outline: none;
}

.color-cycle-btn:focus {
  outline: none;
  box-shadow: none;
}

/* Font size control styles */
.font-size-controls {
  position: absolute;
  top: 4px;
  left: 4px;
  display: flex;
  gap: 4px;
}

.font-size-controls button {
  width: 24px;
  height: 24px;
  border: none;
  border-radius: 50%;
  background-color: transparent;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  padding: 0;
  font-size: 18px;
  color: #333;
}

.font-size-controls button:hover,
.font-size-controls button:focus,
.font-size-controls button:active {
  outline: none;
  border: none;
  background-color: transparent;
  box-shadow: none;
}
</style>