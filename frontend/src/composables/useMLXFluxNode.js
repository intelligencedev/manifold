import { ref, computed, onMounted, watch } from 'vue'

/**
 * Composable for managing MLXFlux node state and functionality
 */
export function useMLXFluxNode(props, vueFlow) {
  const { getEdges, findNode, updateNodeData } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({})
  const imageSrc = ref('') // Holds the image URL
  
  // Computed properties for two-way data binding
  const model = computed({
    get: () => props.data.inputs.model,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, model: value },
      }),
  })
  
  const prompt = computed({
    get: () => props.data.inputs.prompt,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, prompt: value },
      }),
  })
  
  const steps = computed({
    get: () => props.data.inputs.steps,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, steps: value },
      }),
  })
  
  const seed = computed({
    get: () => props.data.inputs.seed,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, seed: value },
      }),
  })
  
  const quality = computed({
    get: () => props.data.inputs.quality,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, quality: value },
      }),
  })
  
  const output = computed({
    get: () => props.data.inputs.output,
    set: (value) =>
      updateNodeData(props.id, {
        ...props.data,
        inputs: { ...props.data.inputs, output: value },
      }),
  })
  
  // Show/hide the handles based on hover state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
  }))
  
  // Function to call the FMLX API (using a Go backend endpoint)
  async function callFMLXAPI(mlxNode) {
    const endpoint = '/api/run-fmlx' // Your Go backend endpoint
    
    // Clear the current image before running
    imageSrc.value = ''
  
    const requestBody = {
      model: mlxNode.data.inputs.model,
      prompt: mlxNode.data.inputs.prompt,
      steps: mlxNode.data.inputs.steps,
      seed: mlxNode.data.inputs.seed,
      quality: mlxNode.data.inputs.quality,
      output: mlxNode.data.inputs.output, // Use the configured output path
    }
  
    try {
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(requestBody),
      })
  
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`API error (${response.status}): ${errorText}`)
      }
  
      const result = await response.json()
      console.log('FMLX API response:', result)

      // Get the current browser URL
      const currentUrl = window.location.href
      // Construct the full image URL using the input file name
      const imageUrl = `${currentUrl}tmp/${mlxNode.data.inputs.output}`
      console.log('Image URL:', imageUrl)

  
      // Update the node data with the response
      updateNodeData(props.id, {
        ...props.data,
        outputs: {
          response: imageUrl,
        },
      })
      
      // Set the image source to display the generated image
      imageSrc.value = imageUrl
  
      return { response: 'OK' }
    } catch (e) {
      console.error('Error calling fmlx api', e)
      return { error: e.message }
    }
  }
  
  // Main run function
  async function run() {
    console.log('Running MLXFlux node:', props.id)
  
    try {
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
  
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
  
        console.log('MLXFlux Connected sources:', connectedSources)
  
        if (sourceNode && sourceNode.data.outputs.result) {
          props.data.inputs.prompt = sourceNode.data.outputs.result.output
        }
      }
  
      return await callFMLXAPI(findNode(props.id))
    } catch (error) {
      console.error('Error in MLXFlux run:', error)
      return { error }
    }
  }
  
  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
  }
  
  // Lifecycle hooks
  onMounted(() => {
    props.data.run = run
    
    // If there's already a response URL stored in the node data, show it
    if (props.data.outputs && props.data.outputs.response) {
      imageSrc.value = props.data.outputs.response
    }
  })
  
  // Watch for changes in the output response and update imageSrc accordingly
  watch(
    () => props.data.outputs.response,
    (newValue) => {
      if (newValue) {
        imageSrc.value = newValue // Update imageSrc with the image URL from the response
      }
    },
    { immediate: true }
  )
  
  return {
    // State refs
    isHovered,
    customStyle,
    imageSrc,
    
    // Computed properties
    model,
    prompt,
    steps,
    seed,
    quality,
    output,
    resizeHandleStyle,
    
    // Methods
    run,
    onResize
  }
}