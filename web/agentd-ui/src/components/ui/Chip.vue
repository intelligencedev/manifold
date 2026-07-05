<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed, type PropType } from "vue";

type ChipSize = "sm" | "md";

const props = defineProps({
  size: { type: String as PropType<ChipSize>, default: "sm" },
  muted: { type: Boolean, default: false },
});

const classes = computed(() => [
  "inline-flex items-center rounded-sm border font-mono tracking-[0.04em] text-muted-foreground",
  props.muted
    ? "border-border bg-surface"
    : "border-[rgb(var(--line-strong))] bg-surface-muted",
  props.size === "md" ? "px-3 py-1 text-xs" : "px-2 py-[3px] text-[11px]",
]);
</script>
