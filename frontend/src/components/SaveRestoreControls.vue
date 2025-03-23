<script setup>
import { ref, defineEmits, onMounted } from "vue";
import { Panel, useVueFlow } from "@vue-flow/core";
import Icon from "./Icon.vue";

const emit = defineEmits(["save", "restore"]);

const fileInput = ref(null);
const templates = ref([]);
const selectedTemplate = ref("");

// Use Vite's import.meta.glob to get all JSON files in the workflows directory
const workflowModules = import.meta.glob("../components/workflows/*.json");

onMounted(async () => {
  // Process the paths to get template names without extensions
  templates.value = Object.keys(workflowModules).map(path => {
    const filename = path.split('/').pop();
    return filename.replace(/\.json$/, '');
  });
});

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

async function onTemplateSelected() {
  if (!selectedTemplate.value) return;
  
  // Find the module path for the selected template
  const templatePath = Object.keys(workflowModules).find(path => 
    path.includes(`${selectedTemplate.value}.json`)
  );
  
  if (templatePath) {
    try {
      // Dynamically import the selected template file
      const module = await workflowModules[templatePath]();
      const flow = module.default || module;
      emit("restore", flow);
      // Reset dropdown after loading
      selectedTemplate.value = "";
    } catch (error) {
      console.error("Error loading template:", error);
    }
  }
}
</script>

<template>
  <div class="controls-container">
    <button title="save graph" @click="onSave" class="text-button">
      save
    </button>
    <button title="restore graph" @click="onRestore" class="text-button">
      load
      <input type="file" ref="fileInput" style="display: none" @change="onFileSelected" accept=".json" />
    </button>
    
    <div class="template-selector">
      <select v-model="selectedTemplate" @change="onTemplateSelected" class="template-dropdown">
        <option value="">Templates</option>
        <option v-for="template in templates" :key="template" :value="template">
          {{ template }}
        </option>
      </select>
    </div>
  </div>
</template>

<style scoped>
.text-button {
  padding: 4px 8px;
  margin: 0 4px;
  background: none;
  border: none;
  color: #eee;
  cursor: pointer;
  font-size: 14px;
  text-transform: lowercase;
}

.text-button:hover {
  color: #fff;
}

.controls-container {
  display: flex;
  align-items: center;
}

.template-selector {
  margin: 0 4px;
}

.template-dropdown {
  padding: 4px 8px;
  background: none;
  border: none;
  color: #eee;
  cursor: pointer;
  font-size: 14px;
  text-transform: lowercase;
  outline: none;
  min-width: 100px;
}

.template-dropdown:hover {
  color: #fff;
}

.template-dropdown option {
  background: #333;
  color: #eee;
}
</style>