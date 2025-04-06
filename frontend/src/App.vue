<template>
  <div id="app">
    <!-- Update Header to include save/restore handlers -->
    <Header @save="onSave" @restore="onRestore" />
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
    <VueFlow class="vue-flow-container" :nodes="nodes" :edges="edges" :edge-types="edgeTypes"
      :zoom-on-scroll="zoomOnScroll" @nodes-initialized="onNodesInitialized" @nodes-change="onNodesChange"
      @edges-change="onEdgesChange" @connect="onConnect" @dragover="onDragOver" @dragleave="onDragLeave" @drop="onDrop"
      :min-zoom="0.2" :max-zoom="4" fit-view-on-init :snap-to-grid="true" :snap-grid="[16, 16]"
      :default-viewport="{ x: 0, y: 0, zoom: 1 }">
      <!-- Node Templates -->
      <template #node-noteNode="noteNodeProps">
        <NoteNode v-bind="noteNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" @contextmenu.prevent="showContextMenu($event, noteNodeProps.id)" />
      </template>
      <template #node-agentNode="agentNodeProps">
        <AgentNode v-bind="agentNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, agentNodeProps.id)" />
      </template>
      <template #node-claudeNode="claudeNodeProps">
        <ClaudeNode v-bind="claudeNodeProps" @contextmenu.native.prevent="showContextMenu($event, claudeNodeProps.id)" />
      </template>
      <template #node-responseNode="responseNodeProps">
        <ResponseNode v-bind="responseNodeProps" @disable-zoom="disableZoom" @enable-zoom="enableZoom"
          @node-resized="updateNodeDimensions" @contextmenu.native.prevent="showContextMenu($event, responseNodeProps.id)" />
      </template>
      <template #node-geminiNode="geminiNodeProps">
        <GeminiNode v-bind="geminiNodeProps" @contextmenu.native.prevent="showContextMenu($event, geminiNodeProps.id)" />
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

      <!-- <Controls :style="{ backgroundColor: '#222', color: '#eee' }" /> -->
      <!-- <MiniMap :background-color="bgColor" :node-color="'#333'" :node-stroke-color="'#555'" :node-stroke-width="2"
        :mask-color="'rgba(40, 40, 40, 0.8)'" /> -->
      <Background :color="bgColor" :variant="bgVariant" :gap="16" :size="1" :pattern-color="'#444'" />

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
  Background,
  BackgroundVariant,
} from '@vue-flow/additional-components';
import SpecialEdge from './components/SpecialEdge.vue';
import { useConfigStore } from '@/stores/configStore';

// Manifold custom components
import { isNodeConnected } from './utils/nodeHelpers.js';
import Header from './components/layout/Header.vue';
import LayoutControls from './components/layout/LayoutControls.vue';
import useDragAndDrop from './composables/useDnD.js';
import NodePalette from './components/NodePalette.vue';
import UtilityPalette from './components/UtilityPalette.vue';
import NoteNode from './components/nodes/NoteNode.vue';
import AgentNode from './components/nodes/AgentNode.vue';
import ClaudeNode from './components/nodes/ClaudeNode.vue';
import ResponseNode from './components/nodes/ResponseNode.vue';
import GeminiNode from './components/nodes/GeminiNode.vue';
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
import MLXFlux from './components/nodes/MLXFlux.vue';
import DocumentsIngest from './components/nodes/DocumentsIngestNode.vue';
import DocumentsRetrieve from './components/nodes/DocumentsRetrieveNode.vue';
import ttsNode from './components/nodes/ttsNode.vue';
import MCPClient from './components/nodes/MCPClient.vue';
import Mermaid from './components/nodes/Mermaid.vue';

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
});

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
  let executionLimit = nodes.value.length * 10; // Safety break for potential loops
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
      console.log(`Node ${nodeId} signaled jump to: ${jumpTargetId}`);
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
        const subExecutionLimit = nodes.value.length * 5; // Smaller safety limit for sub-workflows
        
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
          
          // Add this node's children to the sub-queue
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

// REMOVED loadTemplate function
// async function loadTemplate() {
//   try {
//     const host = window.location.hostname;
//     const port = window.location.port;
//     const response = await fetch(`http://${host}:${port}/templates/basic_completions.json`);
//     if (!response.ok) {
//       throw new Error(`Failed to load template: ${response.statusText}`);
//     }
//     const flowData = await response.json();
//     onRestore(flowData);
//   } catch (error) {
//     console.error('Error loading template:', error);
//   }
// }

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

/* Context Menu Styling */
.context-menu {
  position: absolute;
  background-color: #333;
  border: 1px solid #777;
  border-radius: 8px;
  padding: 8px;
  z-index: 1000;
  box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
}

.context-menu-item {
  color: white;
  padding: 8px 12px;
  cursor: pointer;
}

.context-menu-item:hover {
  background-color: #555;
}
</style>