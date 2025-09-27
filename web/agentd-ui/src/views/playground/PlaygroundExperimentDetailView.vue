<template>
  <div v-if="experiment" class="space-y-6">
    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-2">
      <div class="flex items-center justify-between">
        <div>
          <h1 class="text-xl font-semibold">{{ experiment.name }}</h1>
          <p class="text-sm text-subtle-foreground">Dataset: {{ experiment.datasetId }} · Variants: {{ experiment.variants.length }}</p>
        </div>
        <RouterLink to="/playground/experiments" class="text-sm text-accent hover:underline">Back to experiments</RouterLink>
      </div>
      <div class="text-sm text-subtle-foreground">Created {{ formatDate(experiment.createdAt) }}</div>
    </section>

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Variants</h2>
        <button @click="startRun" class="rounded border border-border/70 px-3 py-2 text-sm">Start run</button>
      </header>
      <table class="w-full text-sm">
        <thead class="text-subtle-foreground">
          <tr>
            <th class="text-left py-2">Variant</th>
            <th class="text-left py-2">Prompt Version</th>
            <th class="text-left py-2">Model</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="variant in experiment.variants" :key="variant.id" class="border-t border-border/60">
            <td class="py-2 font-medium">{{ variant.id }}</td>
            <td class="py-2">{{ variant.promptVersionId }}</td>
            <td class="py-2">{{ variant.model }}</td>
          </tr>
        </tbody>
      </table>
    </section>

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header class="flex items-center justify-between">
        <h2 class="text-lg font-semibold">Runs</h2>
        <button @click="refreshRuns" class="rounded border border-border/70 px-3 py-2 text-sm">Refresh</button>
      </header>
      <table class="w-full text-sm">
        <thead class="text-subtle-foreground">
          <tr>
            <th class="text-left py-2">Run</th>
            <th class="text-left py-2">Status</th>
            <th class="text-left py-2">Started</th>
            <th class="text-left py-2">Completed</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="run in runs" :key="run.id" class="border-t border-border/60">
            <td class="py-2">{{ run.id }}</td>
            <td class="py-2 capitalize">{{ run.status }}</td>
            <td class="py-2">{{ formatDate(run.startedAt) }}</td>
            <td class="py-2">{{ formatDate(run.endedAt) }}</td>
          </tr>
          <tr v-if="loadingRuns"><td colspan="4" class="py-3 text-center text-subtle-foreground">Loading runs…</td></tr>
          <tr v-else-if="runs.length === 0"><td colspan="4" class="py-3 text-center text-subtle-foreground">No runs yet.</td></tr>
        </tbody>
      </table>
    </section>
  </div>
  <p v-else class="text-subtle-foreground text-sm">Loading experiment…</p>
</template>

<script setup lang="ts">
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'
import type { ExperimentSpec } from '@/api/playground'

const route = useRoute()
const router = useRouter()
const store = usePlaygroundStore()
const experimentId = ref(route.params.experimentId as string)
const experiment = ref<ExperimentSpec | null>(null)
const runs = ref(store.runsByExperiment[experimentId.value] ?? [])
const loadingRuns = ref(false)

async function refreshRuns(id: string) {
  loadingRuns.value = true
  await store.refreshExperimentRuns(id)
  runs.value = store.runsByExperiment[id] ?? []
  loadingRuns.value = false
}

onMounted(async () => {
  const ok = await loadExperiment(experimentId.value)
  if (ok) {
    await refreshRuns(experimentId.value)
  }
})

onBeforeUnmount(() => {
  store.clearRunPolling(experimentId.value)
})

watch(
  () => route.params.experimentId,
  async (next) => {
    if (typeof next !== 'string') return
    experimentId.value = next
    const ok = await loadExperiment(next)
    if (ok) {
      await refreshRuns(next)
    }
  }
)

async function startRun() {
  await store.triggerRun(experimentId.value)
  await refreshRuns(experimentId.value)
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function loadExperiment(id: string) {
  experiment.value = await store.ensureExperiment(id)
  if (!experiment.value) {
    await router.replace('/playground/experiments')
    return false
  }
  return true
}
</script>
