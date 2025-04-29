<template>
  <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
    class="node-container flow-control-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Mode Selection Dropdown -->
    <div class="input-field">
      <label for="mode-select" class="input-label">Mode:</label>
      <select id="mode-select" v-model="mode" class="input-select">
        <option value="RunAllChildren">Run All Children</option>
        <option value="JumpToNode">Jump To Node</option>
        <option value="ForEachDelimited">For Each Delimited</option>
        <option value="Wait">Wait</option>
        <option value="Combine">Combine</option>
      </select>
    </div>

    <!-- Conditional Input Field for JumpToNode -->
    <div v-if="mode === 'JumpToNode'">
      <div class="input-field">
        <label :for="`${data.id}-targetNodeId`" class="input-label">Target Node ID:</label>
        <input :id="`${data.id}-targetNodeId`" type="text" v-model="targetNodeId" class="input-text" />
      </div>
    </div>

    <!-- Conditional Input Field for ForEachDelimited -->
    <div v-if="mode === 'ForEachDelimited'">
      <div class="input-field">
        <label :for="`${data.id}-delimiter`" class="input-label">Delimiter:</label>
        <input :id="`${data.id}-delimiter`" type="text" v-model="delimiter" class="input-text" placeholder="e.g. ," />
      </div>
    </div>

    <!-- Conditional Input Field for Wait -->
    <div v-if="mode === 'Wait'">
      <div class="input-field">
        <label :for="`${data.id}-waitTime`" class="input-label">Wait Time (seconds):</label>
        <input :id="`${data.id}-waitTime`" type="number" v-model="waitTime" class="input-text" min="1" />
      </div>
    </div>

    <!-- Conditional Input Field for Combine -->
    <div v-if="mode === 'Combine'">
      <div class="input-field">
        <label class="input-label">Combine Mode:</label>
        <div class="radio-group">
          <label class="radio-label">
            <input type="radio" v-model="combineMode" value="newline" />
            <span>Newline</span>
          </label>
          <label class="radio-label">
            <input type="radio" v-model="combineMode" value="continuous" />
            <span>Continuous</span>
          </label>
        </div>
      </div>
    </div>

    <!-- Input/Output Handles -->
    <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" />
    <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" />

    <!-- NodeResizer -->
    <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
      :width="320" :height="180" :min-width="320" :min-height="180" :node-id="props.id" @resize="onResize" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { Handle, useVueFlow } from '@vue-flow/core'
import { NodeResizer } from '@vue-flow/node-resizer'

const { getEdges, findNode } = useVueFlow();


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
        delimiter: '',        // Delimiter for ForEachDelimited mode
        waitTime: 5,          // Wait time in seconds for Wait mode
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

// Initialize inputs if they don't exist
if (!props.data.inputs) {
  props.data.inputs = {
    mode: 'RunAllChildren',
    targetNodeId: '',
    delimiter: '',
    waitTime: 5
  }
}

