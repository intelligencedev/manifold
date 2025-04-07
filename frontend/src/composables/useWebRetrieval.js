import { computed, watch } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export function useWebRetrieval(props, emit) {
  const { getEdges, findNode } = useVueFlow()

  // ----- Reactivity for inputs -----
  const urls = computed({
    get: () => props.data.inputs.url,
    set: (value) => {
      props.data.inputs.url = value
      updateNodeData()
    },
  })

  /**
   * Manually trigger an update-data emit
   */
  function updateNodeData() {
    emit('update:data', { id: props.id, data: { ...props.data } })
  }

  /**
   * The main "run" method:
   * 1) Finds any connected incoming nodes.
   * 2) Updates input URL field if there's connected source data.
   * 3) Calls backend endpoint to retrieve content from each URL.
   * 4) Stores results in `props.data.outputs.result.output`.
   */
  async function run() {
    console.log('Running WebRetrievalNode:', props.id)

    // Check incoming edges
    const connectedEdges = getEdges.value.filter(e => e.target === props.id)
    if (connectedEdges.length > 0) {
      const connectedSourceIds = connectedEdges.map(edge => edge.source)
      
      if (connectedSourceIds.length > 0) {
        const sourceNode = findNode(connectedSourceIds[0])
        
        if (sourceNode && sourceNode.data.outputs && sourceNode.data.outputs.result) {
          console.log('Connected node data:', sourceNode.data)
          // Update the URL input with the connected node's output
          props.data.inputs.url = sourceNode.data.outputs.result.output
          updateNodeData()
        }
      }
    }

    // Grab the URLs from inputs
    const urlsToFetch = props.data.inputs.url || ''
    if (!urlsToFetch) {
      console.warn('No URLs provided in WebRetrievalNode.')
      props.data.outputs = {
        result: {
          output: '',
        },
        error: 'No URLs provided.'
      }
      updateNodeData()
      return { error: 'No URLs provided.' }
    }

    try {
      // Call to backend API
      const response = await fetch(
        `http://localhost:8080/api/web-content?urls=${encodeURIComponent(urlsToFetch)}`
      )
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`Web Content API error (${response.status}): ${errorText}`)
      }

      const results = await response.json()

      // Combine the text content from all URLs
      let aggregatedWebContent = ''
      for (const url in results) {
        if (results[url].error) {
          console.error(`Error for ${url}: ${results[url].error}`)
        } else {
          aggregatedWebContent += results[url].Content + '\n'
        }
      }

      // Store the aggregated content in the node's output using the consistent structure
      props.data.outputs = {
        result: {
          output: aggregatedWebContent,
        },
      }

      updateNodeData()
      console.log('WebRetrievalNode run result:', props.data.outputs)
      return { response, result: props.data.outputs }
    } catch (error) {
      console.error('Error in WebRetrievalNode run:', error)
      props.data.outputs = {
        result: {
          output: '',
        },
        error: error.message
      }
      updateNodeData()
      return { error: error.message }
    }
  }

  // Watch for changes to props.data and emit updates
  watch(
    () => props.data,
    (newData) => {
      emit('update:data', { id: props.id, data: newData })
    },
    { deep: true }
  )

  // Setup function to initialize the node
  const setup = () => {
    // Initialize the outputs structure if it doesn't exist
    if (!props.data.outputs) {
      props.data.outputs = {
        result: {
          output: '',
        },
      }
    }
    
    // Add the run function to the node data if it doesn't exist
    if (!props.data.run) {
      props.data.run = run
    }
  }

  return {
    urls,
    updateNodeData,
    run,
    setup
  }
}