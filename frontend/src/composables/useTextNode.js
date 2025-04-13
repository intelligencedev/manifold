import { ref, computed, onMounted } from 'vue'
import { useVueFlow } from '@vue-flow/core'

export default function useTextNode(props, emit) {
  const { getEdges, findNode } = useVueFlow()
  
  // Custom style for handling resizes
  const customStyle = ref({})
  
  // Track whether the node is hovered (to show/hide the resize handles)
  const isHovered = ref(false)
  
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
  }))
  
  // Computed property for two-way binding of the text input
  const text = computed({
    get: () => props.data.inputs.text,
    set: (value) => {
      props.data.inputs.text = value
      updateNodeData()
    }
  })
  
  // Clear on run checkbox binding
  const clearOnRun = computed({
    get: () => props.data.clearOnRun || false,
    set: (value) => {
      props.data.clearOnRun = value
      updateNodeData()
    }
  })
  
  onMounted(() => {
    if (!props.data.run) {
      props.data.run = run
    }
    
    // Initialize clearOnRun if not set
    if (props.data.clearOnRun === undefined) {
      props.data.clearOnRun = false
    }
  })
  
  // Execute the node's logic
  async function run() {
    const originalText = props.data.inputs.text
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source)
  
    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0])
      if (sourceNode && sourceNode.data.outputs.result) {
        props.data.inputs.text = props.data.inputs.text + sourceNode.data.outputs.result.output + "\n\n"
      }
    }
  
    // Set the output equal to the current text input
    props.data.outputs = {
      result: {
        output: props.data.inputs.text
      }
    }
    
    // If clearOnRun is enabled, clear the text after setting the output
    if (props.data.clearOnRun) {
      props.data.inputs.text = ''
      updateNodeData()
    }

    // log the output
    console.log('TextNode output:', props.data.outputs.result.output)
    updateNodeData()
  }
  
  // Emit updated node data back to VueFlow
  function updateNodeData() {
    const updatedData = {
      ...props.data,
      inputs: { text: text.value },
      outputs: props.data.outputs,
      clearOnRun: clearOnRun.value
    }
    emit('update:data', { id: props.id, data: updatedData })
  }
  
  // Handle the resize event to update the node dimensions
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    // Also update the node's style data so it persists
    props.data.style.width = `${event.width}px`
    props.data.style.height = `${event.height}px`
    updateNodeData()
    emit('resize', { id: props.id, width: event.width, height: event.height })
  }
  
  return {
    text,
    clearOnRun,
    customStyle,
    isHovered,
    resizeHandleStyle,
    updateNodeData,
    onResize,
    run
  }
}