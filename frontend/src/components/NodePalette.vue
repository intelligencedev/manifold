<template>
  <div class="node-palette" :class="{ 'is-open': isOpen }">
    <div class="toggle-button" @click="togglePalette">
      <svg v-if="isOpen" xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor"
        viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
      </svg>
      <svg v-else xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" viewBox="0 0 16 16">
        <path fill-rule="evenodd"
          d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
      </svg>
    </div>
    <div class="palette-content">
      <div class="scrollable-content">
        <div v-for="(nodes, category) in nodeCategories" :key="category" class="accordion-section">
          <div class="accordion-header" @click="toggleAccordion(category)">
            <span>{{ category }}</span>
            <svg v-if="isExpanded(category)" xmlns="http://www.w3.org/2000/svg" width="12" height="12" fill="currentColor"
              viewBox="0 0 16 16">
              <path fill-rule="evenodd"
                d="M11.354 1.646a.5.5 0 0 1 0 .708L5.707 8l5.647 5.646a.5.5 0 0 1-.708.708l-6-6a.5.5 0 0 1 0-.708l6-6a.5.5 0 0 1 .708 0z" />
            </svg>
            <svg v-else xmlns="http://www.w3.org/2000/svg" width="12" height="12" fill="currentColor"
              viewBox="0 0 16 16">
              <path fill-rule="evenodd"
                d="M4.646 1.646a.5.5 0 0 1 .708 0l6 6a.5.5 0 0 1 0 .708l-6 6a.5.5 0 0 1-.708-.708L10.293 8 4.646 2.354a.5.5 0 0 1 0-.708z" />
            </svg>
          </div>
          <div v-if="isExpanded(category)" class="accordion-content">
            <div v-for="(nodeComponent, key) in nodes" :key="key" class="node-item" draggable="true"
              @dragstart="(event) => onDragStart(event, key)">
              {{ key }}
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import useDragAndDrop from '../composables/useDnD.js'

const { onDragStart } = useDragAndDrop()
const isOpen = ref(false)

function togglePalette() {
  isOpen.value = !isOpen.value
}

const nodeCategories = {
  "Text Completions": {
    "completions": null,
    "responseNode": null,
    "reactAgent": null,
  },
  "Image Generation": {
    "comfyNode": null,
    "mlxFluxNode": null,
  },
  "Speech Generation": {
    "ttsNode": null,
  },
  "Code": {
    "codeRunnerNode": null,
    "webGLNode": null,
  },
  "Web": {
    "webSearchNode": null,
    "webRetrievalNode": null,
  },
  "Documents": {
    "openFileNode": null,
    "saveTextNode": null,
    "textSplitterNode": null,
    "documentsIngestNode": null,
    "documentsRetrieveNode": null,
    "repoConcatNode": null,
  },
  "Utilities": {
    "textNode": null,
    "noteNode": null,
    "embeddingsNode": null,
    "tokenCounterNode": null,
  },
  "Integrations": {
    "mermaidNode": null,
    "datadogNode": null,
    "datadogGraphNode": null,
  },
  "Experimental": {
    "mcpClientNode": null,
    "flowControlNode": null,
    "messageBusNode": null,
  },
}

// Initialize state for expanded/collapsed accordion sections
const expandedCategories = reactive({})
Object.keys(nodeCategories).forEach((category) => {
  expandedCategories[category] = category === "Text Completions"
})

// Toggle individual accordion section; if the section is open, collapse it, and vice versa.
function toggleAccordion(category) {
  expandedCategories[category] = !expandedCategories[category]
}

function isExpanded(category) {
  return expandedCategories[category]
}
</script>

<style scoped>
.node-palette {
  position: fixed;
  top: 50px;
  bottom: 0px;
  width: 250px;
  background-color: #222;
  color: #eee;
  z-index: 1100;
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
  padding: 6px;
  height: 100%;
  box-sizing: border-box;
}

.scrollable-content {
  overflow-y: auto;
  height: 100%;
  padding-right: 10px;
}

.scrollable-content::-webkit-scrollbar {
  width: 8px;
}

.scrollable-content::-webkit-scrollbar-track {
  background: #333;
  border-radius: 4px;
}

.scrollable-content::-webkit-scrollbar-thumb {
  background-color: #666;
  border-radius: 4px;
  border: 2px solid #333;
}

.scrollable-content::-webkit-scrollbar-thumb:hover {
  background-color: #888;
}

.accordion-section {
  margin-bottom: 15px;
}

.accordion-header {
  font-size: 14px;
  font-weight: bold;
  margin-bottom: 8px;
  text-transform: uppercase;
  color: #bbb;
  cursor: pointer;
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 4px;
  background-color: #333;
  border: 1px solid #666;
  border-radius: 4px;
}

.accordion-header:hover {
  background-color: #444;
}

.accordion-content {
  padding-left: 8px;
}

.node-item {
  padding: 8px;
  margin-bottom: 6px;
  background-color: #333;
  border: 1px solid #666;
  border-radius: 5px;
  cursor: grab;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 20px;
}

.node-item:hover {
  background-color: #444;
}
</style>