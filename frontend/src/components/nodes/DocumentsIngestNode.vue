<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="490"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>
    
    <div class="mode-selector flex justify-around mb-2 text-sm">
      <label v-for="option in ['passthrough', 'documents', 'git']" :key="option" class="flex items-center space-x-1">
        <input type="radio" :value="option" v-model="mode" />
        <span>{{ option.charAt(0).toUpperCase() + option.slice(1) }}</span>
      </label>
    </div>

    <BaseInput label="Ingestion Endpoint" v-model="ingestion_endpoint" :disabled="mode === 'git'" />

    <BaseInput v-if="mode === 'git'" label="Git Repo Path" v-model="gitRepoPath" />

    <BaseInput label="Language" v-model="language" placeholder="DEFAULT" :disabled="mode === 'git'" />

    <BaseInput label="Chunk Size" type="number" v-model.number="chunk_size" />

    <BaseInput label="Chunk Overlap" type="number" v-model.number="chunk_overlap" />

    <div v-if="mode === 'documents'">
      <BaseSelect
        label="Selection Type"
        v-model="selectionType"
        :options="[
          { value: 'file', label: 'File' },
          { value: 'folder', label: 'Folder' }
        ]"
      />
      <div class="file-picker flex items-center mt-1">
        <button
          class="bg-gray-600 text-white px-2 py-1 rounded mr-2 text-sm"
          @click="openFilePicker"
        >
          Select {{ selectionType === 'folder' ? 'Folder' : 'File' }}
        </button>
        <span v-if="selectedFileNames.length" class="text-xs text-gray-300">
          {{ selectedFileNames.length }} files in path.
        </span>
        <input
          type="file"
          ref="fileInput"
          class="hidden"
          :webkitdirectory="selectionType === 'folder'"
          multiple
          @change="onFileSelection"
        />
      </div>
    </div>

    <Handle v-if="data.hasInputs" style="width:12px; height:12px" type="target" position="left" />
    <Handle v-if="data.hasOutputs" style="width:12px; height:12px" type="source" position="right" />
    
  </BaseNode>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
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

function onResize({ width, height }) {
  props.data.style.width = `${width}px`
  props.data.style.height = `${height}px`
  updateNodeData()
  emit('resize', { id: props.id, width, height })
}
</script>
