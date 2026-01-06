<template>
  <div
    class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg flex h-full flex-col overflow-hidden"
  >
    <div class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h2 class="text-lg font-semibold text-foreground">Token Usage</h2>
        <p class="text-xs text-faint-foreground">
          Share of prompt vs completion tokens
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
        <div class="flex items-center gap-4">
          <span class="flex items-center gap-2">
            <span class="h-2 w-2 rounded-full bg-sky-500"></span>
            Prompt
          </span>
          <span class="flex items-center gap-2">
            <span class="h-2 w-2 rounded-full bg-purple-500"></span>
            Completion
          </span>
        </div>
      </div>
    </div>

    <div class="mt-4 flex-1 overflow-hidden">
      <div
        v-if="tokenMetricsLoading"
        class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
      >
        Loading token usage…
      </div>
      <div
        v-else-if="tokenMetricsError"
        class="rounded-2xl border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
      >
        Failed to load token usage metrics.
      </div>
      <div v-else class="flex h-full flex-col">
        <div
          v-if="!tokenChartRows.length"
          class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
        >
          No token usage recorded in the selected window.
        </div>
        <div v-else class="flex h-full flex-col">
          <div class="flex-1 space-y-4 overflow-y-auto pr-1">
            <div
              v-for="row in tokenChartRows"
              :key="row.model"
              class="rounded-2xl border border-border/60 bg-muted/10 p-4"
            >
              <div
                class="flex items-center justify-between text-sm font-medium text-foreground"
              >
                <span>{{ row.model }}</span>
                <span class="tabular-nums"
                  >{{ formatNumber(row.total) }} total</span
                >
              </div>
              <div
                class="mt-1 flex items-center justify-between text-xs text-faint-foreground"
              >
                <span>{{ formatNumber(row.prompt) }} prompt</span>
                <span>{{ formatNumber(row.completion) }} completion</span>
              </div>
              <div class="mt-3 h-3 w-full rounded-full bg-border/40">
                <div
                  class="flex h-full overflow-hidden rounded-full"
                  :style="{ width: row.scaleWidth }"
                >
                  <div
                    class="h-full bg-sky-500"
                    :style="{ width: row.promptWidth }"
                  ></div>
                  <div
                    class="h-full bg-purple-500"
                    :style="{ width: row.completionWidth }"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <p class="mt-3 text-xs text-faint-foreground">
            Largest bar: {{ formatNumber(tokenChartMaxTotal) }} tokens
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import {
  TOKEN_METRIC_TIME_RANGES,
  useTokenMetrics,
  type MetricsTimeRangeValue,
} from "@/composables/observability/useTokenMetrics";
import DropdownSelect from "@/components/DropdownSelect.vue";

const selectedRange = ref<MetricsTimeRangeValue>("24h");

const timeRangeDropdownOptions = TOKEN_METRIC_TIME_RANGES.map((option) => ({
  id: option.value,
  label: option.label,
  value: option.value,
}));

const {
  isLoading: tokenMetricsLoading,
  isError: tokenMetricsError,
  tokenChartRows,
  tokenChartMaxTotal,
  formatNumber,
} = useTokenMetrics(selectedRange);
</script>
