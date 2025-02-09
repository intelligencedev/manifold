<script setup>
import { ref, defineEmits } from "vue";
import { Panel, useVueFlow } from "@vue-flow/core";
import Icon from "./Icon.vue";

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
      }
    };
    reader.readAsText(file);
  }
}
</script>

<template>


    <button title="save graph" @click="onSave" class="icon-button">
      <!-- SVG SAVE ICON -->
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
        stroke-linecap="round" stroke-linejoin="round" role="img" aria-label="Save File Icon">
        <!-- Outline of the file -->
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" />
        <!-- Top fold line of the file -->
        <path d="M14 2v6h6" />
        <!-- Downward arrow for 'Save' -->
        <path d="M12 12v6" />
        <path d="M9 15l3 3 3-3" />
      </svg>
    </button>
    <button title="restore graph" @click="onRestore" class="icon-button">
      <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
        stroke-linecap="round" stroke-linejoin="round" role="img" aria-label="Load File Icon">
        <!-- Outline of the file -->
        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z" />
        <!-- Top fold line of the file -->
        <path d="M14 2v6h6" />
        <!-- Upward arrow for 'Load' -->
        <path d="M12 18v-6" />
        <path d="M9 15l3-3 3 3" />
      </svg>
      <input type="file" ref="fileInput" style="display: none" @change="onFileSelected" accept=".json" />
    </button>


</template>

<style scoped>
/* No button background */
.icon-button {
  /* \side by side buttons equal spacing and width */
  padding: 2px;
  line-height: 0;
  background: none;
  border: 1px solid transparent;
}
</style>