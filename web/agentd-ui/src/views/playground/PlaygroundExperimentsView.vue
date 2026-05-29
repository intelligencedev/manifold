<template>
  <!-- Desktop split layout. -->
  <div
    class="grid h-full min-h-0 grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)] gap-6 overflow-hidden"
  >
    <section class="flex h-full min-h-0 flex-col space-y-3 overflow-hidden">
      <header>
        <h2 class="text-lg font-semibold">New Experiment</h2>
        <p class="text-sm text-subtle-foreground">
          Select a dataset and prompt version to compare model outputs.
        </p>
      </header>
      <div class="flex-1 overflow-auto overscroll-contain pr-1">
        <form
          class="grid grid-cols-2 gap-3"
          @submit.prevent="handleCreateExperiment"
        >
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Name</span>
            <input
              v-model="form.name"
              required
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Dataset</span>
            <DropdownSelect
              v-model="form.datasetId"
              required
              aria-label="Dataset"
              class="w-full"
              :options="[
                { id: '', label: 'Select dataset', value: '', disabled: true },
                ...store.datasets.map((dataset) => ({
                  id: dataset.id,
                  label: dataset.name,
                  value: dataset.id,
                })),
              ]"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Prompt</span>
            <DropdownSelect
              v-model="form.promptId"
              required
              aria-label="Prompt"
              class="w-full"
              :options="[
                { id: '', label: 'Select prompt', value: '', disabled: true },
                ...store.prompts.map((prompt) => ({
                  id: prompt.id,
                  label: prompt.name,
                  value: prompt.id,
                })),
              ]"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Prompt version</span>
            <DropdownSelect
              v-model="form.promptVersionId"
              required
              aria-label="Prompt version"
              class="w-full"
              :options="[
                { id: '', label: 'Select version', value: '', disabled: true },
                ...availableVersions.map((version) => ({
                  id: version.id,
                  label: version.semver || version.id,
                  value: version.id,
                })),
              ]"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Model</span>
            <input
              v-model="form.model"
              :required="!form.specialistName"
              :disabled="Boolean(form.specialistName)"
              :placeholder="
                form.specialistName ? 'Specialist model is used' : 'gpt-4o'
              "
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
              :class="{ 'opacity-60': Boolean(form.specialistName) }"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Specialist runner</span>
            <DropdownSelect
              v-model="form.specialistName"
              aria-label="Specialist runner"
              class="w-full"
              :options="[
                { id: 'direct', label: 'Direct LLM', value: '' },
                ...activeSpecialists.map((specialist) => ({
                  id: specialist.name,
                  label: `${specialist.name}${specialist.model ? ` · ${specialist.model}` : ''}`,
                  value: specialist.name,
                })),
              ]"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Slice (optional)</span>
            <input
              v-model="form.sliceExpr"
              placeholder="validation"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            />
          </label>
          <label class="col-span-2 text-sm">
            <span class="text-subtle-foreground mb-1">Notes</span>
            <textarea
              v-model="form.notes"
              rows="2"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            ></textarea>
          </label>
          <div class="col-span-2 flex items-center gap-3">
            <AppButton type="submit" variant="accent">
              Create experiment
            </AppButton>
            <span v-if="createMessage" class="text-sm text-subtle-foreground">{{
              createMessage
            }}</span>
            <span v-if="createError" class="text-sm text-danger-foreground">{{
              createError
            }}</span>
          </div>
        </form>
      </div>
    </section>

    <section class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
      <header class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">Experiments</h2>
          <p class="text-sm text-subtle-foreground">
            Launch runs and inspect outcomes.
          </p>
        </div>
        <AppButton @click="store.loadExperiments" size="sm">
          Refresh
        </AppButton>
      </header>
      <div class="flex-1 min-h-0 overflow-auto overscroll-contain pr-1">
        <div
          v-if="store.experimentsLoading"
          class="text-sm text-subtle-foreground"
        >
          Loading experiments…
        </div>
        <div
          v-else-if="store.experiments.length === 0"
          class="text-sm text-subtle-foreground"
        >
          No experiments yet.
        </div>
        <div v-else class="space-y-3 min-w-0">
          <article
            v-for="experiment in store.experiments"
            :key="experiment.id"
            class="rounded-md border border-border/60 bg-surface-muted/60 p-4 space-y-2"
          >
            <div class="flex items-center justify-between gap-2">
              <div>
                <h3 class="text-base font-semibold">{{ experiment.name }}</h3>
                <p class="text-xs text-subtle-foreground">
                  Dataset: {{ experiment.datasetId }} · Variants:
                  {{ experiment.variants.length }} · Runner:
                  {{ runnerLabel(experiment.execution) }}
                </p>
              </div>
              <div class="flex gap-2">
                <RouterLink
                  :to="`/playground/experiments/${experiment.id}`"
                  class="rounded border border-border/70 px-3 py-2 text-sm"
                  >Details</RouterLink
                >
                <AppButton
                  @click="startRun(experiment.id)"
                  variant="accent"
                  size="sm"
                  :loading="store.runStarting[experiment.id] ?? false"
                  :pressed="store.isExperimentRunning(experiment.id)"
                >
                  {{
                    store.runStarting[experiment.id]
                      ? "Starting run"
                      : store.isExperimentRunning(experiment.id)
                        ? "Queue run"
                        : "Start run"
                  }}
                </AppButton>
                <AppButton
                  @click="deleteExperiment(experiment.id)"
                  variant="danger"
                  size="sm"
                >
                  Delete
                </AppButton>
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2 text-sm">
              <span class="text-subtle-foreground">
                Created {{ formatDate(experiment.createdAt) }}
              </span>
              <span
                v-if="store.runStarting[experiment.id]"
                class="inline-flex items-center gap-2 rounded-full border border-accent/40 bg-accent/12 px-2.5 py-1 text-xs font-medium text-foreground"
              >
                <span
                  class="h-2 w-2 animate-pulse rounded-full bg-accent"
                ></span>
                Starting run…
              </span>
              <span
                v-else-if="store.isExperimentRunning(experiment.id)"
                class="inline-flex items-center gap-2 rounded-full border border-warning/35 bg-warning/10 px-2.5 py-1 text-xs font-medium text-warning"
              >
                <span
                  class="h-2 w-2 animate-pulse rounded-full bg-warning"
                ></span>
                Run in progress
              </span>
            </div>
            <p
              v-if="runErrors[experiment.id]"
              class="text-sm text-danger-foreground"
            >
              {{ runErrors[experiment.id] }}
            </p>
            <div class="text-sm">
              <AppButton
                @click="toggleRuns(experiment.id)"
                variant="ghost"
                size="xs"
                class="px-0 text-accent hover:bg-transparent hover:text-accent"
              >
                {{ expandedRun[experiment.id] ? "Hide runs" : "Show runs" }}
              </AppButton>
            </div>
            <div
              v-if="expandedRun[experiment.id]"
              class="rounded border border-border/60 bg-surface"
            >
              <div
                v-if="store.runsLoading[experiment.id]"
                class="p-3 text-sm text-subtle-foreground"
              >
                Loading runs…
              </div>
              <div
                v-else
                class="max-h-60 overflow-auto overscroll-contain pr-1"
              >
                <table class="w-full text-sm">
                  <thead class="sticky top-0 bg-surface text-subtle-foreground">
                    <tr>
                      <th class="text-left py-2">Run</th>
                      <th class="text-left py-2">Status</th>
                      <th class="text-left py-2">Started</th>
                      <th class="text-left py-2">Completed</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="run in store.runsByExperiment[experiment.id] ?? []"
                      :key="run.id"
                      class="border-t border-border/60"
                    >
                      <td class="py-2 text-sm">{{ run.id }}</td>
                      <td class="py-2 capitalize">{{ run.status }}</td>
                      <td class="py-2">{{ formatDate(run.startedAt) }}</td>
                      <td class="py-2">{{ formatDate(run.endedAt) }}</td>
                    </tr>
                    <tr
                      v-if="
                        (store.runsByExperiment[experiment.id] ?? []).length ===
                        0
                      "
                    >
                      <td
                        colspan="4"
                        class="py-2 text-sm text-subtle-foreground"
                      >
                        No runs yet.
                      </td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </article>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from "vue-router";
