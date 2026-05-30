<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    tone?: "neutral" | "run" | "ok" | "warn" | "danger" | "data";
  }>(),
  {
    tone: "neutral",
  },
);

const toneClasses = {
  neutral:
    "border-[rgb(var(--line-strong))] bg-surface-muted text-muted-foreground",
  run: "border-[rgb(124_134_255_/_0.3)] bg-[rgb(124_134_255_/_0.08)] text-[rgb(var(--accent-hi))]",
  ok: "border-[rgb(70_211_154_/_0.3)] bg-[rgb(70_211_154_/_0.08)] text-success",
  warn: "border-[rgb(232_177_74_/_0.3)] bg-[rgb(232_177_74_/_0.08)] text-warning",
  danger:
    "border-[rgb(240_112_95_/_0.3)] bg-[rgb(240_112_95_/_0.08)] text-danger",
  data: "border-[rgb(79_214_192_/_0.3)] bg-[rgb(79_214_192_/_0.08)] text-[rgb(var(--data))]",
};

const classes = computed(() => [
  "inline-flex items-center rounded-[5px] border px-2 py-[3px] font-mono text-[11px] leading-none tracking-[0.04em]",
  toneClasses[props.tone],
]);
</script>
