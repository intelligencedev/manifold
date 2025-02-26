<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container claude-response-node tool-node" @mouseenter="isHovered = true"
    @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <div class="header">
      <div class="controls">
        <div class="select-container">
          <label for="render-mode">Render Mode:</label>
          <select id="render-mode" v-model="selectedRenderMode">
            <option value="markdown">Markdown</option>
            <option value="raw">Raw Text</option>
          </select>
        </div>
        <div class="font-size-controls">
          <button @click.prevent="decreaseFontSize">-</button>
          <button @click.prevent="increaseFontSize">+</button>
        </div>
        <button class="copy-button" @click="copyToClipboard" :disabled="isCopying">Copy</button>
      </div>
      <div v-if="copyStatus" class="copy-feedback">{{ copyStatus }}</div>
    </div>

    <div class="text-container" ref="textContainer"
      @scroll="handleScroll"
      @mouseenter="$emit('disable-zoom')"
      @mouseleave="$emit('enable-zoom')"
      @wheel.stop
      :style="{ fontSize: `${currentFontSize}px` }">
      <div v-if="selectedRenderMode === 'raw'" class="raw-text">
        {{ response }}
      </div>
      <div v-else-if="selectedRenderMode === 'markdown'" class="markdown-text" v-html="markdownToHtml"></div>
    </div>

    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />

    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :min-width="350" :min-height="400" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, nextTick, onMounted, watch } from "vue";
import { Handle, useVueFlow } from "@vue-flow/core";
import { marked } from "marked";
import { NodeResizer } from "@vue-flow/node-resizer";

const { getEdges, findNode, updateNodeData } = useVueFlow()

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

async function run() {
  console.log("Running ClaudeResponse:", props.id)
  // Get the first connected source node.
  const connectedSources = getEdges.value
    .filter((edge) => edge.target === props.id)
    .map((edge) => edge.source)
  if (connectedSources.length > 0) {
    const sourceNode = findNode(connectedSources[0])
    console.log("Source node:", sourceNode)
    if (sourceNode && sourceNode.data.outputs.response) {
      props.data.inputs.response = sourceNode.data.outputs.response
    }
  }
  // Make the response available downstream.
  props.data.outputs = { result: { output: response.value } }
  updateNodeData()
}

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: "ClaudeResponse_0"
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: "ClaudeResponse",
      labelStyle: { fontWeight: "normal" },
      hasInputs: true,
      hasOutputs: true,
      inputs: { response: "" },
      outputs: { result: { output: "" } },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '350px',
        height: '400px',
      },
    })
  }
})
const emit = defineEmits(["update:data", "disable-zoom", "enable-zoom", "resize"])

const selectedRenderMode = ref("markdown")
const textContainer = ref(null)
const isAutoScrollEnabled = ref(true)
const isHovered = ref(false)
const copyStatus = ref("")
const isCopying = ref(false)
const customStyle = ref({})

const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? "visible" : "hidden",
  width: "12px",
  height: "12px"
}))

const currentFontSize = ref(12)
const minFontSize = 10
const maxFontSize = 24
const fontSizeStep = 2

const increaseFontSize = () => {
  currentFontSize.value = Math.min(currentFontSize.value + fontSizeStep, maxFontSize)
}
const decreaseFontSize = () => {
  currentFontSize.value = Math.max(currentFontSize.value - fontSizeStep, minFontSize)
}

marked.setOptions({
  breaks: true,
  gfm: true,
  headerIds: false
})

const response = computed({
  get: () => props.data.inputs.response,
  set: (value) => { props.data.inputs.response = value }
})

const markdownToHtml = computed(() => {
  return marked(response.value)
})

const scrollToBottom = () => {
  nextTick(() => {
    if (textContainer.value) {
      textContainer.value.scrollTop = textContainer.value.scrollHeight
    }
  })
}

const handleScroll = () => {
  if (textContainer.value) {
    const { scrollTop, scrollHeight, clientHeight } = textContainer.value
    isAutoScrollEnabled.value = scrollTop + clientHeight >= scrollHeight - 10
  }
}

const { zoomIn, zoomOut } = useVueFlow()

const onResize = (event) => {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
}

