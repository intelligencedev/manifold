<template>
  <div
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container documents-retrieve-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <div class="input-field">
      <label class="input-label">Retrieve Endpoint:</label>
      <input type="text" class="input-text" v-model="retrieve_endpoint" />
    </div>

    <div class="input-field">
      <input 
        type="checkbox" 
        :id="`${props.id}-update-from-source`" 
        v-model="updateFromSource"
      />
      <label :for="`${props.id}-update-from-source`" class="input-label">
        Update Input from Source
      </label>
    </div>

    <div class="input-field">
      <label class="input-label">Prompt Text:</label>
      <textarea class="input-text" v-model="prompt" rows="3"></textarea>
    </div>

    <div class="input-field">
      <label class="input-label">Limit:</label>
      <input type="number" class="input-text" v-model.number="limit" />
    </div>

    <div class="input-field">
      <label class="input-label">Merge Mode:</label>
      <select class="input-text" v-model="merge_mode">
        <option value="union">Union</option>
        <option value="intersect">Intersect</option>
        <option value="weighted">Weighted</option>
      </select>
    </div>

    <template v-if="merge_mode === 'weighted'">
      <div class="input-field">
        <label class="input-label">Vector Weight (Alpha):</label>
        <input 
          type="number" 
          step="0.1" 
          min="0" 
          max="1" 
          class="input-text" 
          v-model.number="alpha" 
        />
      </div>
      <div class="input-field">
        <label class="input-label">Keyword Weight (Beta):</label>
        <input 
          type="number" 
          step="0.1" 
          min="0" 
          max="1" 
          class="input-text" 
          v-model.number="beta" 
        />
      </div>
    </template>

    <div class="input-field">
      <input 
        type="checkbox" 
        :id="`${props.id}-return-full-docs`" 
        v-model="return_full_docs" 
      />
      <label :for="`${props.id}-return-full-docs`" class="input-label">
        Return Full Docs
      </label>
    </div>

    <Handle 
      v-if="data.hasInputs" 
      style="width:12px; height:12px" 
      type="target" 
      position="left" 
    />
    <Handle 
      v-if="data.hasOutputs" 
      style="width:12px; height:12px" 
      type="source" 
      position="right" 
    />
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
import { ref, computed, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import { useDocumentsRetrieve } from '../../composables/useDocumentsRetrieve'

const { updateNodeData } = useVueFlow()
const emit = defineEmits(['update:data', 'resize'])

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'DocumentsRetrieve_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'DocumentsRetrieveNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        retrieve_endpoint: 'http://localhost:8080/api/sefii/combined-retrieve',
        text: 'Enter prompt text here...',
        limit: 1,
        merge_mode: 'intersect',
        return_full_docs: true,
      },
      outputs: {
        result: { output: '' },
      },
      updateFromSource: true,
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '150px',
      },
    }),
  },
})

const {
  retrieve_endpoint,
  prompt,
  limit,
  merge_mode,
  return_full_docs,
  updateFromSource,
  alpha,
  beta,
  retrieveDocuments,
  formatOutput,
  getConnectedNodesText
} = useDocumentsRetrieve(props)

const isHovered = ref(false)
const customStyle = ref({})

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

async function run() {
  try {
    const inputText = getConnectedNodesText()
    const responseData = await retrieveDocuments(inputText)
    const outputText = formatOutput(responseData)
    
    props.data.outputs = {
      result: { output: outputText },
    }
    updateNodeData()
    return { responseData }
  } catch (error) {
    console.error('Error in DocumentsRetrieveNode run:', error)
    return { error }
  }
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
.documents-retrieve-node {
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

textarea.input-text {
  resize: vertical;
}
</style>