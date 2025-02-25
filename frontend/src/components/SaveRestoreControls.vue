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
    <button title="save graph" @click="onSave" class="text-button">
      save
    </button>
    <button title="restore graph" @click="onRestore" class="text-button">
      load
      <input type="file" ref="fileInput" style="display: none" @change="onFileSelected" accept=".json" />
    </button>
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
</style>