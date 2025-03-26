import { v4 as uuidv4 } from 'uuid'
import { useVueFlow } from '@vue-flow/core'
import { ref, watch, nextTick } from 'vue'
import NoteNode from './components/NoteNode.vue'
import PythonRunner from './components/PythonRunner.vue'
import WebGLNode from './components/WebGLNode.vue'
import AgentNode from './components/AgentNode.vue'
import ClaudeNode from './components/ClaudeNode.vue'
import ResponseNode from './components/ResponseNode.vue'
import GeminiNode from './components/GeminiNode.vue'
import EmbeddingsNode from './components/EmbeddingsNode.vue'
import WebSearchNode from './components/WebSearchNode.vue'
import WebRetrievalNode from './components/WebRetrievalNode.vue'
import TextNode from './components/TextNode.vue'
import TextSplitterNode from './components/TextSplitterNode.vue'
import OpenFileNode from './components/OpenFileNode.vue'
import SaveTextNode from './components/SaveTextNode.vue'
import DatadogNode from './components/DatadogNode.vue'
import DatadogGraphNode from './components/DatadogGraphNode.vue'
import TokenCounterNode from './components/TokenCounterNode.vue'
import FlowControl from './components/FlowControl.vue'
import RepoConcat from './components/RepoConcat.vue'
import ComfyNode from './components/ComfyNode.vue'
import MLXFlux from './components/MLXFlux.vue'
import DocumentsIngest from './components/DocumentsIngest.vue'
import DocumentsRetrieve from './components/DocumentsRetrieve.vue'
import ttsNode from './components/ttsNode.vue'
import MCPClientNode from './components/MCPClient.vue'
import Mermaid from './components/Mermaid.vue'

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

  const { addNodes, screenToFlowCoordinate, setNodes } = useVueFlow()

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
    const position = screenToFlowCoordinate({
      x: event.clientX,
      y: event.clientY,
    })

    const nodeId = getId(draggedType.value)

    // Get the default data for the component
    let component;
    switch (draggedType.value) {
      case 'noteNode':
        component = NoteNode;
        break;
      case 'pythonRunnerNode':
        component = PythonRunner;
        break;
      case 'webGLNode':
        component = WebGLNode;
        break;
      case 'agentNode':
        component = AgentNode;
        break;
      case 'claudeNode':
        component = ClaudeNode;
        break;
      case 'responseNode':
        component = ResponseNode;
        break;
      case 'geminiNode':
        component = GeminiNode;
        break;
      case 'embeddingsNode':
        component = EmbeddingsNode;
        break;
      case 'webSearchNode':
        component = WebSearchNode;
        break;
      case 'webRetrievalNode':
        component = WebRetrievalNode;
        break;
      case 'textNode':
        component = TextNode;
        break;
      case 'textSplitterNode':
        component = TextSplitterNode;
        break;
      case 'openFileNode':
        component = OpenFileNode;
        break;
      case 'saveTextNode':
        component = SaveTextNode;
        break;
      case 'datadogNode':
        component = DatadogNode;
        break;
      case 'datadogGraphNode':
        component = DatadogGraphNode;
        break;
      case 'tokenCounterNode':
        component = TokenCounterNode;
        break;
      case 'flowControlNode':
        component = FlowControl;
        break;
      case 'repoConcatNode':
        component = RepoConcat;
        break;
      case 'comfyNode':
        component = ComfyNode;
        break;
      case 'mlxFluxNode':
        component = MLXFlux;
        break;
      case 'documentsIngestNode':
        component = DocumentsIngest;
        break;
      case 'documentsRetrieveNode':
        component = DocumentsRetrieve;
        break;
      case 'ttsNode':
        component = ttsNode;
        break;
      case 'mcpClientNode':
        component = MCPClientNode;
        break;
      case 'mermaidNode':
        component = Mermaid;
        break;
      default:
        console.error(`Unknown node type: ${draggedType.value}`);
        return;
    }
    const defaultData = component.props.data.default();

    // Create a new node with the default data
    const newNode = {
      id: nodeId,
      type: draggedType.value,
      position,
      data: defaultData,
    }

    addNodes(newNode);
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