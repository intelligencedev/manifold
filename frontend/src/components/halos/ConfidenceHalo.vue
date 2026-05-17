<template>
  <div class="inline-flex items-center gap-2">
    <span class="inline-block h-3 w-3 rounded-full bg-accent" :style="{ opacity: String(opacity), transform: `scale(${scale})` }"></span>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ opacity: number; scale: number }>();
</script>
