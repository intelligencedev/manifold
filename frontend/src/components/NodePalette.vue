<template>
  <div class="node-palette" :class="{ 'is-open': isOpen }">
    <div class="toggle-button" @click="togglePalette">
      <!-- SVG for left-facing chevron -->
      <svg
        v-if="isOpen"
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        fill="currentColor"
        viewBox="0 0 16 16"
      >
        <path
          fill-rule="evenodd"
          d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z"
        />
      </svg>
      <!-- SVG for right-facing chevron -->
      <svg
        v-else
        xmlns="http://www.w3.org/2000/svg"
        width="16"
        height="16"
        fill="currentColor"
        viewBox="0 0 16 16"
      >
        <path
          fill-rule="evenodd"
          d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z"
        />
      </svg>
    </div>
    <div class="palette-content">
      <!-- Loop through each category -->
      <div v-for="(nodes, category) in nodeCategories" :key="category" class="category">
        <div class="category-title">{{ category }}</div>
        <!-- Loop through the nodes within the category -->
        <div
          v-for="(nodeComponent, key) in nodes"
          :key="key"
          class="node-item"
          draggable="true"
          @dragstart="(event) => onDragStart(event, key)"
        >
          {{ key }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import useDragAndDrop from '../useDnD.js'

const { onDragStart } = useDragAndDrop()
const isOpen = ref(false)

function togglePalette() {
  isOpen.value = !isOpen.value
}

/**
 * Node categorization:
 *
 * - Text Completions: Nodes related to text generation and response handling.
 * - Code: Nodes handling command execution.
 * - Web: Nodes for web search and retrieval.
 * - Monitoring: Nodes for monitoring and graphing (Datadog).
 * - Misc: Utility nodes like text splitting, saving, token counting, etc.
 */
const nodeCategories = {
  "Text Completions": {
    "agentNode": null,
    "responseNode": null,
    "geminiNode": null,
    "geminiResponse": null,
  },
  "Code": {
    "runCmd": null,
    "webGLNode": null,
  },
  "Web": {
    "webSearchNode": null,
    "webRetrievalNode": null,
  },
  "Monitoring": {
    "datadogNode": null,
    "datadogGraphNode": null,
  },
  "Misc": {
    "flowControlNode": null,
    "textNode": null,
    "textSplitterNode": null,
    "noteNode": null,
    "embeddingsNode": null,
    "saveTextNode": null,
    "tokenCounterNode": null,
  },
}
</script>

<style scoped>
.node-palette {
  position: fixed;
  left: 0;
  height: 100%;
  width: 250px;
  background-color: #222;
  color: #eee;
  z-index: 1100; /* Increased from 100 to 1100 */
  transition: transform 0.3s ease-in-out;
  transform: translateX(-100%);
}

.node-palette.is-open {
  transform: translateX(0);
}

.toggle-button {
  position: absolute;
  top: 50%;
  right: -30px;
  width: 30px;
  height: 60px;
  background-color: #222;
  border: 1px solid #666;
  border-left: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-top-right-radius: 8px;
  border-bottom-right-radius: 8px;
}

.toggle-button svg {
  fill: #eee;
  width: 16px;
  height: 16px;
}

.palette-content {
  padding: 20px;
}

.category {
  margin-bottom: 15px;
}

.category-title {
  font-size: 14px;
  font-weight: bold;
  margin-bottom: 8px;
  text-transform: uppercase;
  color: #bbb;
}

.node-item {
  padding: 8px;
  margin-bottom: 6px;
  background-color: #333;
  border: 1px solid #666;
  border-radius: 5px;
  cursor: grab;
  height: 20px;
  align-items: center;
  justify-content: center;
}

.node-item:hover {
  background-color: #444;
}
</style>
