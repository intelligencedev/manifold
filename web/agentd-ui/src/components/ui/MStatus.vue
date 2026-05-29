<template>
  <span class="inline-flex items-center gap-2">
    <span
      :class="[
        'h-[7px] w-[7px] rounded-full',
        dotClass,
        pulse && state === 'run' ? 'halo-pulse' : '',
      ]"
      aria-hidden="true"
    ></span>
    <span
      v-if="label"
      class="font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground"
    >
      {{ label }}
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    state: "run" | "ok" | "warn" | "danger" | "idle";
    label?: string;
    pulse?: boolean;
  }>(),
  {
    pulse: false,
  },
);

const dotClass = computed(() => {
  if (props.state === "run") return "bg-accent";
  if (props.state === "ok") return "bg-success";
  if (props.state === "warn") return "bg-warning";
  if (props.state === "danger") return "bg-danger";
  return "bg-faint-foreground";
});
</script>
