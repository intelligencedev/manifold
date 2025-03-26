import { ref, computed, onMounted } from 'vue'

/**
 * Composable for managing CodeEvalNode state and functionality
 */
export function useCodeEvalNode(props, vueFlow) {
  const { getEdges, findNode } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Computed properties for form binding
  const codeToEvaluate = computed({
    get: () => props.data.inputs.code,
    set: (value) => { props.data.inputs.code = value }
  })
  
  const language = computed({
    get: () => props.data.inputs.language,
    set: (value) => { props.data.inputs.language = value }
  })
  
  const result = computed(() => props.data.outputs?.result?.output || '')
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  // Main run function to evaluate code
  async function run() {
    console.log('Running CodeEvalNode:', props.id)
    
    try {
      // Initialize with empty result
      props.data.outputs = {
        result: {
          output: ''
        }
      }
      
      // Check for connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
      
      // Get code from connected node if available
      let finalCode = props.data.inputs.code
      
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        if (sourceNode && sourceNode.data.outputs?.result?.output) {
          finalCode = sourceNode.data.outputs.result.output
          // Update the code in the node's inputs to show the new code
          props.data.inputs.code = finalCode
        }
      }
      
      // Call the code evaluation endpoint
      const response = await fetch('http://localhost:8080/api/code/eval', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          code: finalCode,
          language: props.data.inputs.language,
        })
      })
      
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`API error (${response.status}): ${errorText}`)
      }
      
      const data = await response.json()
      console.log('Code eval response:', data)
      
      // Update the result with the response
      props.data.outputs.result = {
        output: data.result || data.error || 'No output'
      }
      
      return props.data.outputs
      
    } catch (error) {
      console.error('Error in CodeEvalNode run:', error)
      props.data.outputs.result = {
        output: `Error: ${error.message}`
      }
      return props.data.outputs
    }
  }
  
  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
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
    codeToEvaluate,
    language,
    result,
    resizeHandleStyle,
    
    // Methods
    run,
    onResize
  }
}