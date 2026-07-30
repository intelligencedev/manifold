<template>
  <div class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
    <!-- Toolbar -->
    <header class="flex shrink-0 flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-3">
        <div class="relative">
          <svg
            class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-faint-foreground"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            aria-hidden="true"
          >
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.5-3.5" />
          </svg>
          <input
            v-model="search"
            type="search"
            placeholder="Search name or dataset"
            aria-label="Search experiments"
            class="halo-focus w-72 rounded-md border border-[rgb(var(--line-strong))] bg-surface py-2 pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-faint-foreground"
          />
        </div>
        <span class="font-mono text-xs text-faint-foreground">{{ countLabel }}</span>
      </div>

      <div class="flex items-center gap-2">
        <button
          type="button"
          aria-label="Refresh experiments"
          class="halo-focus rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted p-2 text-muted-foreground transition-colors hover:bg-input hover:text-foreground"
          @click="store.loadExperiments()"
        >
          <svg
            class="h-4 w-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M3 12a9 9 0 0 1 15-6.7L21 8" />
            <path d="M21 3v5h-5" />
            <path d="M21 12a9 9 0 0 1-15 6.7L3 16" />
            <path d="M3 21v-5h5" />
          </svg>
        </button>
        <AppButton variant="accent" size="sm" @click="toggleCreate">
          <svg
            class="h-4 w-4"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
            stroke-linecap="round"
            aria-hidden="true"
          >
            <path d="M12 5v14M5 12h14" />
          </svg>
          New Experiment
        </AppButton>
      </div>
    </header>

    <!-- Create panel (collapsible) -->
    <Transition name="reveal">
      <GlassCard v-if="showCreate" as="section" subtle padded class="shrink-0 p-5">
        <form class="grid grid-cols-2 gap-4" @submit.prevent="handleCreateExperiment">
          <div class="col-span-2 flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold">New Experiment</h3>
              <p class="text-xs text-subtle-foreground">
                Pair a dataset with a prompt version and a model or specialist to compare outputs.
              </p>
            </div>
            <button
              type="button"
              class="rounded-md p-1.5 text-faint-foreground transition-colors hover:bg-surface-muted hover:text-foreground"
              aria-label="Close create panel"
              @click="closeCreate"
            >
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
                <path d="M18 6 6 18M6 6l12 12" />
              </svg>
            </button>
          </div>

          <label class="col-span-2 flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Name</span>
            <input v-model="form.name" required placeholder="e.g. gsm8k · opus vs sonnet" :class="fieldClass" />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Dataset</span>
            <DropdownSelect
              v-model="form.datasetId"
              required
              aria-label="Dataset"
              class="w-full"
              :options="[
                { id: '', label: 'Select dataset', value: '', disabled: true },
                ...store.datasets.map((dataset) => ({ id: dataset.id, label: dataset.name, value: dataset.id })),
              ]"
            />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">
              Slice <span class="text-faint-foreground">(optional)</span>
            </span>
            <input v-model="form.sliceExpr" placeholder="validation" :class="fieldClass" />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Prompt</span>
            <DropdownSelect
              v-model="form.promptId"
              required
              aria-label="Prompt"
              class="w-full"
              :options="[
                { id: '', label: 'Select prompt', value: '', disabled: true },
                ...store.prompts.map((prompt) => ({ id: prompt.id, label: prompt.name, value: prompt.id })),
              ]"
            />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Prompt version</span>
            <DropdownSelect
              v-model="form.promptVersionId"
              required
              aria-label="Prompt version"
              class="w-full"
              :options="[
                { id: '', label: form.promptId ? 'Select version' : 'Select a prompt first', value: '', disabled: true },
                ...availableVersions.map((version) => ({ id: version.id, label: version.semver || version.id, value: version.id })),
              ]"
            />
          </label>

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Specialist runner</span>
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

          <label class="flex flex-col gap-1.5 text-sm">
            <span class="text-xs font-medium text-subtle-foreground">Model</span>
            <input
              v-model="form.model"
              :required="!form.specialistName"
              :disabled="Boolean(form.specialistName)"
              :placeholder="form.specialistName ? 'Specialist model is used' : 'gpt-4o'"
              :class="[fieldClass, form.specialistName ? 'cursor-not-allowed opacity-55' : '']"
            />
          </label>

          <div class="col-span-2 flex items-center gap-3">
            <AppButton type="submit" variant="accent" size="sm">Create experiment</AppButton>
            <AppButton type="button" variant="ghost" size="sm" @click="closeCreate">Cancel</AppButton>
            <Transition name="fade">
              <span v-if="createMessage" class="text-xs font-medium text-accent">{{ createMessage }}</span>
            </Transition>
            <Transition name="fade">
              <span v-if="createError" class="text-xs font-medium text-danger">{{ createError }}</span>
            </Transition>
          </div>
        </form>
      </GlassCard>
    </Transition>

    <!-- Error banner -->
    <div
      v-if="store.experimentsError"
      class="shrink-0 rounded-md border border-danger/50 bg-[rgb(var(--color-danger)_/_0.1)] px-3 py-2 text-sm text-danger"
    >
      {{ store.experimentsError }}
    </div>

    <!-- Grid -->
    <div class="-mx-1 min-h-0 flex-1 overflow-y-auto overscroll-contain px-1">
      <!-- Loading skeletons -->
      <div
        v-if="store.experimentsLoading && !store.experiments.length"
        class="grid gap-3"
        :style="gridStyle"
      >
        <div v-for="n in 4" :key="n" class="halo-surface halo-surface-2 animate-pulse p-5">
          <div class="flex items-center gap-3">
            <div class="h-9 w-9 rounded-md bg-surface-muted"></div>
            <div class="h-4 flex-1 rounded bg-surface-muted"></div>
          </div>
          <div class="mt-4 h-3 w-3/4 rounded bg-surface-muted"></div>
          <div class="mt-2 h-3 w-1/2 rounded bg-surface-muted"></div>
        </div>
      </div>

      <!-- Empty -->
      <div
        v-else-if="!store.experiments.length"
        class="flex h-full flex-col items-center justify-center gap-4 text-center"
      >
        <div class="flex h-14 w-14 items-center justify-center rounded-xl border border-[rgb(var(--line-strong))] bg-surface-muted text-accent">
          <svg class="h-7 w-7" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M9 3h6M10 3v5.6L5.2 18a1.5 1.5 0 0 0 1.3 2.3h11a1.5 1.5 0 0 0 1.3-2.3L14 8.6V3" />
            <path d="M7.5 14h9" />
          </svg>
        </div>
        <div>
          <p class="text-sm font-semibold">No experiments yet</p>
          <p class="mx-auto mt-1 max-w-sm text-sm text-subtle-foreground">
            Pair a dataset with a prompt version, run it against a model or specialist, and compare the outputs.
          </p>
        </div>
        <AppButton variant="accent" size="sm" @click="openCreate">Create your first experiment</AppButton>
      </div>

      <!-- No search matches -->
      <div
        v-else-if="!filteredExperiments.length"
        class="flex h-full flex-col items-center justify-center gap-3 text-center"
      >
        <p class="text-sm font-medium text-subtle-foreground">No experiments match “{{ search }}”.</p>
        <AppButton variant="ghost" size="sm" @click="search = ''">Clear search</AppButton>
      </div>

      <!-- Cards -->
      <div v-else class="grid items-stretch gap-3" :style="gridStyle">
        <GlassCard
          v-for="experiment in filteredExperiments"
          :key="experiment.id"
          as="article"
          padded
          class="group relative flex min-h-[14rem] flex-col gap-3.5 p-5"
        >
          <!-- Header -->
          <div class="flex items-start gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted text-accent">
              <svg class="h-[18px] w-[18px]" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M9 3h6M10 3v5.6L5.2 18a1.5 1.5 0 0 0 1.3 2.3h11a1.5 1.5 0 0 0 1.3-2.3L14 8.6V3" />
                <path d="M7.5 14h9" />
              </svg>
            </div>
            <div class="min-w-0 flex-1 pt-0.5">
              <h3 class="truncate text-sm font-semibold leading-tight text-foreground">{{ experiment.name }}</h3>
              <p class="mt-0.5 truncate font-mono text-[11px] text-faint-foreground">{{ shortId(experiment.id) }}</p>
            </div>
            <button
              class="relative z-20 -mr-1 -mt-1 rounded-md p-1.5 text-faint-foreground opacity-0 transition-all hover:bg-[rgb(var(--color-danger)_/_0.12)] hover:text-danger focus-visible:opacity-100 group-hover:opacity-100"
              :aria-label="`Delete experiment ${experiment.name}`"
              @click="deleteExperiment(experiment.id)"
            >
              <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />
              </svg>
            </button>
          </div>

          <!-- Status -->
          <div v-if="statusOf(experiment.id)" class="-mt-1">
            <span
              v-if="statusOf(experiment.id) === 'starting'"
              class="inline-flex items-center gap-2 rounded-full border border-accent/40 bg-accent/12 px-2.5 py-1 text-[11px] font-medium text-foreground"
            >
              <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-accent"></span>
              Starting run…
            </span>
            <span
              v-else
              class="inline-flex items-center gap-2 rounded-full border border-warning/35 bg-warning/10 px-2.5 py-1 text-[11px] font-medium text-warning"
            >
              <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-warning"></span>
              Run in progress
            </span>
          </div>

          <!-- Facts -->
          <dl class="space-y-1.5 text-xs">
            <div class="flex items-center gap-2">
              <dt class="w-16 shrink-0 text-faint-foreground">Dataset</dt>
              <dd class="min-w-0 flex-1 truncate text-muted-foreground">{{ datasetName(experiment.datasetId) }}</dd>
            </div>
            <div class="flex items-center gap-2">
              <dt class="w-16 shrink-0 text-faint-foreground">Variants</dt>
              <dd class="min-w-0 flex-1 truncate text-muted-foreground">
                {{ experiment.variants.length }} variant{{ experiment.variants.length === 1 ? "" : "s" }}
              </dd>
            </div>
            <div class="flex items-center gap-2">
              <dt class="w-16 shrink-0 text-faint-foreground">Runner</dt>
              <dd class="min-w-0 flex-1 truncate text-muted-foreground">{{ runnerLabel(experiment.execution) }}</dd>
            </div>
            <div v-if="experiment.sliceExpr" class="flex items-center gap-2">
              <dt class="w-16 shrink-0 text-faint-foreground">Slice</dt>
              <dd class="min-w-0 flex-1 truncate font-mono text-muted-foreground">{{ experiment.sliceExpr }}</dd>
            </div>
          </dl>

          <p
            v-if="runErrors[experiment.id]"
            class="line-clamp-3 break-words text-xs text-danger"
            :title="runErrors[experiment.id]"
          >
            {{ runErrors[experiment.id] }}
          </p>

          <!-- Footer -->
          <div class="mt-auto flex items-center justify-between gap-2 border-t border-[rgb(var(--line-soft))] pt-3">
            <span class="font-mono text-[11px] text-faint-foreground">{{ formatDate(experiment.createdAt) }}</span>
            <div class="flex items-center gap-2">
              <RouterLink
                :to="`/playground/experiments/${experiment.id}`"
                class="halo-focus inline-flex items-center gap-1 rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted px-2.5 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-input"
              >
                Details
              </RouterLink>
              <AppButton
                variant="accent"
                size="xs"
                :loading="store.runStarting[experiment.id] ?? false"
                :pressed="store.isExperimentRunning(experiment.id)"
                @click="startRun(experiment.id)"
              >
                {{
                  store.runStarting[experiment.id]
                    ? "Starting"
                    : store.isExperimentRunning(experiment.id)
                      ? "Queue run"
                      : "Start run"
                }}
              </AppButton>
            </div>
          </div>
        </GlassCard>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from "vue-router";
