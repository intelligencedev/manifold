<template>
  <div class="flex h-full min-h-0 flex-col gap-4 overflow-hidden">
    <!-- ================= LIST MODE ================= -->
    <template v-if="!selectedDatasetId">
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
              placeholder="Search name or tag"
              aria-label="Search datasets"
              class="halo-focus w-72 rounded-md border border-[rgb(var(--line-strong))] bg-surface py-2 pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-faint-foreground"
            />
          </div>
          <span class="font-mono text-xs text-faint-foreground">{{ countLabel }}</span>
        </div>

        <div class="flex items-center gap-2">
          <button
            type="button"
            aria-label="Refresh datasets"
            class="halo-focus rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted p-2 text-muted-foreground transition-colors hover:bg-input hover:text-foreground"
            @click="store.loadDatasets()"
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
          <AppButton variant="accent" size="sm" @click="toggleUpload">
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
            New Dataset
          </AppButton>
        </div>
      </header>

      <!-- Upload panel (collapsible) -->
      <Transition name="reveal">
        <GlassCard v-if="showUpload" as="section" subtle padded class="shrink-0 p-5">
          <form class="grid grid-cols-2 gap-4" @submit.prevent="handleCreate">
            <div class="col-span-2 flex items-center justify-between">
              <div>
                <h3 class="text-sm font-semibold">Upload Dataset</h3>
                <p class="text-xs text-subtle-foreground">
                  Provide metadata and rows for quick experiments.
                </p>
              </div>
              <button
                type="button"
                class="rounded-md p-1.5 text-faint-foreground transition-colors hover:bg-surface-muted hover:text-foreground"
                aria-label="Close upload panel"
                @click="closeUpload"
              >
                <svg
                  class="h-4 w-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  aria-hidden="true"
                >
                  <path d="M18 6 6 18M6 6l12 12" />
                </svg>
              </button>
            </div>

            <label class="flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Name</span>
              <input v-model="form.name" required placeholder="e.g. support-intents" :class="fieldClass" />
            </label>

            <label class="flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">
                Tags <span class="text-faint-foreground">(comma separated)</span>
              </span>
              <input v-model="form.tags" placeholder="nlp, eval" :class="fieldClass" />
            </label>

            <label class="col-span-2 flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Description</span>
              <textarea
                v-model="form.description"
                rows="2"
                placeholder="What is this dataset for?"
                :class="[fieldClass, 'resize-none']"
              ></textarea>
            </label>

            <label class="col-span-2 flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Import file</span>
              <input
                type="file"
                accept=".json,.jsonl,.ndjson,.csv,application/json,text/csv"
                class="halo-focus w-full cursor-pointer rounded-md border border-[rgb(var(--line-strong))] bg-surface text-sm text-subtle-foreground outline-none file:mr-3 file:cursor-pointer file:border-0 file:border-r file:border-[rgb(var(--line-strong))] file:bg-surface-muted file:px-3 file:py-2.5 file:text-sm file:font-medium file:text-foreground"
                @change="handleFileImport"
              />
              <span class="text-xs text-faint-foreground">
                JSON, JSONL, or CSV. CSV headers <code class="font-mono">id, inputs, expected, meta, split</code>
                are reserved; all other columns become inputs.
              </span>
            </label>

            <label class="col-span-2 flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Rows (normalized JSON)</span>
              <textarea
                v-model="form.rows"
                rows="6"
                spellcheck="false"
                :class="[fieldClass, 'min-h-[10rem] resize-y font-mono text-xs leading-relaxed']"
                placeholder='[
  { "id": "sample-1", "inputs": { "question": "Hello" }, "expected": "Hi" }
]'
              ></textarea>
            </label>

            <div class="col-span-2 flex items-center gap-3">
              <AppButton type="submit" variant="accent" size="sm">Create dataset</AppButton>
              <AppButton type="button" variant="ghost" size="sm" @click="closeUpload">Cancel</AppButton>
              <Transition name="fade">
                <span v-if="createStatus" class="text-xs font-medium text-accent">{{ createStatus }}</span>
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
        v-if="store.datasetsError"
        class="shrink-0 rounded-md border border-danger/50 bg-[rgb(var(--color-danger)_/_0.1)] px-3 py-2 text-sm text-danger"
      >
        {{ store.datasetsError }}
      </div>

      <!-- Grid -->
      <div class="-mx-1 min-h-0 flex-1 overflow-y-auto overscroll-contain px-1">
        <!-- Loading skeletons -->
        <div
          v-if="store.datasetsLoading && !store.datasets.length"
          class="grid gap-3"
          :style="gridStyle"
        >
          <div v-for="n in 6" :key="n" class="halo-surface halo-surface-2 animate-pulse p-5">
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
          v-else-if="!store.datasets.length"
          class="flex h-full flex-col items-center justify-center gap-4 text-center"
        >
          <div
            class="flex h-14 w-14 items-center justify-center rounded-xl border border-[rgb(var(--line-strong))] bg-surface-muted text-accent"
          >
            <svg
              class="h-7 w-7"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.5"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <rect x="3" y="4" width="18" height="16" rx="1.5" />
              <path d="M3 9h18M3 14.5h18M9 4v16" />
            </svg>
          </div>
          <div>
            <p class="text-sm font-semibold">No datasets yet</p>
            <p class="mx-auto mt-1 max-w-xs text-sm text-subtle-foreground">
              A dataset is a table of rows — inputs and expected answers — you run prompts against in experiments.
            </p>
          </div>
          <AppButton variant="accent" size="sm" @click="openUpload">Upload your first dataset</AppButton>
        </div>

        <!-- No search matches -->
        <div
          v-else-if="!filteredDatasets.length"
          class="flex h-full flex-col items-center justify-center gap-3 text-center"
        >
          <p class="text-sm font-medium text-subtle-foreground">
            No datasets match “{{ search }}”.
          </p>
          <AppButton variant="ghost" size="sm" @click="search = ''">Clear search</AppButton>
        </div>

        <!-- Cards -->
        <div v-else class="grid items-stretch gap-3" :style="gridStyle">
          <GlassCard
            v-for="dataset in filteredDatasets"
            :key="dataset.id"
            as="article"
            padded
            interactive
            role="button"
            tabindex="0"
            :aria-label="`Open dataset ${dataset.name}`"
            class="group relative flex min-h-[13.5rem] cursor-pointer flex-col gap-3.5 p-5"
            @click="selectDataset(dataset.id)"
            @keyup.enter="selectDataset(dataset.id)"
            @keyup.space.prevent="selectDataset(dataset.id)"
          >
            <!-- Header -->
            <div class="flex items-start gap-3">
              <div
                class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-[rgb(var(--line-strong))] bg-surface-muted text-accent"
              >
                <svg
                  class="h-[18px] w-[18px]"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.7"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <rect x="3" y="4" width="18" height="16" rx="1.5" />
                  <path d="M3 9h18M3 14.5h18M9 4v16" />
                </svg>
              </div>
              <div class="min-w-0 flex-1 pt-0.5">
                <h3 class="truncate text-sm font-semibold leading-tight text-foreground">
                  {{ dataset.name }}
                </h3>
                <p class="mt-0.5 truncate font-mono text-[11px] text-faint-foreground">
                  {{ shortId(dataset.id) }}
                </p>
              </div>
              <button
                class="relative z-20 -mr-1 -mt-1 rounded-md p-1.5 text-faint-foreground opacity-0 transition-all hover:bg-[rgb(var(--color-danger)_/_0.12)] hover:text-danger focus-visible:opacity-100 group-hover:opacity-100"
                :aria-label="`Delete dataset ${dataset.name}`"
                @click.stop="deleteDataset(dataset.id)"
              >
                <svg
                  class="h-4 w-4"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.8"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <path d="M3 6h18M8 6V4h8v2M6 6l1 14h10l1-14" />
                </svg>
              </button>
            </div>

            <!-- Description (reserves 2 lines so cards align) -->
            <p
              v-if="dataset.description"
              class="line-clamp-2 min-h-[2.5rem] text-sm leading-snug text-muted-foreground"
            >
              {{ dataset.description }}
            </p>
            <p v-else class="min-h-[2.5rem] text-sm italic text-faint-foreground">No description</p>

            <!-- Tags -->
            <div v-if="dataset.tags?.length" class="flex flex-wrap gap-1.5">
              <Chip v-for="tag in dataset.tags.slice(0, 4)" :key="tag">{{ tag }}</Chip>
              <Chip v-if="dataset.tags.length > 4" muted>+{{ dataset.tags.length - 4 }}</Chip>
            </div>

            <!-- Footer -->
            <div
              class="mt-auto flex items-center justify-between border-t border-[rgb(var(--line-soft))] pt-3 font-mono text-[11px] text-faint-foreground"
            >
              <span>{{ formatDate(dataset.createdAt) }}</span>
              <span
                class="flex items-center gap-1 text-subtle-foreground opacity-0 transition-opacity group-hover:opacity-100"
              >
                Open
                <svg
                  class="h-3.5 w-3.5"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <path d="M5 12h14M13 6l6 6-6 6" />
                </svg>
              </span>
            </div>
          </GlassCard>
        </div>
      </div>
    </template>

    <!-- ================= DETAIL MODE ================= -->
    <template v-else>
      <!-- Header -->
      <header class="flex shrink-0 flex-wrap items-start justify-between gap-3 border-b border-border/60 pb-3">
        <div class="flex min-w-0 items-start gap-3">
          <button
            type="button"
            class="flex shrink-0 items-center gap-1.5 rounded-lg border border-border/70 px-3 py-1.5 text-sm font-medium transition-colors hover:bg-muted/60"
            @click="clearSelection"
          >
            ← Datasets
          </button>
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold leading-tight">
              {{ selectedDataset?.name || "Dataset" }}
            </h2>
            <div
              v-if="selectedDataset"
              class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-subtle-foreground"
            >
              <span>{{ formatDate(selectedDataset.createdAt) }}</span>
              <span class="text-faint-foreground">•</span>
              <span class="font-medium text-foreground">
                {{ selectedRowCount }} row{{ selectedRowCount === 1 ? "" : "s" }}
              </span>
              <template v-for="[split, n] in splitCounts" :key="split">
                <span class="text-faint-foreground">•</span>
                <span>{{ n }} {{ split }}</span>
              </template>
            </div>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <AppButton variant="ghost" size="sm" :disabled="detailLoading" @click="refreshSelected">
            Refresh
          </AppButton>
          <AppButton
            v-if="selectedDatasetId"
            variant="danger"
            size="sm"
            :disabled="detailLoading"
            @click="deleteDataset(selectedDatasetId)"
          >
            Delete
          </AppButton>
        </div>
      </header>

      <!-- Body -->
      <div
        v-if="detailLoading"
        class="flex min-h-0 flex-1 items-center justify-center text-sm text-subtle-foreground"
      >
        Loading dataset…
      </div>
      <div
        v-else-if="selectedDataset"
        class="grid min-h-0 flex-1 grid-cols-12 gap-4 overflow-hidden"
      >
        <!-- Properties -->
        <GlassCard :padded="false" class="col-span-4 flex min-h-0 flex-col overflow-hidden">
          <div class="shrink-0 border-b border-border/60 px-4 py-3 text-sm font-medium">
            Properties
          </div>
          <div class="min-h-0 flex-1 space-y-3 overflow-auto overscroll-contain p-4">
            <label class="flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Name</span>
              <input v-model="editForm.name" required :class="fieldClass" />
            </label>
            <label class="flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">
                Tags <span class="text-faint-foreground">(comma separated)</span>
              </span>
              <input v-model="editForm.tags" :class="fieldClass" />
            </label>
            <label class="flex flex-col gap-1.5 text-sm">
              <span class="text-xs font-medium text-subtle-foreground">Description</span>
              <textarea v-model="editForm.description" rows="4" :class="[fieldClass, 'resize-none']"></textarea>
            </label>
          </div>
          <div class="flex shrink-0 items-center gap-3 border-t border-border/60 bg-surface px-4 py-3">
            <AppButton
              variant="accent"
              size="sm"
              :disabled="detailLoading || !!jsonEditorError"
              @click="handleUpdate"
            >
              Save changes
            </AppButton>
            <Transition name="fade">
              <span v-if="detailStatus" class="text-xs font-medium text-accent">{{ detailStatus }}</span>
            </Transition>
            <Transition name="fade">
              <span v-if="detailError" class="text-xs font-medium text-danger">{{ detailError }}</span>
            </Transition>
          </div>
        </GlassCard>

        <!-- Rows -->
        <GlassCard :padded="false" class="col-span-8 flex min-h-0 flex-col overflow-hidden">
          <div class="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border/60 px-4 py-3">
            <div class="flex min-w-0 items-center gap-2">
              <span class="text-sm font-medium">Rows</span>
              <span v-if="fieldColumns.length" class="truncate text-xs text-subtle-foreground">
                fields:
                <span class="font-mono text-faint-foreground">{{ fieldColumns.join(", ") }}</span>
                <span v-if="hasExpected"> · expected</span>
              </span>
            </div>
            <MSegmented
              v-model="rowViewMode"
              :options="[
                { value: 'table', label: 'Table' },
                { value: 'json', label: 'JSON' },
              ]"
            />
          </div>

          <div class="min-h-0 flex-1 overflow-auto overscroll-contain">
            <div
              v-if="!previewRows.length"
              class="m-4 rounded-md border border-dashed border-border/60 p-6 text-center text-xs text-subtle-foreground"
            >
              No rows in this dataset.
            </div>
            <template v-else>
              <table v-if="rowViewMode === 'table'" class="min-w-full text-xs">
                <thead class="sticky top-0 bg-surface-muted/80 text-faint-foreground backdrop-blur">
                  <tr>
                    <th class="px-3 py-2 text-left font-medium">ID</th>
                    <th class="px-3 py-2 text-left font-medium">Split</th>
                    <th class="px-3 py-2 text-left font-medium">Inputs</th>
                    <th class="px-3 py-2 text-left font-medium">Expected</th>
                    <th class="px-3 py-2 text-left font-medium">Meta</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in previewRows" :key="row.id" class="border-t border-[rgb(var(--line-soft))]">
                    <td class="px-3 py-2 align-top font-mono text-muted-foreground">{{ row.id }}</td>
                    <td class="px-3 py-2 align-top">
                      <span class="rounded-sm border border-border bg-surface px-1.5 py-0.5 font-mono text-[10px] text-subtle-foreground">
                        {{ row.split || "train" }}
                      </span>
                    </td>
                    <td class="px-3 py-2 align-top">
                      <pre class="whitespace-pre-wrap text-[11px] leading-tight text-muted-foreground">{{ formatForPreview(row.inputs) }}</pre>
                    </td>
                    <td class="px-3 py-2 align-top">
                      <pre class="whitespace-pre-wrap text-[11px] leading-tight text-muted-foreground">{{ formatForPreview(row.expected) }}</pre>
                    </td>
                    <td class="px-3 py-2 align-top">
                      <pre class="whitespace-pre-wrap text-[11px] leading-tight text-muted-foreground">{{ formatForPreview(row.meta) }}</pre>
                    </td>
                  </tr>
                </tbody>
              </table>
              <textarea
                v-else
                v-model="editRowsJson"
                spellcheck="false"
                class="h-full min-h-[20rem] w-full resize-none bg-transparent px-4 py-3 font-mono text-[11px] leading-relaxed text-muted-foreground outline-none"
              ></textarea>
            </template>
          </div>

          <div
            v-if="jsonEditorError || hasMorePreview"
            class="shrink-0 border-t border-border/60 px-4 py-2 text-xs"
          >
            <p v-if="jsonEditorError" class="text-danger">{{ jsonEditorError }}</p>
            <p v-else class="text-subtle-foreground">
              Showing first {{ rowPreviewLimit }} of {{ selectedRowCount }} rows.
            </p>
          </div>
        </GlassCard>
      </div>
      <div v-else class="flex min-h-0 flex-1 items-center justify-center text-sm text-danger">
        {{ detailError || "Dataset could not be loaded." }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import type { Dataset, DatasetRow } from "@/api/playground";
import { usePlaygroundStore } from "@/stores/playground";
import {
  formatRowsForEditor,
  parseDatasetRows,
} from "@/lib/playgroundDatasetImport";
import GlassCard from "@/components/ui/GlassCard.vue";
import AppButton from "@/components/ui/AppButton.vue";
import Chip from "@/components/ui/Chip.vue";
import MSegmented from "@/components/ui/MSegmented.vue";

const store = usePlaygroundStore();
const form = reactive({ name: "", description: "", tags: "", rows: "" });
const createStatus = ref("");
const createError = ref("");
const search = ref("");
const showUpload = ref(false);

const editForm = reactive({ name: "", description: "", tags: "" });
const editRowsJson = ref("");

const selectedDatasetId = ref<string | null>(null);
const selectedDataset = ref<Dataset | null>(null);
const detailLoading = ref(false);
const detailStatus = ref("");
const detailError = ref("");
const rowViewMode = ref<"table" | "json">("table");
let detailRequestSeq = 0;

const fieldClass =
  "halo-focus w-full rounded-md border border-[rgb(var(--line-strong))] bg-surface px-[13px] py-2.5 font-sans text-sm text-foreground outline-none placeholder:text-faint-foreground";

const gridStyle = {
  gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
};

const rowPreviewLimit = 50;
const baseRows = computed(() => selectedDataset.value?.rows ?? []);
const jsonEditorState = computed(() => {
  try {
    return {
      rows: parseDatasetRows(editRowsJson.value, { format: "json" }),
      error: "" as string,
    };
  } catch (err) {
    if (!editRowsJson.value.trim()) {
      return { rows: [] as DatasetRow[], error: "" as string };
    }
    return {
      rows: baseRows.value,
      error: extractErr(err, "Rows JSON is invalid."),
    };
  }
});
const jsonEditorError = computed(() => jsonEditorState.value.error);
const effectiveRows = computed(() =>
  jsonEditorError.value ? baseRows.value : jsonEditorState.value.rows,
);
const selectedRowCount = computed(() => effectiveRows.value.length);
const previewRows = computed(() =>
  effectiveRows.value.slice(0, rowPreviewLimit),
);
const hasMorePreview = computed(
  () => effectiveRows.value.length > rowPreviewLimit,
);

const filteredDatasets = computed(() => {
  if (!search.value) return store.datasets;
  const term = search.value.toLowerCase();
  return store.datasets.filter(
    (d) =>
      d.name.toLowerCase().includes(term) ||
      (d.description && d.description.toLowerCase().includes(term)) ||
      (d.tags && d.tags.some((t) => t.toLowerCase().includes(term))),
  );
});

const countLabel = computed(() => {
  const total = store.datasets.length;
  if (!total) return "";
  if (search.value && filteredDatasets.value.length !== total) {
    return `${filteredDatasets.value.length} of ${total}`;
  }
  return `${total} dataset${total === 1 ? "" : "s"}`;
});

const splitCounts = computed<Array<[string, number]>>(() => {
  const counts: Record<string, number> = {};
  for (const row of effectiveRows.value) {
    const split = row.split || "train";
    counts[split] = (counts[split] ?? 0) + 1;
  }
  return Object.entries(counts).sort((a, b) => b[1] - a[1]);
});

const fieldColumns = computed(() => {
  const keys = new Set<string>();
  for (const row of effectiveRows.value) {
    if (row.inputs && typeof row.inputs === "object") {
      for (const k of Object.keys(row.inputs)) keys.add(k);
    }
  }
  return [...keys];
});

const hasExpected = computed(() =>
  effectiveRows.value.some(
    (r) => r.expected !== undefined && r.expected !== null && r.expected !== "",
  ),
);

onMounted(async () => {
  if (!store.datasets.length) {
    await store.loadDatasets();
  }
});

watch(selectedDatasetId, (id) => {
  detailStatus.value = "";
  detailError.value = "";
  void loadSelectedDataset(id);
});

watch(selectedDataset, () => {
  rowViewMode.value = "table";
});

function openUpload() {
  showUpload.value = true;
}

function closeUpload() {
  showUpload.value = false;
}

function toggleUpload() {
  showUpload.value ? closeUpload() : openUpload();
}

function shortId(id: string) {
  return id.length > 12 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id;
}

async function loadSelectedDataset(
  id: string | null,
  options: { force?: boolean } = {},
) {
  if (!id) {
    selectedDataset.value = null;
    detailLoading.value = false;
    resetEditForm();
    return;
  }
  detailLoading.value = true;
  const requestId = ++detailRequestSeq;
  try {
    const dataset = await store.ensureDataset(id, options);
    if (selectedDatasetId.value !== id || requestId !== detailRequestSeq) {
      return;
    }
    if (!dataset) {
      selectedDataset.value = null;
      detailError.value = "Dataset not found.";
      resetEditForm();
      return;
    }
    selectedDataset.value = {
      ...dataset,
      rows: dataset.rows ?? [],
    };
    populateEditForm(selectedDataset.value);
  } catch (err) {
    if (selectedDatasetId.value === id && requestId === detailRequestSeq) {
      detailError.value = extractErr(err, "Failed to load dataset.");
      selectedDataset.value = null;
      resetEditForm();
    }
  } finally {
    if (selectedDatasetId.value === id && requestId === detailRequestSeq) {
      detailLoading.value = false;
    }
  }
}

function selectDataset(id: string) {
  if (selectedDatasetId.value === id) {
    return;
  }
  selectedDatasetId.value = id;
}

function refreshSelected() {
  if (!selectedDatasetId.value) {
    return;
  }
  void loadSelectedDataset(selectedDatasetId.value, { force: true });
}

function clearSelection() {
  selectedDatasetId.value = null;
  selectedDataset.value = null;
  detailStatus.value = "";
  detailError.value = "";
  resetEditForm();
}

async function deleteDataset(id: string) {
  const ok = window.confirm("Delete this dataset and all its rows?");
  if (!ok) return;
  try {
    await store.removeDataset(id);
    if (selectedDatasetId.value === id) {
      clearSelection();
    }
  } catch (err) {
    alert(extractErr(err, "Failed to delete dataset."));
  }
}

async function handleCreate() {
  createError.value = "";
  try {
    const normalized = parseDatasetRows(form.rows, { format: "json" });
    const tags = parseTags(form.tags);
    await store.addDataset(
      { name: form.name, description: form.description, tags },
      normalized,
    );
    createStatus.value = "Dataset created.";
    form.name = "";
    form.description = "";
    form.tags = "";
    form.rows = "";
    setTimeout(() => (createStatus.value = ""), 3_000);
  } catch (err) {
    createError.value = extractErr(err, "Failed to create dataset.");
  }
}

async function handleFileImport(event: Event) {
  createError.value = "";
  createStatus.value = "";
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) {
    return;
  }
  try {
    const text = await readFileText(file);
    const rows = parseDatasetRows(text, { filename: file.name });
    if (rows.length === 0) {
      throw new Error("Dataset file contains no rows.");
    }
    form.rows = formatRowsForEditor(rows);
    createStatus.value = `Imported ${rows.length} row${rows.length === 1 ? "" : "s"} from ${file.name}.`;
  } catch (err) {
    createError.value = extractErr(err, "Failed to import dataset file.");
  } finally {
    input.value = "";
  }
}

