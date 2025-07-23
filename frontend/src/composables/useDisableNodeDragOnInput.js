import { ref, onMounted, onBeforeUnmount } from 'vue'
import { useVueFlow } from '@vue-flow/core'

/**
 * Disables node dragging when interacting with inputs, textareas, selects or contenteditable elements
 * within the node container, and re-enables dragging afterwards.
 *
 * @param {string} nodeId - The id of the node to disable/enable dragging on.
 * @returns {import('vue').Ref<HTMLElement|null>} - A ref to be attached to the node container element.
 */
export function useDisableNodeDragOnInput(nodeId) {
  const containerRef = ref(null)
  
  // Use a simple approach - Vue Flow typically recognizes the "nodrag" class
  const noDragClass = 'nodrag'

  // Define event handler functions
  const onMouseDown = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.add(noDragClass)
    }
  }

  const onMouseUp = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.remove(noDragClass)
    }
  }

  const onTouchStart = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.add(noDragClass)
    }
  }

  const onTouchEnd = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.remove(noDragClass)
    }
  }

  const onFocusIn = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.add(noDragClass)
    }
  }

  const onFocusOut = (e) => {
    const el = containerRef.value
    if (!el) return
    const selector = 'input,textarea,select,[contenteditable="true"]'
    if (e.target.matches(selector)) {
      e.target.classList.remove(noDragClass)
    }
  }

  onMounted(() => {
    const el = containerRef.value
    if (!el) return

    el.addEventListener('mousedown', onMouseDown)
    el.addEventListener('mouseup', onMouseUp)
    el.addEventListener('touchstart', onTouchStart)
    el.addEventListener('touchend', onTouchEnd)
    el.addEventListener('focusin', onFocusIn)
    el.addEventListener('focusout', onFocusOut)
  })

  onBeforeUnmount(() => {
    const el = containerRef.value
    if (!el) return
    el.removeEventListener('mousedown', onMouseDown)
    el.removeEventListener('mouseup', onMouseUp)
    el.removeEventListener('touchstart', onTouchStart)
    el.removeEventListener('touchend', onTouchEnd)
    el.removeEventListener('focusin', onFocusIn)
    el.removeEventListener('focusout', onFocusOut)
  })

  return containerRef
}