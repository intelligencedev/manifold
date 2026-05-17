<template>
  <div class="rounded-2xl border border-border bg-surface/70 p-4">
    <h2 class="mb-3 text-lg font-semibold">Memory Garden</h2>
    <div class="grid gap-3 md:grid-cols-3">
      <div class="rounded-xl border border-border/80 bg-black/10 p-4">
        <div class="text-sm font-medium">Sessions</div>
        <div class="mt-2 text-3xl font-semibold">{{ sessions }}</div>
      </div>
      <div class="rounded-xl border border-border/80 bg-black/10 p-4">
        <div class="text-sm font-medium">Beliefs</div>
        <div class="mt-2 text-3xl font-semibold">{{ beliefs }}</div>
      </div>
      <div class="rounded-xl border border-border/80 bg-black/10 p-4">
        <div class="text-sm font-medium">Transit records</div>
        <div class="mt-2 text-3xl font-semibold">{{ transit }}</div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ sessions: number; beliefs: number; transit: number }>();
</script>
