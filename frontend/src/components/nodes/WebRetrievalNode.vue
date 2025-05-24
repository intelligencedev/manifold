<template>
  <BaseNode :id="id" :data="data" :min-height="180" @resize="onResize">
    <template #header>
      <div :style="data.labelStyle" class="node-label">Web Retrieval</div>
    </template>

    <BaseTextarea
      :id="`${data.id}-url`"
      label="URLs (comma-separated)"
      v-model="urls"
      rows="3"
    />

    <Handle
      style="width: 12px; height: 12px"
      v-if="data.hasInputs"
      type="target"
      position="left"
      id="input"
    />

    <Handle
      style="width: 12px; height: 12px"
      v-if="data.hasOutputs"
      type="source"
      position="right"
      id="output"
    />
  </BaseNode>
</template>

<script setup lang="ts">
import { onMounted } from "vue";
import { Handle } from "@vue-flow/core";
import BaseTextarea from "@/components/base/BaseTextarea.vue";
import BaseNode from "@/components/base/BaseNode.vue";
import { useWebRetrieval } from "@/composables/useWebRetrieval";

// ----- Define props & emits -----
const props = defineProps({
  id: {
    type: String,
    required: false,
    default: "WebRetrieval_0",
  },
  data: {
    type: Object,
    required: false,
    default: () => ({
      type: "WebRetrievalNode",
      labelStyle: {},
      style: {},
      inputs: {
        url: "https://en.wikipedia.org/wiki/Singularity_theory",
      },
      outputs: {},
      hasInputs: true,
      hasOutputs: true,
      inputHandleColor: "#777",
      inputHandleShape: "50%",
      handleColor: "#777",
      outputHandleShape: "50%",
    }),
  },
});

const emit = defineEmits(["update:data", "resize"]);

// Use the composable
const { urls, onResize, setup } = useWebRetrieval(props, emit);

// Initialize the node
onMounted(() => {
  setup();
});
</script>

<!-- No scoped CSS: all styling is via Tailwind -->
