<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }" class="node-container tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div class="node-label">
      <div>Code Runner</div>
    </div>

    <!-- Use a textarea for multiline code or JSON -->
    <div class="input-field">
      <label :for="`${data.id}-command`" class="input-label">Script:</label>
      <textarea
        :id="`${data.id}-command`"
        v-model="command"
        @change="updateNodeData"
        class="input-text-area"
        rows="5"
      ></textarea>
    </div>

    <Handle
      style="width:12px; height:12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />
    <Handle
      style="width:12px; height:12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />
    <!-- Node resizer for adjusting the node dimensions -->
    <NodeResizer
      :is-resizable="true"
      :color="'#666'"
      :handle-style="resizeHandleStyle"
      :line-style="resizeHandleStyle"
      :min-width="200"
      :min-height="150"
      :node-id="id"
      @resize="onResize"
    />
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
import { onMounted, ref, computed } from 'vue'
import { useCodeRunner } from '@/composables/useCodeRunner'
import { NodeResizer } from '@vue-flow/node-resizer'

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
      style: {},
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

// Styles for resizing
const customStyle = ref({})
const isHovered = ref(false)
const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px'
}))

// Handle resize events
function onResize(event) {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
  props.data.style.width = `${event.width}px`
  props.data.style.height = `${event.height}px`
  updateNodeData()
  emit('resize', { id: props.id, width: event.width, height: event.height })
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

<style scoped>
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
  display: flex;
  flex-direction: column;
  position: relative;
  box-sizing: border-box;
  padding: 10px;
}

.node-label {
  color: var(--node-text-color);
  font-size: 16px;
  text-align: center;
  margin-bottom: 10px;
  font-weight: bold;
}

.input-field {
  margin-bottom: 8px;
  width: 100%;
  flex-grow: 1;
  display: flex;
  flex-direction: column;
}

.input-text-area {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  height: 100%;
  box-sizing: border-box;
  resize: vertical;
  flex-grow: 1;
}
</style>
