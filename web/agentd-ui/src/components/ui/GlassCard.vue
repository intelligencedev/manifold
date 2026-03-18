<template>
  <component :is="as" :class="classes">
    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed } from "vue";

type GlassCardTag = keyof HTMLElementTagNameMap;

const props = defineProps<{
  as?: GlassCardTag;
  padded?: boolean;
  interactive?: boolean;
  subtle?: boolean;
  flat?: boolean;
}>();

const classes = computed(() => [
  "relative w-full text-foreground transition-all duration-200",
  props.flat
    ? ""
    : "glass-surface rounded-[var(--radius-lg,26px)] border border-white/10",
  props.flat ? "" : "supports-[backdrop-filter]:backdrop-blur-xl bg-surface/70",
  props.interactive && !props.flat
    ? "hover:border-accent/50 hover:shadow-[0_22px_60px_rgba(0,0,0,0.32)] hover:-translate-y-[1px]"
    : "",
  props.subtle && !props.flat ? "bg-surface/60" : "",
  props.padded === false ? "p-0" : "p-4 md:p-6",
]);

const as = computed(() => props.as || "div");
</script>
