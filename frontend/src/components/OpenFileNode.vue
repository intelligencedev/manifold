<template>
  <div :style="data.style" class="node-container tool-node">
    <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>

    <!-- Filename Input -->
    <div class="input-field">
      <label :for="`${data.id}-filepath`" class="input-label">Filepath:</label>
      <input
        :id="`${data.id}-filepath`"
        type="text"
        v-model="filepath"
        @change="updateNodeData"
        class="input-text"
      />
    </div>
    
    <!-- Checkbox to enable/disable updating input from a connected source -->
    <div class="input-field">
        <input
          type="checkbox"
          :id="`${data.id}-update-from-source`"
          v-model="updateFromSource"
          @change="updateNodeData"
        />
        <label :for="`${data.id}-update-from-source`" class="input-label">
          Update Input from Source
        </label>
    </div>

    <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" id="input" />
    <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" position="right" id="output" />
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue';
import { Handle, useVueFlow } from '@vue-flow/core';

const props = defineProps({
  id: {
    type: String,
    required: true,
    default: 'OpenFile_0',
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: 'openFileNode',
      labelStyle: {},
      style: {},
      inputs: {
        filepath: 'input.txt',
        text: '',
      },
      outputs: {
        result: { output: '' }  // Initialize with empty result
      },
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: '#777',
      outputHandleShape: '50%',
      handleColor: '#777',
      updateFromSource: true,
    }),
  },
});

const emit = defineEmits(['update:data']);
const { getEdges, findNode } = useVueFlow();

const filepath = computed({
  get: () => props.data.inputs.filepath,
  set: (value) => {
    props.data.inputs.filepath = value;
  },
});

const updateFromSource = ref(props.data.updateFromSource)

watch(filepath, () => {
  updateNodeData();
});

watch(
  () => props.data,
  (newData) => {
    filepath.value = newData.inputs?.filepath || 'input.txt';
    emit('update:data', { id: props.id, data: newData });
  },
  { deep: true }
);

onMounted(() => {
  if (!props.data.run) {
    props.data.run = run;
  }
});

async function run() {
    console.log('Running OpenFileNode:', props.id);

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
      props.data.inputs.filepath = sourceData;

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
        payload = {filepath: props.data.inputs.filepath}
    }


    try {
        const response = await fetch('http://localhost:8080/api/open-file', {  // Corrected URL
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({
                filepath: payload.filepath,
            })
        });

        if (!response.ok) {
            const errorData = await response.json();
            console.error('Error reading file content:', errorData.error);
             props.data.outputs.result = {
                error: errorData.error,
            }
            return {error: errorData.error}; // Return error object
        } else {
            const fileContent = await response.text(); // Get as text
            console.log('File content:', fileContent);
             props.data.outputs = {
                result: {
                    output: fileContent,
                },
            }
        }
    } catch (error) {
        console.error('Error opening file:', error);
        props.data.outputs.result = {
            error: error.message,
        }
        return {error: error.message}; // Return error object
    }

    updateNodeData(); // Update data after processing
    return {result: props.data.outputs.result};
}

const updateNodeData = () => {
     const updatedData = {
    ...props.data,
    inputs: {
      filepath: filepath.value,
    },
    outputs: props.data.outputs,
    updateFromSource: updateFromSource.value,
  }
  emit('update:data', { id: props.id, data: updatedData })
};
</script>

<style scoped>
.node-container {
  border: 3px solid var(--node-border-color) !important;
  background-color: var(--node-bg-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

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

.input-text {
  background-color: #333;
  border: 1px solid #666;
  color: #eee;
  padding: 4px;
  font-size: 12px;
  width: calc(100% - 8px);
  box-sizing: border-box;
}

.handle-input,
.handle-output {
  width: 12px;
  height: 12px;
  border: none;
  background-color: #777;
}
</style>