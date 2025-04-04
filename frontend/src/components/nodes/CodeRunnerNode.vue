<template>
  <div :style="data.style" class="node-container tool-node">
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
  </div>
</template>

<script setup>
import { Handle } from '@vue-flow/core'
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

const emit = defineEmits(['update:data'])

// Use the new CodeRunner composable
const { command, updateNodeData, run } = useCodeRunner(props, emit)

// Assign run() function once component is mounted
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})
</script>

<style scoped>
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
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
}

.input-text-area {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  height: auto;
  box-sizing: border-box;
  resize: vertical;
}
</style>
