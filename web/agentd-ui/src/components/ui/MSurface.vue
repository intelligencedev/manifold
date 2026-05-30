<template>
  <component :is="tag" :class="classes">
    <header
      v-if="hasHeader"
      class="mb-5 flex items-start justify-between gap-4"
    >
      <div class="min-w-0 space-y-1">
        <p
          v-if="eyebrow"
          class="font-mono text-[11px] uppercase tracking-[0.18em] text-faint-foreground"
        >
          {{ eyebrow }}
        </p>
        <h2
          v-if="title"
          class="font-display text-xl leading-tight text-foreground"
        >
          {{ title }}
        </h2>
        <p
          v-if="description"
          class="max-w-3xl text-sm leading-relaxed text-muted-foreground"
        >
          {{ description }}
        </p>
      </div>
      <div v-if="$slots.actions" class="flex shrink-0 items-center gap-2">
        <slot name="actions" />
      </div>
    </header>

    <slot />
  </component>
</template>

<script setup lang="ts">
import { computed, useSlots } from "vue";

type SurfaceTag = keyof HTMLElementTagNameMap;

const props = withDefaults(
  defineProps<{
    as?: SurfaceTag;
    flush?: boolean;
    subtle?: boolean;
    title?: string;
    eyebrow?: string;
    description?: string;
  }>(),
  {
    as: "section",
    flush: false,
    subtle: false,
  },
);

const slots = useSlots();

const tag = computed(() => props.as);

const hasHeader = computed(() =>
  Boolean(props.eyebrow || props.title || props.description || slots.actions),
);

const classes = computed(() => [
  "halo-surface w-full text-foreground",
  props.subtle ? "halo-surface-2" : "",
  props.flush ? "p-0" : "p-5",
]);
</script>