import {
  computed,
  onBeforeUnmount,
  onMounted,
  reactive,
  ref,
  watch,
} from "vue";
import { usePlaygroundStore } from "@/stores/playground";
import DropdownSelect from "@/components/DropdownSelect.vue";
import AppButton from "@/components/ui/AppButton.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
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
});
const createMessage = ref("");
const createError = ref("");
const search = ref("");
const showCreate = ref(false);
const runErrors = reactive<Record<string, string>>({});
const availableVersions = ref(store.promptVersions[form.promptId] ?? []);
const specialists = ref<Specialist[]>([]);

const fieldClass =
  "halo-focus w-full rounded-md border border-[rgb(var(--line-strong))] bg-surface px-[13px] py-2.5 font-sans text-sm text-foreground outline-none placeholder:text-faint-foreground";

const gridStyle = {
  gridTemplateColumns: "repeat(auto-fill, minmax(300px, 1fr))",
};

const activeSpecialists = computed(() =>
  specialists.value
    .filter(
      (specialist) =>
        !specialist.paused &&
        specialist.name.trim().toLowerCase() !== "orchestrator",
    )
    .sort((a, b) => a.name.localeCompare(b.name)),
);

const filteredExperiments = computed(() => {
  if (!search.value) return store.experiments;
  const term = search.value.toLowerCase();
  return store.experiments.filter(
    (exp) =>
      exp.name.toLowerCase().includes(term) ||
      datasetName(exp.datasetId).toLowerCase().includes(term),
  );
});

