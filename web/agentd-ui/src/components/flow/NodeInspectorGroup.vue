<template>
  <div class="space-y-3">
    <div class="text-xs text-subtle-foreground">Group Container</div>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Label
      <input
        v-model="labelText"
        type="text"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
        placeholder="Group"
        :disabled="!isDesignMode || hydratingRef"
      />
    </label>

    <div class="space-y-1 text-[11px] text-muted-foreground">
      <div>Color</div>
      <div class="flex flex-wrap gap-2">
        <button
          v-for="preset in colorPresets"
          :key="preset.value"
          type="button"
          class="color-swatch"
          :class="{ active: groupColor === preset.value }"
          :style="{ backgroundColor: preset.display }"
          :title="preset.label"
          :disabled="!isDesignMode || hydratingRef"
          @click="groupColor = preset.value"
        >
          <svg
            v-if="groupColor === preset.value"
            class="h-3 w-3 text-white drop-shadow"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="3"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
        </button>
      </div>
    </div>

    <div class="pt-1 flex items-center justify-end gap-2">
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground"
        >Unsaved</span
      >
      <span
        v-else-if="showAppliedFeedback"
        class="text-[10px] italic text-emerald-400"
        >Applied</span
      >
      <button
        class="rounded px-2 py-1 text-[11px] font-medium transition"
        :class="
          showAppliedFeedback
            ? 'bg-emerald-500 text-white shadow-[0_0_0_1px_rgba(16,185,129,0.3)]'
            : 'bg-accent text-accent-foreground'
        "
        :disabled="(!isDirty && !showAppliedFeedback) || !isDesignMode"
        @click="applyChanges"
      >
        {{ showAppliedFeedback ? 'Applied' : 'Apply' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, ref, watch, type Ref } from "vue";
import { useVueFlow } from "@vue-flow/core";

import type { GroupNodeData } from "@/types/flow";

const props = defineProps<{ nodeId: string; data: GroupNodeData }>();

const { updateNodeData } = useVueFlow();
const modeRef = inject<Ref<"design" | "run">>(
  "flowEditorMode",
  ref<"design" | "run">("design"),
);
const hydratingRef = inject<Ref<boolean>>("flowEditorHydrating", ref(false));

const colorPresets = [
  { value: "default", label: "Default", display: "rgba(148, 163, 184, 0.3)" },
  { value: "blue", label: "Blue", display: "rgba(56, 189, 248, 0.25)" },
  { value: "green", label: "Green", display: "rgba(34, 197, 94, 0.25)" },
  { value: "amber", label: "Amber", display: "rgba(251, 191, 36, 0.25)" },
  { value: "rose", label: "Rose", display: "rgba(251, 113, 133, 0.25)" },
  { value: "purple", label: "Purple", display: "rgba(168, 85, 247, 0.25)" },
];

const isDesignMode = computed(() => modeRef.value === "design");
const labelText = ref("Group");
const groupColor = ref("default");
const isDirty = ref(false);
const showAppliedFeedback = ref(false);
let appliedFeedbackTimer: ReturnType<typeof setTimeout> | null = null;
let suppress = false;

watch(
  () => props.data,
  (next) => {
    suppress = true;
    labelText.value = next?.label?.trim() || "Group";
    groupColor.value = next?.color || "default";
    isDirty.value = false;
    suppress = false;
  },
  { immediate: true, deep: true },
);

watch([labelText, groupColor], () => {
  if (suppress || hydratingRef.value || !isDesignMode.value) return;
  clearAppliedFeedback();
  isDirty.value = true;
});

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return;
  updateNodeData(props.nodeId, {
    ...(props.data ?? { kind: "group" }),
    kind: "group",
    label: labelText.value.trim() || "Group",
    color: groupColor.value,
  });
  isDirty.value = false;
  triggerAppliedFeedback();
}

function triggerAppliedFeedback() {
  showAppliedFeedback.value = true;
  if (appliedFeedbackTimer) clearTimeout(appliedFeedbackTimer);
  appliedFeedbackTimer = setTimeout(() => {
    showAppliedFeedback.value = false;
    appliedFeedbackTimer = null;
  }, 1400);
}

function clearAppliedFeedback() {
  showAppliedFeedback.value = false;
  if (appliedFeedbackTimer) {
    clearTimeout(appliedFeedbackTimer);
    appliedFeedbackTimer = null;
  }
}

onBeforeUnmount(() => {
  clearAppliedFeedback();
});
</script>

<style scoped>
.color-swatch {
  width: 1.75rem;
  height: 1.75rem;
  border-radius: 0.375rem;
  border: 2px solid rgb(var(--color-border) / 0.5);
  cursor: pointer;
  transition: all 150ms;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.color-swatch:hover:enabled {
  border-color: rgb(var(--color-foreground) / 0.6);
  transform: scale(1.05);
}

.color-swatch:disabled {
  cursor: default;
  opacity: 0.55;
}

.color-swatch.active {
  border-color: rgb(var(--color-accent));
  box-shadow: 0 0 0 2px rgb(var(--color-accent) / 0.3);
}
</style>