// Ensure waitTime is initialized if not present
if (props.data.inputs.waitTime === undefined) {
  props.data.inputs.waitTime = 5;
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

const delimiter = computed({
  get: () => props.data.inputs.delimiter || '',
  set: (value) => { props.data.inputs.delimiter = value },
})

const waitTime = computed({
  get: () => props.data.inputs.waitTime || 5,
  set: (value) => { props.data.inputs.waitTime = parseInt(value) || 5 },
})

const combineMode = computed({
  get: () => props.data.inputs.combineMode || 'newline',
  set: (value) => { props.data.inputs.combineMode = value },
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
 * 
 * - In "ForEachDelimited" mode, it splits the input by a delimiter and
 *   runs connected child nodes once for each split part.
 * 
 * - In "Wait" mode, it waits for the specified number of seconds before
 *   continuing execution.
 * 
 * - In "Combine" mode, it aggregates outputs from connected source nodes
 *   into a single string with line breaks.
 */
async function run() {
  console.log(`Running FlowControl node: ${props.id} in mode: ${mode.value}`);

  props.data.outputs = {
        result: {
          output: ''
        }
      }

  const connectedSources = getEdges.value
    .filter((edge) => edge.target === props.id)
    .map((edge) => edge.source);

  console.log(`FlowControl (${props.id}): Connected sources: ${connectedSources}`);

  if (connectedSources.length > 0) {
    for (const sourceId of connectedSources) {
      const sourceNode = findNode(sourceId);
      if (sourceNode) {
        props.data.outputs.result.output = sourceNode.data.outputs.result.output;

        console.log(`FlowControl (${props.id}): Output from source node ${sourceId}: ${props.data.outputs.result.output}`);
      }
    }
  }

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
  } else if (mode.value === 'Wait') {
    const seconds = waitTime.value;
    
    if (!seconds || seconds <= 0) {
      console.warn(`FlowControl (${props.id}): Wait mode selected, but invalid wait time provided: ${seconds}`);
      return null; // Continue execution if invalid time
    }
    
    console.log(`FlowControl (${props.id}): Waiting for ${seconds} seconds...`);
    
    // Use a promise to wait for the specified time
    await new Promise(resolve => setTimeout(resolve, seconds * 1000));
    
    console.log(`FlowControl (${props.id}): Wait complete after ${seconds} seconds.`);
    return null; // Continue normal execution after waiting
  } else if (mode.value === 'ForEachDelimited') {
    // Handle For Each Delimited mode
    const currentDelimiter = delimiter.value;
    
    if (!currentDelimiter) {
      console.warn(`FlowControl (${props.id}): ForEachDelimited mode selected, but no delimiter provided.`);
      return { stopPropagation: true };
    }

    // Get the input text from the source node
    const inputText = props.data.outputs.result.output || '';
    
    if (!inputText) {
      console.warn(`FlowControl (${props.id}): No input text available for splitting.`);
      return { stopPropagation: true };
    }

    // Split the input text by the delimiter
    const splitTexts = inputText.split(currentDelimiter).map(item => item.trim());
    console.log(`FlowControl (${props.id}): Split text into ${splitTexts.length} parts using delimiter: "${currentDelimiter}"`);

    // Get all immediate child nodes
    const childNodeIds = getEdges.value
      .filter(edge => edge.source === props.id)
      .map(edge => edge.target);

    if (childNodeIds.length === 0) {
      console.warn(`FlowControl (${props.id}): No child nodes connected to process split text.`);
      return { stopPropagation: true };
    }

    // Initialize or reset the state
    // Always create a fresh state at the beginning of each workflow run
    if (!props.data.forEachState || props.data.forEachState.reset || props.data.forEachState.completed) {
      console.log(`FlowControl (${props.id}): Initializing fresh state with ${splitTexts.length} items`);
      props.data.forEachState = { 
        currentIndex: 0, 
        totalItems: splitTexts.length,
        reset: false,
        completed: false
      };
    }

    // Check if we've processed all items
    if (props.data.forEachState.currentIndex >= props.data.forEachState.totalItems) {
      // We've finished processing all items, mark as completed
      console.log(`FlowControl (${props.id}): Completed processing all ${props.data.forEachState.totalItems} items.`);
      props.data.forEachState.completed = true;
      return { stopPropagation: true };
    }

    // Get the current item to process
    const currentIndex = props.data.forEachState.currentIndex;
    const splitText = splitTexts[currentIndex];
    
    console.log(`FlowControl (${props.id}): Processing part ${currentIndex+1}/${splitTexts.length}: "${splitText}"`);
    
    // Set the current split text as this node's output
    props.data.outputs.result.output = splitText;
    
    // Increment the index for next iteration
    props.data.forEachState.currentIndex++;
    
    // For each immediate child node, trigger execution
    for (const childId of childNodeIds) {
      // Create a special "jump" signal that will tell the workflow executor
      // to execute from this child node, but prevent normal propagation after completion
      console.log(`FlowControl (${props.id}): Executing workflow from child node ${childId} with input: "${splitText}"`);
      
      // This special signal tells App.vue to run the whole downstream workflow from this point
      // and then return to this node to process the next item
      return { 
        forEachJump: childId, 
        parentId: props.id,
        currentIndex: currentIndex + 1,
        totalItems: splitTexts.length
      };
    }
    
    // This should not happen if there are child nodes (we checked earlier)
    console.warn(`FlowControl (${props.id}): No children to execute for item ${currentIndex+1}.`);
    return { stopPropagation: true };
  } else if (mode.value === 'Combine') {
    console.log(`FlowControl (${props.id}): Combine mode activated.`);

    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    if (connectedSources.length === 0) {
      console.warn(`FlowControl (${props.id}): No connected source nodes to combine.`);
      return { stopPropagation: true };
    }

    let combinedOutput = '';
    for (const sourceId of connectedSources) {
      const sourceNode = findNode(sourceId);
      if (sourceNode && sourceNode.data.outputs.result.output) {
        combinedOutput += combineMode.value === 'newline'
          ? `${sourceNode.data.outputs.result.output}\n`
          : sourceNode.data.outputs.result.output;
      }
    }

    // Remove trailing newline for newline mode
    if (combineMode.value === 'newline') {
      combinedOutput = combinedOutput.trim();
    }

    console.log(`FlowControl (${props.id}): Combined output: "${combinedOutput}"`);
    props.data.outputs.result.output = combinedOutput;

    return null; // Continue normal execution
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
@import '@/assets/css/nodes.css';
/* Assuming common styles */

.flow-control-node {
  /* width: 100%; */
  /* Handled by customStyle */
  /* height: 100%; */
  /* Handled by customStyle */
  display: flex;
  flex-direction: column;
  box-sizing: border-box;
  background-color: var(--node-bg-color, #333);
  border: 1px solid var(--node-border-color, #666);
  border-radius: 12px;
  /* Match AgentNode */
  color: var(--node-text-color, #eee);
  padding: 15px;
  /* Match AgentNode */
}

.node-label {
  color: var(--node-text-color, #eee);
  font-size: 14px;
  /* Standardize label size */
  text-align: center;
  margin-bottom: 15px;
  /* More space */
  font-weight: bold;
}

.input-field {
  position: relative;
  margin-bottom: 12px;
  /* More space */
  text-align: left;
  /* Align labels left */
}

.input-label {
  display: block;
  /* Ensure label is on its own line */
  font-size: 12px;
  margin-bottom: 4px;
  /* Space between label and input */
  color: #ccc;
  /* Lighter label color */
}

.input-select,
.input-text {
  background-color: #222;
  /* Darker input background */
  border: 1px solid #555;
  /* Slightly lighter border */
  color: #eee;
  padding: 8px;
  /* More padding */
  font-size: 12px;
  width: 100%;
  /* Use full width */
  box-sizing: border-box;
  border-radius: 4px;
  /* Rounded corners */
}

.input-select {
  appearance: none;
  /* Custom dropdown arrow */
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='5' viewBox='0 0 10 5'%3E%3Cpath fill='%23ccc' d='M0 0l5 5 5-5z'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;
  padding-right: 25px;
  /* Space for arrow */
}

.radio-group {
  display: flex;
  gap: 10px;
}

.radio-label {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 12px;
  color: #ccc;
}

/* Focus styles for accessibility */
.input-select:focus,
.input-text:focus {
  outline: none;
  border-color: #007bff;
  /* Highlight focus */
  box-shadow: 0 0 0 2px rgba(0, 123, 255, 0.25);
}
</style>
