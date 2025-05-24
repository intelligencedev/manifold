import { ref, computed, onMounted } from 'vue'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for managing ComfyNode state and functionality
 */
export function useComfyNode(props, emit, vueFlow) {
  const { getEdges, findNode } = vueFlow

  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    onResize
  } = useNodeBase(props, emit)

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
      
      // Send the actual proxy request to generate the image
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
      
      // Get the true image URL from the response header
      const originalImageUrl = response.headers.get('X-Comfy-Image-Url')
      if (!originalImageUrl) {
        throw new Error('Image URL not found in response headers')
      }
      
      console.log('Original ComfyUI image URL from response:', originalImageUrl)
      
      // Get the blob from response for displaying in the UI
      const blob = await response.blob()
      
      // Create an object URL from the blob for display purposes
      const displayImageUrl = URL.createObjectURL(blob)
      
      // Update the display image with the blob URL (for UI rendering)
      generatedImage.value = displayImageUrl
      
      // Use the original Comfy URL for the node output
      props.data.outputs.image = originalImageUrl
      
      // Ensure the result output property exists and set it to the original image URL
      if (!props.data.outputs.result) {
        props.data.outputs.result = {};
      }
      props.data.outputs.result.output = originalImageUrl;
      
      console.log("Original ComfyUI image URL:", originalImageUrl);
      
    } catch (error) {
      console.error('Error in ComfyNode run:', error)
    }
  }
  
  // Event handlers are handled by useNodeBase
  
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