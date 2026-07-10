<script setup lang="ts">
import { computed } from "vue";
import type {
  ChatContextMetricSegment,
  ChatContextMetricSegmentKind,
  ChatContextMetrics,
} from "@/types/chat";

const props = defineProps<{
  metrics?: ChatContextMetrics;
  variant?: "message" | "session";
}>();

const segmentLabels: Record<ChatContextMetricSegmentKind, string> = {
  system: "System",
  history: "History",
  user: "User",
  memory: "Memory",
  tools: "Tools",
  summary: "Summary",
  assistant: "Response",
};

const hasMetrics = computed(() => Boolean(props.metrics));
const isSessionGauge = computed(() => props.variant === "session");
const variantClass = computed(() =>
  isSessionGauge.value ? "token-gauge--session" : "token-gauge--message",
);

const displaySegments = computed(() => {
  const metrics = props.metrics;
  if (!metrics?.inputTokens) return [];
  return metrics.segments
    .filter((segment) => segment.tokens > 0)
    .map((segment) => ({
      ...segment,
      label: segmentLabels[segment.kind],
      height: `${Math.max(1.5, (segment.tokens / metrics.inputTokens) * 100)}%`,
    }));
});

const fillPercent = computed(() => {
  const metrics = props.metrics;
  if (!metrics?.summaryThreshold) return 0;
  return Math.min(100, (metrics.inputTokens / metrics.summaryThreshold) * 100);
});

const thresholdPercent = computed(() => {
  return 100;
});

const currentLabelPercent = computed(() => {
  if (!props.metrics?.inputTokens) return 0;
  return Math.min(96, Math.max(4, fillPercent.value));
});

const legendItems = computed(() => {
  const active = new Set(displaySegments.value.map((segment) => segment.kind));
  const kinds: ChatContextMetricSegmentKind[] = [
    "system",
    "history",
    "user",
    "memory",
    "tools",
    "assistant",
    "summary",
  ];
  return kinds
    .filter((kind) => active.has(kind) || kind === "memory" || kind === "tools")
    .map((kind) => ({ kind, label: segmentLabels[kind] }));
});

const gaugeStyle = computed(() => {
  if (!isSessionGauge.value) return {};
  const rows = Math.max(1, legendItems.value.length);
  const rowHeight = 0.625;
  const rowGap = 0.22;
  const legendGap = 0.65;
  const composerClearance = 8.75;
  const legendHeight = rows * rowHeight + Math.max(0, rows - 1) * rowGap;
  return {
    "--token-gauge-bottom-reserve": `${(
      composerClearance +
      legendGap +
      legendHeight
    ).toFixed(2)}rem`,
  };
});

const statusClass = computed(() => {
  const metrics = props.metrics;
  if (!metrics?.summaryThreshold) return "token-gauge--empty";
  const ratio = metrics.inputTokens / metrics.summaryThreshold;
  if (metrics.willSummarize || ratio >= 1) return "token-gauge--danger";
  if (ratio >= 0.85) return "token-gauge--warning";
  return "";
});

const title = computed(() => {
  const metrics = props.metrics;
  if (!metrics) return "No context token metrics yet";
  const lines = [
    `${formatNumber(metrics.inputTokens)} / ${formatNumber(metrics.summaryThreshold)} summary budget tokens`,
  ];
  if (metrics.phase !== "summary_triggered") {
    lines.push(`Context: ${formatNumber(metrics.contextWindow)}`);
    lines.push(`Reserve: ${formatNumber(metrics.reserveTokens)}`);
  }
  if (metrics.phase === "client_estimate") {
    lines.push("Source: local estimate");
  }
  if (metrics.phase === "summary_triggered") {
    lines.push("Summary/compaction threshold reached");
  }
  if (metrics.summarizedCount) {
    lines.push(`Summarized messages: ${formatNumber(metrics.summarizedCount)}`);
  }
  for (const segment of metrics.segments) {
    lines.push(
      `${segmentLabels[segment.kind]}: ${formatNumber(segment.tokens)}`,
    );
  }
  return lines.join("\n");
});

function formatNumber(value?: number) {
  return typeof value === "number" && Number.isFinite(value)
    ? value.toLocaleString()
    : "0";
}

function formatShortNumber(value?: number) {
  if (typeof value !== "number" || !Number.isFinite(value)) return "0";
  if (Math.abs(value) >= 1_000_000) return `${trimFixed(value / 1_000_000)}M`;
  if (Math.abs(value) >= 1_000) return `${trimFixed(value / 1_000)}k`;
  return value.toLocaleString();
}

