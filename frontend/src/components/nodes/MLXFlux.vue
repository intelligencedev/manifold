<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container mlxflux-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Parameters Panel (no accordion) -->
    <div class="parameters-panel">
      <!-- Model Selection -->
      <div class="input-field">
        <label :for="`${data.id}-model`" class="input-label">Model:</label>
        <BaseInput :id="`${data.id}-model`" v-model="model" class="input-text" />
      </div>

      <!-- Prompt Input -->
      <div class="input-field">
        <label :for="`${data.id}-prompt`" class="input-label">Prompt:</label>
        <BaseTextarea :id="`${data.id}-prompt`" v-model="prompt" class="input-textarea" />
      </div>

      <!-- Steps Input -->
      <div class="input-field">
        <label :for="`${data.id}-steps`" class="input-label">Steps:</label>
        <BaseInput :id="`${data.id}-steps`" v-model.number="steps" type="number" min="1" class="input-text" />
      </div>

      <!-- Seed Input -->
      <div class="input-field">
        <label :for="`${data.id}-seed`" class="input-label">Seed:</label>
        <BaseInput :id="`${data.id}-seed`" v-model.number="seed" type="number" class="input-text" />
      </div>

      <!-- Quality Input -->
      <div class="input-field">
        <label :for="`${data.id}-quality`" class="input-label">Quality:</label>
        <BaseInput :id="`${data.id}-quality`" v-model.number="quality" type="number" min="1" class="input-text" />
      </div>

      <!-- Output Path Input -->
      <div class="input-field">
        <label :for="`${data.id}-output`" class="input-label">Output:</label>
        <BaseInput :id="`${data.id}-output`" v-model="output" class="input-text" />
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
import { Handle, useVueFlow } from '@vue-flow/core';
import { NodeResizer } from '@vue-flow/node-resizer';
import BaseInput from '@/components/base/BaseInput.vue';
import BaseTextarea from '@/components/base/BaseTextarea.vue';
import { useMLXFluxNode } from '@/composables/useMLXFluxNode';

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

const emit = defineEmits(['update:data', 'resize']);
const vueFlowInstance = useVueFlow();

// Use the composable to manage state and functionality
const {
  // State refs
  isHovered,
  customStyle,
  imageSrc,
  
  // Computed properties
  model,
  prompt,
  steps,
  seed,
  quality,
  output,
  resizeHandleStyle,
  
  // Methods
  onResize
} = useMLXFluxNode(props, vueFlowInstance);
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
