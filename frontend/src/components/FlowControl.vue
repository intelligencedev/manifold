<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
       class="node-container flow-control-node tool-node"
       @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Mode Selection Dropdown -->
    <div class="input-field">
      <label for="mode-select" class="input-label">Mode:</label>
      <select id="mode-select" v-model="mode" class="input-select">
        <option value="RunAllChildren">Run All Children</option>
        <option value="JumpToNode">Jump To Node</option>
      </select>
    </div>

    <!-- Conditional Input Field for JumpToNode -->
    <div v-if="mode === 'JumpToNode'">
      <div class="input-field">
        <label :for="`${data.id}-targetNodeId`" class="input-label">Target Node ID:</label>
        <input :id="`${data.id}-targetNodeId`" type="text" v-model="targetNodeId" class="input-text" />
      </div>
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle"
                 :line-style="resizeHandleStyle" :width="320" :height="180"
                 :min-width="320" :min-height="180" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'FlowControl_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'FlowControl',
      labelStyle: { fontWeight: 'normal' },
      hasInputs: true,
      hasOutputs: true,
      inputs: {
        mode: 'RunAllChildren', // Default mode
        targetNodeId: '',     // Target node for JumpToNode mode
      },
      outputs: {
        // Flow control nodes typically don't produce data output,
        // but manage execution flow.
      },
      style: {
        border: '1px solid #666',
        borderRadius: '12px',
        backgroundColor: '#333',
        color: '#eee',
        width: '320px',
        height: '180px', // Adjusted default height
      },
    }),
  },
})

const emit = defineEmits(['resize', 'disable-zoom', 'enable-zoom'])

const { findNode } = useVueFlow() // Removed getEdges as it's handled in App.vue

// Initialize inputs if they don't exist
if (!props.data.inputs) {
  props.data.inputs = {
    mode: 'RunAllChildren',
    targetNodeId: ''
  }
}

// Initialize outputs if they don't exist or ensure output structure is consistent
if (!props.data.outputs) {
  props.data.outputs = { result: { output: '' } }
} else if (!props.data.outputs.result) {
  props.data.outputs.result = { output: '' }
} else if (!props.data.outputs.result.output) {
  props.data.outputs.result.output = ''
}

// Computed properties for inputs
const mode = computed({
  get: () => props.data.inputs.mode || 'RunAllChildren', // Ensure default
  set: (value) => { props.data.inputs.mode = value },
})

const targetNodeId = computed({
  get: () => props.data.inputs.targetNodeId || '',
  set: (value) => { props.data.inputs.targetNodeId = value },
})

// --- UI State ---
const isHovered = ref(false)
const customStyle = ref({
  width: props.data.style?.width || '320px',
  height: props.data.style?.height || '180px',
})

const resizeHandleStyle = computed(() => ({
  visibility: isHovered.value ? 'visible' : 'hidden',
  width: '12px',
  height: '12px',
}))

function onResize(event) {
  customStyle.value.width = `${event.width}px`
  customStyle.value.height = `${event.height}px`
  emit('resize', event)
}

function handleTextareaMouseEnter() {
  emit('disable-zoom')
}

function handleTextareaMouseLeave() {
  emit('enable-zoom')
}

// --- Node Logic ---

/**
 * The run() function for FlowControl.
 *
 * - In "RunAllChildren" mode, it does nothing special; the workflow runner
 *   in App.vue will handle propagating execution to children.
 *
 * - In "JumpToNode" mode, it returns a signal to the workflow runner
 *   indicating which node ID to jump execution to next.
 */
async function run() {
  console.log(`Running FlowControl node: ${props.id} in mode: ${mode.value}`);

  props.data.outputs.result.output = ''; // Reset output

  if (mode.value === 'RunAllChildren') {
    // No special action needed here. The main workflow runner will
    // process outgoing edges and continue execution concurrently.
    console.log(`FlowControl (${props.id}): RunAllChildren mode finished.`);
    return null; // Indicate normal completion, allowing propagation
  } else if (mode.value === 'JumpToNode') {
    const targetId = targetNodeId.value;
    if (!targetId) {
      console.warn(`FlowControl (${props.id}): JumpToNode mode selected, but no Target Node ID provided.`);
      return { stopPropagation: true }; // Stop this execution path
    }

    // Check if target node exists
    const target = findNode(targetId);
    if (!target) {
      console.warn(`FlowControl (${props.id}): Target node ID "${targetId}" not found.`);
      return { stopPropagation: true };
    }

    console.log(`FlowControl (${props.id}): JumpToNode mode signaling jump to -> ${targetId}`);
    return { jumpTo: targetId }; // Signal the jump to the workflow runner
  }

  // Default case (shouldn't happen with current modes)
  return null;
}

// Ensure run function is assigned to node data
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
  // Initialize customStyle from potentially loaded data
  customStyle.value.width = props.data.style?.width || '320px';
  customStyle.value.height = props.data.style?.height || '180px';
});

// Watch for data changes if loaded from file, update internal state
watch(() => props.data.inputs, (newInputs) => {
  // This ensures computed props update if data is loaded externally
}, { deep: true });

watch(() => props.data.style, (newStyle) => {
    customStyle.value.width = newStyle?.width || '320px';
    customStyle.value.height = newStyle?.height || '180px';
}, { deep: true });

</script>

<style scoped>
/* Import or define styles */
@import '@/assets/css/nodes.css'; /* Assuming common styles */

.flow-control-node {
  /* width: 100%; */ /* Handled by customStyle */
  /* height: 100%; */ /* Handled by customStyle */
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color, #333);
  border: 1px solid var(--node-border-color, #666);
  border-radius: 12px; /* Match AgentNode */
  color: var(--node-text-color, #eee);
  padding: 15px; /* Match AgentNode */
}

.node-label {
  color: var(--node-text-color, #eee);
  font-size: 14px; /* Standardize label size */
  text-align: center;
  margin-bottom: 15px; /* More space */
  font-weight: bold;
}

.input-field {
  position: relative;
  margin-bottom: 12px; /* More space */
  text-align: left; /* Align labels left */
}

.input-label {
  display: block; /* Ensure label is on its own line */
  font-size: 12px;
  margin-bottom: 4px; /* Space between label and input */
  color: #ccc; /* Lighter label color */
}

.input-select,
.input-text {
  background-color: #222; /* Darker input background */
  border: 1px solid #555; /* Slightly lighter border */
  color: #eee;
  padding: 8px; /* More padding */
  font-size: 12px;
  width: 100%; /* Use full width */
  box-sizing: border-box;
  border-radius: 4px; /* Rounded corners */
}

.input-select {
  appearance: none; /* Custom dropdown arrow */
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='5' viewBox='0 0 10 5'%3E%3Cpath fill='%23ccc' d='M0 0l5 5 5-5z'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;
  padding-right: 25px; /* Space for arrow */
}

/* Focus styles for accessibility */
.input-select:focus,
.input-text:focus {
  outline: none;
  border-color: #007bff; /* Highlight focus */
  box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
}
</style>
