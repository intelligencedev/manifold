<template>
  <header
    class="halo-hairline-b topbar-surface flex h-[52px] items-center gap-3 px-4"
  >
    <button
      type="button"
      class="halo-focus inline-grid h-7 w-7 place-items-center rounded border border-transparent text-subtle-foreground hover:border-border hover:bg-surface-muted hover:text-foreground"
      :aria-label="sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'"
      @click="$emit('toggle-sidebar')"
    >
      <svg
        viewBox="0 0 20 20"
        class="h-3.5 w-3.5"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
      >
        <path :d="sidebarCollapsed ? 'm7 5 5 5-5 5' : 'm12 5-5 5 5 5'" />
      </svg>
    </button>
    <div class="min-w-0">
      <h1
        class="truncate text-[13px] font-semibold leading-none text-foreground"
      >
        {{ title }}
      </h1>
      <p
        v-if="crumb"
        class="mt-1 truncate font-mono text-[9px] uppercase tracking-[0.14em] text-faint-foreground"
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
  sidebarCollapsed?: boolean;
}>();
defineEmits<{ "toggle-sidebar": [] }>();
</script>

<style scoped>
.topbar-surface {
  background: rgb(var(--color-background));
}
</style>
