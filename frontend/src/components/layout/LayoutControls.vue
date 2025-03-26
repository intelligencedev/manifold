<template>
    <button @click="layoutGraph('TB')" title="Vertical Layout" class="icon-button">
      <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24"><path fill="currentColor" d="M19 7.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C17.902 5 17.435 5 16.5 5h-3.75V2a.75.75 0 0 0-1.5 0v3H7.5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C5 6.098 5 6.565 5 7.5s0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C6.098 10 6.565 10 7.5 10h3.75v4H9.5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C7 15.098 7 15.565 7 16.5s0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C8.098 19 8.565 19 9.5 19h1.75v3a.75.75 0 0 0 1.5 0v-3h1.75c.935 0 1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75s0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C15.902 14 15.435 14 14.5 14h-1.75v-4h3.75c.935 0 1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549C19 8.902 19 8.435 19 7.5"/></svg>
    </button>
    <button @click="layoutGraph('LR')" title="Horizontal Layout" class="icon-button">
      <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24"><path fill="currentColor" d="M7.5 5c-.935 0-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C5 6.098 5 6.565 5 7.5v3.75H2a.75.75 0 0 0 0 1.5h3v3.75c0 .935 0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549C6.098 19 6.565 19 7.5 19s1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75v-3.75h4v1.75c0 .935 0 1.402.201 1.75a1.5 1.5 0 0 0 .549.549c.348.201.815.201 1.75.201s1.402 0 1.75-.201a1.5 1.5 0 0 0 .549-.549c.201-.348.201-.815.201-1.75v-1.75h3a.75.75 0 0 0 0-1.5h-3V9.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C17.902 7 17.435 7 16.5 7s-1.402 0-1.75.201a1.5 1.5 0 0 0-.549.549C14 8.098 14 8.565 14 9.5v1.75h-4V7.5c0-.935 0-1.402-.201-1.75a1.5 1.5 0 0 0-.549-.549C8.902 5 8.435 5 7.5 5"/></svg>
    </button>
    <button @click="cycleEdgeType" title="Cycle Edge Type" class="icon-button">
      <svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 24 24"><circle cx="5" cy="5" r="3" fill="currentColor"/><circle cx="19" cy="19" r="3" fill="currentColor"/><path fill="currentColor" fill-rule="evenodd" d="M10.25 5a.75.75 0 0 1 .75-.75h5.132c2.751 0 3.797 3.593 1.476 5.07l-10.41 6.625c-1.056.672-.58 2.305.67 2.305h3.321l-.22-.22a.75.75 0 1 1 1.061-1.06l1.5 1.5a.75.75 0 0 1 0 1.06l-1.5 1.5a.75.75 0 1 1-1.06-1.06l.22-.22H7.867c-2.751 0-3.797-3.593-1.476-5.07l10.411-6.625c1.055-.672.58-2.305-.671-2.305H11a.75.75 0 0 1-.75-.75" clip-rule="evenodd"/></svg>
    </button>
</template>

<script setup>
import { nextTick, onMounted, defineEmits, defineExpose } from "vue";
import { useVueFlow, Panel } from "@vue-flow/core";
import useLayout from "../../composables/useLayout.js";
import Icon from "./Icon.vue";

const emit = defineEmits(["update-nodes", "layout-initialized", "update-edge-type"]);
const { getNodes, getEdges, fitView } = useVueFlow();
const { layout } = useLayout();

const edgeTypes = ['bezier', 'step', 'smoothstep', 'straight'];
let currentEdgeTypeIndex = 0;

async function layoutGraph(direction) {
  // 1) perform dagre layout
  const updatedNodes = layout(getNodes.value, getEdges.value, direction);
  // 2) Tell parent that node positions changed
  emit("update-nodes", updatedNodes);
  // 3) Next tick, fit the view
  await nextTick();
  fitView();
}

function cycleEdgeType() {
  currentEdgeTypeIndex = (currentEdgeTypeIndex + 1) % edgeTypes.length;
  const newEdgeType = edgeTypes[currentEdgeTypeIndex];
  emit("update-edge-type", newEdgeType);
}

// Expose layoutGraph so the parent can call it via this.$refs or the ref in <script setup>.
defineExpose({ layoutGraph, cycleEdgeType });

onMounted(() => {
  emit("layout-initialized");
});
</script>

<style scoped>
.icon-button {
  padding: 2px; /* Reduce padding */
  line-height: 0; /* Reset line-height */
  background: none; /* Remove default background */
  /* Add a border */
  border: 1px solid transparent;
}

.process-panel,
.layout-panel {
  display: flex;
  gap: 10px;
}

.process-panel {
  background-color: #2d3748;
  padding: 10px;
  border-radius: 8px;
  box-shadow: 0 0 10px rgba(0, 0, 0, 0.5);
  display: flex;
  flex-direction: column;
}

.process-panel button {
  border: none;
  cursor: pointer;
  background-color: #4a5568;
  border-radius: 8px;
  color: white;
  box-shadow: 0 0 10px rgba(0, 0, 0, 0.5);
}

.process-panel button {
  font-size: 16px;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.checkbox-panel {
  display: flex;
  align-items: center;
  gap: 10px;
}

.process-panel button:hover,
.layout-panel button:hover {
  background-color: #2563eb;
  transition: background-color 0.2s;
}

.process-panel label {
  color: white;
  font-size: 12px;
}

.stop-btn svg {
  display: none;
}

.stop-btn:hover svg {
  display: block;
}

.stop-btn:hover .spinner {
  display: none;
}

.spinner {
  border: 3px solid #f3f3f3;
  border-top: 3px solid #2563eb;
  border-radius: 50%;
  width: 10px;
  height: 10px;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
}
</style>