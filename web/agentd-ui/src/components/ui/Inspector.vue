<template>
  <aside
    v-if="open"
    class="halo-rail halo-hairline-l h-full w-[360px] overflow-auto p-[18px]"
  >
    <header
      v-if="title || $slots.header"
      class="mb-5 flex items-start justify-between gap-3"
    >
      <slot name="header">
        <h2 class="font-display text-xl leading-tight text-foreground">
          {{ title }}
        </h2>
      </slot>
    </header>
    <slot />
  </aside>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string;
    open?: boolean;
  }>(),
  {
    open: true,
  },
);
</script>
