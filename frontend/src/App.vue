<template>
  <div id="app" class="flex flex-col h-screen w-screen p-0 m-0 text-center text-[#2c3e50] relative font-sans antialiased">
    <!-- Show Login component if not authenticated -->
    <Login v-if="!isAuthenticated" @login-success="handleLoginSuccess" />
    
    <!-- Show main app content when authenticated -->
    <template v-else>
      <Header @save="onSave" @restore="onRestore" @logout="handleLogout" @load-template="handleLoadTemplate" />
      <NodePalette />
      <UtilityPalette />

      <!-- Context Menu Component -->
      <div 
        v-if="contextMenu.show" 
        class="context-menu"
        :style="{
          left: `${contextMenu.x}px`,
          top: `${contextMenu.y}px`
        }"
        @click.stop
      >
        <div class="context-menu-item" @click="copyNodeId">
          Copy Node ID
        </div>
      </div>

      <!-- VueFlow Component -->
      <VueFlow class="flex-1 bg-[#282828] relative overflow-hidden pt-2" :nodes="nodes" :edges="edges" :edge-types="edgeTypes"
        :zoom-on-scroll="zoomOnScroll" @nodes-initialized="onNodesInitialized" @nodes-change="onNodesChange"
        @edges-change="onEdgesChange" @connect="onConnect" @dragover="onDragOver" @dragleave="onDragLeave" @drop="onDrop"
        :min-zoom="0.2" :max-zoom="4" fit-view-on-init :snap-to-grid="true" :snap-grid="[16, 16]"
        :default-viewport="{ x: 0, y: 0, zoom: 1 }">
        <!-- Node Templates -->
        <template #node-noteNode="noteNodeProps">
          <NoteNode v-bind="noteNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.prevent="showContextMenu($event, noteNodeProps.id)" />
        </template>
        <template #node-reactAgent="reactAgentProps">
          <ReactAgent v-bind="reactAgentProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, reactAgentProps.id)" />
        </template>
        <template #node-completions="completionsProps">
          <AgentNode v-bind="completionsProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, completionsProps.id)" />
        </template>
        <template #node-responseNode="responseNodeProps">
          <ResponseNode v-bind="responseNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, responseNodeProps.id)" />
        </template>
        <template #node-codeRunnerNode="codeRunnerNodeProps">
          <CodeRunnerNode v-bind="codeRunnerNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, codeRunnerNodeProps.id)" />
        </template>
        <template #node-webGLNode="webGLNodeProps">
          <WebGLNode v-bind="webGLNodeProps" @contextmenu.native.prevent="showContextMenu($event, webGLNodeProps.id)" />
        </template>
        <template #node-embeddingsNode="embeddingsNodeProps">
          <EmbeddingsNode v-bind="embeddingsNodeProps" @contextmenu.native.prevent="showContextMenu($event, embeddingsNodeProps.id)" />
        </template>
        <template #node-webSearchNode="webSearchNodeProps">
          <WebSearchNode v-bind="webSearchNodeProps" @contextmenu.native.prevent="showContextMenu($event, webSearchNodeProps.id)" />
        </template>
        <template #node-webRetrievalNode="webRetrievalNodeProps">
          <WebRetrievalNode v-bind="webRetrievalNodeProps" @contextmenu.native.prevent="showContextMenu($event, webRetrievalNodeProps.id)" />
        </template>
        <template #node-textSplitterNode="textSplitterNodeProps">
          <TextSplitterNode v-bind="textSplitterNodeProps" @contextmenu.native.prevent="showContextMenu($event, textSplitterNodeProps.id)" />
        </template>
        <template #node-textNode="textNodeProps">
          <TextNode v-bind="textNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, textNodeProps.id)" />
        </template>
        <template #node-openFileNode="openFileNodeProps">
          <OpenFileNode v-bind="openFileNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, openFileNodeProps.id)" />
        </template>
        <template #node-saveTextNode="saveTextNodeProps">
          <SaveTextNode v-bind="saveTextNodeProps" @contextmenu.native.prevent="showContextMenu($event, saveTextNodeProps.id)" />
        </template>
        <template #node-datadogNode="datadogNodeProps">
          <DatadogNode v-bind="datadogNodeProps" @contextmenu.native.prevent="showContextMenu($event, datadogNodeProps.id)" />
        </template>
        <template #node-datadogGraphNode="datadogGraphNodeProps">
          <DatadogGraphNode v-bind="datadogGraphNodeProps" @contextmenu.native.prevent="showContextMenu($event, datadogGraphNodeProps.id)" />
        </template>
        <template #node-tokenCounterNode="tokenCounterNodeProps">
          <TokenCounterNode v-bind="tokenCounterNodeProps" @contextmenu.native.prevent="showContextMenu($event, tokenCounterNodeProps.id)" />
        </template>
        <template #node-flowControlNode="flowControlNodeProps">
          <FlowControl v-bind="flowControlNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
            @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, flowControlNodeProps.id)" />
        </template>
        <template #node-repoConcatNode="repoConcatNodeProps">
          <RepoConcat v-bind="repoConcatNodeProps" @contextmenu.native.prevent="showContextMenu($event, repoConcatNodeProps.id)" />
        </template>
        <template #node-comfyNode="comfyNodeProps">
          <ComfyNode v-bind="comfyNodeProps" @contextmenu.native.prevent="showContextMenu($event, comfyNodeProps.id)" />
        </template>
        <template #node-mlxNode="mlxNodeProps">
          <MLXNode v-bind="mlxNodeProps" @contextmenu.native.prevent="showContextMenu($event, mlxNodeProps.id)" />
        </template>
        <template #node-mlxFluxNode="mlxFluxNodeProps">
          <MLXFlux v-bind="mlxFluxNodeProps" @contextmenu.native.prevent="showContextMenu($event, mlxFluxNodeProps.id)" />
        </template>
        <template #node-documentsIngestNode="documentsIngestNodeProps">
          <DocumentsIngest v-bind="documentsIngestNodeProps" @contextmenu.native.prevent="showContextMenu($event, documentsIngestNodeProps.id)" />
        </template>
        <template #node-documentsRetrieveNode="documentsRetrieveNodeProps">
          <DocumentsRetrieve v-bind="documentsRetrieveNodeProps" @contextmenu.native.prevent="showContextMenu($event, documentsRetrieveNodeProps.id)" />
        </template>
        <template #node-ttsNode="ttsNodeProps">
          <ttsNode v-bind="ttsNodeProps" @contextmenu.native.prevent="showContextMenu($event, ttsNodeProps.id)" />
        </template>
        <template #node-mcpClientNode="mcpClientNodeProps">
          <MCPClient v-bind="mcpClientNodeProps" @contextmenu.native.prevent="showContextMenu($event, mcpClientNodeProps.id)" />
        </template>
        <template #node-mermaidNode="mermaidNodeProps">
          <Mermaid v-bind="mermaidNodeProps" @contextmenu.native.prevent="showContextMenu($event, mermaidNodeProps.id)" />
        </template>
        <template #node-messageBusNode="messageBusNodeProps">
          <MessageBusNode v-bind="messageBusNodeProps" @contextmenu.native.prevent="showContextMenu($event, messageBusNodeProps.id)" />
        </template>

        <Background :color="bgColor" :variant="bgVariant" :gap="16" :size="1" :pattern-color="'#444'" />

        <!-- Run Workflow Button -->
        <div class="absolute bottom-0 left-0 right-0 h-10 flex justify-center items-center z-10">
          <div class="flex justify-evenly items-center"></div>
          <div class="flex justify-center items-center bg-[#222] rounded-xl w-[33vw] h-full p-1 border border-[#777] mb-10">
            <!-- three divs -->
            <div class="flex-1 flex justify-center">
              <!-- Toggle Switch -->
              <div class="tooltip-container flex items-center">
                <label class="switch">
                  <input type="checkbox" v-model="autoPanEnabled">
                  <span class="slider round"></span>
                </label>
                <span class="text-white ml-2 text-sm">Auto-Pan</span>
                <span class="tooltip">When enabled, the view will automatically pan to follow node execution</span>
              </div>
            </div>
            <div class="flex-1 flex justify-center items-center">
              <button class="run-button text-white bg-[#007bff] hover:bg-[#0056b3] font-bold text-base rounded-xl mr-2" @click="runWorkflow">Run</button>
            </div>
            <div class="flex-1 flex justify-center">
              <LayoutControls ref="layoutControls" @update-nodes="updateLayout" :style="{ zIndex: 1000 }"
                @update-edge-type="updateEdgeType" />
            </div>
          </div>
          <div class="flex justify-evenly items-center"></div>
        </div>
      </VueFlow>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, markRaw, watch, onMounted } from 'vue';
