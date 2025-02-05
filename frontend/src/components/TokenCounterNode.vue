<template>
    <div :style="data.style" class="node-container token-counter-node">
      <!-- Display the node type and the token count -->
      <div :style="data.labelStyle" class="node-label">
        {{ data.type }}: {{ data.tokenCount }}
      </div>
  
      <!-- Optional: Input handle if you want to allow connections from other nodes. 
           Remove if you truly want zero handles. -->
      <Handle v-if="data.hasInputs" type="target" position="left" />
    </div>
  </template>
  
  <script setup>
  import { Handle, useVueFlow } from '@vue-flow/core'
  import { onMounted } from 'vue'
  
  /**
   * Grab helpers from Vue Flow
   */
  const { getEdges, findNode, updateNodeData } = useVueFlow()
  
  /**
   * Define props
   */
  const props = defineProps({
    id: {
      type: String,
      required: true,
      default: 'TokenCounter_0',
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: 'TokenCounterNode',
        labelStyle: {
          fontWeight: 'normal',
        },
        // By default, we allow an input handle so this node can receive data
        hasInputs: true,
        hasOutputs: false,
        // Inputs for the node (endpoint, api_key)
        inputs: {
          endpoint: '<my oai compatible endpoint>',
          api_key: '',
        },
        // This node will keep track of the token count
        tokenCount: 0,
      }),
    },
  })
  
  /**
   * Assign the 'run' method if it doesn't exist yet.
   */
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  /**
   * callTokenizeAPI: calls the /v1/tokenize endpoint with the provided text.
   */
  async function callTokenizeAPI(text) {
    const endpoint = props.data.inputs.endpoint
    const apiKey = props.data.inputs.api_key
  
    const response = await fetch(`${endpoint}/tokenize`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${apiKey}`,
      },
      body: JSON.stringify({
        content: text,
        add_special: false,
        with_pieces: false,
      }),
    })
  
    if (!response.ok) {
      const errorText = await response.text()
      throw new Error(`API error (${response.status}): ${errorText}`)
    }
  
    return await response.json()
  }
  
  /**
   * run: invoked by external logic or from a parent node,
   * collects text from connected source nodes, calls callTokenizeAPI,
   * and updates the token count on this node.
   */
  async function run() {
    console.log('Running TokenCounterNode:', props.id)
  
    try {
      // Find this node
      const tokenNode = findNode(props.id)
      if (!tokenNode) {
        throw new Error(`Node with id "${props.id}" not found`)
      }
  
      let combinedText = ''
      // Gather text from all connected source nodes
      const edges = getEdges.value.filter(edge => edge.target === props.id)
      for (const edge of edges) {
        const sourceNode = findNode(edge.source)
        if (sourceNode && sourceNode.data?.outputs?.result?.output) {
          combinedText += sourceNode.data.outputs.result.output
        }
      }
  
      // Call the tokenize endpoint
      const responseData = await callTokenizeAPI(combinedText)
      const tokens = responseData.tokens ?? []
  
      // Update the token count in the node's data
      tokenNode.data.tokenCount = tokens.length

      console.log('Token count:', tokenNode.data.tokenCount)
  
      // Persist data changes with Vue Flow
      updateNodeData({
        id: tokenNode.id,
        data: tokenNode.data.tokenCount,
      })
  
      return { tokens }
    } catch (error) {
      console.error('Error in TokenCounterNode run:', error)
      return { error }
    }
  }
  </script>
  
  <style scoped>
  .token-counter-node {
    --node-border-color: #777 !important;
    --node-bg-color: #1e1e1e !important;
    --node-text-color: #eee;
    --node-label-font-size: 16px;
  
    border: 1px solid var(--node-border-color);
    background-color: var(--node-bg-color);
    color: var(--node-text-color);
    border-radius: 4px;
    padding: 8px;
  }
  
  .node-label {
    font-size: var(--node-label-font-size);
    text-align: center;
    margin-bottom: 4px;
    font-weight: bold;
  }
  </style>
  