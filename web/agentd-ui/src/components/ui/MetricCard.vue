<template>
  <GlassCard :interactive="interactive" class="h-full">
    <div class="flex items-start justify-between gap-3">
      <div class="space-y-1">
        <p
          class="text-[11px] font-semibold uppercase tracking-wide text-subtle-foreground"
        >
          {{ label }}
        </p>
        <p
          class="text-3xl font-semibold leading-tight text-foreground tabular-nums"
        >
          {{ formattedValue }}
        </p>
        <p v-if="secondary" class="text-xs text-faint-foreground">
          {{ secondary }}
        </p>
      </div>
      <div v-if="$slots.icon" class="text-accent">
        <slot name="icon" />
      </div>
    </div>

    <div
      v-if="deltaText"
      :class="deltaClasses"
      class="mt-4 inline-flex items-center gap-1 rounded-full px-3 py-1 text-xs font-semibold"
    >
      <span v-if="trend === 'up'" aria-hidden="true">▲</span>
      <span v-else-if="trend === 'down'" aria-hidden="true">▼</span>
      <span v-else aria-hidden="true">•</span>
      <span>{{ deltaText }}</span>
    </div>
  </GlassCard>
</template>

<script setup lang="ts">
import { computed, type PropType } from "vue";
import GlassCard from "./GlassCard.vue";

type Trend = "up" | "down" | "flat";

const props = defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], required: true },
  secondary: String,
  delta: [Number, String],
  trend: { type: String as PropType<Trend>, default: undefined },
  interactive: { type: Boolean, default: false },
});

const formattedValue = computed(() =>
  typeof props.value === "number" ? props.value.toLocaleString() : props.value,
);

const resolvedTrend = computed<Trend>(() => {
  if (props.trend) return props.trend;
  if (typeof props.delta === "number") {
    if (props.delta > 0) return "up";
    if (props.delta < 0) return "down";
  }
  return "flat";
});

const deltaText = computed(() => {
  if (props.delta === undefined) return "";
  if (typeof props.delta === "number") {
    const sign = props.delta > 0 ? "+" : props.delta < 0 ? "" : "";
    return `${sign}${props.delta}%`;
  }
  return String(props.delta);
});

const deltaClasses = computed(() => {
  const base = ["border", "bg-surface-muted/60"];
  if (resolvedTrend.value === "up")
    return [...base, "border-success/40 text-success"];
  if (resolvedTrend.value === "down")
    return [...base, "border-danger/40 text-danger"];
  return [...base, "border-border/60 text-subtle-foreground"];
});
</script>
