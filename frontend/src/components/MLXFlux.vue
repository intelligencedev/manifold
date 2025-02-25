<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container mlxflux-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Parameters Panel (no accordion) -->
    <div class="parameters-panel">
      <!-- Model Selection -->
      <div class="input-field">
        <label :for="`${data.id}-model`" class="input-label">Model:</label>
        <input type="text" :id="`${data.id}-model`" v-model="model" class="input-text" />
      </div>

      <!-- Prompt Input -->
      <div class="input-field">
        <label :for="`${data.id}-prompt`" class="input-label">Prompt:</label>
        <textarea :id="`${data.id}-prompt`" v-model="prompt" class="input-textarea"></textarea>
      </div>

      <!-- Steps Input -->
      <div class="input-field">
        <label :for="`${data.id}-steps`" class="input-label">Steps:</label>
        <input type="number" :id="`${data.id}-steps`" v-model.number="steps" class="input-text" min="1" />
      </div>

      <!-- Seed Input -->
      <div class="input-field">
        <label :for="`${data.id}-seed`" class="input-label">Seed:</label>
        <input type="number" :id="`${data.id}-seed`" v-model.number="seed" class="input-text" />
      </div>

      <!-- Quality Input -->
      <div class="input-field">
        <label :for="`${data.id}-quality`" class="input-label">Quality:</label>
        <input type="number" :id="`${data.id}-quality`" v-model.number="quality" class="input-text" min="1" />
      </div>

      <!-- Output Path Input -->
      <div class="input-field">
        <label :for="`${data.id}-output`" class="input-label">Output:</label>
        <input type="text" :id="`${data.id}-output`" v-model="output" class="input-text" />
      </div>
    </div>

    <div class="image-preview" v-if="imageSrc">
      <img :src="imageSrc" alt="Generated Image" style="max-width: 100%; max-height: 200px;" />
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :min-width="350" :min-height="560" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue';
const { getEdges } = useVueFlow()
import { Handle, useVueFlow } from '@vue-flow/core';
import { NodeResizer } from '@vue-flow/node-resizer';

const { findNode, updateNodeData } = useVueFlow();
const emit = defineEmits(['update:data', 'resize']);

const imageSrc = ref(''); // Holds the image URL

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'MLXFlux_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'MLXFluxNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        model: 'dev',
        prompt: 'luxury breakfast photograph',
        steps: 20,
        seed: 0,
        quality: 4,
        output: '<path to manifold public>/mlx_out.png',
      },
      outputs: { response: '' },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '350px',
        height: '400px',
      },
    }),
  },
});

// Expose the run() function on mount
onMounted(() => {
  props.data.run = run;
});

// Watch for changes in the output response and update imageSrc accordingly
watch(
  () => props.data.outputs.response,
  (newValue) => {
    if (newValue) {
      imageSrc.value = newValue; // Update imageSrc with the image URL from the response
    }
  },
  { immediate: true }
);

// Function to call the FMLX API (using a Go backend endpoint)
async function callFMLXAPI(mlxNode, finalPrompt) {
  const endpoint = '/api/run-fmlx'; // Your Go backend endpoint

  const requestBody = {
    model: mlxNode.data.inputs.model,
    prompt: finalPrompt,
    steps: mlxNode.data.inputs.steps,
    seed: mlxNode.data.inputs.seed,
    quality: mlxNode.data.inputs.quality,
    output: mlxNode.data.inputs.output, // Use the configured output path
  };

  try {
    const response = await fetch(endpoint, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(requestBody),
    });

    if (!response.ok) {
      const errorText = await response.text();
      throw new Error(`API error (${response.status}): ${errorText}`);
    }

    const result = await response.json();
    console.log('FMLX API response:', result);

    updateNodeData(props.id, {
      ...props.data,
      outputs: {
        response: result.image_url, // Update the output with the image URL
      },
    });

    return { response: 'OK' };
  } catch (e) {
    console.error('Error calling fmlx api', e);
    return { error: e.message };
  }
}

async function run() {
  console.log('Running MLXFlux node:', props.id);

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

    return await callFMLXAPI(findNode(props.id), finalPrompt);
  } catch (error) {
    console.error('Error in MLXFlux run:', error);
    return { error };
  }
}

// Computed properties for two-way data binding
const model = computed({
  get: () => props.data.inputs.model,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, model: value },
    }),
});

const prompt = computed({
  get: () => props.data.inputs.prompt,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, prompt: value },
    }),
});

const steps = computed({
  get: () => props.data.inputs.steps,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, steps: value },
    }),
});

const seed = computed({
  get: () => props.data.inputs.seed,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, seed: value },
    }),
});

const quality = computed({
  get: () => props.data.inputs.quality,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, quality: value },
    }),
});

const output = computed({
  get: () => props.data.inputs.output,
  set: (value) =>
    updateNodeData(props.id, {
      ...props.data,
      inputs: { ...props.data.inputs, output: value },
    }),
});

const isHovered = ref(false);
const customStyle = ref({});

// Show/hide the handles based on hover state
const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px',
}))

function onResize(event) {
  customStyle.value.width = `${event.width}px`;
  customStyle.value.height = `${event.height}px`;
}
</script>

<style scoped>
.mlxflux-node {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color);
  border: 1px solid var(--node-border-color);
  border-radius: 4px;
  color: var(--node-text-color);
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.parameters-panel {
  margin-bottom: 10px;
  padding: 5px;
  border: 1px solid #666;
  border-radius: 4px;
  background-color: #444;
}

.input-field {
  margin-bottom: 8px;
}

.input-label {
  font-size: 12px;
  margin-bottom: 4px;
  display: block;
}

.input-text,
.input-select,
.input-textarea {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-textarea {
  resize: vertical;
}

.image-preview {
  margin-top: 10px;
  text-align: center;
}
</style>
