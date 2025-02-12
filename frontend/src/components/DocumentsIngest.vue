<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container ingest-documents-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Mode Selector -->
    <div class="mode-selector">
      <label>
        <input type="radio" value="passthrough" v-model="mode" />
        Passthrough
      </label>
      <label>
        <input type="radio" value="documents" v-model="mode" />
        Documents
      </label>
    </div>

    <!-- Ingestion Endpoint Input -->
    <div class="input-field">
      <label class="input-label">Ingestion Endpoint:</label>
      <input type="text" class="input-text" v-model="ingestion_endpoint" />
    </div>

    <!-- Additional Parameters (always visible) -->
    <div class="input-field">
      <label class="input-label">Language:</label>
      <input type="text" class="input-text" v-model="language" placeholder="DEFAULT" />
    </div>
    <div class="input-field">
      <label class="input-label">Chunk Size:</label>
      <input type="number" class="input-text" v-model.number="chunk_size" />
    </div>
    <div class="input-field">
      <label class="input-label">Chunk Overlap:</label>
      <input type="number" class="input-text" v-model.number="chunk_overlap" />
    </div>

    <!-- File selection controls (only in documents mode) -->
    <div v-if="mode === 'documents'">
      <div class="input-field">
        <label class="input-label">Selection Type:</label>
        <select v-model="selectionType">
          <option value="file">File</option>
          <option value="folder">Folder</option>
        </select>
      </div>
      <div class="file-picker">
        <button @click="openFilePicker">
          Select {{ selectionType === 'folder' ? 'Folder' : 'File' }}
        </button>
        <span v-if="selectedFileNames.length">
          Selected: {{ selectedFileNames.join(', ') }}
        </span>
        <!-- Hidden file input; allow directory selection if "folder" is chosen -->
        <input
          type="file"
          ref="fileInput"
          style="display: none;"
          :webkitdirectory="selectionType === 'folder'"
          @change="handleFileSelection"
          multiple
        />
      </div>
    </div>

    <!-- Input/Output Handles -->
    <Handle v-if="data.hasInputs" style="width:10px; height:10px" type="target" position="left" />
    <Handle v-if="data.hasOutputs" style="width:10px; height:10px" type="source" position="right" />

    <!-- Node Resizer -->
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="120"
      :width="200"
      :height="120"
      :node-id="props.id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const { getEdges, findNode, updateNodeData } = useVueFlow()
const emit = defineEmits(['update:data', 'resize'])

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'DocumentsIngest_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'DocumentsIngestNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        ingestion_endpoint: 'http://localhost:32190/api/documents/ingest',
        mode: 'documents', // default mode is "documents"
      },
      outputs: {
        result: { output: '' },
      },
      style: {
        border: '1px solid #666',
        borderRadius: '4px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '220px', // increased height for extra fields
      },
    }),
  },
})

// Expose the run function on mount (if not already defined)
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

// Reactive variables for UI styling and state
const isHovered = ref(false)
const customStyle = ref({})

// Two-way binding for ingestion endpoint (mirrors the node’s inputs)
const ingestion_endpoint = computed({
  get: () => props.data.inputs.ingestion_endpoint,
  set: (value) => {
    props.data.inputs.ingestion_endpoint = value
  },
})

// Mode: either "passthrough" (using text from connected nodes) or "documents" (file/folder ingestion)
const mode = ref(props.data.inputs.mode || 'documents')
watch(mode, (newVal) => {
  props.data.inputs.mode = newVal
})

// Additional parameters (always available)
const language = ref('DEFAULT')
const chunk_size = ref(1000)
const chunk_overlap = ref(550)

// File selection controls (only used when mode is "documents")
const selectionType = ref('file')
const selectedFiles = ref([])
const selectedFileNames = computed(() => selectedFiles.value.map((file) => file.name))

