<script setup>
import { ref, defineEmits } from "vue";

const emit = defineEmits(["save", "restore"]);
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
.control-button {
  padding: 6px 12px;
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

.control-button:hover {
  background-color: rgba(85, 85, 85, 0.2); /* Light transparent hover effect */
  border-color: #777;
  color: #fff;
}

/* Hide file input (already done inline, but good practice) */
input[type="file"] {
  display: none;
}
</style>