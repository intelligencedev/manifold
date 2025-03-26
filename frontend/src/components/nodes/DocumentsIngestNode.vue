<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container ingest-documents-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    
    <div class="mode-selector">
      <label v-for="option in ['passthrough', 'documents', 'git']" :key="option">
        <input type="radio" :value="option" v-model="mode" />
        {{ option.charAt(0).toUpperCase() + option.slice(1) }}
      </label>
    </div>

    <div class="input-field">
      <label class="input-label">Ingestion Endpoint:</label>
      <input type="text" class="input-text" v-model="ingestion_endpoint" :disabled="mode === 'git'"/>
    </div>

    <div v-if="mode === 'git'" class="input-field">
      <label class="input-label">Git Repo Path:</label>
      <input type="text" class="input-text" v-model="gitRepoPath" />
    </div>

    <div class="input-field">
      <label class="input-label">Language:</label>
      <input type="text" class="input-text" v-model="language" placeholder="DEFAULT" :disabled="mode === 'git'" />
    </div>

    <div class="input-field">
      <label class="input-label">Chunk Size:</label>
      <input type="number" class="input-text" v-model.number="chunk_size" />
    </div>

    <div class="input-field">
      <label class="input-label">Chunk Overlap:</label>
      <input type="number" class="input-text" v-model.number="chunk_overlap" />
    </div>

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
          {{ selectedFileNames.length }} files in path.
        </span>
        <input
          type="file"
          ref="fileInput"
          style="display: none;"
          :webkitdirectory="selectionType === 'folder'"
          multiple
          @change="onFileSelection"
        />
      </div>
    </div>

    <Handle v-if="data.hasInputs" style="width:12px; height:12px" type="target" position="left" />
    <Handle v-if="data.hasOutputs" style="width:12px; height:12px" type="source" position="right" />
    
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="300"
      :min-height="200"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import { useDocumentsIngest } from '../../composables/useDocumentsIngest'

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
        ingestion_endpoint: 'http://localhost:8080/api/sefii/ingest',
        mode: 'documents',
        documents: "",
      },
      outputs: {
        result: { output: '' },
      },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '350px',
        height: '220px',
      },
    }),
  },
})

const {
  mode,
  ingestion_endpoint,
  gitRepoPath,
  language,
  chunk_size,
  chunk_overlap,
  selectedFiles,
  selectionType,
  selectedFileNames,
  currentEndpoint,
  callIngestAPI,
  callGitIngestAPI,
  callPathIngestAPI,
  readFileAsText,
  handleFileSelection
} = useDocumentsIngest()

const isHovered = ref(false)
const customStyle = ref({})
const fileInput = ref(null)

// Initialize with props data
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
  mode.value = props.data.inputs.mode || 'documents'
  ingestion_endpoint.value = props.data.inputs.ingestion_endpoint
})

function openFilePicker() {
  fileInput.value?.click()
}

function onFileSelection(event) {
  handleFileSelection(event.target.files)
}

async function run() {
  try {
    if (mode.value === 'passthrough') {
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)

      if (connectedSources.length === 0) {
        throw new Error('No input connected for passthrough mode.')
      }

      let inputText = connectedSources
        .map(sourceId => {
          const sourceNode = findNode(sourceId)
          return sourceNode?.data.outputs?.result.output || ''
        })
        .filter(Boolean)
        .join('\n\n')
        .trim()

      const ingestResult = await callIngestAPI(inputText)
      updateOutput(ingestResult)
      return { ingestResult }
    } 
    
    if (mode.value === 'documents') {
      if (selectedFiles.value.length === 0) {
        throw new Error('No file(s) selected for ingestion.')
      }

      const results = []
      for (const file of selectedFiles.value) {
        const text = await readFileAsText(file)
        if (!text.trim()) {
          console.log(`File ${file.name} is empty; skipping.`)
          continue
        }

        const filePath = file.webkitRelativePath || file.name
        const fileLanguage = inferLanguage(filePath)
        const result = await callIngestAPI(text, filePath, fileLanguage)
        results.push({ file: file.name, result })
      }

      updateOutput(results)
      return { results }
    }
    
    if (mode.value === 'git') {
      if (!gitRepoPath.value) {
        throw new Error('Git Repo Path is required.')
      }
      const gitIngestResult = await callGitIngestAPI(gitRepoPath.value)
      updateOutput(gitIngestResult)
      return { gitIngestResult }
    }

    throw new Error('Invalid mode or missing input.')
  } catch (error) {
    console.error('Error in DocumentsIngestNode run:', error)
    return { error }
  }
}

function inferLanguage(filePath) {
  if (language.value !== 'DEFAULT') return language.value
  
  const ext = filePath.split('.').pop().toUpperCase()
  const languageMap = {
    PY: 'PYTHON',
    JS: 'JS',
    TS: 'TS',
    GO: 'GO',
    MD: 'MARKDOWN',
    MARKDOWN: 'MARKDOWN',
    HTML: 'HTML',
    HTM: 'HTML',
    JSON: 'JSON'
  }
  
  return languageMap[ext] || language.value
}

function updateOutput(result) {
  props.data.outputs = {
    result: { output: JSON.stringify(result, null, 2) },
  }
  updateNodeData()
}

const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px',
}))

function onResize(event) {
  customStyle.value = {
    width: `${event.width}px`,
    height: `${event.height}px`,
  }
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