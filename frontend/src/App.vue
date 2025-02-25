<template>
  <div id="app">
    <!-- Update Header to include save/restore handlers -->
    <Header @save="onSave" @restore="onRestore" />
    <NodePalette />
    <UtilityPalette />

    <!-- VueFlow Component -->
    <VueFlow class="vue-flow-container" :nodes="nodes" :edges="edges" :edge-types="edgeTypes"
      :zoom-on-scroll="zoomOnScroll" @nodes-initialized="onNodesInitialized" @nodes-change="onNodesChange"
      @edges-change="onEdgesChange" @connect="onConnect" @dragover="onDragOver" @dragleave="onDragLeave" @drop="onDrop"
      :min-zoom="0.2" :max-zoom="4" fit-view-on-init :snap-to-grid="true" :snap-grid="[20, 20]"
      :default-viewport="{ x: 0, y: 0, zoom: 1 }">
      <!-- Node Templates -->
      <template #node-noteNode="noteNodeProps">
        <NoteNode v-bind="noteNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-agentNode="agentNodeProps">
        <AgentNode v-bind="agentNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-claudeNode="claudeNodeProps">
        <ClaudeNode v-bind="claudeNodeProps" />
      </template>
      <template #node-claudeResponse="claudeResponseProps">
        <ClaudeResponse v-bind="claudeResponseProps" />
      </template>
      <template #node-geminiNode="geminiNodeProps">
        <GeminiNode v-bind="geminiNodeProps" />
      </template>
      <template #node-pythonRunnerNode="pythonRunnerNodeProps">
        <PythonRunner v-bind="pythonRunnerNodeProps" />
      </template>
      <template #node-webGLNode="webGLNodeProps">
        <WebGLNode v-bind="webGLNodeProps" />
      </template>
      <template #node-responseNode="responseNodeProps">
        <ResponseNode v-bind="responseNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-geminiResponse="geminiResponseProps">
        <GeminiResponse v-bind="geminiResponseProps" />
      </template>
      <template #node-embeddingsNode="embeddingsNodeProps">
        <EmbeddingsNode v-bind="embeddingsNodeProps" />
      </template>
      <template #node-webSearchNode="webSearchNodeProps">
        <WebSearchNode v-bind="webSearchNodeProps" />
      </template>
      <template #node-webRetrievalNode="webRetrievalNodeProps">
        <WebRetrievalNode v-bind="webRetrievalNodeProps" />
      </template>
      <template #node-textSplitterNode="textSplitterNodeProps">
        <TextSplitterNode v-bind="textSplitterNodeProps" />
      </template>
      <template #node-textNode="textNodeProps">
        <TextNode v-bind="textNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-openFileNode="openFileNodeProps">
        <OpenFileNode v-bind="openFileNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-saveTextNode="saveTextNodeProps">
        <SaveTextNode v-bind="saveTextNodeProps" />
      </template>
      <template #node-datadogNode="datadogNodeProps">
        <DatadogNode v-bind="datadogNodeProps" />
      </template>
      <template #node-datadogGraphNode="datadogGraphNodeProps">
        <DatadogGraphNode v-bind="datadogGraphNodeProps" />
      </template>
      <template #node-tokenCounterNode="tokenCounterNodeProps">
        <TokenCounterNode v-bind="tokenCounterNodeProps" />
      </template>
      <template #node-flowControlNode="flowControlNodeProps">
        <FlowControl v-bind="flowControlNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" />
      </template>
      <template #node-repoConcatNode="repoConcatNodeProps">
        <RepoConcat v-bind="repoConcatNodeProps" />
      </template>
      <template #node-comfyNode="comfyNodeProps">
        <ComfyNode v-bind="comfyNodeProps" />
      </template>
      <template #node-mlxFluxNode="mlxFluxNodeProps">
        <MLXFlux v-bind="mlxFluxNodeProps" />
      </template>
      <template #node-documentsIngestNode="documentsIngestNodeProps">
        <DocumentsIngest v-bind="documentsIngestNodeProps" />
      </template>
      <template #node-documentsRetrieveNode="documentsRetrieveNodeProps">
        <DocumentsRetrieve v-bind="documentsRetrieveNodeProps" />
      </template>

      <Controls :style="{ backgroundColor: '#222', color: '#eee' }" />
      <MiniMap :background-color="bgColor" :node-color="'#333'" :node-stroke-color="'#555'" :node-stroke-width="2"
        :mask-color="'rgba(40, 40, 40, 0.8)'" />
      <Background :color="bgColor" :variant="bgVariant" :gap="20" :size="1" :pattern-color="'#444'" />

      <!-- Run Workflow Button -->
      <div class="bottom-bar">
        <div style="display: flex; justify-content: space-evenly; align-items: center;"></div>
        <div class="bottom-toolbar">
          <!-- three divs -->
          <div style="flex: 1; display: flex; justify-content: center;">
            <!-- Toggle Switch -->
            <div class="tooltip-container" style="display: flex; align-items: center;">
              <label class="switch">
                <input type="checkbox" v-model="autoPanEnabled">
                <span class="slider round"></span>
              </label>
              <span style="color: white; margin-left: 5px; font-size: 14px;">Auto-Pan</span>
              <span class="tooltip">When enabled, the view will automatically pan to follow node execution</span>
            </div>
          </div>
          <div style="flex: 1; display: flex; justify-content: center; align-items: center;">
            <button class="run-button" @click="runWorkflow">Run</button>
          </div>
          <div style="flex: 1; display: flex; justify-content: center;">
            <LayoutControls ref="layoutControls" @update-nodes="updateLayout" :style="{ zIndex: 1000 }"
              @update-edge-type="updateEdgeType" />
          </div>
        </div>
        <div style="display: flex; justify-content: space-evenly; align-items: center;"></div>
      </div>
    </VueFlow>
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
  Controls,
  MiniMap,
  Background,
  BackgroundVariant,
} from '@vue-flow/additional-components';
import SpecialEdge from './components/SpecialEdge.vue';
import { useConfigStore } from '@/stores/configStore';

