<template>
  <div
    class="flex min-w-0 flex-col gap-3 border-t border-border/50 py-5 first:border-t-0 first:pt-0 last:pb-0"
  >
    <div
      class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-baseline gap-x-3 gap-y-1"
    >
      <h3 class="shrink-0 text-sm font-semibold text-foreground">
        {{ title }}
      </h3>
      <p
        v-if="helper"
        class="min-w-0 break-words text-xs text-subtle-foreground"
      >
        {{ helper }}
      </p>
    </div>
    <div class="min-w-0">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
defineProps<{ title: string; helper?: string }>();
</script>
