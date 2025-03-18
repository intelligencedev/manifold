<template>
    <div :style="{ ...data.style, ...customStyle, width: '100%', height: '100%' }"
      class="node-container mermaid-node tool-node" @mouseenter="isHovered = true" @mouseleave="isHovered = false">
      <div :style="data.labelStyle" class="node-label">{{ data.type }}</div>
  
      <!-- Input and Output Handles -->
      <Handle style="width:12px; height:12px" v-if="data.hasInputs" type="target" position="left" id="input" />
      <Handle style="width:12px; height:12px" v-if="data.hasOutputs" type="source" position="right" id="output" />
          
      <!-- Container for Mermaid diagram -->
      <div class="mermaid-container" ref="mermaidContainer"></div>
  
      <!-- Resizer Component -->
      <NodeResizer :is-resizable="true" :color="'#666'" :handle-style="resizeHandleStyle" :line-style="resizeHandleStyle"
          :min-width="300" :min-height="300" :node-id="props.id" @resize="onResize" />
    </div>
  </template>
  
  <script setup>
  import { ref, computed, onMounted, nextTick, onUnmounted, watch } from "vue";
  import { Handle, useVueFlow } from "@vue-flow/core";
  import { NodeResizer } from "@vue-flow/node-resizer";
  import mermaid from "mermaid";
  
  // --- PROPS & DEFAULTS ---
  const props = defineProps({
    id: {
      type: String,
      required: true,
      default: "MermaidNode_0",
    },
    data: {
      type: Object,
      required: false,
      default: () => ({
        type: "MermaidNode",
        labelStyle: { fontWeight: "normal" },
        hasInputs: true,
        hasOutputs: true,
        inputs: {
          mermaidText: "graph TD\n    A[Connect Input] --> B[To Generate Diagram]",
        },
        outputs: {},
        style: {
          border: "1px solid #666",
          borderRadius: "4px",
          backgroundColor: "#222",
          color: "#eee",
          width: "300px",
          height: "300px",
        },
      }),
    },
  });
  const emit = defineEmits(["update:data", "resize"]);
  
  // --- VUE FLOW HELPERS ---
  const { getEdges, findNode, updateNodeData } = useVueFlow();
  
  // --- REACTIVE REFERENCES ---
  const mermaidContainer = ref(null);
  const customStyle = ref({});
  const isHovered = ref(false);
  
  const resizeHandleStyle = computed(() => ({
    visibility: isHovered.value ? 'visible' : 'hidden',
    width: '12px',
    height: '12px',
  }));
  
  // --- MERMAID TEXT HANDLING ---
  const mermaidText = computed({
    get: () => props.data.inputs?.mermaidText || "",
    set: (value) => {
      props.data.inputs.mermaidText = value;
      emitUpdate();
    },
  });
  
  // --- RUN FUNCTION ---
  async function run() {
    console.log("Running MermaidNode:", props.id);
  
    // Get connected source nodes
    const connectedSources = getEdges.value
      .filter((edge) => edge.target === props.id)
      .map((edge) => edge.source);
  
    if (connectedSources.length > 0) {
      const sourceNode = findNode(connectedSources[0]);
      
      console.log("Source node:", sourceNode);
  
      if (sourceNode && sourceNode.data.outputs.result) {
        // Update mermaid text from the source node's output
        props.data.inputs.mermaidText = sourceNode.data.outputs.result.output;
        
        // Pass the output to our own outputs as well
        props.data.outputs.result = {
          output: props.data.inputs.mermaidText
        };
        
        // Render the updated diagram
        nextTick(() => {
          initializeMermaid();
        });
        
        emitUpdate();
      }
    }
    
    return { output: props.data.inputs.mermaidText };
  }
  
  function emitUpdate() {
    const updatedData = {
      ...props.data,
      inputs: { mermaidText: mermaidText.value },
      outputs: {
        result: {
          output: mermaidText.value
        }
      },
    };
    emit("update:data", { id: props.id, data: updatedData });
  }
  
  // --- INITIALIZE MERMAID ---
  function initializeMermaid() {
    const container = mermaidContainer.value;
    if (!container) {
      console.error("Mermaid container not found");
      return;
    }
  
    // Clear existing diagram
    container.innerHTML = '';
  
    try {
      // Default diagram code that should always work
      const defaultDiagram = "graph TD\n    A[Connect Input] --> B[To Generate Diagram]";
      
      // Use the provided text or fall back to default
      const diagramCode = mermaidText.value || defaultDiagram;
      
      // Add a loading indicator
      container.innerHTML = `<div style="text-align:center; padding:20px;">Rendering diagram...</div>`;
  
      // Create a unique element ID for mermaid to target
      const elementId = `mermaid-diagram-${props.id}-${Date.now()}`;
      
      // Create a new div with the mermaid class
      const mermaidDiv = document.createElement('div');
      mermaidDiv.className = 'mermaid';
      mermaidDiv.id = elementId;
      mermaidDiv.style.width = '100%';
      mermaidDiv.style.height = '100%';
      mermaidDiv.textContent = diagramCode;
      
      // Clear and append the new div
      container.innerHTML = '';
      container.appendChild(mermaidDiv);
      
      // Initialize mermaid with safe options
      mermaid.initialize({
        startOnLoad: true,  // Changed to true for this approach
        theme: 'dark',
        securityLevel: 'loose',
        logLevel: 'error',
        flowchart: {
          useMaxWidth: true,
          htmlLabels: true,
          curve: 'basis'
        }
      });
      
      // Let mermaid process the diagram
      try {
        mermaid.init(undefined, `#${elementId}`);
        
        // After successful rendering, store outputs
        props.data.outputs.result = {
          output: diagramCode
        };
        emitUpdate();
        
        // Make SVG responsive by adding attributes after rendering
        nextTick(() => {
          const svg = container.querySelector('svg');
          if (svg) {
            svg.setAttribute('width', '100%');
            svg.setAttribute('height', '100%');
            svg.style.maxWidth = '100%';
          }
        });
      } catch (initError) {
        console.error("Mermaid init error:", initError);
        
        // If the custom diagram fails, try with the default diagram
        if (diagramCode !== defaultDiagram) {
          console.log("Trying with default diagram instead");
          
          const defaultElementId = `mermaid-default-${props.id}-${Date.now()}`;
          const defaultMermaidDiv = document.createElement('div');
          defaultMermaidDiv.className = 'mermaid';
          defaultMermaidDiv.id = defaultElementId;
          defaultMermaidDiv.textContent = defaultDiagram;
          
          container.innerHTML = '';
          container.appendChild(defaultMermaidDiv);
          
          try {
            mermaid.init(undefined, `#${defaultElementId}`);
            
            // Add an error message overlay
            nextTick(() => {
              container.appendChild(createErrorOverlay("Invalid diagram syntax. Connect a valid source."));
            });
            
            // Still store the original input in outputs
            props.data.outputs.result = { 
              error: initError.message,
              output: diagramCode
            };
            emitUpdate();
          } catch (defaultError) {
            // If even the default diagram fails, show a clean error
            showSimpleErrorMessage(container, "Diagram rendering is not supported in your current environment.");
            
            props.data.outputs.result = { 
              error: defaultError.message,
              output: diagramCode
            };
            emitUpdate();
          }
        } else {
          // The default diagram failed - this is likely a compatibility issue
          showSimpleErrorMessage(container, "Diagram rendering is not supported in your current environment.");
          
          props.data.outputs.result = { 
            error: initError.message,
            output: diagramCode
          };
          emitUpdate();
        }
      }
    } catch (error) {
      console.error("Error in mermaid initialization:", error);
      showSimpleErrorMessage(container, "Could not initialize diagram renderer.");
      
      props.data.outputs.result = { 
        error: error.message,
        output: mermaidText.value
      };
      emitUpdate();
    }
  }
  
  // Helper to create error overlay
  function createErrorOverlay(message) {
    const overlay = document.createElement('div');
    overlay.style.position = 'absolute';
    overlay.style.top = '10px';
    overlay.style.left = '10px';
    overlay.style.background = 'rgba(0,0,0,0.7)';
    overlay.style.padding = '5px';
    overlay.style.borderRadius = '3px';
    overlay.style.color = '#ff6b6b';
    overlay.style.fontSize = '12px';
    overlay.textContent = message;
    return overlay;
  }
  
  // Simpler error message for catastrophic failures
  function showSimpleErrorMessage(container, message) {
    container.innerHTML = `
      <div style="display: flex; height: 100%; align-items: center; justify-content: center; padding: 20px; text-align: center;">
        <div style="color: #ff6b6b;">
          <p style="margin-bottom: 10px;">${message}</p>
          <p style="font-size: 12px; opacity: 0.7;">Try a different browser or check the console for details.</p>
        </div>
      </div>
    `;
  }
  
  // --- WATCHER ---
  watch(
    () => props.data.inputs.mermaidText,
    (newVal, oldVal) => {
      console.log("Mermaid text changed, reinitializing diagram.");
      nextTick(() => {
        initializeMermaid();
      });
    }
  );
  
  // --- RESIZE HANDLER ---
  const onResize = (event) => {
    customStyle.value.width = `${event.width}px`;
    customStyle.value.height = `${event.height}px`;
    nextTick(() => {
      initializeMermaid();
    });
    emit("resize", { id: props.id, width: event.width, height: event.height });
  };
  
  // --- LIFECYCLE HOOKS ---
  onMounted(() => {
    // Register the run function
    if (!props.data.run) {
      props.data.run = run;
    }
    
    // Wait for DOM to be ready before initializing
    nextTick(() => {
      // Ensure we have default text in the inputs
      if (!props.data.inputs || !props.data.inputs.mermaidText) {
        if (!props.data.inputs) props.data.inputs = {};
        props.data.inputs.mermaidText = "graph TD\n    A[Connect Input] --> B[To Generate Diagram]";
      }
      
      initializeMermaid();
    });
  });
  
  onUnmounted(() => {
    // Any cleanup if needed
  });
  </script>
  
  <style scoped>
  .mermaid-node {
    background-color: #222;
    border: 1px solid #666;
    border-radius: 4px;
    color: #eee;
    display: flex;
    flex-direction: column;
    position: relative;
  }
  
  .node-label {
    text-align: center;
    font-size: 16px;
    margin-bottom: 5px;
    padding: 5px;
  }
  
  .mermaid-container {
    flex-grow: 1;
    position: relative;
    overflow: hidden;
  }
  </style>