import type {
  Connection,
  NodeChange,
  EdgeChange,
  GraphNode,
  GraphEdge,
} from '@vue-flow/core';
import {
  useVueFlow,
  VueFlow,
  applyNodeChanges,
  applyEdgeChanges,
  addEdge,
  type Edge,
} from '@vue-flow/core';
import {
  Background,
  BackgroundVariant,
} from '@vue-flow/additional-components';
import SpecialEdge from './components/SpecialEdge.vue';
import { useConfigStore } from '@/stores/configStore';

// Import Login component
import Login from './components/Login.vue';

// Manifold custom components
import { isNodeConnected } from './utils/nodeHelpers.js';
import Header from './components/layout/Header.vue';
import LayoutControls from './components/layout/LayoutControls.vue';
import useDragAndDrop from './composables/useDnD.js';
import NodePalette from './components/NodePalette.vue';
import UtilityPalette from './components/UtilityPalette.vue';
import NoteNode from './components/nodes/NoteNode.vue';
import ReactAgent from './components/nodes/ReactAgentNode.vue';
import AgentNode from './components/nodes/AgentNode.vue';
import ResponseNode from './components/nodes/ResponseNode.vue';
import CodeRunnerNode from './components/nodes/CodeRunnerNode.vue';
import WebGLNode from './components/nodes/WebGLNode.vue';
import EmbeddingsNode from './components/nodes/EmbeddingsNode.vue';
import WebSearchNode from './components/nodes/WebSearchNode.vue';
import WebRetrievalNode from './components/nodes/WebRetrievalNode.vue';
import TextNode from './components/nodes/TextNode.vue';
import TextSplitterNode from './components/nodes/TextSplitterNode.vue';
import OpenFileNode from './components/nodes/OpenFileNode.vue';
import SaveTextNode from './components/nodes/SaveTextNode.vue';
import DatadogNode from './components/nodes/DatadogNode.vue';
import DatadogGraphNode from './components/nodes/DatadogGraphNode.vue';
import TokenCounterNode from './components/nodes/TokenCounterNode.vue';
import FlowControl from './components/FlowControl.vue';
import RepoConcat from './components/nodes/RepoConcat.vue';
import ComfyNode from './components/nodes/ComfyNode.vue';
import MLXNode from './components/nodes/MLXNode.vue';
import MLXFlux from './components/nodes/MLXFlux.vue';
import DocumentsIngest from './components/nodes/DocumentsIngestNode.vue';
import DocumentsRetrieve from './components/nodes/DocumentsRetrieveNode.vue';
import ttsNode from './components/nodes/ttsNode.vue';
import MCPClient from './components/nodes/MCPClient.vue';
import Mermaid from './components/nodes/Mermaid.vue';
import MessageBusNode from './components/MessageBusNode.vue';

