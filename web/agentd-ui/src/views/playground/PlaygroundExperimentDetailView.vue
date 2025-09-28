<template>
  <div v-if="experiment" class="flex h-full min-h-0 flex-col gap-6 overflow-hidden">
    <!-- Header / Summary -->
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

    <!-- Two-column content area -->
    <div class="flex-1 min-h-0 grid gap-6 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
      <!-- Left column: Variants + Runs list (scrollable) -->
      <div class="flex min-h-0 flex-col gap-6 overflow-hidden">
        <!-- Variants -->
        <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
          <header class="flex items-center justify-between">
            <h2 class="text-lg font-semibold">Variants</h2>
            <button @click="startRun" class="rounded border border-border/70 px-3 py-2 text-sm">Start run</button>
          </header>
          <div class="max-h-56 overflow-auto pr-1">
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
          </div>
        </section>

        <!-- Runs -->
        <section class="flex min-h-0 flex-col rounded-2xl border border-border/70 bg-surface p-4 gap-3">
          <header class="flex items-center justify-between">
            <h2 class="text-lg font-semibold">Runs</h2>
            <button @click="refreshRuns(experimentId)" class="rounded border border-border/70 px-3 py-2 text-sm">Refresh</button>
          </header>
          <div class="flex-1 min-h-0 overflow-auto overscroll-contain pr-1">
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
                <tr
                  v-for="run in runs"
                  :key="run.id"
                  class="border-t border-border/60 cursor-pointer transition-colors"
                  :class="{ 'bg-accent/10': run.id === selectedRunId, 'hover:bg-muted/60': run.id !== selectedRunId }"
                  @click="selectRun(run.id)"
                >
                  <td class="py-2 font-medium">{{ run.id }}</td>
                  <td class="py-2 capitalize">{{ run.status }}</td>
                  <td class="py-2">{{ formatDate(run.startedAt) }}</td>
                  <td class="py-2">{{ formatDate(run.endedAt) }}</td>
                </tr>
                <tr v-if="loadingRuns"><td colspan="4" class="py-3 text-center text-subtle-foreground">Loading runs…</td></tr>
                <tr v-else-if="runs.length === 0"><td colspan="4" class="py-3 text-center text-subtle-foreground">No runs yet.</td></tr>
              </tbody>
            </table>
          </div>
        </section>
      </div>

      <!-- Right column: Selected run details and results (scrollable) -->
      <div class="flex min-h-0 flex-col overflow-hidden">
        <section v-if="selectedRun" class="flex-1 min-h-0 flex flex-col rounded-2xl border border-border/70 bg-surface p-4 gap-4">
          <header class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 class="text-lg font-semibold">Run Details</h2>
              <p class="text-sm text-subtle-foreground">Run {{ selectedRun.id }} · Status {{ selectedRun.status }}</p>
            </div>
            <div class="flex items-center gap-2">
              <button
                @click="refreshSelectedRunResults"
                :disabled="loadingSelectedRunResults"
                class="rounded border border-border/70 px-3 py-2 text-sm disabled:opacity-60"
              >
                Refresh results
              </button>
            </div>
          </header>

          <div class="grid gap-3 text-sm md:grid-cols-2">
            <div>
              <p><span class="text-subtle-foreground">Started:</span> {{ formatDate(selectedRun.startedAt) }}</p>
              <p><span class="text-subtle-foreground">Completed:</span> {{ formatDate(selectedRun.endedAt) }}</p>
            </div>
            <div>
              <p><span class="text-subtle-foreground">Error:</span> {{ selectedRun.error ?? '—' }}</p>
              <p><span class="text-subtle-foreground">Plan shards:</span> {{ selectedRun.plan?.shards?.length ?? 0 }}</p>
            </div>
          </div>

          <div v-if="sortedMetrics.length" class="space-y-2">
            <h3 class="text-sm font-semibold text-subtle-foreground">Metrics</h3>
            <div class="flex flex-wrap gap-2">
              <span v-for="[name, value] in sortedMetrics" :key="name" class="rounded-full border border-border/70 px-3 py-1 text-xs">
                {{ name }}: {{ value.toFixed(3) }}
              </span>
            </div>
          </div>

          <div class="flex min-h-0 flex-col">
            <h3 class="text-sm font-semibold text-subtle-foreground mb-2">Results</h3>
            <p v-if="loadingSelectedRunResults" class="text-sm text-subtle-foreground">Loading run results…</p>
            <p v-else-if="runResults.length === 0" class="text-sm text-subtle-foreground">No results recorded for this run.</p>
            <div v-else class="flex-1 min-h-0 overflow-auto overscroll-contain space-y-4 pr-1">
              <article
                v-for="result in runResults"
                :key="result.id"
                class="rounded-xl border border-border/60 bg-muted/20 p-4 space-y-3"
              >
                <header class="flex flex-wrap items-center justify-between gap-2 text-sm">
                  <div class="font-medium">
                    Row {{ result.rowId }} · Variant {{ result.variantId }} · Model {{ result.model || 'default' }}
                  </div>
                  <div class="text-subtle-foreground flex flex-wrap gap-3">
                    <span>Tokens: {{ result.tokens ?? '—' }}</span>
                    <span>Latency: {{ formatLatency(result.latency) }}</span>
                    <span>Provider: {{ result.providerName ?? '—' }}</span>
                  </div>
                </header>
                <div class="grid gap-3 md:grid-cols-2">
                  <div>
                    <h4 class="text-xs font-semibold text-subtle-foreground uppercase tracking-wide">Rendered Prompt</h4>
                    <pre class="mt-1 max-h-64 overflow-auto rounded bg-background/60 p-3 text-xs whitespace-pre-wrap">{{ result.rendered || '—' }}</pre>
                  </div>
                  <div>
                    <h4 class="text-xs font-semibold text-subtle-foreground uppercase tracking-wide">Output</h4>
                    <pre class="mt-1 max-h-64 overflow-auto rounded bg-background/60 p-3 text-xs whitespace-pre-wrap">{{ result.output || '—' }}</pre>
                  </div>
                </div>
                <div class="grid gap-3 md:grid-cols-2">
                  <div>
                    <h4 class="text-xs font-semibold text-subtle-foreground uppercase tracking-wide">Expected</h4>
                    <pre class="mt-1 max-h-48 overflow-auto rounded bg-background/60 p-3 text-xs whitespace-pre-wrap">{{ asPrettyJSON(result.expected) }}</pre>
                  </div>
                  <div v-if="result.scores && Object.keys(result.scores).length">
                    <h4 class="text-xs font-semibold text-subtle-foreground uppercase tracking-wide">Scores</h4>
                    <ul class="mt-1 space-y-1 text-xs">
                      <li v-for="(score, name) in result.scores" :key="name">
                        {{ name }}: {{ typeof score === 'number' ? score.toFixed(3) : score }}
                      </li>
                    </ul>
                  </div>
                  <div v-else>
                    <h4 class="text-xs font-semibold text-subtle-foreground uppercase tracking-wide">Scores</h4>
                    <p class="mt-1 text-xs text-subtle-foreground">—</p>
                  </div>
                </div>
                <div v-if="result.artifacts && Object.keys(result.artifacts).length" class="text-xs">
                  <h4 class="font-semibold text-subtle-foreground uppercase tracking-wide">Artifacts</h4>
                  <ul class="mt-1 space-y-1">
                    <li v-for="(path, name) in result.artifacts" :key="name">
                      {{ name }} → {{ path }}
                    </li>
                  </ul>
                </div>
              </article>
            </div>
          </div>
        </section>
        <section v-else class="rounded-2xl border border-border/70 bg-surface p-4 text-sm text-subtle-foreground">
          Select a run to view details.
        </section>
      </div>
    </div>
  </div>
  <p v-else class="text-subtle-foreground text-sm">Loading experiment…</p>
