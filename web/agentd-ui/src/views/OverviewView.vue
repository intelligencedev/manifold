<template>
  <section class="grid gap-6 xl:grid-cols-4 xl:auto-rows-[minmax(0,1fr)] min-h-0">
    <div class="flex flex-col gap-6 xl:col-span-3 min-h-0">
      <TokenUsagePanel />
      <TracesPanel />
      <MemoryPanel />
    </div>
    <div class="flex flex-col gap-6 self-start xl:col-span-1">
      <div
        v-for="stat in headlineStats"
        :key="stat.label"
        class="ap-card flex h-48 flex-col justify-between rounded-2xl bg-surface p-6"
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
