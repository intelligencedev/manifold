<template>
  <div class="flex flex-col items-center justify-center rounded-xl border border-dashed border-border py-14 text-center">
    <div v-if="icon" class="mb-4 text-4xl" aria-hidden="true">{{ icon }}</div>
    <h3 class="text-sm font-medium text-white/80">{{ title }}</h3>
    <p v-if="description" class="mt-1 text-xs text-white/40">{{ description }}</p>
    <slot class="mt-4" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ icon?: string; title: string; description?: string }>();
</script>
