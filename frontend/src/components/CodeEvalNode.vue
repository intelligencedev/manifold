<template>
  <div 
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }" 
    class="node-container code-eval-node tool-node"
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Language Selection -->
    <BaseSelect
      label="Language"
      v-model="language"
      :options="[
        { value: 'python', label: 'Python' },
        { value: 'javascript', label: 'JavaScript' },
        { value: 'typescript', label: 'TypeScript' },
        { value: 'bash', label: 'Bash' },
        { value: 'go', label: 'Go' },
        { value: 'ruby', label: 'Ruby' }
      ]"
    />

    <!-- Code Editor -->
    <BaseTextarea 
      label="Code"
      v-model="codeToEvaluate"
      class="code-editor"
    />

    <!-- Results Display -->
    <div class="result-container">
      <div class="result-label">Result:</div>
      <div class="result-content">{{ result }}</div>
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
      :min-width="320" 
      :min-height="400"
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import { useCodeEvalNode } from '@/composables/useCodeEvalNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'CodeEval_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'CodeEvalNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        code: 'print("Hello, world!")',
        language: 'python',
      },
      outputs: {
        result: { output: '' }
      },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '400px',
        height: '500px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize'])

// Pass Vue Flow instance to the composable
const vueFlowInstance = useVueFlow()

// Use the composable to manage state and functionality
const {
  // State refs
  isHovered,
  customStyle,
  
  // Computed properties
  codeToEvaluate,
  language,
  result,
  resizeHandleStyle,
  
  // Methods
  onResize
} = useCodeEvalNode(props, vueFlowInstance)
</script>

<style scoped>


.code-eval-node {
  display: flex;
  flex-direction: column;
  padding: 10px;
}

.code-editor {
  flex: 2;
  font-family: monospace;
  min-height: 150px;
}

.result-container {
  flex: 1;
  margin-top: 10px;
  overflow: auto;
  border: 1px solid #666;
  border-radius: 4px;
  padding: 8px;
  background-color: #222;
}

.result-label {
  font-weight: bold;
  margin-bottom: 5px;
}

.result-content {
  white-space: pre-wrap;
  font-family: monospace;
  overflow: auto;
  max-height: 200px;
}
</style>