<template>
  <div :class="[baseClass, $attrs.class]" aria-hidden="true" />
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{ variant?: "line" | "block" | "circle" }>(),
  { variant: "line" }
);
defineOptions({ inheritAttrs: false });

const baseClass = computed(() => [
  "animate-pulse rounded bg-white/8",
  { "h-4 w-full": props.variant === "line", "h-20 w-full rounded-xl": props.variant === "block", "h-8 w-8 rounded-full": props.variant === "circle" },
]);
</script>
