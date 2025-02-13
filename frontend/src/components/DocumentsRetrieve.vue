<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container documents-retrieve-node tool-node" @mouseenter="isHovered = true"
    @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Retrieve Endpoint Input -->
    <div class="input-field">
      <label class="input-label">Retrieve Endpoint:</label>
      <input type="text" class="input-text" v-model="retrieve_endpoint" />
    </div>

    <!-- Checkbox to enable/disable updating input from source -->
    <div class="input-field">
      <input type="checkbox" :id="`${props.id}-update-from-source`" v-model="updateFromSource"
        @change="onUpdateFromSourceChange" />
      <label :for="`${props.id}-update-from-source`" class="input-label">
        Update Input from Source
      </label>
    </div>

    <!-- Prompt Input -->
    <div class="input-field">
      <label class="input-label">Prompt Text:</label>
      <textarea class="input-text" v-model="prompt" rows="3"></textarea>
    </div>

    <!-- Limit Input -->
    <div class="input-field">
      <label class="input-label">Limit:</label>
      <input type="number" class="input-text" v-model.number="limit" />
    </div>

    <!-- Input Handle (for connected source nodes) -->
    <Handle v-if="data.hasInputs" style="width:10px; height:10px" type="target" position="left" />

    <!-- Output Handle -->
    <Handle v-if="data.hasOutputs" style="width:10px; height:10px" type="source" position="right" />

    <!-- Node Resizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :min-width="200" :min-height="120" :width="200" :height="150" :node-id="props.id" @resize="onResize" />
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
        retrieve_endpoint: 'http://localhost:8080/api/documents/retrieve',
        text: 'Enter prompt text here...',
        limit: 1,
      },
      outputs: {
        result: { output: '' },
      },
      updateFromSource: true,
      style: {
        border: '1px solid #666',
        borderRadius: '4px',
        backgroundColor: '#333',
        color: '#eee',
        width: '200px',
        height: '150px',
      },
    }),
  },
})

// Expose the run() function on mount if not already set
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

// Reactive variables for hover state and custom styles
const isHovered = ref(false)
const customStyle = ref({})

// Computed property for the retrieve endpoint input
const retrieve_endpoint = computed({
  get: () => props.data.inputs.retrieve_endpoint,
  set: (value) => {
    props.data.inputs.retrieve_endpoint = value
  },
})

// Computed property for the prompt text input
const prompt = computed({
  get: () => props.data.inputs.text,
  set: (value) => {
    props.data.inputs.text = value
  },
})

// Computed property for the limit parameter
const limit = computed({
  get: () => props.data.inputs.limit,
  set: (value) => {
    props.data.inputs.limit = value
  },
})

// Reactive property for the updateFromSource checkbox (default comes from props)
const updateFromSource = ref(props.data.updateFromSource)
watch(updateFromSource, (newVal) => {
  props.data.updateFromSource = newVal
  updateNodeData()
})
function onUpdateFromSourceChange() {
  // This change is handled by the watch above.
}

// The run() function will be invoked when the node executes.
async function run() {
  console.log('Running DocumentsRetrieveNode:', props.id)
  try {
    const node = findNode(props.id)
    let inputText = ''

    // Get connected source nodes (if any)
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)

    if (connectedSources.length > 0 && updateFromSource.value) {
      console.log('Connected sources detected. Using their output as prompt text.')
      for (const sourceId of connectedSources) {
        const sourceNode = findNode(sourceId)
        if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result) {
          inputText += `\n\n${sourceNode.data.outputs.result.output}`
        }
      }
    } else {
      inputText = prompt.value
    }

    inputText = inputText.trim()
    const payload = {
      prompt: inputText,
      limit: Number(limit.value),
    }

    console.log('Calling Documents Retrieve API with payload:', payload)
    const response = await fetch(retrieve_endpoint.value, {
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
    console.log('Documents Retrieve API response:', responseData)

    // Assume the endpoint returns text in a "text" property.
    // If not, stringify the entire response.
    let outputText = responseData.text
    if (typeof outputText !== 'string') {
      outputText = JSON.stringify(responseData, null, 2)
    }

    // Update the node's output with the retrieved text.
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