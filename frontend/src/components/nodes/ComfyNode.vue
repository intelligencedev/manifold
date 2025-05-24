<template>
  <BaseNode :id="id" :data="data" :min-height="512" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
    </template>

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
        class="h-40"
      />
    </BaseAccordion>

    <!-- Generated Image Panel -->
    <div v-if="generatedImage" class="flex-1 relative overflow-hidden flex items-center justify-center pt-5">
      <img :src="generatedImage" alt="Generated Image" class="max-w-full max-h-full object-contain block" />
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

  </BaseNode>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
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