// Manifold custom components
import Header from './components/Header.vue';
import SaveRestoreControls from './components/SaveRestoreControls.vue';
import LayoutControls from './components/LayoutControls.vue';
import useDragAndDrop from './useDnD';
import NodePalette from './components/NodePalette.vue';
import UtilityPalette from './components/UtilityPalette.vue';
import NoteNode from './components/NoteNode.vue';
import AgentNode from './components/AgentNode.vue';
import ClaudeNode from './components/ClaudeNode.vue';
import ClaudeResponse from './components/ClaudeResponse.vue';
import GeminiNode from './components/GeminiNode.vue';
import GeminiResponse from './components/GeminiResponse.vue';
import PythonRunner from './components/PythonRunner.vue';
import WebGLNode from './components/WebGLNode.vue';
import ResponseNode from './components/ResponseNode.vue';
import EmbeddingsNode from './components/EmbeddingsNode.vue';
import WebSearchNode from './components/WebSearchNode.vue';
import WebRetrievalNode from './components/WebRetrievalNode.vue';
import TextNode from './components/TextNode.vue';
import TextSplitterNode from './components/TextSplitterNode.vue';
import OpenFileNode from './components/OpenFileNode.vue';
import SaveTextNode from './components/SaveTextNode.vue';
import DatadogNode from './components/DatadogNode.vue';
import DatadogGraphNode from './components/DatadogGraphNode.vue';
import TokenCounterNode from './components/TokenCounterNode.vue';
import FlowControl from './components/FlowControl.vue';
import RepoConcat from './components/RepoConcat.vue';
import ComfyNode from './components/ComfyNode.vue';
import MLXFlux from './components/MLXFlux.vue';
import DocumentsIngest from './components/DocumentsIngest.vue';
import DocumentsRetrieve from './components/DocumentsRetrieve.vue';

// --- SETUP ---
interface BgColorInterface {
  value: string;
}

const bgColor: BgColorInterface['value'] = '#303030';
const bgVariant = BackgroundVariant.Dots;

// --- STATE ---

// Destructure fitView along with other methods
const { findNode, getNodes, getEdges, toObject, fromObject, fitView } = useVueFlow();
const nodes = ref<GraphNode[]>([]);
const edges = ref<GraphEdge[]>([]);
const defaultEdgeType = ref<string>('bezier'); // Set the default edge type

const configStore = useConfigStore();

// Load configuration on startup
onMounted(() => {
  configStore.fetchConfig();
  loadTemplate(); // Load the template on mount
});