const fileInput = ref(null)
function openFilePicker() {
  fileInput.value && fileInput.value.click()
}
function handleFileSelection(event) {
  const files = event.target.files
  selectedFiles.value = []
  for (let i = 0; i < files.length; i++) {
    const file = files[i]
    // Accept text files based on MIME type or common file extensions
    if (
      (file.type && file.type.startsWith('text/')) ||
      file.name.match(/\.(txt|md|csv|json)$/i)
    ) {
      selectedFiles.value.push(file)
    }
  }
}

// The run function invoked when the node executes.
async function run() {
  console.log('Running DocumentsIngestNode:', props.id)
  try {
    const node = findNode(props.id)
    let inputText = ''

    // Get connected source nodes (for passthrough mode)
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)

    if (mode.value === 'passthrough' && connectedSources.length > 0) {
      console.log('Passthrough mode: Using text from connected source nodes')
      for (const sourceId of connectedSources) {
        const sourceNode = findNode(sourceId)
        if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result) {
          inputText += `\n\n${sourceNode.data.outputs.result.output}`
        }
      }
      inputText = inputText.trim()
      const ingestResult = await callIngestAPI(inputText)
      props.data.outputs = {
        result: { output: JSON.stringify(ingestResult, null, 2) },
      }
      updateNodeData()
      return { ingestResult }
    } else if (mode.value === 'documents') {
      // In documents mode, require that files have been selected
      if (selectedFiles.value.length === 0) {
        throw new Error('No file(s) selected for ingestion.')
      }
      const results = []
      // Loop over each selected file (or each file within the selected folder)
      for (const file of selectedFiles.value) {
        const text = await readFileAsText(file)
        if (text.trim().length === 0) {
          console.log(`File ${file.name} is empty; skipping.`)
          continue
        }
        const result = await callIngestAPI(text)
        results.push({ file: file.name, result })
      }
      props.data.outputs = {
        result: { output: JSON.stringify(results, null, 2) },
      }
      updateNodeData()
      return { results }
    } else {
      throw new Error('Invalid mode or missing input for passthrough mode.')
    }
  } catch (error) {
    console.error('Error in DocumentsIngestNode run:', error)
    return { error }
  }
}

// Helper function to call the ingestion API.
async function callIngestAPI(text) {
  const payload = {
    text: text,
    language: language.value,
    chunk_size: chunk_size.value,
    chunk_overlap: chunk_overlap.value,
  }
  console.log('Calling Ingest API with payload:', payload)
  const response = await fetch(ingestion_endpoint.value, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) {
    const errorText = await response.text()
    throw new Error(`API error (${response.status}): ${errorText}`)
  }
  const responseData = await response.json()
  console.log('Ingest API response:', responseData)
  return responseData
}

// Helper function to read a File object as text using the FileReader API.
function readFileAsText(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = (e) => resolve(e.target.result)
    reader.onerror = (e) => reject(e)
    reader.readAsText(file)
  })
}

// Control the visibility of the resize handle based on hover state
const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
}))

function onResize(event) {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
}
</script>

<style scoped>
.ingest-documents-node {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color);
  border: 1px solid var(--node-border-color);
  border-radius: 4px;
  color: var(--node-text-color);
  padding: 8px;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.input-field {
  margin-bottom: 8px;
  display: flex;
  flex-direction: column;
}

.input-label {
  font-size: 12px;
  margin-bottom: 4px;
}

.input-text {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: 100%;
  box-sizing: border-box;
}

.mode-selector {
  margin-bottom: 8px;
  display: flex;
  justify-content: space-around;
}

.mode-selector label {
  font-size: 12px;
  color: var(--node-text-color);
}

.file-picker {
  display: flex;
  align-items: center;
}

.file-picker button {
  background-color: #555;
  color: #eee;
  border: 1px solid #666;
  padding: 4px 8px;
  cursor: pointer;
  font-size: 12px;
  margin-right: 8px;
}
</style>
