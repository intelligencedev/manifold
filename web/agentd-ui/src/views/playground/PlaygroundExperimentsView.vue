<template>
  <!-- Responsive, non-overflowing layout: stack on small screens; split columns on large -->
  <div
    class="flex h-full min-h-0 flex-col gap-6 overflow-hidden lg:grid lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]"
  >
    <section
      class="flex min-h-0 max-h-[55vh] flex-col overflow-hidden rounded-2xl border border-border/70 bg-surface p-4 space-y-3 lg:h-full lg:max-h-none"
    >
      <header>
        <h2 class="text-lg font-semibold">New Experiment</h2>
        <p class="text-sm text-subtle-foreground">
          Select a dataset and prompt version to compare model outputs.
        </p>
      </header>
      <div class="flex-1 overflow-auto overscroll-contain pr-1">
        <form
          class="grid gap-3 md:grid-cols-2"
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
              required
              placeholder="gpt-4o"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
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
          <label class="text-sm md:col-span-2">
            <span class="text-subtle-foreground mb-1">Notes</span>
            <textarea
              v-model="form.notes"
              rows="2"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            ></textarea>
          </label>
          <div class="md:col-span-2 flex gap-3 items-center">
            <button
              type="submit"
              class="rounded border border-border/70 px-3 py-2 text-sm font-semibold"
            >
              Create experiment
            </button>
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

    <section
      class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border/70 bg-surface p-4 gap-4 lg:h-full"
    >
      <header class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">Experiments</h2>
          <p class="text-sm text-subtle-foreground">
            Launch runs and inspect outcomes.
          </p>
        </div>
        <button
          @click="store.loadExperiments"
          class="rounded border border-border/70 px-3 py-2 text-sm"
        >
          Refresh
        </button>
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
            class="rounded-xl border border-border/60 bg-surface-muted/60 p-4 space-y-2"
          >
            <div
              class="flex flex-col md:flex-row md:items-center md:justify-between gap-2"
            >
              <div>
                <h3 class="text-base font-semibold">{{ experiment.name }}</h3>
                <p class="text-xs text-subtle-foreground">
                  Dataset: {{ experiment.datasetId }} · Variants:
                  {{ experiment.variants.length }}
                </p>
              </div>
              <div class="flex gap-2">
                <RouterLink
                  :to="`/playground/experiments/${experiment.id}`"
                  class="rounded border border-border/70 px-3 py-2 text-sm"
                  >Details</RouterLink
                >
                <button
                  @click="startRun(experiment.id)"
                  class="rounded border border-border/70 px-3 py-2 text-sm"
                >
                  Start run
                </button>
                <button
                  @click="deleteExperiment(experiment.id)"
                  class="rounded border border-danger/60 text-danger/60 px-3 py-2 text-sm"
                >
                  Delete
                </button>
              </div>
            </div>
            <div class="text-sm text-subtle-foreground">
              Created {{ formatDate(experiment.createdAt) }}
            </div>
            <div class="text-sm">
              <button
                @click="toggleRuns(experiment.id)"
                class="text-accent text-sm hover:underline"
              >
                {{ expandedRun[experiment.id] ? "Hide runs" : "Show runs" }}
              </button>
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

const store = usePlaygroundStore();
const form = reactive({
  name: "",
  datasetId: "",
  promptId: "",
  promptVersionId: "",
  model: "",
  sliceExpr: "",
  notes: "",
});
const createMessage = ref("");
const createError = ref("");
const expandedRun = reactive<Record<string, boolean>>({});
const availableVersions = ref(store.promptVersions[form.promptId] ?? []);

onMounted(async () => {
  if (!store.prompts.length) await store.loadPrompts();
  if (!store.datasets.length) await store.loadDatasets();
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
          model: form.model,
          params: {},
        },
      ],
      evaluators: [],
      budgets: {},
      concurrency: {},
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
    form.notes = "";
    setTimeout(() => (createMessage.value = ""), 3_000);
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
  await store.triggerRun(experimentId);
  expandedRun[experimentId] = true;
  await store.refreshExperimentRuns(experimentId);
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
</script>