// Watchers for debugging
watch(getNodes, (newNodes) => console.log('nodes changed', newNodes));
watch(getEdges, (newEdges) => console.log('edges changed', newEdges));

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

  if (nodes.value) {
    const targetNode = nodes.value.find((node) => node.id === params.target);
    if (targetNode) {
      if (params.targetHandle === 'input') {
        let connectedTo = targetNode.data.connectedTo || [];
        if (!Array.isArray(connectedTo)) {
          connectedTo = [connectedTo];
        }
        if (!connectedTo.includes(params.source)) {
          connectedTo.push(params.source);
        }
        targetNode.data = {
          ...targetNode.data,
          connectedTo,
        };
        nodes.value = nodes.value.map((node) =>
          node.id === targetNode.id ? { ...node, data: { ...node.data } } : node
        );
      }
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

/**
 * Refactored workflow execution.
 *
 * We build an adjacency list that records, for each node, its children along with the
 * output handle ("continue" vs. "loopback"). When a FlowControl node is executed,
 * its loopback branch is run repeatedly (based on loopCount) and after each loopback run,
 * the FlowControl node is re-run to aggregate updated inputs (e.g. from a ResponseNode).
 * Only after looping does the FlowControl node propagate its "continue" branch.
 */

// Auto-pan toggle
const autoPanEnabled = ref(true);

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

// Refactored sequential workflow execution using a queue
async function runWorkflowConcurrently() {
  // 1. Build an adjacency list that records outgoing edges with their sourceHandle,
  // and compute in-degree for each node.
  type ChildEdge = { target: string; handle: string };
  const adj: Record<string, ChildEdge[]> = {};
  const inDegree: Record<string, number> = {};

  // Initialize for each node
  for (const node of nodes.value) {
    adj[node.id] = [];
    inDegree[node.id] = 0;
  }

  // Populate the adjacency list and in-degree counts.
  // We use edge.sourceHandle to distinguish between "continue" and "loopback" edges.
  for (const edge of edges.value) {
    const handle = edge.sourceHandle || 'continue';
    adj[edge.source].push({ target: edge.target, handle });
    inDegree[edge.target]++;
  }

  // 2. Start with nodes that have in-degree 0.
  let queue = nodes.value.filter((node) => inDegree[node.id] === 0).map((node) => node.id);

  // 3. Process nodes sequentially.
  while (queue.length > 0) {
    const nodeId = queue.shift()!;
    const node = findNode(nodeId);
    if (!node) continue;

    // Smoothly fit view to the node about to run
    await smoothlyFitViewToNode(node);

    // Change edge styles for visual feedback
    changeEdgeStyles(nodeId);

    // Execute the node's logic
    await node.data.run();

    // If this is a FlowControl node, handle looping:
    if (node.type === 'flowControlNode') {
      const loopCount = node.data.inputs.loopCount || 1;
      // For each additional loop iteration:
      for (let i = 1; i < loopCount; i++) {
        // Execute each loopback branch.
        const loopbackEdges = adj[nodeId].filter((edge) => edge.handle === 'loopback');
        for (const edge of loopbackEdges) {
          const childNode = findNode(edge.target);
          if (childNode) {
            await smoothlyFitViewToNode(childNode);
            changeEdgeStyles(childNode.id);
            await childNode.data.run();
          }
        }
        // Re-run the FlowControl node to aggregate updated inputs.
        await smoothlyFitViewToNode(node);
        changeEdgeStyles(nodeId);
        await node.data.run();
      }
      // Process "continue" branches.
      const continueEdges = adj[nodeId].filter((edge) => edge.handle === 'continue');
      for (const edge of continueEdges) {
        inDegree[edge.target]--;
        if (inDegree[edge.target] === 0) {
          queue.push(edge.target);
        }
      }
    } else {
      // For non-FlowControl nodes, process all outgoing edges.
      for (const edge of adj[nodeId]) {
        inDegree[edge.target]--;
        if (inDegree[edge.target] === 0) {
          queue.push(edge.target);
        }
      }
    }
  }
}

async function runWorkflow() {
  // Clear response nodes and gemini response nodes
  const responseNodes = nodes.value.filter((node) => node.type === 'responseNode');
  for (const node of responseNodes) {
    node.data.inputs.response = '';
    node.data.outputs = {};
  }
  const geminiResponseNodes = nodes.value.filter((node) => node.type === 'geminiResponse');
  for (const node of geminiResponseNodes) {
    node.data.inputs.response = '';
    node.data.outputs = {};
  }

  console.log('Running workflow with current nodes and edges:', nodes.value, edges.value);
  await runWorkflowConcurrently();
  console.log('Workflow execution complete.');
}

// Define custom edge types.
const edgeTypes = {
  specialEdge: markRaw(SpecialEdge),
};

// Function each node runs during workflow to change edge styles.
const changeEdgeStyles = (nodeId: string) => {
  const connectedEdges = edges.value.filter(
    (edge) => edge.source === nodeId || edge.target === nodeId
  );
  console.log('Connected edges:', connectedEdges);
  connectedEdges.forEach((edge) => {
    if (edge.target === nodeId) {
      edge.animated = false;
      edge.style = { strokeWidth: 1 };
    }
    if (edge.source === nodeId) {
      edge.animated = true;
      edge.style = { strokeWidth: 4, stroke: 'darkorange' };
    }
  });
  edges.value = edges.value.map((edge) =>
    connectedEdges.find((e) => e.id === edge.id) || edge
  );
};

// Function to update the edge type.
function updateEdgeType(newEdgeType: string) {
  defaultEdgeType.value = newEdgeType;
  edges.value = edges.value.map((edge) => ({
    ...edge,
    type: newEdgeType,
  }));
}

// Load template function
async function loadTemplate() {
  try {
    const host = window.location.hostname;
    const port = window.location.port;
    const response = await fetch(`http://${host}:${port}/templates/basic_completions.json`);
    if (!response.ok) {
      throw new Error(`Failed to load template: ${response.statusText}`);
    }
    const flowData = await response.json();
    onRestore(flowData);
  } catch (error) {
    console.error('Error loading template:', error);
  }
}

</script>

<style>
@import '@vue-flow/core/dist/style.css';
@import '@vue-flow/core/dist/theme-default.css';

#app {
  font-family: Avenir, Helvetica, Arial, sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
  display: flex;
  flex-direction: column;
  height: 100vh;
  width: 100vw;
  padding: 0;
  margin: 0;
  text-align: center;
  color: #2c3e50;
  position: relative;
}

/* Header Styling */
header {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  z-index: 1000;
  height: 120px;
}

/* VueFlow Container */
.vue-flow-container {
  flex: 1;
  background-color: #282828;
  position: relative;
  overflow: hidden;
  padding-top: 8px;
}

/* Node Container */
.node-container {
  border: 3px solid var(--node-border-color) !important;
  background-color: var(--node-bg-color) !important;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
  padding: 15px;
  border-radius: 8px;
  color: var(--node-text-color);
  font-family: 'Roboto', sans-serif;
}

.bottom-bar {
  position: absolute;
  bottom: 0px;
  left: 0;
  right: 0;
  height: 40px;
  display: flex;
  justify-content: center;
  align-items: center;
  z-index: 10;
}

.bottom-toolbar {
  display: flex;
  justify-content: center;
  align-items: center;
  background-color: #222;
  border-radius: 12px;
  width: 33vw;
  height: 100%;
  padding: 4px;
  border: 1px solid #777;
  margin-bottom: 40px;
}

/* Run Button Styling */
.run-button {
  font-size: 16px;
  font-weight: bold;
  color: #fff;
  background-color: #007bff;
  border: none;
  border-radius: 12px;
  cursor: pointer;
  margin-right: 10px;
  /* Add some margin between button and toggle */
}

.run-button:hover {
  background-color: #0056b3;
}

/* Tool Node Styling */
.tool-node {
  --node-border-color: #777 !important;
  --node-bg-color: #1e1e1e !important;
  --node-text-color: #eee;
}

/* Toggle Switch Styling */
.switch {
  position: relative;
  display: inline-block;
  width: 40px;
  height: 20px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: #ccc;
  -webkit-transition: .4s;
  transition: .4s;
}

.slider:before {
  position: absolute;
  content: "";
  height: 16px;
  width: 16px;
  left: 2px;
  bottom: 2px;
  background-color: white;
  -webkit-transition: .4s;
  transition: .4s;
}

input:checked+.slider {
  background-color: #2196F3;
}

input:focus+.slider {
  box-shadow: 0 0 1px #2196F3;
}

input:checked+.slider:before {
  -webkit-transform: translateX(20px);
  -ms-transform: translateX(20px);
  transform: translateX(20px);
}

/* Rounded sliders */
.slider.round {
  border-radius: 34px;
}

.slider.round:before {
  border-radius: 50%;
}

.tooltip-container {
  position: relative;
}

.tooltip {
  white-space: normal;
  /* Changed from pre-wrap */
  width: 200px;
  /* Increased width to accommodate text */
  visibility: hidden;
  position: absolute;
  bottom: 200%;
  left: 50%;
  transform: translateX(-50%);
  background-color: rgba(255, 140, 0, 0.9);
  color: white;
  padding: 8px 12px;
  border-radius: 12px;
  font-size: 12px;
  font-weight: bold;
  /* Added bold text */
  z-index: 1000;
  text-align: center;
  /* Added for better text alignment */
}

.tooltip-container:hover .tooltip {
  visibility: visible;
}

/* Optional: Add an arrow to the tooltip */
.tooltip::after {
  content: "";
  position: absolute;
  top: 100%;
  left: 50%;
  margin-left: -5px;
  border-width: 5px;
  border-style: solid;
  border-color: rgba(255, 140, 0, 0.9) transparent transparent transparent;
}
</style>