// --- SETUP ---
interface BgColorInterface {
  value: string;
}

const bgColor: BgColorInterface['value'] = '#303030';
const bgVariant = BackgroundVariant.Dots;

// --- STATE ---

// Authentication state
const isAuthenticated = ref(false);
const authToken = ref('');

// Destructure fitView along with other methods
const { findNode, getNodes, getEdges, toObject, fromObject, fitView, updateNodeData } = useVueFlow();
const nodes = ref<GraphNode[]>([]);
const edges = ref<GraphEdge[]>([]);
const defaultEdgeType = ref<string>('bezier'); // Set the default edge type

const configStore = useConfigStore();

// Load configuration and check authentication on startup
onMounted(() => {
  configStore.fetchConfig();
  checkAuthentication();
});

// Check if the user is already authenticated
function checkAuthentication() {
  // Check localStorage for token
  const token = localStorage.getItem('jwt_token');
  
  if (token) {
    // Validate token by making a request to the backend
    fetch('/api/restricted/user', {
      headers: {
        'Authorization': `Bearer ${token}`
      }
    })
    .then(response => {
      if (response.ok) {
        // Token is valid
        authToken.value = token;
        isAuthenticated.value = true;
        return response.json();
      } else {
        // Token is invalid or expired
        localStorage.removeItem('jwt_token');
        isAuthenticated.value = false;
        throw new Error('Invalid token');
      }
    })
    .then(userData => {
      console.log('Authenticated as:', userData.username);
    })
    .catch(error => {
      console.error('Authentication check failed:', error);
      isAuthenticated.value = false;
    });
  } else {
    isAuthenticated.value = false;
  }
}

// Handle successful login
function handleLoginSuccess(data) {
  authToken.value = data.token;
  isAuthenticated.value = true;
  console.log('Login successful');
}

