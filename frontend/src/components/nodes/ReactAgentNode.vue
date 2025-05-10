<template>
  <div 
    :style="computedContainerStyle"
    class="node-container openai-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">ReAct Agent</div>

    <!-- User Prompt -->
    <BaseTextarea 
      :id="`${data.id}-user_prompt`" 
      label="User Prompt" 
      v-model="user_prompt"
      fullHeight
      class="user-prompt-area"
      @mouseenter="handleTextareaMouseEnter" 
      @mouseleave="handleTextareaMouseLeave"
    />

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
      :width="380" 
      :height="600" 
      :min-width="380" 
      :min-height="500"
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import { useReactAgent } from '@/composables/useReactAgent'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'ReactAgent_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'ReactAgent',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        // Hardcoded endpoint for agent
        endpoint: '/api/agents/react',
        api_key: "",
        user_prompt: 'What can I help you with today?',
      },
      outputs: { response: '' },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '380px',
        height: '600px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

// Pass Vue Flow instance to the useAgentNode composable
const vueFlowInstance = useVueFlow()
props.vueFlowInstance = vueFlowInstance

// Ensure that the data.outputs structure is properly initialized
if (!props.data.outputs) {
  props.data.outputs = { response: '', error: null }
}

// Use the composable to manage state and functionality
const {
  // State
  isHovered,
  
  // Computed properties
  user_prompt,
  resizeHandleStyle,
  computedContainerStyle,
  
  // Methods
  onResize,
  handleTextareaMouseEnter,
  handleTextareaMouseLeave,
  sendToCodeEditor
} = useReactAgent(props, emit)
</script>

<style scoped>
@import '@/assets/css/nodes.css';

/* Add styling for the think tags that may come from LLMs or our ReAct agent */
:deep(think), :deep(think) {
  display: block;
  background-color: rgba(30, 30, 30, 0.7);
  border-left: 3px solid #666;
  padding: 8px;
  margin: 8px 0;
  font-family: monospace;
  white-space: pre-wrap;
  color: #aaa;
}
</style>