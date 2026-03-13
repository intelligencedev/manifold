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
}>();

const classes = computed(() => [
  "glass-surface relative w-full rounded-[22px] border border-border/60 text-foreground transition-colors duration-200",
  "supports-[backdrop-filter]:backdrop-blur-lg bg-surface/78",
  props.interactive
    ? "hover:border-accent/32 hover:bg-surface-muted/70"
    : "",
  props.subtle ? "bg-surface/62" : "",
  props.padded === false ? "p-0" : "p-5 md:p-6",
]);

const as = computed(() => props.as || "div");
</script>
