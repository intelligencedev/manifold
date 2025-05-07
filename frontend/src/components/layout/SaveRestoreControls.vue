<script setup>
import { ref, defineEmits, onMounted } from "vue";
import { Panel, useVueFlow } from "@vue-flow/core";
// Assuming Icon component is not strictly needed for these controls based on provided snippet
// import Icon from "./Icon.vue";

const emit = defineEmits(["save", "restore"]);

const fileInput = ref(null);
const templates = ref([]);
const selectedTemplate = ref("");

onMounted(async () => {
  // Fetch templates from the backend API
  await fetchTemplates();
});

// Function to fetch available templates from the server
async function fetchTemplates() {
  try {
    const response = await fetch('/api/workflows/templates');
    if (!response.ok) {
      throw new Error('Failed to fetch templates');
    }
    
    const data = await response.json();
    // Process templates for display
    templates.value = data.map(template => ({
      id: template.id, 
      name: formatTemplateName(template.name)
    }));
  } catch (error) {
    console.error('Error fetching workflow templates:', error);
    templates.value = []; // Set empty array on error
  }
}

// Format template names for display (removes numbers and underscores, capitalizes words)
function formatTemplateName(filename) {
  // Remove any leading numbers and underscores (like "1_" in "1_chat_completion")
  const nameWithoutPrefix = filename.replace(/^\d+_/, '');
  
  // Replace underscores with spaces and capitalize each word
  return nameWithoutPrefix
    .replace(/_/g, ' ')
    .replace(/\b\w/g, letter => letter.toUpperCase());
}

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
        alert("Failed to load file. Ensure it is a valid JSON workflow.");
      }
    };
    reader.onerror = (e) => {
        console.error("Error reading file:", e);
        alert("Error reading file.");
    }
    reader.readAsText(file);
  }
  // Reset file input value to allow selecting the same file again
  event.target.value = null;
}

async function onTemplateSelected() {
  if (!selectedTemplate.value) return;

  try {
    // Find the selected template from our templates array
    const template = templates.value.find(t => t.name === selectedTemplate.value);
    
    if (!template) {
      throw new Error(`Template not found: ${selectedTemplate.value}`);
    }
    
    // Fetch the selected template from the backend
    const response = await fetch(`/api/workflows/templates/${template.id}`);
    if (!response.ok) {
      throw new Error(`Failed to fetch template: ${selectedTemplate.value}`);
    }
    
    // Get the flow data from the response
    const flow = await response.json();
    
    // Emit the restore event with the flow data
    emit("restore", flow);
    
    // Reset the dropdown selection
    selectedTemplate.value = "";
  } catch (error) {
    console.error("Error loading template:", error);
    alert(`Error loading template: ${error.message}`);
    selectedTemplate.value = "";
  }
}
</script>

<template>
  <div class="controls-container">
    <button title="Save current workflow" @click="onSave" class="control-button">
      Save
    </button>
    <button title="Load workflow from file" @click="onRestore" class="control-button">
      Load
      <input type="file" ref="fileInput" style="display: none" @change="onFileSelected" accept=".json" />
    </button>

    <div class="control-select-wrapper">
      <select
        v-model="selectedTemplate"
        @change="onTemplateSelected"
        class="control-select"
        title="Load a workflow template"
      >
        <!-- Use a disabled option for the placeholder -->
        <option disabled value="">Templates</option>
        <option v-for="template in templates" :key="template.id" :value="template.name">
          {{ template.name }}
        </option>
      </select>
    </div>
  </div>
</template>

<style scoped>
.controls-container {
  display: flex;
  align-items: center;
  gap: 8px; /* Use gap for spacing between flex items */
  padding: 8px; /* Add some padding around the controls */
  border-radius: 6px; /* Optional: Rounded corners for the container */
}

/* Base styles for interactive elements */
.control-button,
.control-select {
  padding: 6px 12px;
  /* Removed margin, using gap on parent */
  background-color: transparent; /* Changed to transparent */
  border: 1px solid #555; /* Keeping the subtle border */
  color: #eee;
  cursor: pointer;
  font-size: 14px;
  border-radius: 4px; /* Consistent rounded corners */
  height: 32px; /* Explicit height */
  line-height: 18px; /* Aligns text vertically (Height - 2*PaddingY - 2*Border) */
  text-transform: capitalize; /* Consistent capitalization */
  transition: background-color 0.2s ease, border-color 0.2s ease, color 0.2s ease;
  box-sizing: border-box; /* Include padding and border in element's total width and height */
  vertical-align: middle; /* Helps alignment if container wasn't flex */
  white-space: nowrap; /* Prevent text wrapping */
}

.control-button:hover,
.control-select:hover {
  background-color: rgba(85, 85, 85, 0.2); /* Light transparent hover effect */
  border-color: #777;
  color: #fff;
}

/* Specific styles for the select wrapper and element */
.control-select-wrapper {
  position: relative;
  display: inline-block; /* Or block if needed */
  vertical-align: middle;
}

.control-select {
  appearance: none; /* Remove default OS styling */
  -webkit-appearance: none;
  -moz-appearance: none;
  padding-right: 30px; /* Make space for the custom arrow */
  min-width: 130px; /* Adjust as needed */
  background-image: none; /* Ensure no residual background images */
  text-align: left; /* More standard alignment */
  text-overflow: ellipsis; /* Handle long template names */
}

/* Style for the placeholder option */
.control-select option[disabled] {
  color: #999; /* Dim the placeholder text */
}

.control-select option {
  background: #333;
  color: #eee;
}

/* Custom dropdown arrow using pseudo-element on the wrapper */
.control-select-wrapper::after {
  content: '▼';
  font-size: 12px;
  color: #aaa;
  position: absolute;
  right: 10px; /* Position inside the padding area */
  top: 50%;
  transform: translateY(-50%);
  pointer-events: none; /* So it doesn't interfere with clicking the select */
  z-index: 1; /* Ensure it's above the select */
  transition: color 0.2s ease;
}

.control-select-wrapper:hover::after {
  color: #eee; /* Match text color on hover */
}

/* Hide file input (already done inline, but good practice) */
input[type="file"] {
  display: none;
}
</style>