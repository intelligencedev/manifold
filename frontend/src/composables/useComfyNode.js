import { ref, computed, onMounted } from 'vue'

/**
 * Composable for managing ComfyNode state and functionality
 */
export function useComfyNode(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow
  
  // State variables
  const isHovered = ref(false)
  const customStyle = ref({
    width: '360px',
    height: '660px'
  })
  const generatedImage = ref('')
  
  // Computed properties for form binding
  const endpoint = computed({
    get: () => props.data.inputs.endpoint,
    set: (value) => { props.data.inputs.endpoint = value }
  })
  
  const prompt = computed({
    get: () => props.data.inputs.prompt,
    set: (value) => { props.data.inputs.prompt = value }
  })
  
  // UI state
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))
  
  // Main run function
  async function run() {
    console.log('Running ComfyNode:', props.id)
    generatedImage.value = ''
    props.data.outputs.image = ''
    
    try {
      // Check for connected source nodes
      const connectedSources = getEdges.value
        .filter((edge) => edge.target === props.id)
        .map((edge) => edge.source)
      
      // Update prompt from connected node if available
      if (connectedSources.length > 0) {
        const sourceNode = findNode(connectedSources[0])
        console.log('Comfy Connected sources:', connectedSources)
        
        if (sourceNode && sourceNode.data.outputs.result) {
          props.data.inputs.prompt = sourceNode.data.outputs.result.output
        }
      }
      
      // Send the user's endpoint + prompt to the backend proxy
      const response = await fetch('/api/comfy-proxy', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          targetEndpoint: endpoint.value,
          prompt: prompt.value
        })
      })
      
      if (!response.ok) {
        const errorText = await response.text()
        throw new Error(`API error (${response.status}): ${errorText}`)
      }
      
      // Get the blob from response
      const blob = await response.blob()
      
      // Create an object URL from the blob
      const imageUrl = URL.createObjectURL(blob)
      
      // Update both the display image and node output
      generatedImage.value = imageUrl
      props.data.outputs.image = imageUrl
      
      // Ensure the result output property exists and set it to the exact same image URL
      // that is being used to render the image
      if (!props.data.outputs.result) {
        props.data.outputs.result = {};
      }
      props.data.outputs.result.output = imageUrl;
      
    } catch (error) {
      console.error('Error in ComfyNode run:', error)
    }
  }
  
  // Event handlers
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    if (emit) {
      emit('resize', event)
    }
  }
  
  // Lifecycle hooks
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })
  
  return {
    // State
    isHovered,
    customStyle,
    generatedImage,
    
    // Computed properties
    endpoint,
    prompt,
    resizeHandleStyle,
    
    // Methods
    run,
    onResize
  }
}