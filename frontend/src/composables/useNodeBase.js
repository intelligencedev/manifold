import { ref, computed } from 'vue'

/**
 * Basic node behavior shared across multiple nodes.
 */
export function useNodeBase(props, emit) {
  const isHovered = ref(false)
  const customStyle = ref({})

  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))

  const computedContainerStyle = computed(() => ({
    ...props.data.style,
    ...customStyle.value,
    width: '100%',
    height: '100%'
  }))

  function onResize(event) {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
    if (emit) {
      emit('resize', { id: props.id, width: event.width, height: event.height })
    }
  }

  return { isHovered, customStyle, resizeHandleStyle, computedContainerStyle, onResize }
}
