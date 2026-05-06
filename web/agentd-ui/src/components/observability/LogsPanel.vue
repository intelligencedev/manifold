<template>
  <div
    class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg flex h-full flex-col overflow-hidden"
  >
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-foreground">Logs</h2>
        <p class="text-xs text-faint-foreground">
          Recent application logs shipped via OpenTelemetry.
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
        <label class="flex items-center gap-2 text-foreground">
          <span>Level</span>
          <DropdownSelect
            v-model="selectedLevel"
            size="sm"
            class="text-xs"
            :options="levelDropdownOptions"
          />
        </label>
        <span
          class="rounded-full border border-border/70 bg-muted/20 px-2 py-1 text-[11px] font-medium uppercase tracking-wide text-subtle-foreground"
          :title="sourceTitle"
        >
          {{ sourceLabel }}
        </span>
      </div>
    </div>

    <div class="mt-4 flex-1 overflow-hidden">
      <div
        v-if="logsLoading"
        class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
      >
        Loading logs…
      </div>
      <div
        v-else-if="logsError"
        class="rounded-2xl border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
      >
        Failed to load logs.
      </div>
        <div v-else class="flex h-full flex-col">
        <div
          v-if="!filteredLogs.length"
          class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
        >
          No logs recorded in the selected window.
        </div>
        <div v-else class="flex-1 overflow-y-auto pr-1">
          <div class="space-y-1">
            <button
              v-for="(log, index) in filteredLogs"
              :key="log.key || index"
              type="button"
              :class="[
                'flex w-full items-start gap-3 rounded-xl px-2 py-2 text-left leading-relaxed transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/60 hover:bg-muted/10',
                isSelected(log) ? 'bg-muted/20 ring-1 ring-border/80' : '',
                getLogLevelClass(log.level),
              ]"
              @click="selectLog(log)"
            >
              <span class="text-faint-foreground shrink-0">
                {{ formatTimestamp(log.timestamp) }}
              </span>
              <span
                class="font-semibold shrink-0 uppercase"
                :class="getLevelColorClass(log.level)"
              >
                {{ log.level || "info" }}
              </span>
              <span class="min-w-0 flex-1 text-foreground break-all">{{ log.message }}</span>
              <span
                v-if="log.service"
                class="shrink-0 rounded-full border border-border/70 bg-muted/20 px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide text-subtle-foreground"
              >
                {{ log.service }}
              </span>
            </button>
          </div>
        </div>
        <p class="mt-3 text-xs text-faint-foreground">
          Showing {{ filteredLogs.length }} log<span
            v-if="filteredLogs.length !== 1"
            >s</span
          >
          in this window.
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import {
  TOKEN_METRIC_TIME_RANGES,
  type MetricsTimeRangeValue,
} from "@/composables/observability/useTokenMetrics";
import {
  useLogMetrics,
  type LogDisplayRow,
} from "@/composables/observability/useLogMetrics";

const props = defineProps<{
  selectedLogId?: string | null;
}>();

const emit = defineEmits<{
  selectLog: [payload: { id: string; window: MetricsTimeRangeValue }];
}>();

const selectedRange = ref<MetricsTimeRangeValue>("1h");
const selectedLevel = ref("all");

const timeRangeDropdownOptions = TOKEN_METRIC_TIME_RANGES.map((option) => ({
  id: option.value,
  label: option.label,
  value: option.value,
}));

const levelDropdownOptions = [
  { id: "all", label: "All", value: "all" },
  { id: "error", label: "Error", value: "error" },
  { id: "warn", label: "Warn", value: "warn" },
  { id: "info", label: "Info", value: "info" },
  { id: "debug", label: "Debug", value: "debug" },
];

const {
  data,
  isLoading: logsLoading,
  isError: logsError,
  logRows,
} = useLogMetrics(selectedRange);

const sourceLabel = computed(() => formatSource(data.value?.source));
const sourceTitle = computed(() => sourceTooltip(data.value?.source));

const filteredLogs = computed(() => {
  if (selectedLevel.value === "all") return logRows.value;
  return logRows.value.filter(
    (log) => normalizeLevel(log.level) === selectedLevel.value,
  );
});

function selectLog(log: LogDisplayRow) {
  if (!log.id) return;
  emit("selectLog", { id: log.id, window: selectedRange.value });
}

function isSelected(log: LogDisplayRow) {
  return Boolean(props.selectedLogId) && log.id === props.selectedLogId;
}

function getLogLevelClass(level: string) {
  const normalized = normalizeLevel(level);
  if (normalized === "error" || normalized === "fatal") return "bg-danger/5";
  if (normalized === "warn" || normalized === "warning") return "bg-warning/5";
  if (normalized === "info") return "bg-info/5";
  return "";
}

function getLevelColorClass(level: string) {
  const normalized = normalizeLevel(level);
  if (normalized === "error" || normalized === "fatal") return "text-danger";
  if (normalized === "warn" || normalized === "warning") return "text-warning";
  if (normalized === "info") return "text-info";
  if (normalized === "debug") return "text-subtle-foreground";
  return "text-foreground";
}

function formatTimestamp(timestamp: string | number): string {
  const date = toDate(timestamp);
  return date.toLocaleTimeString("en-US", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function normalizeLevel(level: string | undefined) {
  return (level || "info").toLowerCase();
}

function toDate(value: string | number) {
  if (typeof value === "number") {
    const ms = value < 1_000_000_000_000 ? value * 1000 : value;
    return new Date(ms);
  }
  const parsed = Date.parse(value);
  if (!Number.isNaN(parsed)) return new Date(parsed);
  return new Date();
}

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
