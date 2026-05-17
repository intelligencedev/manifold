<template>
  <span
    class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium tabular-nums"
    :class="variantClass"
  >
    <span v-if="dot" class="mr-1.5 h-1.5 w-1.5 rounded-full" :class="dotClass" aria-hidden="true" />
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from "vue";

type Variant = "success" | "warning" | "danger" | "info" | "muted" | "accent" | "running" | "failed" | "completed";

const props = withDefaults(defineProps<{ variant?: Variant; dot?: boolean }>(), { variant: "muted", dot: false });

const variantClass = computed(() => ({
  "bg-emerald-500/15 text-emerald-300 ring-1 ring-inset ring-emerald-500/30": props.variant === "success" || props.variant === "running",
  "bg-amber-500/15 text-amber-300 ring-1 ring-inset ring-amber-500/30": props.variant === "warning",
  "bg-red-500/15 text-red-300 ring-1 ring-inset ring-red-500/30": props.variant === "danger" || props.variant === "failed",
  "bg-sky-500/15 text-sky-300 ring-1 ring-inset ring-sky-500/30": props.variant === "info",
  "bg-white/8 text-white/60 ring-1 ring-inset ring-white/10": props.variant === "muted" || props.variant === "completed",
  "bg-accent/15 text-accent/90 ring-1 ring-inset ring-accent/30": props.variant === "accent",
}));

const dotClass = computed(() => ({
  "bg-emerald-400": props.variant === "success" || props.variant === "running",
  "bg-amber-400": props.variant === "warning",
  "bg-red-400": props.variant === "danger" || props.variant === "failed",
  "bg-sky-400": props.variant === "info",
  "bg-white/40": props.variant === "muted" || props.variant === "completed",
  "bg-accent": props.variant === "accent",
}));
</script>
