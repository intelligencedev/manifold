<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container comfy-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Parameters Accordion -->
    <details class="parameters-accordion" open>
      <summary>Parameters</summary>

      <!-- ComfyUI Endpoint Input -->
      <div class="input-field">
        <label class="input-label">ComfyUI Endpoint:</label>
        <input type="text" class="input-text" v-model="endpoint" placeholder="http://comfyui-host:8188/prompt" />
      </div>

      <!-- Image Prompt Input -->
      <div class="input-field">
        <label :for="`${data.id}-prompt`" class="input-label">Image Prompt:</label>
        <textarea :id="`${data.id}-prompt`" v-model="prompt" class="input-textarea"></textarea>
      </div>
    </details>

    <!-- Generated Image Panel -->
    <div class="generated-image-panel" v-if="generatedImage">
      <img :src="generatedImage" alt="Generated Image" class="generated-image" />
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :width="360" :height="673" :min-width="360" :min-height="673" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'ComfyNode_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'ComfyNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        endpoint: 'http://192.168.1.200:32182/prompt',
        prompt: 'A cute small robot playing with toy building blocks.',
      },
      outputs: { image: '' },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '360px',
        height: '673px',
      },
    }),
  },
})

const { getEdges, findNode } = useVueFlow()

const isHovered = ref(false)
const customStyle = ref({
  width: '360px',
  height: '660px',
})

// Computed properties for the inputs
const endpoint = computed({
  get: () => props.data.inputs.endpoint,
  set: (value) => { props.data.inputs.endpoint = value },
})

const prompt = computed({
  get: () => props.data.inputs.prompt,
  set: (value) => { props.data.inputs.prompt = value },
})

// For storing the generated image URL
const generatedImage = ref('')

const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px',
}))

function onResize(event) {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
}

onMounted(() => {
  // Expose this node’s run() method so a higher-level flow runner can call it.
  if (!props.data.run) {
    props.data.run = run
  }
})

async function run() {
  console.log('Running ComfyNode:', props.id)
  generatedImage.value = ''
  props.data.outputs.image = ''

  try {
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)

    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])

      console.log('Comfy Connected sources:', connectedSources)

      if (sourceNode && sourceNode.data.outputs.result) {
        props.data.inputs.prompt = sourceNode.data.outputs.result.output
      }
    }

    // Send the user's endpoint + prompt to the backend proxy
    const response = await fetch('/api/comfy-proxy', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        targetEndpoint: endpoint.value,
        prompt: prompt.value,
      }),
    })

    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }

    // Get the blob from response
    const blob = await response.blob()

    // Create an object URL from the blob
    const imageUrl = URL.createObjectURL(blob)

    // Update both the display image and node output
    generatedImage.value = imageUrl
    props.data.outputs.image = imageUrl

  } catch (error) {
    console.error('Error in ComfyNode run:', error)
  }
}
</script>

<style scoped>
.comfy-node {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color);
  border: 1px solid var(--node-border-color);
  border-radius: 4px;
  color: var(--node-text-color);
  overflow: hidden;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.input-field {
  position: relative;
  margin: 5px;
}

.input-label {
  font-size: 12px;
  color: #eee;
}

.input-text,
.input-textarea {
  margin-top: 5px;
  margin-bottom: 5px;
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-textarea {
  rows: 10;
  height: 160px;
}

.parameters-accordion {
  border: 1px solid #666;
  border-radius: 4px;
  background-color: #444;
  color: #eee;
  padding: 5px;
  width: 100%;
  box-sizing: border-box;
}

.parameters-accordion summary {
  cursor: pointer;
  padding: 5px;
  font-weight: bold;
}

/* Force the generated image panel to fill available space 
   and clip/scroll anything larger than the panel. */
.generated-image-panel {
  flex: 1 1 auto;
  position: relative;
  overflow: hidden;
  /* or 'auto' if you want scrolling */
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 20px;
}

/* Make sure the image scales down to fit inside its container */
.generated-image {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  /* or 'cover' depending on your needs */
  display: block;
}
</style>