import { computed, onMounted, reactive, ref, watch } from "vue";
import { usePlaygroundStore } from "@/stores/playground";
import DropdownSelect from "@/components/DropdownSelect.vue";
import AppButton from "@/components/ui/AppButton.vue";
import { listSpecialists, type Specialist } from "@/api/client";
import type { ExecutionConfig } from "@/api/playground";

const store = usePlaygroundStore();
const form = reactive({
  name: "",
  datasetId: "",
  promptId: "",
  promptVersionId: "",
  model: "",
  specialistName: "",
  sliceExpr: "",
  notes: "",
});
const createMessage = ref("");
const createError = ref("");
const expandedRun = reactive<Record<string, boolean>>({});
const runErrors = reactive<Record<string, string>>({});
const availableVersions = ref(store.promptVersions[form.promptId] ?? []);
const specialists = ref<Specialist[]>([]);
const activeSpecialists = computed(() =>
  specialists.value
    .filter(
      (specialist) =>
        !specialist.paused &&
        specialist.name.trim().toLowerCase() !== "orchestrator",
    )
    .sort((a, b) => a.name.localeCompare(b.name)),
);

onMounted(async () => {
  if (!store.prompts.length) await store.loadPrompts();
  if (!store.datasets.length) await store.loadDatasets();
  await loadSpecialists();
  await store.loadExperiments();
});

