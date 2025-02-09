<template>
    <div :style="data.style" class="node-container tool-node">
      <!-- Node label -->
      <div class="node-label">
        <input
          v-model="label"
          @change="updateNodeData"
          class="label-input"
          :style="data.labelStyle"
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
  
      <!-- Input for Paths (comma separated) -->
      <div class="input-field">
        <label :for="`${data.id}-paths`" class="input-label">
          Paths (comma separated):
        </label>
        <input
          type="text"
          :id="`${data.id}-paths`"
          v-model="paths"
          @change="updateNodeData"
          class="input-text"
        />
      </div>
  
      <!-- Input for Types (comma separated) -->
      <div class="input-field">
        <label :for="`${data.id}-types`" class="input-label">
          Types (comma separated):
        </label>
        <input
          type="text"
          :id="`${data.id}-types`"
          v-model="types"
          @change="updateNodeData"
          class="input-text"
        />
      </div>
  
      <!-- Checkbox for Recursive -->
      <div class="input-field">
        <input
          type="checkbox"
          :id="`${data.id}-recursive`"
          v-model="recursive"
          @change="updateNodeData"
        />
        <label :for="`${data.id}-recursive`" class="input-label">
          Recursive
        </label>
      </div>
  
      <!-- Input for Ignore Pattern -->
      <div class="input-field">
        <label :for="`${data.id}-ignore`" class="input-label">
          Ignore Pattern:
        </label>
        <input
          type="text"
          :id="`${data.id}-ignore`"
          v-model="ignorePattern"
          @change="updateNodeData"
          class="input-text"
        />
      </div>
  
      <!-- Node connection handles -->
      <Handle style="width:10px; height:10px" v-if="data.hasInputs" type="target" position="left" id="input" />
      <Handle style="width:10px; height:10px" v-if="data.hasOutputs" type="source" position="right" id="output" />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, onMounted } from 'vue'
  import { Handle, useVueFlow } from '@vue-flow/core'
  
  const { getEdges, findNode } = useVueFlow()
  
  // Define the component props; note that we set default node data values for our inputs.
  const props = defineProps({
    id: {
      type: String,
      required: false,
      default: 'RepoConcat_0',
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        style: {},
        labelStyle: {},
        type: 'RepoConcatNode',
        inputs: {
          // Defaults: a single path and a comma‐separated list of file types
          paths: ".",
          types: ".go, .md, .vue, .js, .css, .html",
          recursive: false,
          ignorePattern: ""
        },
        outputs: {},
        hasInputs: true,
        hasOutputs: true,
        updateFromSource: true,
      }),
    },
  })
  
  const emit = defineEmits(['update:data'])
  
  // Local reactive variable for the update-from-source checkbox
  const updateFromSource = ref(props.data.updateFromSource)
  
  // Mount the run method on the node data so that VueFlow can invoke it.
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  /**
   * run() gathers input parameters either from connected nodes (if any and enabled)
   * or from this node’s own inputs, sends a POST request to /api/repoconcat,
   * and then updates the node’s outputs with the returned concatenated result.
   */
  async function run() {
    try {
      // Clear previous output
      props.data.outputs.result = ''
  
      // Check for connected source nodes (to optionally update input parameters)
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)
  
      let payload
      if (connectedSources.length > 0 && updateFromSource.value) {
        const sourceData = findNode(connectedSources[0]).data.outputs.result.output
        console.log('Connected source data:', sourceData)
        try {
          payload = JSON.parse(sourceData)
        } catch (err) {
          console.error('Error parsing JSON from connected node:', err)
          props.data.outputs.result = { error: 'Invalid JSON from connected node' }
          return { error: 'Invalid JSON from connected node' }
        }
      } else {
        // Use the values entered in this node's input fields.
        const pathsInput = props.data.inputs.paths
        const typesInput = props.data.inputs.types
        const recursiveValue = props.data.inputs.recursive
        const ignorePatternValue = props.data.inputs.ignorePattern
  
        // Convert comma-separated strings into arrays.
        const pathsArray = pathsInput.split(',').map(s => s.trim()).filter(s => s)
        const typesArray = typesInput.split(',').map(s => s.trim()).filter(s => s)
  
        payload = {
          paths: pathsArray,
          types: typesArray,
          recursive: recursiveValue,
          ignorePattern: ignorePatternValue,
        }
      }
  
      // POST the parameters to the /api/repoconcat endpoint.
      const response = await fetch('http://localhost:8080/api/repoconcat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
  
      if (!response.ok) {
        const errorMsg = await response.text()
        console.error('Error response from server:', errorMsg)
        props.data.outputs.result = { error: errorMsg }
        return { error: errorMsg }
      }
  
      // The API returns plain text – the concatenated output.
      const result = await response.text()
      console.log('RepoConcat run result:', result)
  
      props.data.outputs = {
        result: {
          output: result,
        },
      }
  
      updateNodeData()
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      props.data.outputs.result = { error: error.message }
      return { error }
    }
  }
  
  // Computed property for the node label.
  const label = computed({
    get: () => props.data.type,
    set: (value) => {
      props.data.type = value
      updateNodeData()
    },
  })
  
  // Computed property for the "paths" input.
  const paths = computed({
    get: () => props.data.inputs?.paths || '',
    set: (value) => {
      props.data.inputs.paths = value
      updateNodeData()
    },
  })
  
  // Computed property for the "types" input.
  const types = computed({
    get: () => props.data.inputs?.types || '',
    set: (value) => {
      props.data.inputs.types = value
      updateNodeData()
    },
  })
  
  // Computed property for the "recursive" checkbox.
  const recursive = computed({
    get: () => props.data.inputs?.recursive || false,
    set: (value) => {
      props.data.inputs.recursive = value
      updateNodeData()
    },
  })
  
  // Computed property for the "ignorePattern" input.
  const ignorePattern = computed({
    get: () => props.data.inputs?.ignorePattern || '',
    set: (value) => {
      props.data.inputs.ignorePattern = value
      updateNodeData()
    },
  })
  
  // Emit updated node data to VueFlow.
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: {
        paths: paths.value,
        types: types.value,
        recursive: recursive.value,
        ignorePattern: ignorePattern.value,
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
  
  .input-field {
    margin-bottom: 8px;
  }
  
  /* Styling for standard text inputs */
  .input-text {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
  }
  
  /* You can keep using your existing label-input styling from your other components */
  .label-input {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 16px;
    width: 100%;
    box-sizing: border-box;
  }
  </style>
  