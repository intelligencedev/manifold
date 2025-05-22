import { ref, computed } from 'vue'

// CSS selector used as the drag handle for nodes
export const dragHandle = '.node-header'

/**
 * Basic node behavior shared across multiple nodes.
 */
export function useNodeBase(props, emit) {
  const isHovered = ref(false)
  const customStyle = ref({})
  const isExecuting = ref(false); // New ref to track execution state

  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))

  const computedContainerStyle = computed(() => ({
    ...props.data.style,
    ...customStyle.value,
    width: '100%',
    height: '100%',
    // Apply dynamic shadow based on execution state
    boxShadow: isExecuting.value ? '0 20px 25px -5px rgba(167, 139, 250, 0.5), 0 10px 10px -5px rgba(167, 139, 250, 0.4)' : (props.data.style?.boxShadow || 'none')
  }))

  const width = computed(() => {
    const w = customStyle.value.width || props.data.style?.width
    return w ? parseInt(w) : undefined
  })

  const height = computed(() => {
    const h = customStyle.value.height || props.data.style?.height
    return h ? parseInt(h) : undefined
  })

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    if (emit) {
      emit('resize', { id: props.id, width: event.width, height: event.height })
    }
  }

  // Function to be called by the node when execution starts/ends
  function setExecuting(executing) {
    isExecuting.value = executing;
  }

  return { isHovered, customStyle, resizeHandleStyle, computedContainerStyle, width, height, onResize, setExecuting }
}