// Handle logout
async function handleLogout() {
  try {
    // Call logout API
    await fetch('/api/restricted/logout', {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${authToken.value}`
      }
    });
    
    // Clear local token and auth state
    localStorage.removeItem('jwt_token');
    authToken.value = '';
    isAuthenticated.value = false;
  } catch (error) {
    console.error('Logout error:', error);
  }
}

// Watchers for debugging
watch(getNodes, (newNodes) => console.log('nodes changed', newNodes));
watch(getEdges, (newEdges) => console.log('edges changed', newEdges));
watch(
  () => configStore.config,
  (newConfig) => {
    if (newConfig) {
      console.log('DataPath:', newConfig.dataPath);
    }
  }
);

// --- CONTROLS ---
const { onDragOver, onDrop, onDragLeave } = useDragAndDrop();

// Disable zoom on scroll
const zoomOnScroll = ref(true);
const disableZoom = () => {
  zoomOnScroll.value = false;
};
const enableZoom = () => {
  zoomOnScroll.value = true;
};

const layoutInitialized = ref(false);
const onNodesInitialized = async () => {
  layoutInitialized.value = true;
};

interface UpdatedNode extends Partial<GraphNode> {
  id: string;
}

// Update node dimensions when a node is resized
interface Dimensions {
  width?: number;
  height?: number;
}

interface UpdatedNodeWithDimensions {
  id: string;
  dimensions?: Dimensions;
}

// This is UI sugar, but helpful for debugging or user awareness.
// use this in the node styles or class binding to visually differentiate them.
nodes.value = nodes.value.map((node) => ({
  ...node,
  data: {
    ...node.data,
    disconnected: !isNodeConnected(node.id, edges.value)
  }
}));

function updateNodeDimensions(updatedNode: UpdatedNodeWithDimensions): void {
  // Make sure each node's .dimensions is current
  nodes.value = nodes.value.map((node: GraphNode) => {
    if (node.id === updatedNode.id) {
      // Merge in new width/height if provided
      return {
        ...node,
        dimensions: {
          width: updatedNode.dimensions?.width || node.dimensions?.width || 150,
          height: updatedNode.dimensions?.height || node.dimensions?.height || 50,
        },
      };
    }
    return node;
  });
}

// Update layout based on updated nodes
const updateLayout = (updatedNodes: UpdatedNode[] | UpdatedNode): void => {
  nodes.value = nodes.value.map((node: GraphNode) => {
    const updatedNode = Array.isArray(updatedNodes)
      ? updatedNodes.find((n: UpdatedNode) => n.id === node.id)
      : null;
    return updatedNode ? { ...node, ...updatedNode } : node;
  });
};

// Handle node changes correctly
function onNodesChange(changes: NodeChange[]) {
  nodes.value = applyNodeChanges(changes, nodes.value);
}

// Handle edge changes correctly
function onEdgesChange(changes: EdgeChange[]) {
  console.log('Edge changes:', changes);
  edges.value = applyEdgeChanges(changes, edges.value);
}

// Handle new connections (edges)
function onConnect(params: Connection) {
  const newEdge: Edge = {
    id: `edge-${Math.random()}`,
    source: params.source,
    target: params.target,
    sourceHandle: params.sourceHandle,
    targetHandle: params.targetHandle,
    data: {
      label: 'New Edge',
    },
    type: defaultEdgeType.value,
  };

  edges.value = addEdge(newEdge, edges.value) as GraphEdge[];

  // Only update target node if target handle is 'input'
  if (params.targetHandle === 'input') {
    const targetNode = findNode(params.target);
    if (targetNode) {
      let connectedTo = targetNode.data?.connectedTo || [];
      if (!Array.isArray(connectedTo)) {
        connectedTo = [connectedTo];
      }
      
      // Only add source if not already connected
      if (!connectedTo.includes(params.source)) {
        connectedTo.push(params.source);
      }
      
      // Use updateNodeData to update the node
      updateNodeData(params.target, {
        ...targetNode.data,
        connectedTo
      });
    }
  }
}

// Save the current flow to a JSON file
function onSave() {
  const flow = toObject();
  const data = JSON.stringify(flow, null, 2);
  const blob = new Blob([data], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'manifold-flow.json';
  a.click();
  URL.revokeObjectURL(url);
}

interface Flow {
  nodes: GraphNode[];
  edges: GraphEdge[];
  position: [number, number];
  zoom: number;
  viewport: {
    x: number;
    y: number;
    zoom: number;
  };
}

// Restore a saved flow from a JSON object
function onRestore(flow: Flow) {
  if (flow) {
    fromObject(flow);
    console.log('Loaded flow:', flow);
    nodes.value = flow.nodes;
    edges.value = flow.edges;
  }
}

// Function to load a template when selected from the dropdown
async function handleLoadTemplate(templateId) {
  try {
    // Fetch the selected template from the backend API
    const response = await fetch(`/api/workflows/templates/${templateId}`);
    if (!response.ok) {
      throw new Error(`Failed to fetch template: ${response.statusText}`);
    }
    
    const flowData = await response.json();
    // Use the existing restore function to load the template
    onRestore(flowData);
  } catch (error) {
    console.error('Error loading template:', error);
    alert(`Failed to load template: ${error.message}`);
  }
}

// Auto-pan toggle
const autoPanEnabled = ref(false);

// Helper: smoothly fit the view to a node using fitView
async function smoothlyFitViewToNode(node: GraphNode) {
  if (autoPanEnabled.value) {
    await fitView({
      nodes: [node.id],
      duration: 800, // duration in ms
      padding: 0.6,
    });
  }
}

// --- WORKFLOW EXECUTION ---

// Helper to reset edge styles (call before starting workflow or after a jump)
function resetEdgeStyles() {
  edges.value = edges.value.map(edge => ({
    ...edge,
    animated: false,
    // Reset stroke style, attempt to preserve original color if available, otherwise default
    style: { 
      ...(edge.style || {}), 
      strokeWidth: 1, 
      stroke: edge.data?.originalStroke || '#b1b1b7' 
    }
  }));
}

// Helper to change edge styles during execution
function changeEdgeStyles(nodeId: string) {
  const connectedEdges = edges.value.filter(
    (edge) => edge.source === nodeId || edge.target === nodeId
  );
  
  connectedEdges.forEach((edge) => {
    // Store original stroke color if not already stored
    if (!edge.data) edge.data = {};
    if (!edge.data.originalStroke && edge.style) {
      edge.data.originalStroke = typeof edge.style === 'object' ? edge.style.stroke || '#b1b1b7' : '#b1b1b7';
    }

    if (edge.target === nodeId) {
      // Style for incoming edge to the currently running node (less emphasis)
      edge.animated = false;
      edge.style = { 
        ...(typeof edge.style === 'object' ? edge.style : {}),
        strokeWidth: 1, 
        stroke: edge.data.originalStroke 
      };
    }
    if (edge.source === nodeId) {
      // Style for outgoing edge from the currently running node (highlight)
      edge.animated = true;
      edge.style = { 
        ...(typeof edge.style === 'object' ? edge.style : {}),
        strokeWidth: 4, 
        stroke: 'darkorange' 
      };
    }
  });
  // Trigger reactivity by creating a new array
  edges.value = [...edges.value];
}

// Refactored workflow execution using a queue and handling jumps
async function runWorkflowConcurrently() {
  console.log('Starting workflow execution...');
  resetEdgeStyles(); // Reset styles at the beginning

  // 1. Build adjacency list and compute in-degrees
  type ChildEdge = { target: string; handle: string | null }; // handle might be null
  const adj: Record<string, ChildEdge[]> = {};
  const inDegree: Record<string, number> = {};
  const allNodeIds = new Set(nodes.value.map(n => n.id));

  // Initialize for all nodes
  nodes.value.forEach(node => {
    adj[node.id] = [];
    inDegree[node.id] = 0;
  });

  // Populate adj list and in-degrees from edges
  edges.value.forEach(edge => {
    // Ensure source and target exist before processing edge
    if (allNodeIds.has(edge.source) && allNodeIds.has(edge.target)) {
        if (!adj[edge.source]) adj[edge.source] = []; // Ensure source entry exists
        adj[edge.source].push({ target: edge.target, handle: edge.sourceHandle || null });

        if (inDegree[edge.target] === undefined) inDegree[edge.target] = 0; // Ensure target entry exists
        inDegree[edge.target]++;
    } else {
        console.warn(`Skipping edge ${edge.id} due to missing source/target node.`);
    }
  });

  // 2. Initialize queue with nodes having in-degree 0
  let queue = nodes.value
    .filter(node => inDegree[node.id] === 0)
    .map(node => node.id);

  // 3. Set to track processed nodes in the current execution flow (cleared on jump)
  const processed = new Set<string>();
  let executionLimit = nodes.value.length * 9999; // Safety break for potential loops
  let executionSteps = 0;

  // Define node execution function to support concurrent execution
  async function executeNode(nodeId: string) {
    if (processed.has(nodeId)) {
      return null; // Skip if already processed
    }

    const node = findNode(nodeId);
    if (!node || !node.data || typeof node.data.run !== 'function') {
      console.warn(`Node ${nodeId} not found or has no run function. Skipping.`);
      return null;
    }

    processed.add(nodeId); // Mark as processed for this sequence
    console.log(`(${executionSteps}) Executing node: ${nodeId} (Type: ${node.type})`);

    // --- Visual Feedback ---
    await smoothlyFitViewToNode(node);
    changeEdgeStyles(nodeId); // Highlight outgoing edges
    
    // --- Check for virtualSources before executing ---
    // When a FlowControl node uses "Jump To Node" mode, it sets itself as a virtual source
    // This allows the target node to receive the FlowControl node's output as input
    if (node.data.virtualSources) {
      const virtualSourceEntries = Object.entries(node.data.virtualSources);
      if (virtualSourceEntries.length > 0) {
        console.log(`Node ${nodeId} has ${virtualSourceEntries.length} virtual source(s)`);
        
        // If we have a virtual source, use its output from most recently connected FlowControl node
        // Sort by timestamp to get the most recent virtual source if multiple exist
        const [sourceId, sourceData] = virtualSourceEntries
          .sort((a, b) => (b[1].timestamp || 0) - (a[1].timestamp || 0))[0];
          
        console.log(`Using virtual source: ${sourceId} for node ${nodeId}`);
        
        // If the node has inputs, set the input from the virtual source
        if (node.data.inputs && sourceData.output) {
          if (typeof node.data.inputs === 'object') {
            // Different node types might have different input property names
            // For text nodes it's usually 'text', for agents it's 'user_prompt', etc.
            // Try common property names for inputs
            const inputKeys = ['text', 'user_prompt', 'prompt', 'command', 'input', 'response'];
            const nodeInputKeys = Object.keys(node.data.inputs);
            
            // Find a suitable input property
            let targetInputKey = null;
            for (const key of inputKeys) {
              if (nodeInputKeys.includes(key)) {
                targetInputKey = key;
                break;
              }
            }
            
            if (targetInputKey) {
              console.log(`Setting ${nodeId}'s input.${targetInputKey} from virtual source ${sourceId}`);
              node.data.inputs[targetInputKey] = sourceData.output;
            } else {
              console.log(`Couldn't find a suitable input property for ${nodeId}`);
            }
          }
        }
      }
    }

    // --- Execute Node Logic ---
    let result = null;
    try {
      result = await node.data.run();
    } catch (error) {
      console.error(`Error running node ${nodeId}:`, error);
      // Update node state to show error
      if (node.data) {
        node.data.error = error instanceof Error ? error.message : String(error);
      }
      return null; // Skip children processing for this errored node
    }

    return { nodeId, node, result };
  }

  // Process the queue with support for parallel execution
  while (queue.length > 0 && executionSteps < executionLimit) {
    executionSteps++;
    const nodeId = queue.shift()!;

    // Skip if already processed in this sequence
    if (processed.has(nodeId)) {
      continue;
    }

    const executionResult = await executeNode(nodeId);
    if (!executionResult) {
      continue; // Skip further processing if execution failed or node was skipped
    }

    const { node, result } = executionResult;

    // --- Handle Flow Control Results ---
    if (result && result.jumpTo) {
      const jumpTargetId = result.jumpTo;
      const virtualSourceId = result.virtualSourceId;
      console.log(`Node ${nodeId} signaled jump to: ${jumpTargetId}${virtualSourceId ? ' with virtual source: ' + virtualSourceId : ''}`);
      
      const targetNode = findNode(jumpTargetId);
      if (targetNode) {
        // Jump: Clear queue, reset processed set, add target
        queue = [jumpTargetId];
        processed.clear(); // Allow all nodes to run again after jump
        resetEdgeStyles(); // Reset styles for the new flow sequence
        console.log(`Queue reset. Next node: ${jumpTargetId}`);
        continue; // Continue the while loop immediately with the new queue
      } else {
        console.warn(`Jump target node ${jumpTargetId} not found. Stopping this path.`);
        continue; // Stop processing children of the jump node
      }
    } else if (result && result.forEachJump) {
      // --- Handle ForEachDelimited special signal ---
      const jumpTargetId = result.forEachJump;
      const parentId = result.parentId;
      console.log(`ForEachDelimited: Node ${nodeId} triggered workflow execution from child: ${jumpTargetId}`);
      
      // Execute this branch of workflow completely
      const targetNode = findNode(jumpTargetId);
      if (targetNode) {
        // Create a temporary queue and processed set for this sub-workflow execution
        const subQueue = [jumpTargetId];
        const subProcessed = new Set<string>();
        
        // Record the FlowControl node so we can return to it later
        const flowControlNode = findNode(parentId);
        
        // Execute the sub-workflow
        let subExecutionSteps = 0;
        const subExecutionLimit = nodes.value.length * 9999; // Smaller safety limit for sub-workflows
        
        console.log(`Starting sub-workflow execution from node ${jumpTargetId}`);
        
        // Inner execution loop for this branch
        while (subQueue.length > 0 && subExecutionSteps < subExecutionLimit) {
          subExecutionSteps++;
          const subNodeId = subQueue.shift()!;
          
          // Skip if already processed in this sub-execution
          if (subProcessed.has(subNodeId) || subNodeId === parentId) {
            continue; // Skip the parent FlowControl node if we encounter it
          }
          
          // Execute this node
          const subNode = findNode(subNodeId);
          if (!subNode || !subNode.data || typeof subNode.data.run !== 'function') {
            console.warn(`Node ${subNodeId} in sub-workflow not found or has no run function. Skipping.`);
            continue;
          }
          
          subProcessed.add(subNodeId); // Mark as processed for this sub-execution
          console.log(`(Sub ${subExecutionSteps}) Executing node: ${subNodeId} (Type: ${subNode.type})`);
          
          // Visual feedback for this node execution
          await smoothlyFitViewToNode(subNode);
          changeEdgeStyles(subNodeId);
          
          // Execute node and handle its result
          let subResult = null;
          try {
            subResult = await subNode.data.run();
          } catch (error) {
            console.error(`Error in sub-workflow running node ${subNodeId}:`, error);
            if (subNode.data) {
              subNode.data.error = error instanceof Error ? error.message : String(error);
            }
            continue;
          }
          
          // Handle result from this sub-workflow node
          if (subResult && (subResult.jumpTo || subResult.forEachJump)) {
            // Don't allow further jumps inside a sub-workflow - just log the attempt
            console.warn(`Node ${subNodeId} attempted to signal jump/forEach which is not allowed in a sub-workflow.`);
            // We'll ignore the jump and continue with normal propagation
          } else if (subResult && subResult.stopPropagation) {
            console.log(`Node ${subNodeId} signaled stopPropagation in sub-workflow. Stopping this path.`);
            continue; // Skip processing children
          }
          
          // Check if this is a FlowControl node with RunAllChildren mode
          const isFlowControlWithConcurrency = 
            subNode.type === 'flowControlNode' && 
            subNode.data.inputs && 
            subNode.data.inputs.mode === 'RunAllChildren';

          // Handle nodes that need concurrency within the sub-workflow
          if (isFlowControlWithConcurrency) {
            console.log(`Sub-workflow: FlowControl node ${subNodeId} in RunAllChildren mode - executing children concurrently`);
            
            // Gather all immediate children of this node
            const childrenIds = adj[subNodeId]
              ? adj[subNodeId]
                .filter(edge => edge.target !== parentId) // Don't include parent node
                .map(edge => edge.target)
              : [];
            
            if (childrenIds.length > 0) {
              console.log(`Sub-workflow: Starting concurrent execution of ${childrenIds.length} children: ${childrenIds.join(', ')}`);
              
              // Execute all children in parallel
              await Promise.all(
                childrenIds.map(async (childId) => {
                  // Skip if already processed or is parent
                  if (subProcessed.has(childId) || childId === parentId) {
                    return;
                  }
                  
                  // Mark as processed and execute
                  subProcessed.add(childId);
                  
                  const childNode = findNode(childId);
                  if (!childNode || !childNode.data || typeof childNode.data.run !== 'function') {
                    console.warn(`Node ${childId} in sub-workflow not found or has no run function. Skipping.`);
                    return;
                  }
                  
                  console.log(`(Sub ${subExecutionSteps}) Executing child node concurrently: ${childId} (Type: ${childNode.type})`);
                  
                  // Visual feedback
                  await smoothlyFitViewToNode(childNode);
                  changeEdgeStyles(childId);
                  
                  // Execute node
                  try {
                    const childResult = await childNode.data.run();
                    
                    // Handle child result (no jumps allowed)
                    if (childResult && (childResult.jumpTo || childResult.forEachJump)) {
                      console.warn(`Child node ${childId} attempted to signal jump/forEach which is not allowed in a sub-workflow.`);
                    } else if (childResult && childResult.stopPropagation) {
                      console.log(`Child node ${childId} signaled stopPropagation in sub-workflow.`);
                      return; // Skip adding this node's children
                    }
                    
                    // Add this child's children to the queue
                    if (adj[childId]) {
                      for (const edge of adj[childId]) {
                        if (edge.target !== parentId && !subProcessed.has(edge.target)) {
                          subQueue.push(edge.target);
                        }
                      }
                    }
                  } catch (error) {
                    console.error(`Error in sub-workflow running child node ${childId}:`, error);
                    if (childNode.data) {
                      childNode.data.error = error instanceof Error ? error.message : String(error);
                    }
                  }
                })
              );
              
              console.log(`Sub-workflow: Concurrent execution of children for ${subNodeId} completed.`);
              continue; // Skip normal child processing
            }
          }
          
          // Add this node's children to the sub-queue (normal sequential processing)
          if (adj[subNodeId]) {
            for (const edge of adj[subNodeId]) {
              // Don't include the parent FlowControl node in propagation
              if (edge.target !== parentId) {
                subQueue.push(edge.target);
              }
            }
          }
        }
        
        console.log(`Sub-workflow from ${jumpTargetId} completed in ${subExecutionSteps} steps.`);
        
        // After sub-workflow completes, return control to the FlowControl node
        // to process the next item if there are more items to process
        if (flowControlNode && flowControlNode.data) {
          if (flowControlNode.data.forEachState && 
              flowControlNode.data.forEachState.currentIndex < flowControlNode.data.forEachState.totalItems) {
            // More items to process - re-run the flow control node
            console.log(`ForEachDelimited: Re-running flow control node ${parentId} for next item. ` +
                      `(${flowControlNode.data.forEachState.currentIndex}/${flowControlNode.data.forEachState.totalItems})`);
            queue.unshift(parentId); // Add back to the front of the queue to process next item
            processed.delete(parentId); // Allow the FlowControl node to run again
          } else {
            // All items processed or no state tracking - finish the loop
            console.log(`ForEachDelimited: All items processed for flow control node ${parentId}.`);
            // Don't add back to queue, effectively ending the loop
          }
        }
        
        continue; // Skip the normal child propagation
      } else {
        console.warn(`ForEachDelimited jump target ${jumpTargetId} not found. Skipping.`);
        // Continue with normal execution
      }
    } else if (result && result.stopPropagation) {
      console.log(`Node ${nodeId} signaled stopPropagation. Stopping this path.`);
      continue; // Stop processing children of this node
    }

    // --- Check if node is FlowControl with RunAllChildren mode ---
    const isFlowControlWithConcurrency = 
      node.type === 'flowControlNode' && 
      node.data.inputs && 
      node.data.inputs.mode === 'RunAllChildren';

    // --- Normal Propagation (if no jump/stop occurred) ---
    if (adj[nodeId]) {
      // If this is a FlowControl node in RunAllChildren mode, run its children concurrently
      if (isFlowControlWithConcurrency) {
        console.log(`FlowControl node ${nodeId} in RunAllChildren mode - executing children concurrently`);
        
        // Gather all immediate children of this node
        const childrenIds = adj[nodeId].map(edge => edge.target);
        
        if (childrenIds.length > 0) {
          console.log(`Starting concurrent execution of ${childrenIds.length} children: ${childrenIds.join(', ')}`);
          
          // Execute all children in parallel using Promise.all
          // We need to mark all children as processed before starting execution to prevent duplicate execution
          childrenIds.forEach(id => processed.add(id));
          
          // Run all child nodes concurrently and wait for all to complete
          await Promise.all(
            childrenIds.map(async (childId) => {
              processed.delete(childId); // Remove from processed to allow execution

              // Execute the child node
              const childExecResult = await executeNode(childId);
              if (!childExecResult) return;
              
              // Now process the children of this node sequentially
              const childNodeId = childExecResult.nodeId;
              
              // Process children of this child node normally
              for (const edge of adj[childNodeId] || []) {
                const targetId = edge.target;
                if (inDegree[targetId] !== undefined) {
                  inDegree[targetId]--;
                  if (inDegree[targetId] === 0 && !processed.has(targetId)) {
                    queue.push(targetId);
                  }
                }
              }
            })
          );
          
          console.log(`Concurrent execution of children for ${nodeId} completed.`);
          // After parallel execution of direct children completes, 
          // continue with normal sequential execution for their descendants
          continue;
        }
      }
      
      // Normal sequential propagation for non-FlowControl nodes
      for (const edge of adj[nodeId]) {
        const targetId = edge.target;
        // Check if target exists before decrementing inDegree
        if (inDegree[targetId] !== undefined) {
            inDegree[targetId]--;
            if (inDegree[targetId] === 0) {
              queue.push(targetId);
            } else if (inDegree[targetId] < 0) {
                // This might happen if jump logic interferes, reset to 0
                console.warn(`Node ${targetId} has negative inDegree (${inDegree[targetId]}). Resetting to 0.`);
                inDegree[targetId] = 0;
                // Check if it should be queued now that it's 0
                if (!processed.has(targetId)) { // Avoid re-queueing if already processed in this jump sequence
                    queue.push(targetId);
                }
            }
        } else {
            console.warn(`Target node ${targetId} for edge from ${nodeId} not found in inDegree map.`);
        }
      }
    }
  } // End while loop

  if (executionSteps >= executionLimit) {
    console.error("Workflow execution limit reached. Possible infinite loop detected.");
  }

  console.log('Workflow execution finished or stopped.');
}

