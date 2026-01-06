<template>
  <section
    class="flex min-h-0 flex-col rounded-[14px] border border-border/60 bg-surface p-4"
  >
    <div class="shrink-0">
      <h3 class="text-sm font-semibold text-foreground">{{ title }}</h3>
      <p v-if="helper" class="text-xs text-subtle-foreground">{{ helper }}</p>
    </div>
    <div class="mt-3 flex min-h-0 flex-1 flex-col">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
defineProps<{ title: string; helper?: string }>();
</script>