function readFileText(file: File): Promise<string> {
  if (typeof file.text === "function") {
    return file.text();
  }
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () =>
      reject(reader.error ?? new Error("Failed to read file."));
    reader.onload = () =>
      resolve(typeof reader.result === "string" ? reader.result : "");
    reader.readAsText(file);
  });
}

async function handleUpdate() {
  if (!selectedDatasetId.value) {
    return;
  }
  detailError.value = "";
  detailStatus.value = "";
  try {
    const normalized = parseDatasetRows(editRowsJson.value, { format: "json" });
    const tags = parseTags(editForm.tags);
    const dataset = await store.saveDataset(
      selectedDatasetId.value,
      {
        name: editForm.name,
        description: editForm.description,
        tags,
      },
      normalized,
    );
    if (selectedDatasetId.value !== dataset.id) {
      return;
    }
    selectedDataset.value = {
      ...dataset,
      rows: dataset.rows ?? [],
    };
    populateEditForm(selectedDataset.value);
    detailStatus.value = "Dataset updated.";
    setTimeout(() => {
      detailStatus.value = "";
    }, 3_000);
  } catch (err) {
    detailError.value = extractErr(err, "Failed to update dataset.");
  }
}

function populateEditForm(dataset: Dataset) {
  editForm.name = dataset.name;
  editForm.description = dataset.description ?? "";
  editForm.tags = dataset.tags?.join(", ") ?? "";
  editRowsJson.value = formatRowsForEditor(dataset.rows ?? []);
}

function resetEditForm() {
  editForm.name = "";
  editForm.description = "";
  editForm.tags = "";
  editRowsJson.value = "";
}

function parseTags(value: string): string[] {
  return value
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

function formatForPreview(value: unknown): string {
  if (value === null || value === undefined || value === "") {
    return "—";
  }
  if (typeof value === "string") {
    return value;
  }
  try {
    return JSON.stringify(value, null, 2);
  } catch (err) {
    return String(value);
  }
}

function extractErr(err: unknown, fallback: string): string {
  const anyErr = err as any;
  if (anyErr?.response?.data?.error) return anyErr.response.data.error;
  return anyErr?.message || fallback;
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
