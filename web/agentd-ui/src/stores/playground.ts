import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import {
  type Prompt,
  type PromptVersion,
  type Dataset,
  type DatasetRow,
  type ExperimentSpec,
  type Run,
  createDataset,
  createExperiment,
  createPrompt,
  createPromptVersion,
  getDataset,
  getExperiment,
  getPrompt,
  listDatasets,
  listExperiments,
  listExperimentRuns,
  listPrompts,
  listPromptVersions,
  startExperimentRun,
} from '@/api/playground'

export interface PromptForm {
  name: string
  description?: string
  tags: string
}

export interface PromptVersionForm {
  semver?: string
  template: string
  variables?: string
  guardrails?: string
}

export const usePlaygroundStore = defineStore('playground', () => {
  const prompts = ref<Prompt[]>([])
  const promptsLoading = ref(false)
  const promptsError = ref<string | null>(null)
  const promptCache = ref<Record<string, Prompt>>({})
  const promptVersions = ref<Record<string, PromptVersion[]>>({})

  const datasets = ref<Dataset[]>([])
  const datasetsLoading = ref(false)
  const datasetsError = ref<string | null>(null)
  const datasetCache = ref<Record<string, Dataset>>({})

  const experiments = ref<ExperimentSpec[]>([])
  const experimentsLoading = ref(false)
  const experimentsError = ref<string | null>(null)
  const experimentCache = ref<Record<string, ExperimentSpec>>({})
const runsByExperiment = ref<Record<string, Run[]>>({})
const runsLoading = ref<Record<string, boolean>>({})
const runPollTimers = new Map<string, ReturnType<typeof setTimeout>>()

  const promptCount = computed(() => prompts.value.length)
  const datasetCount = computed(() => datasets.value.length)
  const experimentCount = computed(() => experiments.value.length)

  async function loadPrompts(params?: { q?: string; tag?: string }) {
    promptsLoading.value = true
    promptsError.value = null
    try {
      prompts.value = await listPrompts(params)
      for (const p of prompts.value) {
        promptCache.value[p.id] = p
      }
    } catch (err) {
      promptsError.value = extractErr(err, 'Failed to load prompts.')
    } finally {
      promptsLoading.value = false
    }
  }

  async function ensurePrompt(id: string): Promise<Prompt | null> {
    if (promptCache.value[id]) {
      return promptCache.value[id]
    }
    try {
      const prompt = await getPrompt(id)
      promptCache.value[id] = prompt
      return prompt
    } catch (err) {
      promptsError.value = extractErr(err, 'Failed to load prompt.')
      return null
    }
  }

  async function loadPromptVersions(promptId: string) {
    try {
      promptVersions.value[promptId] = await listPromptVersions(promptId)
    } catch (err) {
      promptsError.value = extractErr(err, 'Failed to load prompt versions.')
    }
  }

  async function addPrompt(payload: PromptForm) {
    const tags = payload.tags
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
    const created = await createPrompt({ name: payload.name, description: payload.description, tags })
    prompts.value = [created, ...prompts.value]
    promptCache.value[created.id] = created
  }

  async function addPromptVersion(promptId: string, payload: PromptVersionForm) {
    let variables: Record<string, any> | undefined
    if (payload.variables && payload.variables.trim().length > 0) {
      variables = JSON.parse(payload.variables)
    }
    let guardrails: Record<string, any> | undefined
    if (payload.guardrails && payload.guardrails.trim().length > 0) {
      guardrails = JSON.parse(payload.guardrails)
    }
    const created = await createPromptVersion(promptId, {
      semver: payload.semver,
      template: payload.template,
      variables,
      guardrails,
    })
    const existing = promptVersions.value[promptId] ?? []
    promptVersions.value[promptId] = [created, ...existing]
  }

  async function loadDatasets() {
    datasetsLoading.value = true
    datasetsError.value = null
    try {
      const items = await listDatasets()
      datasets.value = items
      for (const ds of items) {
        datasetCache.value[ds.id] = ds
      }
    } catch (err) {
      datasetsError.value = extractErr(err, 'Failed to load datasets.')
    } finally {
      datasetsLoading.value = false
    }
  }

  async function ensureDataset(id: string): Promise<Dataset | null> {
    if (datasetCache.value[id]) {
      return datasetCache.value[id]
    }
    try {
      const ds = await getDataset(id)
      datasetCache.value[id] = ds
      return ds
    } catch (err) {
      datasetsError.value = extractErr(err, 'Failed to load dataset.')
      return null
    }
  }

  async function addDataset(dataset: { name: string; description?: string; tags: string[] }, rows: DatasetRow[]) {
    const payload = await createDataset({ dataset, rows })
    datasets.value = [payload, ...datasets.value]
    datasetCache.value[payload.id] = payload
  }

  async function loadExperiments() {
    experimentsLoading.value = true
    experimentsError.value = null
    try {
      const items = await listExperiments()
      experiments.value = items
      for (const exp of items) {
        experimentCache.value[exp.id] = exp
      }
    } catch (err) {
      experimentsError.value = extractErr(err, 'Failed to load experiments.')
    } finally {
      experimentsLoading.value = false
    }
  }

  async function ensureExperiment(id: string): Promise<ExperimentSpec | null> {
    if (experimentCache.value[id]) {
      return experimentCache.value[id]
    }
    try {
      const spec = await getExperiment(id)
      experimentCache.value[id] = spec
      return spec
    } catch (err) {
      experimentsError.value = extractErr(err, 'Failed to load experiment.')
      return null
    }
  }

  async function addExperiment(spec: ExperimentSpec) {
    const created = await createExperiment(spec)
    experiments.value = [created, ...experiments.value]
    experimentCache.value[created.id] = created
  }

  async function refreshExperimentRuns(experimentId: string) {
    runsLoading.value[experimentId] = true
    try {
      runsByExperiment.value[experimentId] = await listExperimentRuns(experimentId)
      scheduleRunPolling(experimentId)
    } catch (err) {
      experimentsError.value = extractErr(err, 'Failed to load runs.')
    } finally {
      runsLoading.value[experimentId] = false
    }
  }

  async function triggerRun(experimentId: string) {
    await startExperimentRun(experimentId)
    await refreshExperimentRuns(experimentId)
  }

  function scheduleRunPolling(experimentId: string) {
    const runs = runsByExperiment.value[experimentId] ?? []
    const hasActive = runs.some((run) => run.status === 'pending' || run.status === 'running')
    if (hasActive) {
      clearRunPolling(experimentId)
      const handle = setTimeout(async () => {
        await refreshExperimentRuns(experimentId)
      }, 2000)
      runPollTimers.set(experimentId, handle)
    } else {
      clearRunPolling(experimentId)
    }
  }

  function clearRunPolling(experimentId: string) {
    const handle = runPollTimers.get(experimentId)
    if (handle) {
      clearTimeout(handle)
      runPollTimers.delete(experimentId)
    }
  }

  function extractErr(err: unknown, fallback: string): string {
    const anyErr = err as any
    return anyErr?.response?.data?.error || anyErr?.message || fallback
  }

  return {
    // state
    prompts,
    promptsLoading,
    promptsError,
    promptVersions,
    datasets,
    datasetsLoading,
    datasetsError,
    experiments,
    experimentsLoading,
    experimentsError,
    runsByExperiment,
    runsLoading,
    promptCount,
    datasetCount,
    experimentCount,
    // actions
    loadPrompts,
    ensurePrompt,
    loadPromptVersions,
    addPrompt,
    addPromptVersion,
    loadDatasets,
    ensureDataset,
    addDataset,
    loadExperiments,
    ensureExperiment,
    addExperiment,
    refreshExperimentRuns,
    triggerRun,
    clearRunPolling,
  }
})
