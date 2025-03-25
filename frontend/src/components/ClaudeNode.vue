<template>
  <div 
    :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }" 
    class="node-container claude-node tool-node" 
    @mouseenter="isHovered = true" 
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Model Selection -->
    <BaseSelect 
      :id="`${data.id}-model`" 
      label="Model" 
      v-model="selectedModel" 
      :options="models"
    />

    <!-- Predefined System Prompt Dropdown -->
    <BaseSelect 
      id="system-prompt-select" 
      label="Predefined System Prompt" 
      v-model="selectedSystemPrompt" 
      :options="Object.entries(systemPromptOptions).map(([key, value]) => ({ value: key, label: value.role }))"
    />

    <!-- System Prompt -->
    <BaseTextarea 
      :id="`${data.id}-system_prompt`" 
      label="System Prompt (Optional)" 
      v-model="system_prompt"
    />

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

    <!-- Max Tokens -->
    <BaseInput 
      :id="`${data.id}-max_tokens`" 
      label="Max Tokens" 
      type="number" 
      v-model.number="max_tokens"
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
      :min-width="380" 
      :min-height="560" 
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
import { useClaudeNode } from '@/composables/useClaudeNode'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'Claude_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'ClaudeNode',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        api_key: '',
        model: 'claude-3-7-sonnet-latest',
        system_prompt: '',
        user_prompt: 'Hello, Claude!',
        max_tokens: 1024
      },
      outputs: { response: '' },
      models: ['claude-3-7-sonnet-latest', 'claude-3-5-sonnet-latest', 'claude-3-5-haiku-latest'],
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '380px',
        height: '560px'
      }
    })
  }
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

// Pass Vue Flow instance to the composable
const vueFlowInstance = useVueFlow()

// Use the composable to manage state and functionality
const {
  // State
  isHovered,
  selectedSystemPrompt,
  
  // Options
  systemPromptOptions,
  
  // Computed properties
  selectedModel,
  models,
  system_prompt,
  user_prompt,
  max_tokens,
  api_key,
  resizeHandleStyle,
  customStyle,
  
  // Methods
  onResize,
  handleTextareaMouseEnter,
  handleTextareaMouseLeave
} = useClaudeNode(props, emit, vueFlowInstance)
</script>

<style scoped>
@import '@/assets/css/nodes.css';
</style>