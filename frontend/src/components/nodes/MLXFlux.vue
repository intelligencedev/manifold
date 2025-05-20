<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container mlxflux-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Parameters Panel (using BaseAccordion) -->
    <BaseAccordion title="Parameters" :initiallyOpen="true">
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

        <!-- Quality Input with Radio Buttons -->
        <div class="input-field">
          <label class="input-label">Quality:</label>
          <div class="radio-group">
            <label class="radio-label">
              <input type="radio" :name="`${data.id}-quality`" :value="4" v-model.number="quality" />
              4bit
            </label>
            <label class="radio-label">
              <input type="radio" :name="`${data.id}-quality`" :value="8" v-model.number="quality" />
              8bit
            </label>
          </div>
        </div>

        <!-- Output Path Input -->
        <div class="input-field">
          <label :for="`${data.id}-output`" class="input-label">File Name:</label>
          <BaseInput :id="`${data.id}-output`" v-model="output" class="input-text" />
        </div>
      </div>
    </BaseAccordion>

    <!-- Generated Image Panel -->
    <div class="generated-image-panel" v-if="imageSrc">
      <img :src="imageSrc" alt="Generated Image" class="generated-image" />
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :min-width="350" :min-height="720" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core';
import { NodeResizer } from '@vue-flow/node-resizer';
import BaseInput from '@/components/base/BaseInput.vue';
import BaseTextarea from '@/components/base/BaseTextarea.vue';
import BaseAccordion from '@/components/base/BaseAccordion.vue';
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
        height: '1024px',
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
  text-align: left;
  margin-bottom: 10px;
  font-weight: bold;
  padding-left: 10px;
}

.parameters-panel {
  padding: 5px;
  background-color: var(--accordion-bg-color, #444);
}

.input-field {
  margin-bottom: 16px;
}

.input-label {
  font-size: 12px;
  margin-bottom: 4px;
  display: block;
  text-align: left;
}

.input-text,
.input-select,
.input-textarea {
  background-color: var(--input-bg-color, #333);
  border: 1px solid var(--input-border-color, #666);
  color: var(--input-text-color, #eee);
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.input-textarea {
  resize: vertical;
  min-height: 80px;
}

.radio-group {
  display: flex;
  gap: 12px;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  cursor: pointer;
}

/* Generated Image Panel - Similar to ComfyNode */
.generated-image-panel {
  flex: 1 1 auto;
  position: relative;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 20px;
}

.generated-image {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  display: block;
}
</style>
