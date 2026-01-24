import { computed, type Ref } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { fetchLogMetrics, type LogMetricsRow } from "@/api/client";
import type { MetricsTimeRangeValue } from "@/composables/observability/useTokenMetrics";

export type LogDisplayRow = LogMetricsRow & { key: string };

export function useLogMetrics(selectedRange: Ref<MetricsTimeRangeValue>) {
  const query = useQuery({
    queryKey: computed(() => ["log-metrics", selectedRange.value]),
    queryFn: () => fetchLogMetrics({ window: selectedRange.value, limit: 200 }),
    keepPreviousData: true,
    staleTime: 15_000,
    refetchInterval: 15_000,
  });

  const logRows = computed<LogDisplayRow[]>(() => {
    const logs = query.data.value?.logs ?? [];
    return logs
      .slice()
      .sort((a, b) => Number(b.timestamp ?? 0) - Number(a.timestamp ?? 0))
      .map((log, idx) => ({
        ...log,
        key: log.traceId || log.spanId || `${log.timestamp}-${idx}`,
      }));
  });

  return {
    ...query,
    logRows,
  };
}