const copyToClipboard = async () => {
  if (isCopying.value) return
  isCopying.value = true
  try {
    await navigator.clipboard.writeText(response.value)
    copyStatus.value = "Copied!"
  } catch (error) {
    console.error("Failed to copy text: ", error)
    copyStatus.value = "Failed to copy."
  }
  setTimeout(() => {
    copyStatus.value = ""
    isCopying.value = false
  }, 2000)
}

watch(() => props.data, (newData) => {
  emit("update:data", { id: props.id, data: newData })
  if (isAutoScrollEnabled.value) scrollToBottom()
}, { deep: true })

watch(() => props.data.inputs.response, () => {
  if (isAutoScrollEnabled.value) scrollToBottom()
})
</script>

<style scoped>
.claude-response-node {
  background-color: #333;
  border: 1px solid #666;
  border-radius: 4px;
  color: #eee;
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px;
  position: relative;
}

.controls {
  display: flex;
  align-items: center;
}

.select-container {
  display: flex;
  align-items: center;
  margin-right: 10px;
}

label {
  font-size: 12px;
  margin-right: 5px;
}

select {
  background-color: #222;
  border: 1px solid #666;
  color: #eee;
  font-size: 12px;
  padding: 2px 5px;
}

/* Font Size Controls */
.font-size-controls {
  display: flex;
  gap: 5px;
  margin-right: 10px;
}

.font-size-controls button {
  background-color: #444;
  border: 1px solid #666;
  color: #eee;
  font-size: 12px;
  padding: 5px 8px;
  border-radius: 3px;
  cursor: pointer;
  transition: background-color 0.3s;
}

.font-size-controls button:hover {
  background-color: #555;
}

/* Styling for the Copy Button */
.copy-button {
  background-color: #444;
  border: 1px solid #666;
  color: #eee;
  font-size: 12px;
  padding: 5px 10px;
  border-radius: 3px;
  cursor: pointer;
  transition: background-color 0.3s;
}

.copy-button:hover {
  background-color: #555;
}

.copy-button:disabled {
  background-color: #555;
  cursor: not-allowed;
}

/* Styling for the Copy Feedback Message */
.copy-feedback {
  position: absolute;
  top: 40px;
  right: 10px;
  background-color: #555;
  color: #fff;
  padding: 3px 8px;
  border-radius: 3px;
  font-size: 10px;
  opacity: 0.9;
}

.node-container {
    display: flex;
    flex-direction: column;
    box-sizing: border-box;
}

.text-container {
  flex-grow: 1;
  overflow-y: auto;
  padding: 10px;
  margin-top: 0;
  margin-bottom: 0;
  width: auto;
  height: auto;
  min-height: 0;
  max-height: none;
  white-space: normal;
  text-align: left;
}

.raw-text,
.markdown-text {
  line-height: 1.5;
}

/* Ensure markdown renders correctly */
.markdown-text img {
  max-width: 100%;
  height: auto;
}

.markdown-text a {
  color: #1e90ff;
  text-decoration: underline;
}

.markdown-text h1,
.markdown-text h2,
.markdown-text h3,
.markdown-text h4,
.markdown-text h5,
.markdown-text h6 {
  color: #fff;
  margin-top: 16px;
  margin-bottom: 8px;
}

.markdown-text ul,
.markdown-text ol {
  padding-left: 20px;
  margin-bottom: 16px;
}

.markdown-text blockquote {
  border-left: 4px solid #555;
  padding-left: 10px;
  margin-left: 0;
  color: #ccc;
}

.markdown-text code {
  background-color: #444;
  padding: 2px 4px;
  border-radius: 3px;
  font-family: monospace;
}

.markdown-text pre {
  background-color: #222;
  padding: 10px;
  border-radius: 4px;
  overflow-x: auto;
}

.markdown-text pre code {
  background-color: transparent;
  padding: 0;
}

.markdown-text table {
  border-collapse: collapse;
  width: 100%;
  margin-bottom: 16px;
}

.markdown-text th,
.markdown-text td {
  border: 1px solid #555;
  padding: 8px;
  text-align: left;
}

.markdown-text th {
  background-color: #444;
}
</style>