async function runWorkflow() {
  // Clear response nodes (or other stateful nodes) before starting
  const responseNodes = nodes.value.filter((node) => node.type === 'responseNode');
  for (const node of responseNodes) {
    if (node.data && node.data.inputs) {
        node.data.inputs.response = ''; // Clear input display
    }
    if (node.data && node.data.outputs) {
        node.data.outputs = { result: { output: '' } }; // Clear stored output
    }
    // Clear any previous error state
    if (node.data?.error) {
        delete node.data.error;
    }
  }
  
  // Reset FlowControl nodes with ForEachDelimited mode before starting
  const flowControlNodes = nodes.value.filter((node) => node.type === 'flowControlNode');
  for (const node of flowControlNodes) {
    if (node.data && node.data.inputs && node.data.inputs.mode === 'ForEachDelimited') {
      // Reset the forEachState to make sure it starts fresh
      if (node.data.forEachState) {
        console.log(`Resetting state for FlowControl node ${node.id} before starting workflow`);
        // Mark as needing reset, the node will reinitialize when it runs
        node.data.forEachState = { reset: true };
      }
    }
    // Clear any previous error state
    if (node.data?.error) {
        delete node.data.error;
    }
  }

  // Clear errors for all other nodes
  nodes.value.forEach(node => {
    if (node.data?.error) {
        delete node.data.error;
    }
  });

  console.log('Running workflow with current nodes and edges:', nodes.value.map(n=>n.id), edges.value.map(e=>e.id));
  await runWorkflowConcurrently(); // Use the refactored execution logic
  console.log('Workflow execution complete.');
}

