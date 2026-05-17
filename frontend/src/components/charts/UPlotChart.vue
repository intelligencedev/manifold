<template>
  <div class="rounded-xl border border-border bg-surface/80 p-4">
    <div class="mb-2 text-sm font-semibold">{{ title }}</div>
    <div class="text-4xl font-semibold">{{ value }}</div>
    <div class="mt-2 text-xs text-white/60">{{ subtitle }}</div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string; value: string | number; subtitle?: string }>();
</script>
