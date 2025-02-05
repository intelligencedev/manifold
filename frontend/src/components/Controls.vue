<script setup>
import { ref, nextTick, onMounted, defineEmits, defineExpose } from "vue";
import { Panel, useVueFlow } from "@vue-flow/core";
import useLayout from "../useLayout.js";

// Define all emitted events from both functionalities.
const emit = defineEmits([
  "save",
  "restore",
  "update-nodes",
  "layout-initialized",
  "update-edge-type",
]);

// --- Save / Restore Logic ---
const fileInput = ref(null);

function onSave() {
  emit("save");
}

function onRestore() {
  fileInput.value.click();
}

function onFileSelected(event) {
  const file = event.target.files[0];
  if (file) {
    const reader = new FileReader();
    reader.onload = (e) => {
      try {
        const flow = JSON.parse(e.target.result);
        emit("restore", flow);
      } catch (error) {
        console.error("Error parsing JSON file:", error);
      }
    };
    reader.readAsText(file);
  }
}

// --- Layout Controls Logic ---
const { getNodes, getEdges, fitView } = useVueFlow();
const { layout } = useLayout();

// List of available edge types.
const edgeTypes = ["bezier", "step", "smoothstep", "straight"];
let currentEdgeTypeIndex = 0;

async function layoutGraph(direction) {
  // 1) Apply the dagre layout algorithm.
  const updatedNodes = layout(getNodes.value, getEdges.value, direction);
  // 2) Emit new node positions.
  emit("update-nodes", updatedNodes);
  // 3) After DOM updates, adjust the view.
  await nextTick();
  fitView();
}

function cycleEdgeType() {
  currentEdgeTypeIndex = (currentEdgeTypeIndex + 1) % edgeTypes.length;
  const newEdgeType = edgeTypes[currentEdgeTypeIndex];
  emit("update-edge-type", newEdgeType);
}

// Expose methods in case the parent needs to trigger a layout programmatically.
defineExpose({ layoutGraph, cycleEdgeType });

// Inform the parent that the layout controls are ready.
onMounted(() => {
  emit("layout-initialized");
});
</script>

<template>
  <!-- Panel with fixed right positioning, no background -->
  <Panel position="top-right" class="controls-panel">
    <div class="controls">
      <!-- Save / Restore Section -->
      <div class="section save-restore">
        <button title="save graph" @click="onSave" class="icon-button">
          <!-- SVG for Save -->
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            stroke-width="2" stroke-linecap="round" stroke-linejoin="round" role="img"
            aria-label="Save File Icon">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" />
            <path d="M14 2v6h6" />
            <path d="M12 12v6" />
            <path d="M9 15l3 3 3-3" />
          </svg>
        </button>
        <button title="restore graph" @click="onRestore" class="icon-button">
          <!-- SVG for Restore -->
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor"
            stroke-width="2" stroke-linecap="round" stroke-linejoin="round" role="img"
            aria-label="Load File Icon">
            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" />
            <path d="M14 2v6h6" />
            <path d="M12 18v-6" />
            <path d="M9 15l3-3 3 3" />
          </svg>
          <!-- Hidden file input for JSON file upload -->
          <input type="file" ref="fileInput" style="display: none" @change="onFileSelected" accept=".json" />
        </button>
      </div>

      <!-- Layout Controls Section -->
      <div class="section layout-controls">
        <button @click="layoutGraph('TB')" title="Vertical Layout" class="icon-button">
          <!-- SVG for Vertical Layout -->
          <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24">
            <path fill="currentColor"
              d="M19 7.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C17.902 5 17.435 5 16.5 5h-3.75V2a.75.75 0 0 0-1.5 0v3H7.5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C5 6.098 5 6.565 5 7.5s0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C6.098 10 6.565 10 7.5 10h3.75v4H9.5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C7 15.098 7 15.565 7 16.5s0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C8.098 19 8.565 19 9.5 19h1.75v3a.75.75 0 0 0 1.5 0v-3h1.75c.935 0 1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75s0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C15.902 14 15.435 14 14.5 14h-1.75v-4h3.75c.935 0 1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549C19 8.902 19 8.435 19 7.5"/>
          </svg>
        </button>
        <button @click="layoutGraph('LR')" title="Horizontal Layout" class="icon-button">
          <!-- SVG for Horizontal Layout -->
          <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24">
            <path fill="currentColor"
              d="M7.5 5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C5 6.098 5 6.565 5 7.5v3.75H2a.75.75 0 0 0 0 1.5h3v3.75c0 .935 0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C6.098 19 6.565 19 7.5 19s1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75v-3.75h4v1.75c0 .935 0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549c.348.201.815.201 1.75.201s1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75v-1.75h3a.75.75 0 0 0 0-1.5h-3V9.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C17.902 7 17.435 7 16.5 7s-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C14 8.098 14 8.565 14 9.5v1.75h-4V7.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C8.902 5 8.435 5 7.5 5"/>
          </svg>
        </button>
        <button @click="cycleEdgeType" title="Cycle Edge Type" class="icon-button">
          <!-- SVG for Cycle Edge Type -->
          <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24">
            <circle cx="5" cy="5" r="3" fill="currentColor" />
            <circle cx="19" cy="19" r="3" fill="currentColor" />
            <path fill="currentColor" fill-rule="evenodd"
              d="M10.25 5a.75.75 0 0 1 .75-.75h5.132c2.751 0 3.797 3.593 1.476 5.07l-10.41 6.625c-1.056.672-.58 2.305.67 2.305h3.321l-.22-.22a.75.75 0 1 1 1.061-1.06l1.5 1.5a.75.75 0 0 1 0 1.06l-1.5 1.5a.75.75 0 1 1-1.06-1.06l.22-.22H7.867c-2.751 0-3.797-3.593-1.476-5.07l10.411-6.625c1.055-.672.58-2.305-.671-2.305H11a.75.75 0 0 1-.75-.75"
              clip-rule="evenodd" />
          </svg>
        </button>
      </div>
    </div>
  </Panel>
</template>

<style scoped>
.controls-panel {
  height: 100vh;           /* Full vertical height */
  right: 0;                /* Align to the right edge */
  top: 0;                  /* Align to the top */
  position: fixed;         /* Fixed position so it stays visible */
  display: flex;
  justify-content: center; /* Center buttons horizontally */
  align-items: center;     /* Center buttons vertically */
  background: none;        /* No background color */
}

.controls {
  display: flex;
  flex-direction: column;
}

.section {
  display: flex;
  flex-direction: column;
  margin-bottom: 10px;     /* Spacing between sections */
}

.icon-button {
  padding: 2px;
  line-height: 0;
  background: none;
  border: 1px solid transparent;
  cursor: pointer;
  margin-bottom: 5px;      /* Spacing between buttons */
}

.icon-button svg {
  display: block;
}
</style>
