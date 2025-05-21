// Auto-generated node registry
import type { Component } from 'vue'

export interface NodeRegistration {
  type: string
  component: Component
  category: string
  defaultData: () => any
}

// Node components
import AgentNode from './AgentNode.vue'
import CodeRunnerNode from './CodeRunnerNode.vue'
import ComfyNode from './ComfyNode.vue'
import DatadogGraphNode from './DatadogGraphNode.vue'
import DatadogNode from './DatadogNode.vue'
import DocumentsIngestNode from './DocumentsIngestNode.vue'
import DocumentsRetrieveNode from './DocumentsRetrieveNode.vue'
import EmbeddingsNode from './EmbeddingsNode.vue'
import GLSLNode from './GLSLNode.vue'
import MCPClient from './MCPClient.vue'
import MLXNode from './MLXNode.vue'
import MLXFlux from './MLXFlux.vue'
import Mermaid from './Mermaid.vue'
import NoteNode from './NoteNode.vue'
import OpenFileNode from './OpenFileNode.vue'
import ReactAgentNode from './ReactAgentNode.vue'
import RepoConcat from './RepoConcat.vue'
import ResponseNode from './ResponseNode.vue'
import SaveTextNode from './SaveTextNode.vue'
import TextNode from './TextNode.vue'
import TextSplitterNode from './TextSplitterNode.vue'
import TokenCounterNode from './TokenCounterNode.vue'
import WebGLNode from './WebGLNode.vue'
import WebRetrievalNode from './WebRetrievalNode.vue'
import WebSearchNode from './WebSearchNode.vue'
import TtsNode from './ttsNode.vue'

export const nodeRegistry: NodeRegistration[] = [
  {
    type: 'completions',
    component: AgentNode,
    category: 'Text Completions',
    defaultData: () => (AgentNode as any).props.data.default(),
  },
  {
    type: 'responseNode',
    component: ResponseNode,
    category: 'Text Completions',
    defaultData: () => (ResponseNode as any).props.data.default(),
  },
  {
    type: 'reactAgent',
    component: ReactAgentNode,
    category: 'Text Completions',
    defaultData: () => (ReactAgentNode as any).props.data.default(),
  },
  {
    type: 'comfyNode',
    component: ComfyNode,
    category: 'Image Generation',
    defaultData: () => (ComfyNode as any).props.data.default(),
  },
  {
    type: 'mlxNode',
    component: MLXNode,
    category: 'Image Generation',
    defaultData: () => (MLXNode as any).props.data.default(),
  },
  {
    type: 'mlxFluxNode',
    component: MLXFlux,
    category: 'Image Generation',
    defaultData: () => (MLXFlux as any).props.data.default(),
  },
  {
    type: 'ttsNode',
    component: TtsNode,
    category: 'Speech Generation',
    defaultData: () => (TtsNode as any).props.data.default(),
  },
  {
    type: 'codeRunnerNode',
    component: CodeRunnerNode,
    category: 'Code',
    defaultData: () => (CodeRunnerNode as any).props.data.default(),
  },
  {
    type: 'webGLNode',
    component: WebGLNode,
    category: 'Code',
    defaultData: () => (WebGLNode as any).props.data.default(),
  },
  {
    type: 'glslNode',
    component: GLSLNode,
    category: 'Code',
    defaultData: () => (GLSLNode as any).props.data.default(),
  },
  {
    type: 'webSearchNode',
    component: WebSearchNode,
    category: 'Web',
    defaultData: () => (WebSearchNode as any).props.data.default(),
  },
  {
    type: 'webRetrievalNode',
    component: WebRetrievalNode,
    category: 'Web',
    defaultData: () => (WebRetrievalNode as any).props.data.default(),
  },
  {
    type: 'openFileNode',
    component: OpenFileNode,
    category: 'Documents',
    defaultData: () => (OpenFileNode as any).props.data.default(),
  },
  {
    type: 'saveTextNode',
    component: SaveTextNode,
    category: 'Documents',
    defaultData: () => (SaveTextNode as any).props.data.default(),
  },
  {
    type: 'textSplitterNode',
    component: TextSplitterNode,
    category: 'Documents',
    defaultData: () => (TextSplitterNode as any).props.data.default(),
  },
  {
    type: 'documentsIngestNode',
    component: DocumentsIngestNode,
    category: 'Documents',
    defaultData: () => (DocumentsIngestNode as any).props.data.default(),
  },
  {
    type: 'documentsRetrieveNode',
    component: DocumentsRetrieveNode,
    category: 'Documents',
    defaultData: () => (DocumentsRetrieveNode as any).props.data.default(),
  },
  {
    type: 'repoConcatNode',
    component: RepoConcat,
    category: 'Documents',
    defaultData: () => (RepoConcat as any).props.data.default(),
  },
  {
    type: 'textNode',
    component: TextNode,
    category: 'Utilities',
    defaultData: () => (TextNode as any).props.data.default(),
  },
  {
    type: 'noteNode',
    component: NoteNode,
    category: 'Utilities',
    defaultData: () => (NoteNode as any).props.data.default(),
  },
  {
    type: 'embeddingsNode',
    component: EmbeddingsNode,
    category: 'Utilities',
    defaultData: () => (EmbeddingsNode as any).props.data.default(),
  },
  {
    type: 'tokenCounterNode',
    component: TokenCounterNode,
    category: 'Utilities',
    defaultData: () => (TokenCounterNode as any).props.data.default(),
  },
  {
    type: 'mermaidNode',
    component: Mermaid,
    category: 'Integrations',
    defaultData: () => (Mermaid as any).props.data.default(),
  },
  {
    type: 'datadogNode',
    component: DatadogNode,
    category: 'Integrations',
    defaultData: () => (DatadogNode as any).props.data.default(),
  },
  {
    type: 'datadogGraphNode',
    component: DatadogGraphNode,
    category: 'Integrations',
    defaultData: () => (DatadogGraphNode as any).props.data.default(),
  },
  {
    type: 'mcpClientNode',
    component: MCPClient,
    category: 'Experimental',
    defaultData: () => (MCPClient as any).props.data.default(),
  },
]

export function getNodeCategories() {
  const grouped: Record<string, NodeRegistration[]> = {}
  for (const node of nodeRegistry) {
    if (!grouped[node.category]) grouped[node.category] = []
    grouped[node.category].push(node)
  }
  return grouped
}