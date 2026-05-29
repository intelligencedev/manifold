<template>
  <div
    class="min-w-0 flex-1 border-l border-[rgb(var(--color-border))] px-[22px] py-1 first:border-l-0"
  >
    <p
      class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
    >
      {{ k }}
    </p>
    <p
      :class="[
        'mt-1 flex items-baseline gap-1 font-display text-[34px] font-semibold leading-none tracking-[-0.02em] tabular-nums',
        data ? 'text-[rgb(var(--data))]' : 'text-foreground',
      ]"
    >
      <span>{{ formattedValue }}</span>
      <span v-if="unit" class="text-sm text-muted-foreground">{{ unit }}</span>
    </p>
    <p
      v-if="trend"
      :class="[
        'mt-2 font-mono text-[11px] uppercase tracking-[0.08em]',
        trend === 'up'
          ? 'text-success'
          : trend === 'down'
            ? 'text-danger'
            : 'text-faint-foreground',
      ]"
    >
      <span aria-hidden="true">{{ trendGlyph }}</span>
      {{ trend }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{
    k: string;
    v: string | number;
    unit?: string;
    trend?: "up" | "down" | "flat";
    data?: boolean;
  }>(),
  {
    data: false,
  },
);

const formattedValue = computed(() =>
  typeof props.v === "number" ? props.v.toLocaleString() : props.v,
);

const trendGlyph = computed(() => {
  if (props.trend === "up") return "▲";
  if (props.trend === "down") return "▼";
  return "•";
});
</script>
