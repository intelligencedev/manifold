import { computed, watch, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'
import { useNodeBase } from './useNodeBase'

/**
 * Composable for managing SaveTextNode functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - SaveTextNode functionality
 */
export function useSaveTextNode(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  const {
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    onResize: baseOnResize
  } = useNodeBase(props, emit)

  if (!props.data.style) {
    props.data.style = {
      border: '1px solid #666',
      borderRadius: '12px',
      backgroundColor: '#333',
      color: '#eee',
      width: '240px',
      height: '100px',
    }
  }
  customStyle.value.width = props.data.style.width || '240px'
  customStyle.value.height = props.data.style.height || '100px'

  // Filename computed property
  const filename = computed({
    get: () => props.data.inputs.filename,
    set: (value) => {
      props.data.inputs.filename = value
    }
  })

  /**
   * Update node data and emit changes
   */
  const updateNodeData = () => {
    emit('update:data', { id: props.id, data: { ...props.data } })
  }

  // Watch for filename changes to update node data
  watch(filename, () => {
    updateNodeData()
  })

  // Handle node resizing
  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    baseOnResize(event)
  }

  /**
   * Main run function that saves text to a file
   * @returns {Promise<Object>} - Result of save operation
   */
  async function run() {
    console.log('Running SaveTextNode:', props.id)

    // Find the connected source node
    const connectedEdges = getEdges.value.filter((edge) => edge.target === props.id)
    if (connectedEdges.length === 0) {
      console.warn('SaveTextNode has no input connection.')
      return { error: 'No input connection' }
    }

    const sourceNode = findNode(connectedEdges[0].source)
    if (!sourceNode || !sourceNode.data?.outputs?.result?.output) {
      console.warn('Source node output not found.')
      return { error: 'Source node output not found' }
    }

    const content = sourceNode.data.outputs.result.output
    const finalFilename = props.data.inputs.filename

    try {
      const response = await fetch('http://localhost:8080/api/save-file', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          filepath: finalFilename,
          content: content
        })
      })

      if (!response.ok) {
        const errorData = await response.json()
        console.error('Error saving content:', errorData.error)
        return { error: errorData.error }
      } else {
        console.log('Content saved to:', finalFilename)
        return { success: true, filename: finalFilename }
      }
    } catch (error) {
      console.error('Error saving file:', error)
      return { error: error.message }
    }
  }

  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
  })

  return {
    // Base node state
    isHovered,
    customStyle,
    resizeHandleStyle,
    computedContainerStyle,
    // Node fields
    filename,
    // Methods
    onResize,
    updateNodeData,
    run
  }
}