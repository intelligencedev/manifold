import { ref, computed, nextTick } from 'vue'
import { useVueFlow } from '@vue-flow/core'

/**
 * Composable for managing NoteNode functionality
 * @param {Object} props - Component props
 * @param {Function} emit - Component emit function
 * @returns {Object} - NoteNode functionality
 */
export function useNoteNode(props, emit) {
  // Note text computed property
  const noteText = computed({
    get: () => props.data.inputs.note,
    set: (value) => {
      props.data.inputs.note = value
    }
  })

  // UI state
  const isHovered = ref(false)
  const customStyle = ref({})
  
  // Show/hide resize handles when hovering
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
  }))

  // Font size control
  const currentFontSize = ref(14) // Default font size
  const minFontSize = 10
  const maxFontSize = 24
  const fontSizeStep = 2
  
  const increaseFontSize = () => {
    currentFontSize.value = Math.min(currentFontSize.value + fontSizeStep, maxFontSize)
  }
  
  const decreaseFontSize = () => {
    currentFontSize.value = Math.max(currentFontSize.value - fontSizeStep, minFontSize)
  }

  // References to DOM elements
  const textContainer = ref(null)
  
  // Auto-scroll control
  const isAutoScrollEnabled = ref(true)
  
  // Access zoom functions from VueFlow
  const { zoomIn, zoomOut } = useVueFlow()
  
  // Function to scroll to the bottom of the text container
  const scrollToBottom = () => {
    nextTick(() => {
      if (textContainer.value) {
        textContainer.value.scrollTop = textContainer.value.scrollHeight
      }
    })
  }
  
  // Handle scroll events to toggle auto-scroll
  const handleScroll = () => {
    if (textContainer.value) {
      const { scrollTop, scrollHeight, clientHeight } = textContainer.value
      if (scrollTop + clientHeight < scrollHeight) {
        isAutoScrollEnabled.value = false
      } else {
        isAutoScrollEnabled.value = true
      }
    }
  }
  
  // Handle resize events
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`
    customStyle.value.height = `${event.height}px`
  }

  // Define pastel colors for sticky note background
  const pastelColors = ['#FFB3BA', '#FFDFBA', '#FFFFBA', '#BAFFC9', '#BAE1FF']
  const currentColorIndex = ref(0)
  const currentColor = computed(() => pastelColors[currentColorIndex.value])
  
  // Add computed properties for scroll bar colors based on current background color
  const scrollbarTrackColor = computed(() => currentColor.value)
  const scrollbarBorderColor = computed(() => currentColor.value)
  
  // Function to cycle through colors
  const cycleColor = () => {
    currentColorIndex.value = (currentColorIndex.value + 1) % pastelColors.length
    // Update the data style with the new color
    props.data.style.backgroundColor = currentColor.value
  }

  // Basic run function
  const run = async () => {
    return
  }

  return {
    // State
    noteText,
    isHovered,
    customStyle,
    currentFontSize,
    textContainer,
    currentColor,
    scrollbarTrackColor,
    scrollbarBorderColor,
    resizeHandleStyle,
    
    // Methods
    increaseFontSize,
    decreaseFontSize,
    handleScroll,
    onResize,
    cycleColor,
    run,
  }
}