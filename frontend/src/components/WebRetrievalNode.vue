<!-- /src/components/WebRetrievalNode.vue -->
<template>
    <div :style="data.style" class="node-container tool-node">
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- URLs Input -->
      <div class="input-field">
        <label :for="`${data.id}-url`" class="input-label">URLs (comma-separated):</label>
        <textarea
          :id="`${data.id}-url`"
          v-model="urls"
          @change="updateNodeData"
          class="input-textarea"
        ></textarea>
      </div>
  
      <!-- Input Handle (Optional Connection) -->
      <Handle style="width:12px; height:12px"
        v-if="data.hasInputs"
        type="target"
        :position="Position.Left"
        id="input"
      />
  
      <!-- Output Handle -->
      <Handle style="width:12px; height:12px"
        v-if="data.hasOutputs"
        type="source"
        :position="Position.Right"
        id="output"
      />
    </div>
  </template>
  
  <script setup lang="ts">
  import { onMounted, watch, computed } from 'vue'
  import { Handle, useVueFlow, Position } from '@vue-flow/core'
  
  // ----- Define props & emits -----
  const props = defineProps({
    id: {
      type: String,
      required: false,
      default: 'WebRetrieval_0',
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'WebRetrievalNode',
        labelStyle: {},
        style: {},
        inputs: {
          url: 'https://en.wikipedia.org/wiki/Singularity_theory',
        },
        outputs: {},
        hasInputs: true,
        hasOutputs: true,
        inputHandleColor: '#777',
        inputHandleShape: '50%',
        handleColor: '#777',
        outputHandleShape: '50%',
      }),
    },
  })
  
  const emit = defineEmits(['update:data'])
  
  // Pull in some utility methods if you need to check connected nodes, edges, etc.
  const { getEdges, getNodes } = useVueFlow()
  
  onMounted(() => {
    // Assign a run method if it doesn't exist
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  /**
   * The main "run" method:
   * 1) Finds any connected incoming nodes (optional).
   * 2) Splits the URLs from `props.data.inputs.url`.
   * 3) Calls a hypothetical backend endpoint to retrieve content from each URL.
   * 4) Stores results in `props.data.outputs.result.output`.
   */
  async function run() {
    console.log('Running WebRetrievalNode:', props.id)
  
    // Check incoming edges
    const connectedEdges = getEdges.value.filter(e => e.target === props.id)
    if (connectedEdges.length) {
      console.log('Incoming edges:', connectedEdges)

      // Get the connected nodes
      const connectedNodes = getNodes.value.filter(n => n.id === connectedEdges[0].source)
      if (connectedNodes.length) {
        console.log('Connected nodes:', connectedNodes)

        // Get the data from the connected node
        const sourceNode = connectedNodes[0]
        console.log('Connected node data:', sourceNode.data)

        // update the URL input with the connected node's output
        props.data.inputs.url = sourceNode.data.outputs.urls;

        // Update the node data
        updateNodeData()
      }
    }
  
    // Grab the URLs from inputs
    const urlsToFetch = props.data.inputs.url || ''
    if (!urlsToFetch) {
      console.warn('No URLs provided in WebRetrievalNode.')
      return { error: 'No URLs provided.' }
    }
  
    try {
      // Hypothetical call to your backend. Adjust to match your real API.
      const response = await fetch(
        `http://localhost:8080/api/web-content?urls=${encodeURIComponent(urlsToFetch)}`
      )
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`Web Content API error (${response.status}): ${errorText}`)
      }
  
      const results = await response.json() // { url1: {...}, url2: {...}, etc. }
  
      // OPTIONAL: If you want to combine *only* the text content:
      let aggregatedWebContent = ''
      for (const url in results) {
        if (results[url].error) {
          console.error(`Error for ${url}: ${results[url].error}`)
        } else {
          aggregatedWebContent += results[url].Content + '\n'
        }
      }
  
      // The AgentNode looks for sourceNode.data.outputs.result.output
      // So, store your aggregated text (or whatever you want) in the same structure:
      props.data.outputs = {
        result: {
          output: aggregatedWebContent, // or results, or both, depending on your preference
        },
      }
  
      console.log('Node-level run result:', props.data.outputs)
      return { response, result: props.data.outputs }
    } catch (error: any) {
      console.error('Error in WebRetrievalNode run:', error)
      props.data.error = error.message
      return { error: error.message }
    }
  }
  
  // ----- Reactivity for inputs -----
  const urls = computed({
    get: () => props.data.inputs.url,
    set: (value) => {
      props.data.inputs.url = value
      updateNodeData()
    },
  })
  
  // ----- Emit updates to parent -----
  watch(
    () => props.data,
    (newData) => {
      emit('update:data', { id: props.id, data: newData })
    },
    { deep: true }
  )
  
  /**
   * Manually trigger an update-data emit
   */
  function updateNodeData() {
    emit('update:data', { id: props.id, data: { ...props.data } })
  }
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
  
  .input-textarea {
    background-color: #333;
    border: 1px solid #666;
    color: #eee;
    padding: 4px;
    font-size: 12px;
    width: calc(100% - 8px);
    box-sizing: border-box;
    min-height: 60px;
  }
  
  .handle-input,
  .handle-output {
    width: 12px;
    height: 12px;
    border: none;
    background-color: #777;
  }
  </style>
  