<template>
  <div 
    :style="computedContainerStyle"
    class="node-container openai-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">Open AI / Local</div>

    <!-- Provider Selection -->
    <BaseSelect 
      label="Provider" 
      v-model="provider" 
      :options="providerOptions"
    />

    <!-- Parameters Accordion -->
    <BaseAccordion title="Parameters">
      <!-- Endpoint Input -->
      <BaseInput 
        label="Endpoint" 
        v-model="endpoint"
      />

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

      <!-- Model Selection -->
      <BaseInput 
        :id="`${data.id}-model`" 
        label="Model" 
        v-model="model"
      />

      <!-- Max Completion Tokens Input -->
      <BaseInput 
        :id="`${data.id}-max_completion_tokens`" 
        label="Max Completion Tokens" 
        type="number" 
        v-model.number="max_completion_tokens"
        min="1"
      />

      <!-- Temperature Input -->
      <BaseInput 
        :id="`${data.id}-temperature`" 
        label="Temperature" 
        type="number" 
        v-model.number="temperature"
        step="0.1" 
        min="0" 
        max="2"
      />

      <!-- Toggle for Tool/Function Calling -->
      <BaseCheckbox 
        label="Enable Tool/Function Calls" 
        v-model="enableToolCalls" 
      />

      <!-- Predefined System Prompt Dropdown -->
      <BaseSelect 
        label="Predefined System Prompt" 
        v-model="selectedSystemPrompt" 
        :options="systemPromptOptionsList"
      />

      <!-- System Prompt -->
      <BaseTextarea 
        :id="`${data.id}-system_prompt`" 
        label="System Prompt" 
        v-model="system_prompt"
      />
    </BaseAccordion>

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
      :height="760" 
      :min-width="380" 
      :min-height="760"
      :node-id="id" 
      @resize="onResize" 
    />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseCheckbox from '@/components/base/BaseCheckbox.vue'
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useAgentNode } from '@/composables/useAgentNode'

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
        endpoint: '',
        api_key: "",
        model: 'local',
        system_prompt: 'You are a helpful assistant.',
        user_prompt: 'Write a haiku about manifolds.',
        max_completion_tokens: 8192,
        temperature: 0.6,
      },
      outputs: { response: '' },
      models: ['local', 'chatgpt-4o-latest', 'gpt-4o', 'gpt-4o-mini', 'o1-mini', 'o1', 'o3-mini'],
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '760px',
      },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

// Pass Vue Flow instance to the useAgentNode composable
const vueFlowInstance = useVueFlow()
props.vueFlowInstance = vueFlowInstance

// Use the composable to manage state and functionality
const {
  // State
  showApiKey,
  enableToolCalls,
  selectedSystemPrompt,
  isHovered,
  
  // Options
  providerOptions,
  systemPromptOptionsList,
  
  // Computed properties
  provider,
  endpoint,
  api_key,
  model,
  max_completion_tokens,
  temperature,
  system_prompt,
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
