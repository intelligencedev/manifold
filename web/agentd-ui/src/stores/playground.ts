import { defineStore } from "pinia";
import { ref, computed } from "vue";
import {
  type Prompt,
  type PromptVersion,
  type Dataset,
  type DatasetRow,
  type ExperimentSpec,
  type Run,
  type RunResult,
  createDataset,
  updateDataset,
  createExperiment,
  deleteExperiment,
  createPrompt,
  deletePrompt,
  createPromptVersion,
  getDataset,
  getExperiment,
  getPrompt,
  listDatasets,
  listExperiments,
  listExperimentRuns,
  listRunResults,
  listPrompts,
  listPromptVersions,
  startExperimentRun,
  deleteDataset,
} from "@/api/playground";

export interface PromptForm {
  name: string;
  description?: string;
  tags: string;
}

export interface PromptVersionForm {
  semver?: string;
  template: string;
  variables?: string;
  guardrails?: string;
}

export const usePlaygroundStore = defineStore("playground", () => {
  const prompts = ref<Prompt[]>([]);
  const promptsLoading = ref(false);
  const promptsError = ref<string | null>(null);
  const promptCache = ref<Record<string, Prompt>>({});
  const promptVersions = ref<Record<string, PromptVersion[]>>({});

  const datasets = ref<Dataset[]>([]);
  const datasetsLoading = ref(false);
  const datasetsError = ref<string | null>(null);
  const datasetCache = ref<Record<string, Dataset>>({});

  const experiments = ref<ExperimentSpec[]>([]);
  const experimentsLoading = ref(false);
  const experimentsError = ref<string | null>(null);
  const experimentCache = ref<Record<string, ExperimentSpec>>({});
  const runsByExperiment = ref<Record<string, Run[]>>({});
  const runsLoading = ref<Record<string, boolean>>({});
  const runResultsByRun = ref<Record<string, RunResult[]>>({});
  const runResultsLoading = ref<Record<string, boolean>>({});
  const runPollTimers = new Map<string, ReturnType<typeof setTimeout>>();

  const promptCount = computed(() => prompts.value.length);
  const datasetCount = computed(() => datasets.value.length);
  const experimentCount = computed(() => experiments.value.length);

  async function loadPrompts(params?: { q?: string; tag?: string }) {
    promptsLoading.value = true;
    promptsError.value = null;
    try {
      prompts.value = await listPrompts(params);
      for (const p of prompts.value) {
        promptCache.value[p.id] = p;
      }
    } catch (err) {
      promptsError.value = extractErr(err, "Failed to load prompts.");
    } finally {
      promptsLoading.value = false;
    }
  }

  async function ensurePrompt(id: string): Promise<Prompt | null> {
    if (promptCache.value[id]) {
      return promptCache.value[id];
    }
    try {
      const prompt = await getPrompt(id);
      promptCache.value[id] = prompt;
      return prompt;
    } catch (err) {
      promptsError.value = extractErr(err, "Failed to load prompt.");
      return null;
    }
  }

  async function loadPromptVersions(promptId: string) {
    try {
      promptVersions.value[promptId] = await listPromptVersions(promptId);
    } catch (err) {
      promptsError.value = extractErr(err, "Failed to load prompt versions.");
    }
  }

  async function addPrompt(payload: PromptForm) {
    const tags = payload.tags
      .split(",")
      .map((t) => t.trim())
      .filter(Boolean);
    const created = await createPrompt({
      name: payload.name,
      description: payload.description,
      tags,
    });
    prompts.value = [created, ...prompts.value];
    promptCache.value[created.id] = created;
  }

  async function removePrompt(id: string) {
    await deletePrompt(id);
    prompts.value = prompts.value.filter((p) => p.id !== id);
    delete promptCache.value[id];
    delete promptVersions.value[id];
  }

  async function addPromptVersion(
    promptId: string,
    payload: PromptVersionForm,
  ) {
    let variables: Record<string, any> | undefined;
    if (payload.variables && payload.variables.trim().length > 0) {
      variables = JSON.parse(payload.variables);
    }
    let guardrails: Record<string, any> | undefined;
    if (payload.guardrails && payload.guardrails.trim().length > 0) {
      guardrails = JSON.parse(payload.guardrails);
    }
    const created = await createPromptVersion(promptId, {
      semver: payload.semver,
      template: payload.template,
      variables,
      guardrails,
    });
    const existing = promptVersions.value[promptId] ?? [];
    promptVersions.value[promptId] = [created, ...existing];
  }

  async function loadDatasets() {
    datasetsLoading.value = true;
    datasetsError.value = null;
    try {
      const items = await listDatasets();
      datasets.value = items;
      for (const ds of items) {
        const cached = datasetCache.value[ds.id];
        const merged: Dataset = {
          ...ds,
          metadata: ds.metadata ?? cached?.metadata,
          rows: cached?.rows ? [...cached.rows] : undefined,
        };
        datasetCache.value[ds.id] = merged;
      }
    } catch (err) {
      datasetsError.value = extractErr(err, "Failed to load datasets.");
    } finally {
      datasetsLoading.value = false;
    }
  }

  async function ensureDataset(
    id: string,
    options?: { force?: boolean },
  ): Promise<Dataset | null> {
    const force = options?.force ?? false;
    const cached = datasetCache.value[id];
    if (!force && cached?.rows) {
      return cached;
    }
    try {
      const ds = await getDataset(id);
      const merged: Dataset = {
        ...ds,
        metadata: ds.metadata ?? cached?.metadata,
        rows: ds.rows
          ? [...ds.rows]
          : cached?.rows
            ? [...cached.rows]
            : undefined,
      };
      datasetCache.value[id] = merged;
      datasets.value = datasets.value.map((item) =>
        item.id === id ? { ...item, ...ds } : item,
      );
      return merged;
    } catch (err) {
      datasetsError.value = extractErr(err, "Failed to load dataset.");
      return null;
    }
  }

  async function addDataset(
    dataset: { name: string; description?: string; tags: string[] },
    rows: DatasetRow[],
  ) {
    const payload = await createDataset({ dataset, rows });
    datasets.value = [payload, ...datasets.value];
    datasetCache.value[payload.id] = { ...payload, rows: [...rows] };
  }

  async function removeDataset(id: string) {
    await deleteDataset(id);
    datasets.value = datasets.value.filter((d) => d.id !== id);
    delete datasetCache.value[id];
  }

  async function saveDataset(
    id: string,
    dataset: { name: string; description?: string; tags: string[] },
    rows: DatasetRow[],
  ): Promise<Dataset> {
    const response = await updateDataset(id, {
      dataset: { ...dataset, id },
      rows,
    });
    const existing = datasetCache.value[id];
    const merged: Dataset = {
      ...(existing ?? {}),
      ...response,
      metadata: response.metadata ?? existing?.metadata,
      rows: response.rows ? [...response.rows] : [...rows],
    };
    datasetCache.value[id] = merged;
    datasets.value = datasets.value.map((item) =>
      item.id === id ? { ...item, ...response } : item,
    );
    return merged;
  }

  async function loadExperiments() {
    experimentsLoading.value = true;
    experimentsError.value = null;
    try {
      const items = await listExperiments();
      experiments.value = items;
      for (const exp of items) {
        experimentCache.value[exp.id] = exp;
      }
    } catch (err) {
      experimentsError.value = extractErr(err, "Failed to load experiments.");
    } finally {
      experimentsLoading.value = false;
    }
  }

  async function ensureExperiment(id: string): Promise<ExperimentSpec | null> {
    if (experimentCache.value[id]) {
      return experimentCache.value[id];
    }
    try {
      const spec = await getExperiment(id);
      experimentCache.value[id] = spec;
      return spec;
    } catch (err) {
      experimentsError.value = extractErr(err, "Failed to load experiment.");
      return null;
    }
  }

  async function addExperiment(spec: ExperimentSpec) {
    const created = await createExperiment(spec);
    experiments.value = [created, ...experiments.value];
    experimentCache.value[created.id] = created;
  }

  async function removeExperiment(id: string) {
    await deleteExperiment(id);
    experiments.value = experiments.value.filter((e) => e.id !== id);
    delete experimentCache.value[id];
    delete runsByExperiment.value[id];
  }

  async function refreshExperimentRuns(experimentId: string) {
    runsLoading.value[experimentId] = true;
    try {
      runsByExperiment.value[experimentId] =
        await listExperimentRuns(experimentId);
      scheduleRunPolling(experimentId);
    } catch (err) {
      experimentsError.value = extractErr(err, "Failed to load runs.");
    } finally {
      runsLoading.value[experimentId] = false;
    }
  }

  async function triggerRun(experimentId: string) {
    await startExperimentRun(experimentId);
    await refreshExperimentRuns(experimentId);
  }

  async function ensureRunResults(runId: string) {
    if (runResultsByRun.value[runId]) {
      return runResultsByRun.value[runId];
    }
    return refreshRunResults(runId);
  }

  async function refreshRunResults(runId: string) {
    runResultsLoading.value[runId] = true;
    try {
      const results = await listRunResults(runId);
      runResultsByRun.value[runId] = results;
      return results;
    } catch (err) {
      experimentsError.value = extractErr(err, "Failed to load run results.");
      throw err;
    } finally {
      runResultsLoading.value[runId] = false;
    }
  }

  function scheduleRunPolling(experimentId: string) {
    const runs = runsByExperiment.value[experimentId] ?? [];
    const hasActive = runs.some(
      (run) => run.status === "pending" || run.status === "running",
    );
    if (hasActive) {
      clearRunPolling(experimentId);
      const handle = setTimeout(async () => {
        await refreshExperimentRuns(experimentId);
      }, 2000);
      runPollTimers.set(experimentId, handle);
    } else {
      clearRunPolling(experimentId);
    }
  }

  function clearRunPolling(experimentId: string) {
    const handle = runPollTimers.get(experimentId);
    if (handle) {
      clearTimeout(handle);
      runPollTimers.delete(experimentId);
    }
  }

  function extractErr(err: unknown, fallback: string): string {
    const anyErr = err as any;
    return anyErr?.response?.data?.error || anyErr?.message || fallback;
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
    runResultsByRun,
    runResultsLoading,
    promptCount,
    datasetCount,
    experimentCount,
    // actions
    loadPrompts,
    ensurePrompt,
    loadPromptVersions,
    addPrompt,
    removePrompt,
    addPromptVersion,
    loadDatasets,
    ensureDataset,
    addDataset,
    removeDataset,
    saveDataset,
    loadExperiments,
    ensureExperiment,
    addExperiment,
    removeExperiment,
    refreshExperimentRuns,
    triggerRun,
    ensureRunResults,
    refreshRunResults,
    clearRunPolling,
  };
});
