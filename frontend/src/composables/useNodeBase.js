import { ref, computed, onMounted, nextTick } from 'vue'

// CSS selector used as the drag handle for nodes
export const dragHandle = '.node-header'

/**
 * Basic node behaviour shared across multiple nodes.
 * – Tracks hover state (for handle visibility)
 * – Implements resize handling
 * – Computes dynamic min-height based on actual content
 */
export function useNodeBase (props, emit) {
  /* ---------- reactive state ---------- */
  const containerRef        = ref(null)          // <div> element of the node
  const isHovered           = ref(false)         // hover state
  const customStyle         = ref({})            // user-controlled width/height
  const contentMinHeight    = ref(0)             // auto-computed min-height
  const baseMinWidth        = 352                // constant across all nodes

  /* ---------- handle style ---------- */
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px'
  }))

  /* ---------- computed container style ---------- */
  const computedContainerStyle = computed(() => ({
    ...props.data?.style,
    ...customStyle.value,
    /* dynamic min-height so children never clip */
    minHeight: `${contentMinHeight.value}px`
  }))

  /* ---------- internal helpers ---------- */
  function updateContentMinHeight () {
    if (containerRef.value) {
      // use scrollHeight so padding + margins are respected
      const measured = Math.max(
        containerRef.value.scrollHeight,
        containerRef.value.offsetHeight
      )
      contentMinHeight.value = measured
    }
  }

  /* ---------- life-cycle ---------- */
  onMounted(() => {
    nextTick(() => {
      updateContentMinHeight()

      // Track any change inside the node (accordion open/close, etc.)
      const ro = new ResizeObserver(updateContentMinHeight)
      ro.observe(containerRef.value)
    })
  })

  /* ---------- resize callback (user drag) ---------- */
  function onResize (event) {
    // Persist the user-chosen size
    customStyle.value.width  = `${event.width}px`
    customStyle.value.height = `${event.height}px`

    // Never let contentMinHeight shrink below manual drag
    if (event.height > contentMinHeight.value) {
      contentMinHeight.value = event.height
    }

    if (emit) {
      emit('resize', { id: props.id, width: event.width, height: event.height })
    }
  }

  return {
    /* reactive refs returned to component */
    containerRef,
    isHovered,
    resizeHandleStyle,
    computedContainerStyle,
    onResize,
    /* size constraints */
    contentMinHeight,
    baseMinWidth
  }
}
