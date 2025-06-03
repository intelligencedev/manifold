<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="800"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle" class="node-label">
        {{ provider === 'anthropic' ? 'Claude / Anthropic' :
           provider === 'google' ? 'Gemini / Google' :
           'Open AI / Local' }}
      </div>
    </template>

    <!-- Provider -->
    <BaseDropdown label="Provider" v-model="provider" :options="providerOptions" />

    <!-- Parameters -->
    <BaseAccordion title="Parameters">
      <BaseInput label="Endpoint" v-model="endpoint" @blur="fetchLlamaServerModel" />

      <div class="relative">
        <BaseInput
          :id="`${data.id}-api_key`"
          :label="provider === 'anthropic' ? 'Anthropic API Key' : provider === 'google' ? 'Google AI API Key' : 'OpenAI API Key'"
          v-model="api_key"
          :type="showApiKey ? 'text' : 'password'"
        >
          <template #suffix><BaseTogglePassword v-model="showApiKey" /></template>
        </BaseInput>
      </div>

      <!-- Model selection: dropdown for Anthropic/Google, text input for others -->
      <template v-if="provider === 'anthropic' || provider === 'google'">
        <BaseDropdown :id="`${data.id}-model`" label="Model" v-model="model" :options="modelOptions" />
      </template>
      <template v-else-if="provider === 'llama-server'">
        <div class="relative">
          <BaseInput :id="`${data.id}-model`" label="Model" v-model="model" :disabled="isLoadingModel" />
          <span v-if="isLoadingModel" class="absolute right-10 top-1/2 transform -translate-y-1/2 text-xs text-blue-400">Loading...</span>
        </div>
      </template>
      <template v-else>
        <BaseInput :id="`${data.id}-model`" label="Model" v-model="model" />
      </template>
      
      <div class="grid grid-cols-2 gap-4">
        <BaseInput
          :id="`${data.id}-max_tokens`"
          label="Max Tokens"
          type="number"
          v-model.number="max_completion_tokens"
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
      </div>

      <!-- Extra LLM params for openai, llama-server, mlx_lm.server -->
      <template v-if="['openai','llama-server','mlx_lm.server'].includes(provider)">
        <div class="grid grid-cols-2 gap-4 mt-2">
          <BaseInput
            :id="`${data.id}-presence_penalty`"
            label="Presence Penalty"
            type="number"
            v-model.number="presence_penalty"
            step="0.01"
            min="-2"
            max="2"
          />
          <BaseInput
            :id="`${data.id}-top_p`"
            label="Top P"
            type="number"
            v-model.number="top_p"
            step="0.01"
            min="0"
            max="1"
          />
          <BaseInput
            :id="`${data.id}-top_k`"
            label="Top K"
            type="number"
            v-model.number="top_k"
            min="0"
            :disabled="provider !== 'mlx_lm.server'"
          />
          <BaseInput
            :id="`${data.id}-min_p`"
            label="Min P"
            type="number"
            v-model.number="min_p"
            step="0.01"
            min="0"
            max="1"
          />
        </div>
      </template>

      <BaseDropdown
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
      :style="computedContainerStyle"      
      label="User Prompt"
      v-model="user_prompt"
      class="user-prompt-area flex-1 w-full h-full"
      @mouseenter="handleTextareaMouseEnter"
      @mouseleave="handleTextareaMouseLeave"
    />

    <button
      v-if="data.outputs && data.outputs.response"
      class="flex items-center justify-center my-2 px-3 py-1 bg-gray-700 text-white rounded text-sm transition-colors hover:bg-blue-800"
      @click="sendToCodeEditor"
      title="Send code to the editor"
    >
      <span class="mr-1">📝</span> Send to Code Editor
    </button>

    <Handle v-if="data.hasInputs" type="target" position="left" style="width:12px;height:12px" />
    <Handle v-if="data.hasOutputs" type="source" position="right" style="width:12px;height:12px" />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseInput from '@/components/base/BaseInput.vue'
import BaseDropdown from '@/components/base/BaseDropdown.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseTogglePassword from '@/components/base/BaseTogglePassword.vue'
import BaseAccordion from '@/components/base/BaseAccordion.vue'
import BaseNode from '@/components/base/BaseNode.vue'
import { useAgentNode } from '@/composables/useAgentNode'

const props = defineProps({
  id: { type:String, required:true, default:'Completions_0' },
  data:{
    type:Object,
    default:()=>({
      type:'Completions', labelStyle:{ fontWeight:'normal' },
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
    })
  }
})

const emit = defineEmits(['update:data','resize','disable-zoom','enable-zoom'])

const {
  showApiKey, selectedSystemPrompt,
  providerOptions, systemPromptOptionsList, modelOptions,
  provider, endpoint, api_key, model, max_completion_tokens, temperature,
  presence_penalty, top_p, top_k, min_p,
  system_prompt, user_prompt,
  onResize, handleTextareaMouseEnter, handleTextareaMouseLeave, sendToCodeEditor,
  isLoadingModel
} = useAgentNode(props, emit)

if (!props.data.outputs) props.data.outputs = { response:'', error:null }
</script>
