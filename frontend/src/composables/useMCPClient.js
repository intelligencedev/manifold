import { ref, computed, onMounted } from 'vue'

/**
 * Composable for managing MCPClient state and functionality.
 * It handles reading the command input, checking connected nodes,
 * sending the payload to the MCP endpoint, and updating node data.
 */
export function useMCPClient(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow

  // Local state for UI interactions.
  const isHovered = ref(false)
  const customStyle = ref({})

  // Two-way binding for the MCP command input.
  const command = computed({
    get: () => props.data.inputs.command || '',
    set: (value) => {
      props.data.inputs.command = value
    }
  })

  // (Optional) Computed style for resize handles.
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))

  /**
   * run() - Processes the command input:
   *   - Clears previous output.
   *   - Checks for connected source nodes.
   *   - Parses the user input (or connected node's output) as JSON.
   *   - Sends the payload to the MCP endpoint.
   *   - Updates the node's outputs based on the response.
   */
  async function run() {
    console.log('Running MCPClient:', props.id)
    try {
      // Clear previous output.
      props.data.outputs.result = ''

      // Identify connected source nodes.
      const connectedSources = getEdges.value
        .filter(edge => edge.target === props.id)
        .map(edge => edge.source)
      let payload

      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs?.result?.output) {
          const sourceData = sourceNode.data.outputs.result.output
          console.log('Connected source data:', sourceData)
          try {
            // Fix: Use global JSON.parse instead of attempting to call method on sourceData
            payload = JSON.parse(sourceData)
          } catch (err) {
            console.warn("Invalid JSON from connected source; using default payload.")
            return { error: "Invalid JSON from connected source" }
          }
          // Update the input field with the connected source's output.
          // Fix: Use JSON.stringify to properly display the object as formatted JSON
          props.data.inputs.command = typeof payload === 'object' ? 
            JSON.stringify(payload, null, 2) : 
            payload
        }
      } else {
        // If no connected source, parse the user's input.
        let userInput = props.data.inputs.command
        try {
          payload = JSON.parse(userInput)
        } catch (_err) {
          console.warn("Invalid JSON in MCPClient textarea input; expected JSON object.")
          payload = { action: "listTools" }
        }
      }

      // POST the payload to the MCP execution endpoint.
      const response = await fetch('http://localhost:8080/api/executeMCP', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      })

      if (!response.ok) {
        const errorMsg = await response.text()
        console.error('Error response from server:', errorMsg)
        props.data.outputs.result = { output: `Error: ${errorMsg}` }
        return { error: errorMsg }
      }

      const result = await response.json()
      console.log('MCP Client run result:', result)

      // Extract a result string from stdout or stderr, or fallback to full JSON.
      const resultStr = result.stdout || result.stderr || JSON.stringify(result, null, 2)
      props.data.outputs = { result: { output: resultStr } }

      updateNodeData()
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      props.data.outputs.result = { output: `Error: ${error.message}` }
      return { error }
    }
  }

  /**
   * updateNodeData() - Emits updated node data to VueFlow.
   */
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { command: command.value },
      outputs: props.data.outputs
    }
    emit('update:data', { id: props.id, data: updatedData })
  }

  /**
   * onResize() - Handler for resize events (if needed).
   */
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })

  return {
    isHovered,
    customStyle,
    command,
    resizeHandleStyle,
    run,
    onResize,
    updateNodeData
  }
}
