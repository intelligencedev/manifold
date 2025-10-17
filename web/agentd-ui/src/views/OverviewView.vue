<template>
  <section class="grid gap-6 xl:grid-cols-4 xl:auto-rows-[minmax(0,1fr)] min-h-0">
    <div class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg xl:col-span-3 flex h-full flex-col overflow-hidden">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-foreground">Token Usage Graph</h2>
          <p class="text-xs text-faint-foreground">Share of prompt vs completion tokens</p>
        </div>
        <div class="flex items-center gap-4 text-xs text-faint-foreground">
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

      <div class="mt-4 flex-1 overflow-hidden">
        <div v-if="tokenMetricsLoading" class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground">
          Loading token usage…
        </div>
        <div v-else-if="tokenMetricsError" class="rounded-2xl border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground">
          Failed to load token usage metrics.
        </div>
        <div v-else class="flex h-full flex-col">
          <div v-if="!tokenChartRows.length" class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground">
            No token usage recorded in the selected window.
          </div>
          <div v-else class="flex h-full flex-col">
            <div class="flex-1 space-y-4 overflow-y-auto pr-1">
              <div v-for="row in tokenChartRows" :key="row.model" class="rounded-2xl border border-border/60 bg-muted/10 p-4">
                <div class="flex items-center justify-between text-sm font-medium text-foreground">
                  <span>{{ row.model }}</span>
                  <span class="tabular-nums">{{ formatNumber(row.total) }} total</span>
                </div>
                <div class="mt-1 flex items-center justify-between text-xs text-faint-foreground">
                  <span>{{ formatNumber(row.prompt) }} prompt</span>
                  <span>{{ formatNumber(row.completion) }} completion</span>
                </div>
                <div class="mt-3 h-3 w-full rounded-full bg-border/40">
                  <div class="flex h-full overflow-hidden rounded-full" :style="{ width: row.scaleWidth }">
                    <div class="h-full bg-sky-500" :style="{ width: row.promptWidth }"></div>
                    <div class="h-full bg-purple-500" :style="{ width: row.completionWidth }"></div>
                  </div>
                </div>
              </div>
            </div>
            <p class="mt-3 text-xs text-faint-foreground">Largest bar: {{ formatNumber(tokenChartMaxTotal) }} tokens</p>
          </div>
        </div>
      </div>
    </div>

    <div class="flex flex-col gap-6 self-start xl:col-span-1">
      <div
        v-for="stat in headlineStats"
        :key="stat.label"
        class="flex h-48 flex-col justify-between rounded-2xl border border-border/70 bg-surface p-6 shadow-lg"
      >
        <p class="text-sm font-medium text-subtle-foreground">{{ stat.label }}</p>
        <p class="mt-4 text-3xl font-semibold text-foreground">{{ stat.value }}</p>
        <p class="mt-2 text-xs text-faint-foreground">{{ stat.helper }}</p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchAgentRuns, fetchAgentStatus, fetchTokenMetrics, listSpecialists, type TokenMetricsRow } from '@/api/client'

const { data: agentData } = useQuery({
  queryKey: ['agent-status'],
  queryFn: fetchAgentStatus,
  staleTime: 30_000,
})

const { data: specialistsData } = useQuery({
  queryKey: ['specialists'],
  queryFn: listSpecialists,
  staleTime: 30_000,
})

const { data: runsData } = useQuery({
  queryKey: ['agent-runs'],
  queryFn: fetchAgentRuns,
  staleTime: 15_000,
})

const {
  data: tokenMetricsData,
  isLoading: tokenMetricsLoading,
  isError: tokenMetricsError,
} = useQuery({
  queryKey: ['token-metrics'],
  queryFn: fetchTokenMetrics,
  staleTime: 60_000,
  refetchInterval: 60_000,
})

const agents = computed(() => {
  const base = (agentData.value ?? []).slice()
  // If the orchestrator specialist is present in the specialists list, expose
  // it as a synthetic agent in the Overview. The backend exposes a synthetic
  // "orchestrator" specialist via /api/specialists; convert it to an
  // AgentStatus-like object for rendering here.
  const specs = specialistsData?.value ?? []
  const orch = specs.find((s: any) => String(s.name).toLowerCase().trim() === 'orchestrator')
  if (orch) {
    const exists = base.find((a: any) => String(a.id).toLowerCase().trim() === String(orch.name).toLowerCase().trim())
    if (!exists) {
      base.unshift({
        id: orch.name || 'orchestrator',
        name: orch.name || 'orchestrator',
        state: orch.paused ? 'offline' : 'online',
        model: orch.model || '',
        updatedAt: new Date().toISOString(),
      })
    }
  }
  return base
})
const runs = computed(() => runsData.value ?? [])
type TokenChartRow = TokenMetricsRow & {
  scaleWidth: string
  promptWidth: string
  completionWidth: string
}

const tokenUsageRows = computed<TokenMetricsRow[]>(() => tokenMetricsData.value?.models ?? [])
const tokenChartRows = computed<TokenChartRow[]>(() => {
  const rows = tokenUsageRows.value
  if (!rows.length) return []
  const maxTotal = rows.reduce((max, row) => Math.max(max, Number(row?.total ?? 0)), 0)
  const safeMax = maxTotal > 0 ? maxTotal : 1
  return rows.map((row) => {
    const prompt = Number(row?.prompt ?? 0)
    const completion = Number(row?.completion ?? 0)
    const totalBase = Number(row?.total ?? prompt + completion)
    const total = totalBase > 0 ? totalBase : prompt + completion
    const scaleWidth = clampPercentage((total / safeMax) * 100)
    const promptShare = total > 0 ? clampPercentage((prompt / total) * 100) : 0
    const completionShare = total > 0 ? clampPercentage((completion / total) * 100) : 0
    return {
      model: row.model,
      prompt,
      completion,
      total,
      scaleWidth: `${scaleWidth}%`,
      promptWidth: `${promptShare}%`,
      completionWidth: `${completionShare}%`,
    }
  })
})
const tokenChartMaxTotal = computed(() => tokenChartRows.value.reduce((max, row) => Math.max(max, row.total), 0))

const headlineStats = computed(() => [
  {
    label: 'Active Agents',
    value: agents.value.length,
    helper: 'Configured specialists currently active',
  },
  {
    label: 'Runs Today',
    value: runs.value.filter((run) => isToday(run.createdAt)).length,
    helper: 'Completed tasks in the last 24h',
  },
])

const numberFormatter = new Intl.NumberFormat(undefined, { maximumFractionDigits: 0 })

function formatNumber(value: number | undefined | null) {
  if (value == null) return '0'
  return numberFormatter.format(value)
}

function clampPercentage(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

function isToday(value: string) {
  const date = new Date(value)
  const now = new Date()
  return (
    date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear()
  )
}
</script>
