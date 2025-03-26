<template>
  <div 
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container comfy-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Parameters Accordion -->
    <BaseAccordion title="Parameters" :initiallyOpen="true">
      <!-- ComfyUI Endpoint Input -->
      <BaseInput 
        label="ComfyUI Endpoint" 
        v-model="endpoint" 
        placeholder="http://comfyui-host:8188/prompt"
      />

      <!-- Image Prompt Input -->
      <BaseTextarea 
        :id="`${data.id}-prompt`" 
        label="Image Prompt" 
        v-model="prompt"
        class="image-prompt-textarea"
      />
    </BaseAccordion>

    <!-- Generated Image Panel -->
    <div class="generated-image-panel" v-if="generatedImage">
      <img :src="generatedImage" alt="Generated Image" class="generated-image" />
    </div>

    <!-- Input/Output Handles -->
    <Handle 
      style="width:12px; height:12px" 
      v-if="data.hasInputs" 
      type="target" 
      position="left" 
    />
    <Handle 
      style="width:12px; height:12px" 
      v-if="data.hasOutputs" 
      type="source" 
      position="right" 
    />

    <!-- NodeResizer -->
    <NodeResizer 
      :is-resizable="true" 
      :color="'#666'" 
      :handle-style="resizeHandleStyle" 
      :line-style="resizeHandleStyle"
      :width="360" 
      :height="673" 
      :min-width="360" 
      :min-height="673" 
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useComfyNode } from '@/composables/useComfyNode'

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

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

// Pass Vue Flow instance to the composable
const vueFlowInstance = useVueFlow()

// Use the composable to manage state and functionality
const {
  // State
  isHovered,
  customStyle,
  generatedImage,
  
  // Computed properties
  endpoint,
  prompt,
  resizeHandleStyle,
  
  // Methods
  onResize
} = useComfyNode(props, emit, vueFlowInstance)
</script>

<style scoped>
@import '@/assets/css/nodes.css';

/* ComfyNode specific styles */
.image-prompt-textarea {
  height: 160px;
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