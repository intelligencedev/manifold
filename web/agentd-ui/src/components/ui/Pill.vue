<template>
  <span :class="classes">
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed, type PropType } from "vue";

type PillTone =
  | "accent"
  | "neutral"
  | "success"
  | "danger"
  | "warning"
  | "info";
type PillSize = "sm" | "md";

const props = defineProps({
  tone: { type: String as PropType<PillTone>, default: "neutral" },
  size: { type: String as PropType<PillSize>, default: "md" },
  glow: { type: Boolean, default: false },
});

const toneClasses: Record<PillTone, string> = {
  accent:
    "bg-[rgb(124_134_255_/_0.08)] text-[rgb(var(--accent-hi))] border border-[rgb(124_134_255_/_0.3)]",
  neutral:
    "bg-surface-muted text-muted-foreground border border-[rgb(var(--line-strong))]",
  success:
    "bg-[rgb(70_211_154_/_0.08)] text-success border border-[rgb(70_211_154_/_0.3)]",
  danger:
    "bg-[rgb(240_112_95_/_0.08)] text-danger border border-[rgb(240_112_95_/_0.3)]",
  warning:
    "bg-[rgb(232_177_74_/_0.08)] text-warning border border-[rgb(232_177_74_/_0.3)]",
  info: "bg-[rgb(79_214_192_/_0.08)] text-[rgb(var(--data))] border border-[rgb(79_214_192_/_0.3)]",
};

const classes = computed(() => [
  "inline-flex items-center gap-1 rounded-[5px] font-mono leading-none tracking-[0.04em]",
  props.size === "sm" ? "px-2 py-[3px] text-[11px]" : "px-2.5 py-1 text-xs",
  toneClasses[props.tone],
]);
</script>
