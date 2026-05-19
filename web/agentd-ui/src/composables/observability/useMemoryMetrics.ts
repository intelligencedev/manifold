import { computed, type Ref } from "vue";
import { keepPreviousData, useQuery } from "@tanstack/vue-query";
import {
  fetchMemoryMetrics,
  type MemoryReasonMetric,
  type MemoryResultMetric,
  type MemorySizeMetric,
} from "@/api/client";
import { type MetricsTimeRangeValue } from "@/composables/observability/useTokenMetrics";

const numberFormatter = new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 0,
});

const decimalFormatter = new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 1,
});

export interface MemoryBreakdownRow {
  key: string;
  label: string;
  value: number;
  width: string;
}

export interface MemorySizeRow extends MemorySizeMetric {
  label: string;
  width: string;
}

export function useMemoryMetrics(selectedRange: Ref<MetricsTimeRangeValue>) {
  const query = useQuery({
    queryKey: computed(() => ["memory-metrics", selectedRange.value]),
    queryFn: () => fetchMemoryMetrics({ window: selectedRange.value }),
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    refetchInterval: 60_000,
  });

  const totals = computed(
    () =>
      query.data.value?.totals ?? {
        searches: 0,
        hits: 0,
        avgHitsPerSearch: 0,
        evolves: 0,
        evolveErrors: 0,
        smartMerges: 0,
        pruned: 0,
      },
  );

  const latencyAvgMs = computed(() => query.data.value?.latency?.avgMs ?? 0);

  const evolveErrorRate = computed(() => {
    const evolves = totals.value.evolves || 0;
    if (!evolves) return 0;
    return (totals.value.evolveErrors / evolves) * 100;
  });

  const sizeRows = computed<MemorySizeRow[]>(() => {
    const rows = query.data.value?.sizes ?? [];
    const max = rows.reduce((current, row) => Math.max(current, row.size), 0);
    const safeMax = max > 0 ? max : 1;
    return rows.slice(0, 6).map((row) => ({
      ...row,
      label: `${row.session || "default"}`,
      width: `${clampPercentage((row.size / safeMax) * 100)}%`,
    }));
  });

  const pruneRows = computed(() =>
    normalizeBreakdown(query.data.value?.prunedByReason ?? [], "reason"),
  );

  const evolveRows = computed(() =>
    normalizeBreakdown(query.data.value?.evolvesByResult ?? [], "result"),
  );

  function formatNumber(value: number | undefined | null) {
    if (value == null) return "0";
    return numberFormatter.format(value);
  }

  function formatDecimal(value: number | undefined | null) {
    if (value == null) return "0";
    return decimalFormatter.format(value);
  }

  return {
    ...query,
    totals,
    latencyAvgMs,
    evolveErrorRate,
    sizeRows,
    pruneRows,
    evolveRows,
    formatNumber,
    formatDecimal,
  };
}

function normalizeBreakdown(
  rows: Array<MemoryReasonMetric | MemoryResultMetric>,
  keyName: "reason" | "result",
): MemoryBreakdownRow[] {
  const max = rows.reduce((current, row) => Math.max(current, row.count), 0);
  const safeMax = max > 0 ? max : 1;
  return rows
    .slice()
    .sort((a, b) => b.count - a.count)
    .slice(0, 5)
    .map((row) => {
      const rawLabel =
        keyName === "reason" && "reason" in row
          ? row.reason
          : "result" in row
            ? row.result
            : "";
      const label = rawLabel || "unknown";
      return {
        key: label,
        label,
        value: row.count,
        width: `${clampPercentage((row.count / safeMax) * 100)}%`,
      };
    });
}

function clampPercentage(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, value));
}