function trimFixed(value: number) {
  return value.toFixed(value >= 10 ? 0 : 1).replace(/\.0$/, "");
}
</script>

<template>
  <div
    class="token-gauge"
    :class="[statusClass, variantClass]"
    :style="gaugeStyle"
    :title="title"
    role="meter"
    :aria-valuenow="metrics?.inputTokens ?? 0"
    :aria-valuemin="0"
    :aria-valuemax="metrics?.summaryThreshold ?? 1"
    aria-label="Context token usage"
  >
    <div v-if="isSessionGauge && metrics" class="token-gauge__labels">
      <span class="token-gauge__label token-gauge__label--budget">
        <span class="token-gauge__label-text">
          {{ formatShortNumber(metrics.summaryThreshold) }}
        </span>
        <span class="token-gauge__label-line"></span>
      </span>
      <span
        class="token-gauge__label token-gauge__label--current"
        :style="{ bottom: `${currentLabelPercent}%` }"
      >
        <span class="token-gauge__label-text">
          {{ formatShortNumber(metrics.inputTokens) }}
        </span>
        <span class="token-gauge__label-line"></span>
      </span>
    </div>
    <div class="token-gauge__track" :data-has-metrics="hasMetrics">
      <span
        v-for="tick in [25, 50, 75]"
        :key="tick"
        class="token-gauge__tick"
        :style="{ bottom: `${tick}%` }"
      ></span>
      <span
        class="token-gauge__threshold"
        :style="{ bottom: `${thresholdPercent}%` }"
      ></span>
      <div class="token-gauge__fill" :style="{ height: `${fillPercent}%` }">
        <span
          v-for="segment in displaySegments"
          :key="`${segment.kind}:${segment.tokens}`"
          class="token-gauge__segment"
          :class="`token-gauge__segment--${segment.kind}`"
          :style="{ height: segment.height }"
          :aria-label="`${segment.label}: ${formatNumber(segment.tokens)} tokens`"
        ></span>
      </div>
    </div>
    <div v-if="isSessionGauge && metrics" class="token-gauge__legend">
      <span
        v-for="item in legendItems"
        :key="item.kind"
        class="token-gauge__legend-item"
      >
        <span
          class="token-gauge__legend-swatch"
          :class="`token-gauge__legend-swatch--${item.kind}`"
        ></span>
        <span>{{ item.label }}</span>
      </span>
    </div>
  </div>
</template>

<style scoped>
.token-gauge {
  z-index: 2;
  width: 0.62rem;
  min-height: 2.8rem;
  pointer-events: auto;
}

.token-gauge--message {
  position: absolute;
  top: 0.35rem;
  bottom: 0.35rem;
  left: -0.95rem;
}

.token-gauge--session {
  position: absolute;
  top: 6.25rem;
  bottom: var(--token-gauge-bottom-reserve, 11rem);
  left: 0.55rem;
  z-index: 8;
  height: auto;
  min-height: 10rem;
  margin: 0;
  width: 4.5rem;
}

.token-gauge--session .token-gauge__track {
  width: 0.72rem;
  margin-left: 0;
}

.token-gauge__track {
  position: relative;
  height: 100%;
  width: 100%;
  overflow: hidden;
  border: 1px solid rgb(var(--color-border) / 0.68);
  border-radius: 999px;
  background:
    linear-gradient(
      180deg,
      rgb(var(--color-surface) / 0.88),
      rgb(var(--color-surface-muted) / 0.72)
    ),
    rgb(var(--color-surface-muted));
  box-shadow:
    inset 0 0 0 1px rgb(255 255 255 / 0.025),
    0 6px 14px rgb(0 0 0 / 0.18);
}

.token-gauge__fill {
  position: absolute;
  right: 1px;
  bottom: 1px;
  left: 1px;
  display: flex;
  min-height: 2px;
  flex-direction: column-reverse;
  overflow: hidden;
  border-radius: 999px;
  transition: height 220ms ease;
}

.token-gauge__segment {
  display: block;
  min-height: 1px;
  opacity: 0.92;
  transition: height 220ms ease;
}

.token-gauge__segment--system {
  background: rgb(var(--color-subtle-foreground) / 0.72);
}

.token-gauge__segment--history {
  background: rgb(88 166 255 / 0.82);
}

.token-gauge__segment--summary {
  background: repeating-linear-gradient(
    135deg,
    rgb(220 226 238 / 0.92) 0 2px,
    rgb(130 143 164 / 0.9) 2px 4px
  );
}

