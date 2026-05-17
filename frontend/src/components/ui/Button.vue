<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    class="inline-flex items-center justify-center gap-2 rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 disabled:pointer-events-none disabled:opacity-40"
    :class="[sizeClass, variantClass]"
    v-bind="$attrs"
  >
    <svg v-if="loading" class="h-4 w-4 animate-spin" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z" />
    </svg>
    <slot />
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue";

type Variant = "primary" | "secondary" | "ghost" | "danger";
type Size = "sm" | "md" | "lg";

const props = withDefaults(
  defineProps<{ variant?: Variant; size?: Size; type?: "button" | "submit" | "reset"; disabled?: boolean; loading?: boolean }>(),
  { variant: "secondary", size: "md", type: "button", disabled: false, loading: false }
);

const variantClass = computed(() => ({
  "bg-accent text-white hover:bg-accent/90 active:bg-accent/80": props.variant === "primary",
  "border border-white/15 bg-white/5 text-white/80 hover:bg-white/10 active:bg-white/8": props.variant === "secondary",
  "text-white/70 hover:bg-white/5 hover:text-white active:bg-white/8": props.variant === "ghost",
  "bg-red-500/20 text-red-300 hover:bg-red-500/30 border border-red-500/30": props.variant === "danger",
}));

const sizeClass = computed(() => ({
  "h-7 px-2.5 text-xs": props.size === "sm",
  "h-9 px-4": props.size === "md",
  "h-11 px-6 text-base": props.size === "lg",
}));
</script>
