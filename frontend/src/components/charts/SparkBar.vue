<!-- Pure-CSS horizontal sparkline bar chart -->
<template>
  <div class="space-y-1">
    <div class="flex items-center justify-between text-[10px] text-white/40">
      <span>{{ label }}</span>
      <span class="tabular-nums">{{ formatter ? formatter(total) : total }}</span>
    </div>
    <div class="flex h-6 items-end gap-px overflow-hidden rounded">
      <div
        v-for="(val, i) in normalised"
        :key="i"
        class="flex-1 rounded-sm transition-all duration-300"
        :class="barColor"
        :style="{ height: `${Math.max(val * 100, val > 0 ? 8 : 0)}%` }"
        :title="String(values[i])"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = withDefaults(
  defineProps<{ values: number[]; label?: string; barColor?: string; formatter?: (v: number) => string }>(),
  { barColor: "bg-accent/60" }
);

const total = computed(() => props.values.reduce((a, b) => a + b, 0));

const normalised = computed(() => {
  const max = Math.max(...props.values, 1);
  return props.values.map((v) => v / max);
});
</script>