const countLabel = computed(() => {
  const total = store.experiments.length;
  if (!total) return "";
  if (search.value && filteredExperiments.value.length !== total) {
    return `${filteredExperiments.value.length} of ${total}`;
  }
  return `${total} experiment${total === 1 ? "" : "s"}`;
});

onMounted(async () => {
  if (!store.prompts.length) await store.loadPrompts();
  if (!store.datasets.length) await store.loadDatasets();
  await loadSpecialists();
  await store.loadExperiments();
});

onBeforeUnmount(() => {
  for (const experiment of store.experiments) {
    store.clearRunPolling(experiment.id);
  }
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

function openCreate() {
  showCreate.value = true;
}

function closeCreate() {
  showCreate.value = false;
}

function toggleCreate() {
  showCreate.value ? closeCreate() : openCreate();
}

function shortId(id: string) {
  return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
}

function datasetName(id: string) {
  return store.datasets.find((d) => d.id === id)?.name ?? shortId(id);
}

function statusOf(experimentId: string): "starting" | "running" | null {
  if (store.runStarting[experimentId]) return "starting";
  if (store.isExperimentRunning(experimentId)) return "running";
  return null;
}

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
  } catch (err) {
    runErrors[experimentId] = extractErr(err);
  }
}

async function deleteExperiment(id: string) {
  const ok = window.confirm("Delete this experiment and its runs/results?");
  if (!ok) return;
  await store.removeExperiment(id);
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
}

function runnerLabel(execution?: ExecutionConfig) {
  const name = execution?.specialistName?.trim();
  return name ? `Specialist: ${name}` : "Direct LLM";
}
</script>

<style scoped>
.reveal-enter-active,
.reveal-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}
.reveal-enter-from,
.reveal-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
