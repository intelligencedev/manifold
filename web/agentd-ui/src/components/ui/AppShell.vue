<template>
  <div
    class="halo grid h-screen min-w-0 overflow-hidden bg-background text-foreground"
    :style="{ gridTemplateColumns, gridTemplateRows: '52px minmax(0, 1fr)' }"
  >
    <div class="col-start-1 row-span-2 row-start-1 min-h-0">
      <slot name="rail" />
    </div>

    <div class="col-start-2 row-start-1 min-w-0">
      <slot name="topbar" />
    </div>

    <main class="col-start-2 row-start-2 min-h-0 min-w-0 overflow-auto p-2.5">
      <slot />
    </main>

    <aside
      v-if="inspector"
      class="col-start-3 row-span-2 row-start-1 min-h-0 min-w-0"
    >
      <slot name="inspector" />
    </aside>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    inspector?: boolean;
    sidebarCollapsed?: boolean;
  }>(),
  {
    inspector: false,
    sidebarCollapsed: false,
  },
);

const gridTemplateColumns = computed(() =>
  props.inspector
    ? `${props.sidebarCollapsed ? "52px" : "232px"} minmax(0, 1fr) 320px`
    : `${props.sidebarCollapsed ? "52px" : "232px"} minmax(0, 1fr)`,
);
</script>
