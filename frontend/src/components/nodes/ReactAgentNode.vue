<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="212"
    :style="customStyle"
    @resize="onResize"
    @mouseenter="$emit('disable-zoom')"
    @mouseleave="$emit('enable-zoom')"
  >
    <template #header>
      <div :style="data.labelStyle">ReAct Agent</div>
    </template>

    <BaseTextarea
      :id="`${data.id}-user_prompt`"
      :style="customStyle"
      label="User Prompt"
      v-model="user_prompt"
      class="user-prompt-area"
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
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import BaseNode from '@/components/base/BaseNode.vue'
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
        endpoint: '/api/agents/react',
        api_key: "",
        user_prompt: 'What can I help you with today?',
      },
      outputs: { response: '' },
    }),
  },
})

const emit = defineEmits(['update:data', 'resize', 'disable-zoom', 'enable-zoom'])

const {
  isHovered,
  user_prompt,
  resizeHandleStyle,
  computedContainerStyle,
  customStyle,
  onResize,
  handleTextareaMouseEnter,
  handleTextareaMouseLeave,
  sendToCodeEditor
} = useReactAgent(props, emit)
</script>
