<template>
  <div
    :style="computedContainerStyle"
    class="node-container openai-node tool-node"
    @mouseenter="isHovered = true"
    @mouseleave="isHovered = false"
  >
    <div :style="data.labelStyle" class="node-label">Open AI / Local</div>

    <!-- Provider -->
    <BaseSelect label="Provider" v-model="provider" :options="providerOptions" />

    <BaseAccordion title="Parameters">
      <BaseInput label="Endpoint" v-model="endpoint" />

      <div class="input-wrapper">
        <BaseInput
          :id="`${data.id}-api_key`"
          label="OpenAI API Key"
          v-model="api_key"
          :type="showApiKey ? 'text' : 'password'"
        >
          <template #suffix><BaseTogglePassword v-model="showApiKey" /></template>
        </BaseInput>
      </div>

      <BaseInput :id="`${data.id}-model`" label="Model" v-model="model" />
      <BaseInput
        :id="`${data.id}-max_tokens`"
        label="Max Tokens"
        type="number"
        v-model.number="max_tokens"
        min="1"
      />
      <BaseInput
        :id="`${data.id}-temperature`"
        label="Temperature"
        type="number"
        v-model.number="temperature"
        step="0.1"
        min="0"
        max="2"
      />

      <BaseCheckbox label="Agent mode (/api/agents/react)" v-model="agentMode" />

      <BaseSelect
        label="Predefined System Prompt"
        v-model="selectedSystemPrompt"
        :options="systemPromptOptionsList"
      />

      <BaseTextarea
        :id="`${data.id}-system_prompt`"
        label="System Prompt"
        v-model="system_prompt"
      />
    </BaseAccordion>

    <BaseTextarea
      :id="`${data.id}-user_prompt`"
      label="User Prompt"
      v-model="user_prompt"
      fullHeight
      class="user-prompt-area"
      @mouseenter="handleTextareaMouseEnter"
      @mouseleave="handleTextareaMouseLeave"
    />

    <button
      v-if="data.outputs && data.outputs.response"
      class="code-editor-button"
      @click="sendToCodeEditor"
      title="Send code to the editor"
    >
      <span class="button-icon">📝</span> Send to Code Editor
    </button>

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />

    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :width="380"
      :height="906"
      :min-width="380"
      :min-height="906"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseSelect from '@/components/base/BaseSelect.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseCheckbox from '@/components/base/BaseCheckbox.vue'
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import { useAgentNode } from '@/composables/useAgentNode'

const props = defineProps({
  id: { type:String, required:true, default:'Agent_0' },
  data:{
    type:Object,
    default:()=>({
      type:'AgentNode', labelStyle:{ fontWeight:'normal' },
      hasInputs:true, hasOutputs:true,
      inputs:{
        endpoint:'',
        api_key:'',
        model:'local',
        system_prompt:'You are a helpful assistant.',
        user_prompt:'Write a haiku about manifolds.',
        max_completion_tokens:8192,
        temperature:0.6
      },
      outputs:{ response:'' },
      style:{
        border:'1px solid #666', borderRadius:'12px',
        background:'#333', color:'#eee',
        width:'380px', height:'906px'
      }
    })
  }
})

const emit = defineEmits(['update:data','resize','disable-zoom','enable-zoom'])

const {
  showApiKey, agentMode, selectedSystemPrompt, isHovered,
  providerOptions, systemPromptOptionsList,
  provider, endpoint, api_key, model, max_tokens, temperature,
  system_prompt, user_prompt,
  resizeHandleStyle, computedContainerStyle,
  onResize, handleTextareaMouseEnter, handleTextareaMouseLeave, sendToCodeEditor
} = useAgentNode(props, emit)

if (!props.data.outputs) props.data.outputs = { response:'', error:null }
</script>

<style scoped>
@import '@/assets/css/nodes.css';
.input-wrapper{position:relative}
.code-editor-button{
  display:flex;align-items:center;justify-content:center;margin:10px 0;
  padding:6px 12px;background:#4a5568;color:#fff;border:none;border-radius:4px;
  cursor:pointer;font-size:.9em;transition:background-color .2s;
}
.code-editor-button:hover{background:#2c5282}
.button-icon{margin-right:5px}
</style>
