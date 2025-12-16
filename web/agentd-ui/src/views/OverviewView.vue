<template>
  <section class="flex h-full min-h-0 flex-col gap-6">
    <!-- Page header / key stats -->
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-lg font-semibold text-foreground">Overview</h1>
        <p class="text-sm text-subtle-foreground">
          Live usage, traces, memory, Avg. Prompt Tokens, and agent activity across your
          deployment.
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-3 text-xs text-faint-foreground">
        <div class="rounded-3 border border-border/60 bg-surface px-3 py-2">
          <p class="text-[11px] font-medium text-subtle-foreground">Active Agents</p>
          <p class="mt-1 text-base font-semibold text-foreground tabular-nums">
            {{ agents.length }}
          </p>
        </div>
        <div class="rounded-3 border border-border/60 bg-surface px-3 py-2">
          <p class="text-[11px] font-medium text-subtle-foreground">Runs Today</p>
          <p class="mt-1 text-base font-semibold text-foreground tabular-nums">
            {{ runsToday }}
          </p>
        </div>
      </div>
    </header>

    <div
      class="grid min-h-0 gap-6 xl:grid-cols-[minmax(0,2fr)_minmax(260px,1fr)] xl:auto-rows-[minmax(0,1fr)]"
    >
      <!-- Main observability surface -->
      <div class="flex min-h-0 flex-col gap-6">
        <TokenUsagePanel />
        <TracesPanel />
        <MemoryPanel />
      </div>

      <!-- Side column: agents + recent runs -->
      <aside class="flex min-h-0 flex-col gap-4 self-start">
        <!-- Agents list -->
        <section class="ap-panel ap-hover flex min-h-0 flex-col rounded-2xl bg-surface p-4">
          <header class="flex items-center justify-between gap-2">
            <h2 class="text-sm font-semibold text-foreground">Agents</h2>
            <span class="text-[11px] text-faint-foreground">
              {{ agents.length ? `${agents.length} total` : 'No agents' }}
            </span>
          </header>

          <p v-if="!agents.length" class="mt-3 text-xs text-faint-foreground">
            No agents reported from the backend yet.
          </p>

          <ul v-else class="mt-3 space-y-2 overflow-y-auto pr-1 text-xs">
            <li
              v-for="agent in agents"
              :key="agent.id"
              class="flex items-center justify-between gap-2 rounded-lg border border-border/50 bg-surface-muted/40 px-3 py-2"
            >
              <div class="min-w-0">
                <p class="truncate text-xs font-medium text-foreground">
                  {{ agent.name || agent.id }}
                </p>
                <p class="mt-0.5 truncate text-[11px] text-faint-foreground">
                  {{ agent.model || 'Model not set' }}
                </p>
              </div>
              <div class="flex flex-col items-end gap-1">
                <span
                  :class="[
                    'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold',
                    agent.state === 'online'
                      ? 'bg-success/15 text-success'
                      : agent.state === 'degraded'
                        ? 'bg-warning/15 text-warning'
                        : 'bg-border/50 text-subtle-foreground',
                  ]"
                >
                  {{ agent.state }}
                </span>
                <span class="text-[10px] text-faint-foreground">
                  {{ formatRelativeTime(agent.updatedAt) }}
                </span>
              </div>
            </li>
          </ul>
        </section>

        <!-- Recent runs -->
        <section
          class="ap-panel ap-hover flex min-h-0 flex-col overflow-hidden rounded-2xl bg-surface p-4"
        >
          <header class="flex items-center justify-between gap-2">
            <h2 class="text-sm font-semibold text-foreground">Recent Runs</h2>
            <span class="text-[11px] text-faint-foreground">
              {{ recentRuns.length ? `${recentRuns.length} shown` : 'None' }}
            </span>
          </header>

          <p v-if="!recentRuns.length" class="mt-3 text-xs text-faint-foreground">
            No recent runs in the last 24 hours.
          </p>

          <ul
            v-else
            class="mt-3 min-h-0 max-h-[50vh] space-y-2 overflow-y-auto pr-1 text-xs"
          >
            <li
              v-for="run in recentRuns"
              :key="run.id"
              class="rounded-lg border border-border/50 bg-surface-muted/40 px-3 py-2"
            >
              <p class="line-clamp-2 text-xs font-medium text-foreground">
                {{ run.prompt || 'Untitled run' }}
              </p>
              <div
                class="mt-1 flex items-center justify-between gap-2 text-[11px] text-faint-foreground"
              >
                <span
                  :class="[
                    'inline-flex items-center rounded-full px-2 py-0.5 text-[10px] font-semibold',
                    run.status === 'completed'
                      ? 'bg-success/15 text-success'
                      : run.status === 'running'
                        ? 'bg-accent/15 text-accent'
                        : 'bg-danger/10 text-danger',
                  ]"
                >
                  {{ run.status }}
                </span>
                <span>{{ formatRelativeTime(run.createdAt) }}</span>
              </div>
            </li>
          </ul>
        </section>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import TokenUsagePanel from '@/components/observability/TokenUsagePanel.vue'
import TracesPanel from '@/components/observability/TracesPanel.vue'
import MemoryPanel from '@/components/observability/MemoryPanel.vue'
import { fetchAgentRuns, fetchAgentStatus, listSpecialists } from '@/api/client'

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

const runsToday = computed(
  () => runs.value.filter((run) => isToday(run.createdAt)).length,
)

const recentRuns = computed(() =>
  runs.value
    .filter((run) => isToday(run.createdAt))
    .slice()
    .sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    )
    .slice(0, 5),
)

function isToday(value: string) {
  const date = new Date(value)
  const now = new Date()
  return (
    date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear()
  )
}

function formatRelativeTime(value: string) {
  const date = new Date(value)
  const now = new Date()
  const diffSeconds = Math.floor((now.getTime() - date.getTime()) / 1000)

  if (!Number.isFinite(diffSeconds)) return ''
  if (diffSeconds < 45) return 'just now'

  const minutes = Math.floor(diffSeconds / 60)
  if (minutes < 60) return `${minutes} min${minutes === 1 ? '' : 's'} ago`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours} h${hours === 1 ? '' : 's'} ago`

  const days = Math.floor(hours / 24)
  return `${days} d${days === 1 ? '' : 's'} ago`
}
</script>
