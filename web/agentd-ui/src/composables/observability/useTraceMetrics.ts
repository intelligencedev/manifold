import { computed, type Ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchTraceMetrics, type TraceMetricRow } from '@/api/client'
import { type MetricsTimeRangeValue } from '@/composables/observability/useTokenMetrics'

export interface TraceDisplayRow {
  key: string
  name: string
  modelLabel: string
  status: string
  durationLabel: string
  tokenLabel: string
  timeLabel: string
  timestamp: number
}

const numberFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 })
const timeFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

export function useTraceMetrics(selectedRange: Ref<MetricsTimeRangeValue>) {
  const query = useQuery({
    queryKey: computed(() => ['trace-metrics', selectedRange.value]),
    queryFn: () => fetchTraceMetrics({ window: selectedRange.value, limit: 200 }),
    keepPreviousData: true,
    staleTime: 60_000,
    refetchInterval: 60_000,
  })

  const traceRows = computed<TraceDisplayRow[]>(() => {
    const traces = query.data.value?.traces ?? []
    return traces
      .slice()
      .sort((a, b) => Number(b.timestamp ?? 0) - Number(a.timestamp ?? 0))
      .map((trace, idx) => {
        const prompt = Number(trace.promptTokens ?? 0)
        const completion = Number(trace.completionTokens ?? 0)
        const total = Number(trace.totalTokens ?? prompt + completion)
        return {
          key: trace.traceId || `${trace.name}-${trace.timestamp}-${idx}`,
          name: trace.name || 'LLM trace',
          modelLabel: trace.model || 'unknown model',
          status: trace.status || 'ok',
          durationLabel: formatDuration(trace.durationMillis),
          tokenLabel: formatTokenLabel(prompt, completion, total),
          timeLabel: formatTimestamp(trace.timestamp),
          timestamp: Number(trace.timestamp ?? 0),
        }
      })
  })

  return {
    ...query,
    traceRows,
    formatDuration,
    formatTimestamp,
  }
}

function formatDuration(ms?: number): string {
  if (ms == null || ms <= 0) return '—'
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) {
    const seconds = ms / 1000
    return `${seconds.toFixed(seconds >= 10 ? 0 : 1)} s`
  }
  const minutes = ms / 60000
  return `${minutes.toFixed(minutes >= 10 ? 0 : 1)} min`
}

function formatTimestamp(seconds?: number): string {
  if (seconds == null || !Number.isFinite(seconds)) return 'Unknown time'
  return timeFormatter.format(new Date(seconds * 1000))
}

function formatTokenLabel(prompt: number, completion: number, total: number): string {
  if (total <= 0) return 'Tokens not reported'
  return `${numberFormatter.format(total)} tokens · ${numberFormatter.format(prompt)} prompt / ${numberFormatter.format(completion)} completion`
}
