import { ref, computed, watch, onMounted } from 'vue'

/**
 * Composable for managing DatadogNode state and functionality
 */
export function useDatadogNode(props, vueFlow) {
  const { getEdges, findNode, updateNodeData } = vueFlow
  
  // Toggle state for password fields
  const showApiKey = ref(false)
  const showAppKey = ref(false)
  
  // Computed properties for form binding
  const apiKey = computed({
    get: () => props.data.inputs?.apiKey || '',
    set: (value) => {
      props.data.inputs.apiKey = value
      updateNodeData()
    }
  })
  
  const appKey = computed({
    get: () => props.data.inputs?.appKey || '',
    set: (value) => {
      props.data.inputs.appKey = value
      updateNodeData()
    }
  })
  
  const site = computed({
    get: () => props.data.inputs?.site || 'datadoghq.com',
    set: (value) => {
      props.data.inputs.site = value
      updateNodeData()
    }
  })
  
  const operation = computed({
    get: () => props.data.inputs.operation || 'getLogs',
    set: (value) => {
      props.data.inputs.operation = value
    }
  })
  
  const query = computed({
    get: () => props.data.inputs.query,
    set: (value) => {
      props.data.inputs.query = value
    }
  })
  
  const fromTime = computed({
    get: () => props.data.inputs.fromTime,
    set: (value) => {
      props.data.inputs.fromTime = value
    }
  })
  
  const toTime = computed({
    get: () => props.data.inputs.toTime,
    set: (value) => {
      props.data.inputs.toTime = value
    }
  })
  
  // The main function to execute the Datadog API call
  async function run(queryOverride = null) {
    console.log('Running DatadogNode:', props.id)
  
    // Check for input from connected nodes
    const connectedTargetEdges = getEdges.value.filter(
      (edge) => edge.target === props.id
    )
  
    let llmQuery = null
  
    if (connectedTargetEdges.length > 0) {
      const targetEdge = connectedTargetEdges[0]
      const sourceNode = findNode(targetEdge.source)
      if (sourceNode && sourceNode.data.outputs.result?.output) {
        llmQuery = sourceNode.data.outputs.result.output
      }
    }
  
    // Use override or input from connected node or node's query value
    queryOverride = queryOverride || llmQuery || query.value
  
    // Update the query text in the node
    query.value = queryOverride
  
    // Prepare request body for API call
    const requestBody = {
      apiKey: apiKey.value,
      appKey: appKey.value,
      site: site.value,
      operation: operation.value,
      query: queryOverride,
      fromTime: fromTime.value,
      toTime: toTime.value
    }
  
    try {
      // TODO: Make this backend endpoint configurable
      const response = await fetch('http://localhost:8080/api/datadog', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
      })
  
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`Backend error (${response.status}): ${errorText}`)
      }
  
      const data = await response.json()
      console.log('Datadog API response:', data)
  
      // Update node outputs with response data
      props.data.outputs = {
        result: {
          output: JSON.stringify(data.result.output, null, 2)
        }
      }
  
      return { result: props.data.outputs.result }
    } catch (error) {
      console.error('Error in DatadogNode run:', error)
      props.data.error = error.message
      return { error: error.message }
    }
  }
  
  // Lifecycle hooks
  onMounted(() => {
    console.log("[Node Type: " + props.data.type + "] onMounted - Assigning run function")
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  return {
    // State refs
    showApiKey,
    showAppKey,
    
    // Computed properties
    apiKey,
    appKey,
    site,
    operation,
    query,
    fromTime,
    toTime,
    
    // Methods
    run
  }
}