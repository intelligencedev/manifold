<template>
  <FlowBaseNode
    :collapsed="true"
    :min-width="FLOW_STEP_NODE_COLLAPSED.width"
    :min-height="FLOW_STEP_NODE_COLLAPSED.height"
    :min-width-px="`${FLOW_STEP_NODE_COLLAPSED.width}px`"
    :min-height-px="`${FLOW_STEP_NODE_COLLAPSED.height}px`"
    :show-resizer="false"
    :root-class="rootClass"
    :selected="props.selected"
  >
    <template #front>
      <!-- Header -->
      <div class="flex items-start justify-between gap-2">
        <div class="flex-1 min-w-0">
          <div class="text-sm font-semibold text-foreground select-none whitespace-nowrap">
            {{ headerLabel }}
          </div>
        </div>
      </div>
      <!-- Step ID chip row (below header, always visible) -->
      <div class="mt-1 flex items-center justify-between gap-2">
        <button
          class="hidden sm:inline-flex max-w-[200px] items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono text-foreground/80 hover:bg-muted/60"
          :title="copied ? 'Copied!' : `Copy step id: ${props.id}`"
          @click.prevent.stop="copyStepId"
        >
          <svg
            v-if="!copied"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="h-3.5 w-3.5"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
            <path
              d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
            ></path>
          </svg>
          <svg
            v-else
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="h-3.5 w-3.5 text-green-500"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
          <span class="truncate">{{ props.id }}</span>
        </button>
      </div>


    </template>
  </FlowBaseNode>
</template>

<script setup lang="ts">
import { computed, inject, provide, ref, watch, onMounted } from "vue";
import { useVueFlow, type NodeProps } from "@vue-flow/core";

import FlowBaseNode from "./FlowBaseNode.vue";
import type { StepNodeData } from "@/types/flow";
import type { FlowEditorTool } from "@/types/flowEditor";
import type { Ref } from "vue";
import { FLOW_STEP_NODE_COLLAPSED } from "@/constants/flowNodes";

const props = defineProps<NodeProps<StepNodeData>>();

const { updateNodeData } = useVueFlow();

provide("flowEditorNodeId", props.id);

const toolsRef = inject<Ref<FlowEditorTool[]>>(
  "flowEditorTools",
  ref<FlowEditorTool[]>([]),
);

const toolOptions = computed(() => {
  const options = [...(toolsRef?.value ?? [])];
  const current = props.data?.step?.tool?.name;
  if (current && !options.some((tool) => tool.name === current)) {
    options.push({ name: current });
  }
  return options;
});

const toolName = ref("");
const rootClass = computed(() => [
  "min-w-[160px] min-h-[72px] w-fit",
  "transition-colors duration-150 ease-out",
]);
const copied = ref(false);

const currentTool = computed(
  () => toolOptions.value.find((tool) => tool.name === toolName.value) ?? null,
);
const headerLabel = computed(() => {
  const label = (props.data?.label ?? "").trim();
  if (label) return label;
  return currentTool.value?.name ?? "Workflow Step";
});

// Keep toolName in sync with step data
watch(
  () => props.data?.step,
  (nextStep) => {
    toolName.value = nextStep?.tool?.name ?? "";
  },
  { immediate: true, deep: true },
);

async function copyStepId() {
  try {
    await navigator.clipboard.writeText(props.id);
    copied.value = true;
    setTimeout(() => (copied.value = false), 1200);
  } catch (err) {
    window.prompt("Copy step id", props.id);
  }
}

// Mark node data as collapsed on mount so hitbox matches visual size
onMounted(() => {
  updateNodeData(props.id, { ...(props.data ?? { order: 0 }), collapsed: true });
});
</script>
