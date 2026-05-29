<template>
  <header
    class="halo-hairline-b flex h-[52px] items-center gap-3.5 px-[18px]"
  >
    <div class="min-w-0">
      <h1 class="truncate font-display text-[18px] leading-none text-foreground">
        {{ title }}
      </h1>
      <p
        v-if="crumb"
        class="mt-1 truncate font-mono text-[10px] uppercase tracking-[0.14em] text-faint-foreground"
      >
        {{ crumb }}
      </p>
    </div>
    <div class="min-w-0 flex-1"></div>
    <div v-if="$slots.actions" class="flex items-center gap-2">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
defineProps<{
  title: string;
  crumb?: string;
}>();
</script>
