import { ref, computed, onMounted, watch } from 'vue'

/**
 * Composable for managing MCPClient state and functionality
 */
export function useMCPClient(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Computed property for form binding
  const command = computed({
    get: () => props.data.inputs.command || '',
    set: (value) => { props.data.inputs.command = value }
  })
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  // Main run function
  async function run() {
    console.log('Running MCPClient:', props.id)
    
    try {
      // Clear previous output
      props.data.outputs.result = ''
      
      // Identify connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
      
      let payload
      
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs?.result?.output) {
          const sourceData = sourceNode.data.outputs.result.output
          console.log('Connected source data:', sourceData)
          try {
            payload = JSON.parse(sourceData)
          } catch (err) {
            payload = { config: sourceData }
          }
          // Overwrite the input field with the connected source's result
          props.data.inputs.command = JSON.stringify(payload, null, 2)
        }
      } else {
        // If no connected source, parse the user's input
        let userInput = props.data.inputs.command
        try {
          payload = JSON.parse(userInput)
        } catch (_err) {
          payload = { config: userInput }
        }
      }
      
      // Ensure payload includes an action; default to "listTools" if missing
      if (!payload.action) {
        payload.action = 'listTools'
      }
      
      // POST to the MCP execution endpoint
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
      
      // Extract a result string from stdout or stderr, or fallback to full JSON
      const resultStr = result.stdout || result.stderr || JSON.stringify(result, null, 2)
      
      props.data.outputs = {
        result: { output: resultStr }
      }
      
      // Update node data
      updateNodeData()
      return { response, result }
    } catch (error) {
      console.error('Error in run():', error)
      props.data.outputs.result = { output: `Error: ${error.message}` }
      return { error }
    }
  }
  
  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    emit('resize', event)
  }
  
  // Update node data
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { command: command.value },
      outputs: props.data.outputs
    }
    emit('update:data', { id: props.id, data: updatedData })
  }
  
  // Lifecycle hooks
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  return {
    // State refs
    isHovered,
    customStyle,
    
    // Computed properties
    command,
    resizeHandleStyle,
    
    // Methods
    run,
    onResize,
    updateNodeData
  }
}