<template>
  <span
    :class="[
      'inline-flex items-center gap-2 rounded-sm border px-2 py-[3px] font-mono text-[11px] uppercase tracking-[0.08em]',
      statusClasses,
    ]"
  >
    <span :class="['h-[7px] w-[7px] rounded-full', dotClass]"></span>
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  state:
    | "online"
    | "offline"
    | "degraded"
    | "queued"
    | "running"
    | "failed"
    | "completed";
}>();

const statusClasses = computed(() => {
  switch (props.state) {
    case "online":
    case "completed":
      return "border border-success/40 bg-success/10 text-success";
    case "running":
      return "border-[rgb(124_134_255_/_0.3)] bg-[rgb(124_134_255_/_0.08)] text-[rgb(var(--accent-hi))]";
    case "queued":
      return "border border-warning/40 bg-warning/10 text-warning";
    case "degraded":
      return "border border-warning/40 bg-warning/10 text-warning";
    case "failed":
    case "offline":
    default:
      return "border border-danger/40 bg-danger/10 text-danger";
  }
});

const dotClass = computed(() => {
  switch (props.state) {
    case "online":
    case "completed":
      return "bg-success";
    case "running":
      return "bg-accent halo-pulse";
    case "queued":
      return "animate-pulse bg-warning";
    case "degraded":
      return "animate-pulse bg-warning";
    case "failed":
    case "offline":
    default:
      return "bg-danger";
  }
});
</script>
