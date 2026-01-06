import { computed, type Ref } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { fetchTokenMetrics, type TokenMetricsRow } from "@/api/client";

export type MetricsTimeRangeValue = "1h" | "6h" | "12h" | "24h" | "7d" | "30d";

export interface TimeRangeOption {
  label: string;
  value: MetricsTimeRangeValue;
}

export interface TokenChartRow extends TokenMetricsRow {
  scaleWidth: string;
  promptWidth: string;
  completionWidth: string;
}

const numberFormatter = new Intl.NumberFormat(undefined, {
  maximumFractionDigits: 0,
});

export const TOKEN_METRIC_TIME_RANGES: TimeRangeOption[] = [
  { label: "Last 1h", value: "1h" },
  { label: "Last 6h", value: "6h" },
  { label: "Last 12h", value: "12h" },
  { label: "Last 24h", value: "24h" },
  { label: "Last 7d", value: "7d" },
  { label: "Last 30d", value: "30d" },
];

export function useTokenMetrics(selectedRange: Ref<MetricsTimeRangeValue>) {
  const query = useQuery({
    queryKey: computed(() => ["token-metrics", selectedRange.value]),
    queryFn: () => fetchTokenMetrics({ window: selectedRange.value }),
    keepPreviousData: true,
    staleTime: 60_000,
    refetchInterval: 60_000,
  });

  const tokenUsageRows = computed<TokenMetricsRow[]>(
    () => query.data.value?.models ?? [],
  );

  const tokenChartRows = computed<TokenChartRow[]>(() => {
    const rows = tokenUsageRows.value;
    if (!rows.length) return [];

    const maxTotal = rows.reduce(
      (max, row) => Math.max(max, Number(row?.total ?? 0)),
      0,
    );
    const safeMax = maxTotal > 0 ? maxTotal : 1;

    return rows.map((row) => {
      const prompt = Number(row?.prompt ?? 0);
      const completion = Number(row?.completion ?? 0);
      const totalBase = Number(row?.total ?? prompt + completion);
      const total = totalBase > 0 ? totalBase : prompt + completion;
      const scaleWidth = clampPercentage((total / safeMax) * 100);
      const promptShare =
        total > 0 ? clampPercentage((prompt / total) * 100) : 0;
      const completionShare =
        total > 0 ? clampPercentage((completion / total) * 100) : 0;

      return {
        model: row.model,
        prompt,
        completion,
        total,
        scaleWidth: `${scaleWidth}%`,
        promptWidth: `${promptShare}%`,
        completionWidth: `${completionShare}%`,
      };
    });
  });

  const tokenChartMaxTotal = computed(() =>
    tokenChartRows.value.reduce((max, row) => Math.max(max, row.total), 0),
  );

  function formatNumber(value: number | undefined | null) {
    if (value == null) return "0";
    return numberFormatter.format(value);
  }

  return {
    ...query,
    formatNumber,
    tokenChartRows,
    tokenChartMaxTotal,
    tokenUsageRows,
  };
}

function clampPercentage(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, value));
}
