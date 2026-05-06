<template>
  <button
    v-bind="forwardedAttrs"
    :type="resolvedType"
    :disabled="isDisabled"
    :aria-busy="loading ? 'true' : undefined"
    :data-loading="loading ? 'true' : undefined"
    :class="classes"
  >
    <span class="relative inline-flex items-center justify-center gap-2">
      <svg
        v-if="loading"
        class="h-[1em] w-[1em] animate-spin"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        aria-hidden="true"
      >
        <circle class="opacity-25" cx="12" cy="12" r="9" />
        <path class="opacity-80" d="M21 12a9 9 0 0 0-9-9" />
      </svg>
      <slot />
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed, useAttrs } from "vue";

type ButtonType = "button" | "submit" | "reset";

defineOptions({
  inheritAttrs: false,
});

const props = withDefaults(
  defineProps<{
    variant?: "neutral" | "accent" | "danger" | "ghost";
    size?: "xs" | "sm" | "md";
    loading?: boolean;
    disabled?: boolean;
    pressed?: boolean;
    block?: boolean;
  }>(),
  {
    variant: "neutral",
    size: "md",
    loading: false,
    disabled: false,
    pressed: false,
    block: false,
  },
);

const attrs = useAttrs();

const isDisabled = computed(() => props.disabled || props.loading);

const resolvedType = computed<ButtonType>(() => {
  const type = attrs.type;
  if (type === "submit" || type === "reset") {
    return type;
  }
  return "button";
});

const forwardedAttrs = computed(() => {
  const { class: _class, type: _type, disabled: _disabled, ...rest } = attrs;
  return rest;
});

const classes = computed(() => {
  const base = [
    "group relative inline-flex select-none items-center justify-center overflow-hidden rounded-lg border text-sm font-medium transition-[transform,background-color,border-color,color,box-shadow,opacity] duration-150 ease-out",
    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/70 focus-visible:ring-offset-2 focus-visible:ring-offset-background",
    "active:translate-y-px active:scale-[0.985] disabled:translate-y-0 disabled:scale-100 disabled:cursor-not-allowed disabled:opacity-55",
  ];

  const sizes = {
    xs: "min-h-8 px-2.5 py-1 text-xs",
    sm: "min-h-9 px-3 py-2 text-sm",
    md: "min-h-10 px-3.5 py-2.5 text-sm",
  };

  const variants = {
    neutral:
      "border-border/70 bg-surface-muted/55 text-foreground shadow-[inset_0_1px_0_rgba(255,255,255,0.03)] hover:border-border hover:bg-surface-muted/80",
    accent:
      "border-accent/45 bg-accent/18 text-foreground shadow-[0_10px_30px_rgba(90,89,211,0.16)] hover:border-accent/60 hover:bg-accent/26",
    danger:
      "border-danger/45 bg-danger/8 text-danger-foreground hover:border-danger/60 hover:bg-danger/16",
    ghost:
      "border-transparent bg-transparent text-subtle-foreground hover:border-border/60 hover:bg-surface-muted/55 hover:text-foreground",
  };

  return [
    ...base,
    sizes[props.size],
    variants[props.variant],
    props.block ? "w-full" : "",
    props.loading ? "cursor-progress" : "",
    props.pressed
      ? "border-accent/55 bg-accent/14 text-foreground shadow-[inset_0_1px_0_rgba(255,255,255,0.04),0_0_0_1px_rgba(90,89,211,0.18)]"
      : "",
    attrs.class,
  ];
});
</script>
