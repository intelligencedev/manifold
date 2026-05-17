<template>
  <div class="rounded-xl border border-border bg-surface" :class="[noPadding ? '' : 'p-4', $attrs.class]">
    <div v-if="title || $slots.header" class="mb-3 flex items-center justify-between">
      <div>
        <h2 v-if="title" class="text-sm font-semibold text-foreground">{{ title }}</h2>
        <p v-if="description" class="mt-0.5 text-xs text-white/50">{{ description }}</p>
      </div>
      <slot name="header" />
    </div>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ title?: string; description?: string; noPadding?: boolean }>();
defineOptions({ inheritAttrs: false });
</script>
