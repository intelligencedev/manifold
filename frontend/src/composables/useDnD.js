import { v4 as uuidv4 } from 'uuid'
import { useVueFlow } from '@vue-flow/core'
import { ref, watch } from 'vue'
import { nodeRegistry } from '../components/nodes/nodeRegistry.ts'
import MessageBusNode from '@/components/MessageBusNode.vue'

let id = 0

/**
 * @returns {string} - A unique id.
 */
function getId(nodeType) {
  return `${nodeType}_${uuidv4()}`
}

/**
 * In a real world scenario you'd want to avoid creating refs in a global scope like this as they might not be cleaned up properly.
 * @type {{draggedType: Ref<string|null>, isDragOver: Ref<boolean>, isDragging: Ref<boolean>}}
 */
const state = {
  /**
   * The type of the node being dragged.
   */
  draggedType: ref(null),
  isDragOver: ref(false),
  isDragging: ref(false),
}

export default function useDragAndDrop() {
  const { draggedType, isDragOver, isDragging } = state

  const { addNodes, screenToFlowCoordinate, toObject, setViewport } = useVueFlow()

  watch(isDragging, (dragging) => {
    document.body.style.userSelect = dragging ? 'none' : ''
  })

  function onDragStart(event, type) {
    if (event.dataTransfer) {
      event.dataTransfer.setData('application/vueflow', type)
      event.dataTransfer.effectAllowed = 'move'
    }

    draggedType.value = type
    isDragging.value = true

    document.addEventListener('drop', onDragEnd)
  }

  /**
   * Handles the drag over event.
   *
   * @param {DragEvent} event
   */
  function onDragOver(event) {
    event.preventDefault()

    if (draggedType.value) {
      isDragOver.value = true

      if (event.dataTransfer) {
        event.dataTransfer.dropEffect = 'move'
      }
    }
  }

  function onDragLeave() {
    isDragOver.value = false
  }

  function onDragEnd() {
    isDragging.value = false
    isDragOver.value = false
    draggedType.value = null
    document.removeEventListener('drop', onDragEnd)
  }

  /**
   * Handles the drop event.
   *
   * @param {DragEvent} event
   */
  function onDrop(event) {
    const { viewport } = toObject()
    const position = screenToFlowCoordinate({
      x: event.clientX,
      y: event.clientY,
    })

    const nodeId = getId(draggedType.value)

    const registration = nodeRegistry.find((n) => n.type === draggedType.value)
    let defaultData
    if (!registration && draggedType.value === 'messageBusNode') {
      defaultData = MessageBusNode.props.data.default()
    } else if (registration) {
      defaultData = registration.defaultData()
    } else {
      console.error(`Unknown node type: ${draggedType.value}`)
      return
    }

    // Create a new node with the default data
    const newNode = {
      id: nodeId,
      type: draggedType.value,
      position,
      data: defaultData,
    }

    addNodes(newNode);

    if (typeof setViewport === 'function') {
      setViewport(viewport)
    }
  }

  return {
    draggedType,
    isDragOver,
    isDragging,
    onDragStart,
    onDragLeave,
    onDragOver,
    onDrop,
  }
}