.token-gauge__segment--user {
  background: rgb(71 199 132 / 0.92);
}

.token-gauge__segment--memory {
  background: rgb(245 177 66 / 0.94);
}

.token-gauge__segment--tools {
  background: rgb(168 130 255 / 0.92);
}

.token-gauge__segment--assistant {
  background: rgb(75 214 230 / 0.9);
}

.token-gauge__tick,
.token-gauge__threshold {
  position: absolute;
  left: 50%;
  z-index: 3;
  height: 1px;
  transform: translateX(-50%);
  pointer-events: none;
}

.token-gauge__tick {
  width: 45%;
  background: rgb(var(--color-foreground) / 0.28);
}

.token-gauge__threshold {
  width: 92%;
  background: rgb(var(--color-warning) / 0.95);
  box-shadow: 0 0 7px rgb(var(--color-warning) / 0.55);
}

.token-gauge__labels {
  position: absolute;
  inset: 0 0 0 auto;
  width: 100%;
  pointer-events: none;
}

.token-gauge__label {
  position: absolute;
  left: 0.72rem;
  display: inline-flex;
  flex-direction: row-reverse;
  align-items: center;
  transform: translateY(50%);
  font-family:
    ui-monospace, SFMono-Regular, "SF Mono", Consolas, "Liberation Mono",
    monospace;
  font-size: 0.625rem;
  font-weight: 600;
  line-height: 1;
  color: rgb(var(--color-foreground) / 0.86);
  white-space: nowrap;
}

.token-gauge__label--budget {
  top: 0;
  transform: translateY(-15%);
  color: rgb(var(--color-warning) / 0.98);
}

.token-gauge__label-text {
  display: inline-block;
  min-width: 1.85rem;
  padding: 0;
  text-align: left;
}

.token-gauge__label-line {
  display: block;
  width: 0.42rem;
  height: 1px;
  background: currentColor;
  opacity: 0.72;
}

.token-gauge__legend {
  position: absolute;
  top: calc(100% + 0.65rem);
  bottom: auto;
  left: 0.15rem;
  display: grid;
  grid-template-columns: 1fr;
  gap: 0.22rem;
  width: 3.45rem;
  pointer-events: none;
  color: rgb(var(--color-subtle-foreground) / 0.88);
  font-size: 0.625rem;
  line-height: 1;
}

.token-gauge__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 0.28rem;
  min-width: 0;
}

.token-gauge__legend-swatch {
  width: 0.42rem;
  height: 0.42rem;
  flex: 0 0 auto;
  border-radius: 999px;
  box-shadow: 0 0 0 1px rgb(var(--color-border) / 0.42);
}

.token-gauge__legend-swatch--system {
  background: rgb(var(--color-subtle-foreground) / 0.72);
}

.token-gauge__legend-swatch--history {
  background: rgb(88 166 255 / 0.82);
}

.token-gauge__legend-swatch--summary {
  background: repeating-linear-gradient(
    135deg,
    rgb(220 226 238 / 0.92) 0 2px,
    rgb(130 143 164 / 0.9) 2px 4px
  );
}

.token-gauge__legend-swatch--user {
  background: rgb(71 199 132 / 0.92);
}

.token-gauge__legend-swatch--memory {
  background: rgb(245 177 66 / 0.94);
}

.token-gauge__legend-swatch--tools {
  background: rgb(168 130 255 / 0.92);
}

.token-gauge__legend-swatch--assistant {
  background: rgb(75 214 230 / 0.9);
}

.token-gauge--warning .token-gauge__track {
  border-color: rgb(var(--color-warning) / 0.72);
}

.token-gauge--empty .token-gauge__track {
  border-style: dashed;
  border-color: rgb(var(--color-border) / 0.72);
  opacity: 0.64;
  box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.02);
}

.token-gauge--empty .token-gauge__fill {
  display: none;
}

.token-gauge--empty .token-gauge__threshold {
  background: rgb(var(--color-foreground) / 0.32);
  box-shadow: none;
}

.token-gauge--danger .token-gauge__track {
  border-color: rgb(var(--color-danger) / 0.78);
  box-shadow:
    inset 0 0 0 1px rgb(var(--color-danger) / 0.14),
    0 0 16px rgb(var(--color-danger) / 0.2);
  animation: tokenGaugePulse 1.4s ease-in-out infinite;
}

@keyframes tokenGaugePulse {
  0%,
  100% {
    filter: saturate(1);
  }
  50% {
    filter: saturate(1.45);
  }
}
</style>
