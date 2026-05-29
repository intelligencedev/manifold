<template>
  <div
    class="rounded-lg border border-border/70 bg-surface p-6 flex h-full flex-col overflow-hidden"
  >
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-foreground">Traces</h2>
        <p class="text-xs text-faint-foreground">
          Recent LLM spans in the selected window
        </p>
      </div>
      <div
        class="flex flex-wrap items-center justify-end gap-3 text-xs text-faint-foreground"
      >
        <label class="flex items-center gap-2 text-foreground">
          <span>Time Range</span>
          <DropdownSelect
            v-model="selectedRange"
            size="sm"
            class="text-xs"
            :options="timeRangeDropdownOptions"
          />
        </label>
        <span
          class="rounded-full border border-border/70 bg-muted/20 px-2 py-1 text-[11px] font-medium font-mono uppercase tracking-[0.12em] text-faint-foreground"
          :title="sourceTitle"
        >
          {{ sourceLabel }}
        </span>
      </div>
    </div>

    <div class="mt-4 flex-1 overflow-hidden">
      <div
        v-if="tracesLoading"
        class="rounded-lg border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
      >
        Loading traces…
      </div>
      <div
        v-else-if="tracesError"
        class="rounded-lg border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
      >
        Failed to load traces.
      </div>
      <div v-else class="flex h-full flex-col">
        <div
          v-if="!traceRows.length"
          class="rounded-lg border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
        >
          No traces recorded in the selected window.
        </div>
        <div v-else class="flex-1 space-y-3 overflow-y-auto pr-1">
          <div
            v-for="trace in traceRows"
            :key="trace.key"
            class="rounded-lg border border-border/60 bg-muted/10 p-4"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="flex items-start gap-3">
                <span
                  :class="[
                    'mt-1 h-2.5 w-2.5 rounded-full',
                    trace.status === 'error'
                      ? 'bg-danger-foreground'
                      : 'bg-emerald-400',
                  ]"
                ></span>
                <div>
                  <p class="text-sm font-semibold text-foreground">
                    {{ trace.name }}
                  </p>
                  <p class="text-xs text-faint-foreground">
                    {{ trace.modelLabel }} · {{ trace.timeLabel }}
                  </p>
                </div>
              </div>
              <div class="text-right">
                <p class="text-sm font-semibold text-foreground">
                  {{ trace.durationLabel }}
                </p>
                <p class="text-xs text-faint-foreground">
                  {{ trace.tokenLabel }}
                </p>
              </div>
            </div>
          </div>
        </div>
        <p class="mt-3 text-xs text-faint-foreground">
          Showing up to {{ traceRows.length }} trace<span
            v-if="traceRows.length !== 1"
            >s</span
          >
          for this window.
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import {
  TOKEN_METRIC_TIME_RANGES,
  type MetricsTimeRangeValue,
} from "@/composables/observability/useTokenMetrics";
import { useTraceMetrics } from "@/composables/observability/useTraceMetrics";
import DropdownSelect from "@/components/DropdownSelect.vue";

const selectedRange = ref<MetricsTimeRangeValue>("24h");

const timeRangeDropdownOptions = TOKEN_METRIC_TIME_RANGES.map((option) => ({
  id: option.value,
  label: option.label,
  value: option.value,
}));

const {
  data,
  isLoading: tracesLoading,
  isError: tracesError,
  traceRows,
} = useTraceMetrics(selectedRange);

const sourceLabel = computed(() => formatSource(data.value?.source));
const sourceTitle = computed(() => sourceTooltip(data.value?.source));

function formatSource(source?: string) {
  if (source === "clickhouse") return "ClickHouse";
  if (source === "process") return "Local";
  return "Disabled";
}

function sourceTooltip(source?: string) {
  if (source === "clickhouse") return "Persistent telemetry from ClickHouse.";
  if (source === "process") return "Bounded process-local telemetry. Resets when agentd restarts.";
  return "No telemetry provider is enabled.";
}
</script>
