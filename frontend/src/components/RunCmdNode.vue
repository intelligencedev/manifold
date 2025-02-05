<template>
  <div :style="data.style" class="node-container tool-node">
    <div class="node-label">
      <input
        v-model="label"
        @change="updateNodeData"
        class="label-input"
        :style="data.labelStyle"
      />
    </div>

    <!-- Checkbox to enable/disable updating input from source -->
    <div class="input-field">
      <input
        type="checkbox"
        :id="`${data.id}-update-from-source`"
        v-model="updateFromSource"
        @change="updateNodeData"
      />
      <label :for="`${data.id}-update-from-source`" class="input-label">Update Input from Source</label>
    </div>

    <!-- Use a textarea for multiline code or JSON -->
    <div class="input-field">
      <label :for="`${data.id}-command`" class="input-label">Command / Code / JSON:</label>
      <textarea
        :id="`${data.id}-command`"
        v-model="command"
        @change="updateNodeData"
        class="input-text-area"
        rows="5"
      ></textarea>
    </div>

    <Handle v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle v-if="data.hasOutputs" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { Handle, useVueFlow } from '@vue-flow/core'
import { ref, computed, onMounted } from 'vue'

const { getEdges, findNode } = useVueFlow()

const props = defineProps({
  id: {
    type: String,
    required: false,
    default: 'RunCmd_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      style: {},
      labelStyle: {},
      type: 'RunCmdNode',
      inputs: {
        // By default, a simple Python snippet
        command: "print('Hello world!')",
      },
      outputs: {
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      inputHandleShape: '50%',
      handleColor: '#777',
      outputHandleShape: '50%',
      updateFromSource: true, // Default to updating from source
    }),
  },
})

const emit = defineEmits(['update:data'])

// Ref for the checkbox state
const updateFromSource = ref(props.data.updateFromSource)

// Assign run() function once component is mounted
onMounted(() => {
  if (!props.data.run) {
    props.data.run = run
  }
})

async function run() {
  try {
    // Clear previous output
    props.data.outputs.result = '';

    // Identify connected source nodes
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);

    let payload;

    if (connectedSources.length > 0 && updateFromSource.value) {
      // Source node might produce JSON
      const sourceData = findNode(connectedSources[0]).data.outputs.result.output;
      console.log('Connected source data:', sourceData);

      // Update the input field with the connected source data
      props.data.inputs.command = sourceData;

      // Attempt to parse JSON
      try {
        payload = JSON.parse(sourceData);
      } catch (err) {
        console.error('Error parsing JSON from connected node:', err);
        props.data.outputs.result = {
          error: 'Invalid JSON from connected node',
        };
        return { error: 'Invalid JSON from connected node' };
      }
    } else {
      // No connected nodes or updateFromSource is false => user typed something in the textarea
      let userInput = props.data.inputs.command;
      // Attempt to parse as JSON
      try {
        payload = JSON.parse(userInput);
      } catch (_err) {
        // Not JSON => treat as raw Python code
        // **FIX: Do NOT escape newlines for general code**
        payload = {
          code: userInput, // Pass code as is, without escaping newlines
          dependencies: [],
        };
      }
    }

    // Default to empty code if none
    if (!payload.code) {
      payload.code = '';
    }
    // Ensure dependencies is an array
    if (!Array.isArray(payload.dependencies)) {
      payload.dependencies = [];
    }

    // POST to /api/executePython
    // TODO: Make the backend configurable
    const response = await fetch('http://localhost:8080/api/executePython', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });

    if (!response.ok) {
      const errorMsg = await response.text();
      console.error('Error response from server:', errorMsg);
      props.data.outputs.result = { error: errorMsg };
      return { error: errorMsg };
    }

    const result = await response.json();
    console.log('Node-level run result:', result);

    // Parse the json for a stdout and stderr key and only return one or the other if its not empty
    const resultStr = result.stdout || result.stderr || '';

    //const resultStr = JSON.stringify(result, null, 2);

    props.data.outputs = {
        result: {
            output: resultStr, // or results, or both, depending on your preference
        },
    }

    updateNodeData();

    return { response, result };
  } catch (error) {
    console.error('Error in run():', error);
    props.data.outputs.result = { error: error.message };
    return { error };
  }
}

// A computed property for the node label
const label = computed({
  get: () => props.data.type,
  set: (value) => {
    props.data.type = value
    updateNodeData()
  },
})

// A computed property for the multiline user input or JSON
const command = computed({
  get: () => props.data.inputs?.command || '',
  set: (value) => {
    props.data.inputs.command = value
    updateNodeData()
  },
})

// Ensure the data is emitted to VueFlow
function updateNodeData() {
  const updatedData = {
    ...props.data,
    inputs: {
      command: command.value,
    },
    outputs: props.data.outputs,
    updateFromSource: updateFromSource.value,
  }
  emit('update:data', { id: props.id, data: updatedData })
}
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

/* Replacing the single-line input with a multiline textarea */
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
  resize: vertical; /* Allow user to resize vertically */
}
</style>