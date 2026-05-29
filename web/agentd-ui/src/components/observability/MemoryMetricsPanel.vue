<template>
  <section
    class="flex h-full flex-col overflow-hidden rounded-lg border border-border/70 bg-surface p-6"
  >
    <header class="flex flex-wrap items-start justify-between gap-4 shrink-0">
      <div>
        <h2 class="text-lg font-semibold text-foreground">Evolving Memory Metrics</h2>
        <p class="text-xs text-faint-foreground">
          Search, write, pruning, and size signals from telemetry
        </p>
      </div>
      <div class="flex flex-wrap items-center justify-end gap-3 text-xs">
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
    </header>

    <div class="mt-4 min-h-0 flex-1 overflow-hidden">
      <div
        v-if="isLoading"
        class="rounded-lg border border-border/70 bg-surface-muted/40 p-4 text-sm text-faint-foreground"
      >
        Loading memory metrics...
      </div>
      <div
        v-else-if="isError"
        class="rounded-lg border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
      >
        Failed to load memory metrics.
      </div>
      <div v-else class="flex h-full flex-col gap-4 overflow-hidden">
        <div class="grid shrink-0 grid-cols-2 gap-3 xl:grid-cols-4">
          <div
            v-for="metric in kpis"
            :key="metric.label"
            class="rounded-lg border border-border/60 bg-muted/10 px-3 py-2"
          >
            <p class="text-[10px] font-semibold uppercase tracking-wide text-faint-foreground">
              {{ metric.label }}
            </p>
            <p class="mt-1 text-xl font-semibold leading-none text-foreground tabular-nums">
              {{ metric.value }}
            </p>
            <p class="mt-1 truncate text-[11px] text-faint-foreground">
              {{ metric.detail }}
            </p>
          </div>
        </div>

        <div
          v-if="data?.source === 'none'"
          class="rounded-lg border border-dashed border-border bg-surface-muted/40 p-4 text-sm text-faint-foreground"
        >
          Memory telemetry is not connected to a queryable metrics backend yet.
        </div>
        <div v-else-if="hasNoData" class="rounded-lg border border-dashed border-border bg-surface-muted/40 p-4 text-sm text-faint-foreground">
          No evolving memory telemetry was recorded in this window.
        </div>

        <div class="grid min-h-0 flex-1 gap-4 overflow-hidden xl:grid-cols-2">
          <div class="flex min-h-0 flex-col gap-3 overflow-hidden">
            <MetricBreakdown
              title="Writes by result"
              empty-label="No writes recorded"
              :rows="evolveRows"
            />
            <MetricBreakdown
              title="Pruned by reason"
              empty-label="No pruning recorded"
              :rows="pruneRows"
            />
          </div>

          <div class="flex min-h-0 flex-col overflow-hidden rounded-lg border border-border/60 bg-muted/10 p-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h3 class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground">
                  Largest Sessions
                </h3>
                <p class="text-[11px] text-faint-foreground">Latest reported memory size</p>
              </div>
              <span class="text-[11px] text-faint-foreground">{{ data?.source || "none" }}</span>
            </div>
            <div v-if="!sizeRows.length" class="mt-4 text-sm text-faint-foreground">
              No size gauges in this window.
            </div>
            <div v-else class="mt-4 min-h-0 flex-1 space-y-3 overflow-y-auto pr-1">
              <div v-for="row in sizeRows" :key="`${row.user}:${row.session}`">
                <div class="flex items-center justify-between gap-3 text-xs">
                  <span class="truncate font-medium text-foreground">{{ row.label }}</span>
                  <span class="shrink-0 tabular-nums text-faint-foreground">{{ formatNumber(row.size) }}</span>
                </div>
                <div class="mt-1 h-2 rounded-full bg-border/40">
                  <div class="h-full rounded-full bg-emerald-500" :style="{ width: row.width }"></div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <p v-if="data?.warnings?.length" class="shrink-0 truncate text-[11px] text-faint-foreground">
          {{ data.warnings[0] }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, type PropType } from "vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import {
  TOKEN_METRIC_TIME_RANGES,
  type MetricsTimeRangeValue,
} from "@/composables/observability/useTokenMetrics";
import {
  useMemoryMetrics,
  type MemoryBreakdownRow,
} from "@/composables/observability/useMemoryMetrics";

const selectedRange = ref<MetricsTimeRangeValue>("24h");

const timeRangeDropdownOptions = TOKEN_METRIC_TIME_RANGES.map((option) => ({
  id: option.value,
  label: option.label,
  value: option.value,
}));

const {
  data,
  isLoading,
  isError,
  totals,
  latencyAvgMs,
  evolveErrorRate,
  sizeRows,
  pruneRows,
  evolveRows,
  formatNumber,
  formatDecimal,
} = useMemoryMetrics(selectedRange);

const hasNoData = computed(
  () =>
    totals.value.searches === 0 &&
    totals.value.evolves === 0 &&
    totals.value.pruned === 0 &&
    totals.value.smartMerges === 0 &&
    sizeRows.value.length === 0,
);

const sourceLabel = computed(() => formatSource(data.value?.source));
const sourceTitle = computed(() => sourceTooltip(data.value?.source));

const kpis = computed(() => [
  {
    label: "Searches",
    value: formatNumber(totals.value.searches),
    detail: `${formatDecimal(totals.value.avgHitsPerSearch)} hits/search`,
  },
  {
    label: "Latency",
    value: `${formatDecimal(latencyAvgMs.value)} ms`,
    detail: "Average search time",
  },
  {
    label: "Writes",
    value: formatNumber(totals.value.evolves),
    detail: `${formatDecimal(evolveErrorRate.value)}% errors`,
  },
  {
    label: "Maintenance",
    value: formatNumber(totals.value.pruned + totals.value.smartMerges),
    detail: `${formatNumber(totals.value.pruned)} pruned · ${formatNumber(totals.value.smartMerges)} merged`,
  },
]);

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

const MetricBreakdown = defineComponent({
  name: "MetricBreakdown",
  props: {
    title: { type: String, required: true },
    emptyLabel: { type: String, required: true },
    rows: { type: Array as PropType<MemoryBreakdownRow[]>, required: true },
  },
  setup(props) {
    return () =>
      h(
        "div",
        { class: "rounded-lg border border-border/60 bg-muted/10 p-4" },
        [
          h(
            "h3",
            { class: "font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground" },
            props.title,
          ),
          props.rows.length === 0
            ? h("p", { class: "mt-4 text-sm text-faint-foreground" }, props.emptyLabel)
            : h(
                "div",
                { class: "mt-4 space-y-3" },
                props.rows.map((row) =>
                  h("div", { key: row.key }, [
                    h("div", { class: "flex items-center justify-between gap-3 text-xs" }, [
                      h("span", { class: "truncate font-medium text-foreground" }, row.label),
                      h("span", { class: "shrink-0 tabular-nums text-faint-foreground" }, formatNumber(row.value)),
                    ]),
                    h("div", { class: "mt-1 h-2 rounded-full bg-border/40" }, [
                      h("div", {
                        class: "h-full rounded-full bg-sky-500",
                        style: { width: row.width },
                      }),
                    ]),
                  ]),
                ),
              ),
        ],
      );
  },
});
</script>