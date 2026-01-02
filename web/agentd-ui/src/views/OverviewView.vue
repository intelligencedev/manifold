<template>
  <section class="flex h-full min-h-0 flex-col gap-6 overflow-hidden">
    <Panel
      title="Overview"
      description="Live usage, traces, memory, prompt tokens, and agent activity across your deployment."
    >
      <template #actions>
        <button
          type="button"
          class="inline-flex items-center gap-2 rounded-full border border-white/10 px-3 py-2 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
          @click="resetLayout"
        >
          Reset layout
        </button>
      </template>

      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard label="Active Agents" :value="agents.length" secondary="Reporting status" />
        <MetricCard label="Runs Today" :value="runsToday" :secondary="runsSummary" />
        <MetricCard label="Recent Runs" :value="recentRuns.length" secondary="Past 24 hours" />
        <MetricCard label="Specialists" :value="specialistCount" secondary="Available roles" />
      </div>
    </Panel>

    <div class="min-h-0 flex-1 pb-6">
      <DashboardGrid
        ref="dashboardGridRef"
        :layout="dashboardLayout"
        storage-key="overview-dashboard-layout"
        @layout-change="onLayoutChange"
      >
        <template #item-tokens>
          <GlassCard class="h-full">
            <TokenUsagePanel />
          </GlassCard>
        </template>

        <template #item-traces>
          <GlassCard class="h-full">
            <TracesPanel />
          </GlassCard>
        </template>

        <template #item-memory>
          <GlassCard class="h-full">
            <MemoryPanel />
          </GlassCard>
        </template>

        <template #item-agents>
          <GlassCard class="flex h-full flex-col">
            <header class="flex items-center justify-between gap-2">
              <div>
                <p class="text-xs uppercase tracking-wide text-subtle-foreground">Agents</p>
                <h2 class="text-base font-semibold text-foreground">Status</h2>
              </div>
              <Pill tone="neutral" size="sm">{{ agents.length ? `${agents.length} total` : 'None' }}</Pill>
            </header>

            <p v-if="!agents.length" class="mt-4 text-xs text-faint-foreground">
              No agents reported from the backend yet.
            </p>

            <ul v-else class="mt-4 space-y-2 overflow-y-auto pr-1 text-xs">
              <li
                v-for="agent in agents"
                :key="agent.id"
                class="flex items-center justify-between gap-2 rounded-[14px] border border-white/10 bg-surface-muted/40 px-3 py-2"
              >
                <div class="min-w-0">
                  <p class="truncate text-xs font-semibold text-foreground">
                    {{ agent.name || agent.id }}
                  </p>
                  <p class="mt-0.5 truncate text-[11px] text-faint-foreground">
                    {{ agent.model || 'Model not set' }}
                  </p>
                </div>
                <div class="flex flex-col items-end gap-1">
                  <Pill :tone="agentTone(agent.state)" size="sm">{{ agent.state }}</Pill>
                  <span class="text-[10px] text-faint-foreground">
                    {{ formatRelativeTime(agent.updatedAt) }}
                  </span>
                </div>
              </li>
            </ul>
          </GlassCard>
        </template>

        <template #item-runs>
          <GlassCard class="flex h-full flex-col overflow-hidden">
            <header class="flex items-center justify-between gap-2">
              <div>
                <p class="text-xs uppercase tracking-wide text-subtle-foreground">Recent Runs</p>
                <h2 class="text-base font-semibold text-foreground">Past 24 hours</h2>
              </div>
              <Pill tone="neutral" size="sm">{{ recentRuns.length ? `${recentRuns.length} shown` : 'None' }}</Pill>
            </header>

            <p v-if="!recentRuns.length" class="mt-4 text-xs text-faint-foreground">
              No recent runs in the last 24 hours.
            </p>

            <ul v-else class="mt-4 space-y-2 overflow-y-auto pr-1 text-xs">
              <li
                v-for="run in recentRuns"
                :key="run.id"
                class="rounded-[14px] border border-white/10 bg-surface-muted/40 px-3 py-2"
              >
                <p class="line-clamp-2 text-xs font-semibold text-foreground">
                  {{ run.prompt || 'Untitled run' }}
                </p>
                <div class="mt-2 flex items-center justify-between gap-2 text-[11px] text-faint-foreground">
                  <Pill :tone="runTone(run.status)" size="sm">{{ run.status }}</Pill>
                  <span>{{ formatRelativeTime(run.createdAt) }}</span>
                </div>
              </li>
            </ul>
          </GlassCard>
        </template>
      </DashboardGrid>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import DashboardGrid, { type GridItemConfig } from '@/components/DashboardGrid.vue'
import TokenUsagePanel from '@/components/observability/TokenUsagePanel.vue'
import TracesPanel from '@/components/observability/TracesPanel.vue'
import MemoryPanel from '@/components/observability/MemoryPanel.vue'
import GlassCard from '@/components/ui/GlassCard.vue'
import Panel from '@/components/ui/Panel.vue'
import Pill from '@/components/ui/Pill.vue'
import MetricCard from '@/components/ui/MetricCard.vue'
import { fetchAgentRuns, fetchAgentStatus, listSpecialists } from '@/api/client'

const dashboardGridRef = ref<InstanceType<typeof DashboardGrid>>()

// Define default dashboard layout
// 12 columns grid, row height = 80px + 16px margin = 96px per row
const dashboardLayout = ref<GridItemConfig[]>([
  // Token Usage - wide, tall (takes up more space)
  { i: 'tokens', x: 0, y: 0, w: 8, h: 4, minW: 4, minH: 3 },
  // Agents - sidebar
  { i: 'agents', x: 8, y: 0, w: 4, h: 4, minW: 3, minH: 3 },
  // Traces - wide, tall
  { i: 'traces', x: 0, y: 4, w: 8, h: 5, minW: 4, minH: 4 },
  // Recent Runs - sidebar
  { i: 'runs', x: 8, y: 4, w: 4, h: 5, minW: 3, minH: 3 },
  // Memory - full width
  { i: 'memory', x: 0, y: 9, w: 12, h: 4, minW: 4, minH: 3 },
])

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

const specialistCount = computed(() => (specialistsData?.value ?? []).length)

const runsToday = computed(
  () => runs.value.filter((run) => isToday(run.createdAt)).length,
)

const runsSummary = computed(() => `${runsToday.value} started today`)

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

function onLayoutChange(newLayout: GridItemConfig[]) {
  // Layout changes are automatically saved via DashboardGrid component
  console.log('Dashboard layout updated:', newLayout)
}

function resetLayout() {
  dashboardGridRef.value?.resetLayout()
}

function agentTone(state: string) {
  if (state === 'online') return 'success'
  if (state === 'degraded') return 'warning'
  if (state === 'offline') return 'danger'
  return 'neutral'
}

function runTone(status: string) {
  if (status === 'completed') return 'success'
  if (status === 'running') return 'accent'
  return 'danger'
}
</script>
