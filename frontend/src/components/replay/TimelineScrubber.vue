<template>
  <div class="rounded-xl border border-border bg-surface/70 p-4">
    <div class="mb-2 flex items-center justify-between">
      <button class="rounded bg-white/5 px-3 py-1 text-sm" @click="$emit('prev')">Prev</button>
      <div class="text-sm">{{ index + 1 }} / {{ total }}</div>
      <button class="rounded bg-white/5 px-3 py-1 text-sm" @click="$emit('next')">Next</button>
    </div>
    <input class="w-full" type="range" :min="0" :max="Math.max(total - 1, 0)" :value="index" @input="$emit('set', Number(($event.target as HTMLInputElement).value))" />
  </div>
</template>

<script setup lang="ts">
defineProps<{ index: number; total: number }>();
defineEmits(["prev", "next", "set"]);
</script>
