<template>
  <div v-if="message" role="alert" class="flex items-start gap-3 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3">
    <span class="mt-0.5 text-red-400" aria-hidden="true">⚠</span>
    <div class="flex-1 text-sm text-red-300">{{ message }}</div>
    <button class="ml-auto shrink-0 text-red-400/60 hover:text-red-300" @click="$emit('dismiss')" aria-label="Dismiss">✕</button>
  </div>
</template>

<script setup lang="ts">
defineProps<{ message?: string }>();
defineEmits(["dismiss"]);
</script>
