<template>
  <header :class="wrapperClasses">
    <div class="grid grid-cols-[auto_1fr_auto] items-center gap-3 md:gap-4">
      <div class="flex items-center gap-3 min-w-0">
        <slot name="logo" />
      </div>

      <nav
        class="hidden md:flex items-center justify-center gap-2 text-sm font-medium"
      >
        <slot name="nav" />
      </nav>

      <div class="flex items-center justify-end gap-2 md:gap-3">
        <slot name="meta" />
        <slot name="actions" />
      </div>
    </div>
  </header>
</template>

<script setup lang="ts">
import { computed } from "vue";

type TopbarProps = {
  sticky?: boolean;
};

const props = defineProps<TopbarProps>();

const wrapperClasses = computed(() => [
  "glass-surface border border-border/60",
  "px-4 py-3 md:px-6 md:py-4 rounded-[var(--radius-lg,26px)]",
  "w-full",
  props.sticky === false ? "" : "sticky top-0 z-30",
]);
</script>
