<template>
  <section class="flex h-full min-h-0 flex-col overflow-y-auto">
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
          <TokenUsagePanel />
        </template>

        <template #item-traces>
          <TracesPanel />
        </template>

        <template #item-memory>
          <MemoryPanel />
        </template>

        <template #item-agents>
          <AgentsPanel :agents="agents" />
        </template>

        <template #item-runs>
          <RecentRunsPanel :runs="recentRuns" />
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
import AgentsPanel from '@/components/overview/AgentsPanel.vue'
import RecentRunsPanel from '@/components/overview/RecentRunsPanel.vue'
import Panel from '@/components/ui/Panel.vue'
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

function onLayoutChange(newLayout: GridItemConfig[]) {
  // Layout changes are automatically saved via DashboardGrid component
  console.log('Dashboard layout updated:', newLayout)
}

function resetLayout() {
  dashboardGridRef.value?.resetLayout()
}
</script>
