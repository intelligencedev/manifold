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
            <div
              v-for="(log, index) in filteredLogs"
              :key="log.key || index"
              :class="[
                'flex gap-3 leading-relaxed hover:bg-muted/10 px-2 py-1 rounded',
                getLogLevelClass(log.level),
              ]"
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
              <span class="text-foreground break-all">{{ log.message }}</span>
            </div>
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
import { useLogMetrics } from "@/composables/observability/useLogMetrics";

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
  isLoading: logsLoading,
  isError: logsError,
  logRows,
} = useLogMetrics(selectedRange);

const filteredLogs = computed(() => {
  if (selectedLevel.value === "all") return logRows.value;
  return logRows.value.filter(
    (log) => normalizeLevel(log.level) === selectedLevel.value,
  );
});

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
</script>