// Define custom edge types.
const edgeTypes = {
  specialEdge: markRaw(SpecialEdge),
};

// Function to update the edge type.
function updateEdgeType(newEdgeType: string) {
  defaultEdgeType.value = newEdgeType;
  edges.value = edges.value.map((edge) => ({
    ...edge,
    type: newEdgeType,
  }));
}

// --- CONTEXT MENU STATE ---
const contextMenu = ref({
  show: false,
  x: 0,
  y: 0,
  nodeId: null as string | null,
});

// Show context menu
function showContextMenu(event: MouseEvent, nodeId: string) {
  contextMenu.value = {
    show: true,
    x: event.clientX,
    y: event.clientY,
    nodeId,
  };
}

// Hide context menu
function hideContextMenu() {
  contextMenu.value.show = false;
}

// Copy node ID to clipboard
async function copyNodeId() {
  if (contextMenu.value.nodeId) {
    try {
      await navigator.clipboard.writeText(contextMenu.value.nodeId);
      console.log(`Copied node ID: ${contextMenu.value.nodeId}`);
    } catch (err) {
      console.error('Failed to copy node ID:', err);
    }
  }
  hideContextMenu();
}

// Hide context menu on click outside
document.addEventListener('click', hideContextMenu);

// Remove event listener on component unmount
import { onBeforeUnmount } from 'vue';
onBeforeUnmount(() => {
  document.removeEventListener('click', hideContextMenu);
});
</script>

