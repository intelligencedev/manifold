<template>
  <section class="space-y-10">
    <div class="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
      <div
        v-for="stat in headlineStats"
        :key="stat.label"
        class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg"
      >
        <p class="text-sm font-medium text-subtle-foreground">{{ stat.label }}</p>
        <p class="mt-4 text-3xl font-semibold text-foreground">{{ stat.value }}</p>
        <p class="mt-2 text-xs text-faint-foreground">{{ stat.helper }}</p>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="lg:col-span-2 space-y-6">
        <div class="flex items-center justify-between">
          <h2 class="text-lg font-semibold text-foreground">Agents</h2>
          <RouterLink to="/specialists" class="text-sm font-semibold text-accent hover:text-accent/80">Manage</RouterLink>
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <AgentCard v-for="agent in agents" :key="agent.id" :agent="agent" />
        </div>
        <div v-if="agentsLoading" class="rounded-2xl border border-border/70 bg-surface p-6">
          <p class="text-sm text-faint-foreground">Loading agent status…</p>
        </div>
        <div v-if="agentsError" class="rounded-2xl border border-danger/60 bg-danger/10 p-6">
          <p class="text-sm text-danger-foreground">Failed to load agents. Check connectivity.</p>
        </div>
      </div>

      <aside class="space-y-6">
        <h2 class="text-lg font-semibold text-foreground">Recent Runs</h2>
        <RunTable :runs="runs" />
        <div
          v-if="runsLoading"
          class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-faint-foreground"
        >
          Loading runs…
        </div>
        <div
          v-if="runsError"
          class="rounded-2xl border border-danger/60 bg-danger/10 p-4 text-sm text-danger-foreground"
        >
          Failed to load recent runs.
        </div>
      </aside>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import { useQuery } from '@tanstack/vue-query'
import { fetchAgentRuns, fetchAgentStatus, listSpecialists } from '@/api/client'
import AgentCard from '@/components/AgentCard.vue'
import RunTable from '@/components/RunTable.vue'

const {
  data: agentData,
  isLoading: agentsLoading,
  isError: agentsError,
} = useQuery({
  queryKey: ['agent-status'],
  queryFn: fetchAgentStatus,
  staleTime: 30_000,
})

const { data: specialistsData } = useQuery({
  queryKey: ['specialists'],
  queryFn: listSpecialists,
  staleTime: 30_000,
})

const {
  data: runsData,
  isLoading: runsLoading,
  isError: runsError,
} = useQuery({
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
  {
    label: 'Avg. Prompt Tokens',
    value: averageTokens(runs.value),
    helper: 'Past 10 runs',
  },
])

function averageTokens(input: typeof runs.value) {
  const last = input.slice(0, 10)
  if (!last.length) return '—'
  const total = last.reduce((sum, run) => sum + (run.tokens ?? 0), 0)
  return Math.round(total / last.length)
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
