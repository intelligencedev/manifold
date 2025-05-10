<template>
  <div 
    :style="computedContainerStyle"
    class="node-container openai-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">Agent</div>

    <!-- OpenAI API Key Input -->
    <div class="input-wrapper">
      <BaseInput 
        :id="`${data.id}-api_key`" 
        label="OpenAI API Key" 
        v-model="api_key"
        :type="showApiKey ? 'text' : 'password'"
      >
        <template #suffix>
          <BaseTogglePassword v-model="showApiKey" />
        </template>
      </BaseInput>
    </div>

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

    <!-- Send to Code Editor button - only visible when there's response content -->
    <button 
      v-if="data.outputs && data.outputs.response" 
      class="code-editor-button"
      @click="sendToCodeEditor"
      title="Send code to the editor"
    >
      <span class="button-icon">📝</span> Send to Code Editor
    </button>

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
import BaseInput from '@/components/base/BaseInput.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import { useAgentNode } from '@/composables/useCompletionsNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Agent_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'AgentNode',
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
  showApiKey,
  isHovered,
  
  // Computed properties
  api_key,
  user_prompt,
  resizeHandleStyle,
  computedContainerStyle,
  
  // Methods
  onResize,
  handleTextareaMouseEnter,
  handleTextareaMouseLeave,
  sendToCodeEditor
} = useAgentNode(props, emit)
</script>

<style scoped>
@import '@/assets/css/nodes.css';

/* Additional component-specific styles */
.input-wrapper {
  position: relative;
}

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

/* Code Editor Button Styles */
.code-editor-button {
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 10px 0;
  padding: 6px 12px;
  background-color: #4a5568;
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  font-size: 0.9em;
  transition: background-color 0.2s;
}

.code-editor-button:hover {
  background-color: #2c5282;
}

.button-icon {
  margin-right: 5px;
}
</style>