</template>

<script setup lang="ts">
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { onMounted, onBeforeUnmount, ref, watch, computed } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'
import type { ExperimentSpec, Run, RunResult } from '@/api/playground'

const route = useRoute()
const router = useRouter()
const store = usePlaygroundStore()
const experimentId = ref(route.params.experimentId as string)
const experiment = ref<ExperimentSpec | null>(null)
const runs = ref(store.runsByExperiment[experimentId.value] ?? [])
const loadingRuns = ref(false)
const selectedRunId = ref<string | null>(null)

const selectedRun = computed<Run | null>(() => {
  if (!selectedRunId.value) return null
  return runs.value.find((run) => run.id === selectedRunId.value) ?? null
})

const runResults = computed<RunResult[]>(() => {
  if (!selectedRunId.value) return []
  return store.runResultsByRun[selectedRunId.value] ?? []
})

const loadingSelectedRunResults = computed(() => {
  if (!selectedRunId.value) return false
  return store.runResultsLoading[selectedRunId.value] ?? false
})

const sortedMetrics = computed(() => {
  if (!selectedRun.value?.metrics) return [] as Array<[string, number]>
  return Object.entries(selectedRun.value.metrics).sort(([a], [b]) => a.localeCompare(b))
})

async function refreshRuns(id: string) {
  loadingRuns.value = true
  await store.refreshExperimentRuns(id)
  runs.value = store.runsByExperiment[id] ?? []
  loadingRuns.value = false
  ensureRunSelection()
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
    selectedRunId.value = null
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

function ensureRunSelection() {
  const currentRuns = runs.value
  if (selectedRunId.value && currentRuns.some((run) => run.id === selectedRunId.value)) {
    return
  }
  const next = currentRuns[0]
  if (next) {
    void selectRun(next.id)
  } else {
    selectedRunId.value = null
  }
}

async function selectRun(runId: string) {
  if (selectedRunId.value === runId) {
    if (!store.runResultsByRun[runId]) {
      await store.ensureRunResults(runId)
    }
    return
  }
  selectedRunId.value = runId
  try {
    await store.ensureRunResults(runId)
  } catch (err) {
    console.error('Failed to load run results', err)
  }
}

async function refreshSelectedRunResults() {
  if (!selectedRunId.value) return
  try {
    await store.refreshRunResults(selectedRunId.value)
  } catch (err) {
    console.error('Failed to refresh run results', err)
  }
}

function formatLatency(latency?: number) {
  if (!latency || Number.isNaN(latency)) {
    return '—'
  }
  const ms = latency / 1_000_000
  if (ms < 1) {
    return `${latency}ns`
  }
  return `${ms.toFixed(ms < 100 ? 2 : 0)} ms`
}

function asPrettyJSON(value: unknown) {
  if (value == null) return '—'
  if (typeof value === 'string') return value
  try {
    return JSON.stringify(value, null, 2)
  } catch (err) {
    return String(value)
  }
}

watch(runs, ensureRunSelection, { immediate: true })
</script>
