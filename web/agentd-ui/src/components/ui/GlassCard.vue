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
  props.flat ? "" : "halo-surface",
  props.interactive && !props.flat
    ? "hover:border-accent hover:bg-surface-muted"
    : "",
  props.subtle && !props.flat ? "halo-surface-2" : "",
  props.padded === false ? "p-0" : "p-5",
]);

const as = computed(() => props.as || "div");
</script>