<style>
@reference './style.css';
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';

.run-button {
  @apply text-white bg-[#007bff] hover:bg-[#0056b3] font-bold text-base rounded-xl mr-2 cursor-pointer;
}

.switch {
  @apply relative inline-block w-10 h-5;
}

.switch input {
  @apply opacity-0 w-0 h-0;
}

.slider {
  @apply absolute cursor-pointer inset-0 bg-gray-300 transition duration-300;
}

.slider:before {
  content: '';
  @apply absolute h-4 w-4 left-[2px] bottom-[2px] bg-white transition duration-300;
}

input:checked + .slider {
  @apply bg-blue-500;
}

input:focus + .slider {
  box-shadow: 0 0 1px #2196F3;
}

input:checked + .slider:before {
  transform: translateX(20px);
}

.slider.round {
  @apply rounded-full;
}

.slider.round:before {
  @apply rounded-full;
}

.tooltip-container {
  @apply relative;
}

.tooltip {
  @apply whitespace-normal w-[200px] invisible absolute bottom-[200%] left-1/2 -translate-x-1/2 bg-orange-500/90 text-white px-3 py-2 rounded-xl text-xs font-bold z-[1000] text-center;
}

.tooltip-container:hover .tooltip {
  @apply visible;
}

.tooltip::after {
  content: '';
  @apply absolute top-full left-1/2 -translate-x-1/2 border-[5px] border-solid border-orange-500/90 border-t-transparent border-l-transparent border-r-transparent;
}

.context-menu {
  @apply absolute bg-[#333] border border-[#777] rounded-lg p-2 z-[1000] shadow-md;
}

.context-menu-item {
  @apply text-white px-3 py-2 cursor-pointer;
}

.context-menu-item:hover {
  @apply bg-[#555];
}
</style>