watch(
  () => form.promptId,
  async (next) => {
    if (!next) {
      availableVersions.value = [];
      form.promptVersionId = "";
      return;
    }
    await store.loadPromptVersions(next);
    availableVersions.value = store.promptVersions[next] ?? [];
  },
);

async function handleCreateExperiment() {
  createError.value = "";
  if (!form.datasetId || !form.promptVersionId) {
    createError.value = "Dataset and prompt version are required.";
    return;
  }
  if (!form.specialistName && !form.model.trim()) {
    createError.value = "Model is required for direct LLM experiments.";
    return;
  }
  try {
    const now = new Date().toISOString();
    const variantId = crypto.randomUUID();
    const spec = {
      id: crypto.randomUUID(),
      name: form.name,
      datasetId: form.datasetId,
      sliceExpr: form.sliceExpr || undefined,
      variants: [
        {
          id: variantId,
          promptVersionId: form.promptVersionId,
          model: form.specialistName ? "" : form.model,
          params: {},
        },
      ],
      evaluators: [],
      budgets: {},
      concurrency: {},
      execution: form.specialistName
        ? { specialistName: form.specialistName }
        : undefined,
      createdAt: now,
      createdBy: "ui",
    };
    await store.addExperiment(spec);
    createMessage.value = "Experiment created.";
    form.name = "";
    form.sliceExpr = "";
    form.promptId = "";
    form.promptVersionId = "";
    form.model = "";
    form.specialistName = "";
    form.notes = "";
    setTimeout(() => (createMessage.value = ""), 3_000);
  } catch (err) {
    createError.value = extractErr(err);
  }
}

async function loadSpecialists() {
  try {
    specialists.value = await listSpecialists();
  } catch (err) {
    createError.value = extractErr(err);
  }
}

function extractErr(err: unknown): string {
  const anyErr = err as any;
  if (anyErr?.response?.data?.error) return anyErr.response.data.error;
  return anyErr?.message || "Failed to create experiment.";
}

async function startRun(experimentId: string) {
  runErrors[experimentId] = "";
  try {
    await store.triggerRun(experimentId);
    expandedRun[experimentId] = true;
  } catch (err) {
    runErrors[experimentId] = extractErr(err);
  }
}

async function deleteExperiment(id: string) {
  const ok = window.confirm("Delete this experiment and its runs/results?");
  if (!ok) return;
  await store.removeExperiment(id);
}

async function toggleRuns(experimentId: string) {
  const next = !expandedRun[experimentId];
  expandedRun[experimentId] = next;
  if (next) {
    await store.refreshExperimentRuns(experimentId);
  } else {
    store.clearRunPolling(experimentId);
  }
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

function runnerLabel(execution?: ExecutionConfig) {
  const name = execution?.specialistName?.trim();
  return name ? `Specialist: ${name}` : "Direct LLM";
}
</script>
