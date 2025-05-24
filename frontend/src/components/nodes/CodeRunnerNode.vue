<template>
  <BaseNode
    :id="id"
    :data="data"
    :min-height="200"
    @resize="onResize"
  >
    <template #header>
      <div :style="data.labelStyle">Code Runner</div>
    </template>

    <BaseTextarea
      :id="`${data.id}-command`"
      label="Script"
      v-model="command"
      rows="5"
      class="h-32"
    />

    <Handle
      style="width:12px;height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />
    <Handle
      style="width:12px;height:12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />
  </BaseNode>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import BaseNode from '@/components/base/BaseNode.vue'
import BaseTextarea from '@/components/base/BaseTextarea.vue'
import { onMounted } from 'vue'
import { useCodeRunner } from '@/composables/useCodeRunner'

const props = defineProps({
  id: {
    type: String,
    required: false,
    default: 'Code_Runner_0'
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      labelStyle: {},
      type: 'Code Runner',
      inputs: {
        // By default, a simple JS snippet
        command: 'console.log("Hello world!")'
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%'
    })
  }
})

const emit = defineEmits(['update:data', 'resize'])

// Use the new CodeRunner composable
const { command, updateNodeData, run } = useCodeRunner(props, emit)

function onResize({ width, height }) {
  props.data.style.width = `${width}px`
  props.data.style.height = `${height}px`
  updateNodeData()
  emit('resize', { id: props.id, width, height })
}

// Assign run() function once component is mounted
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
  // Initialize default style if missing
  if (!props.data.style) props.data.style = {}
})
</script>
