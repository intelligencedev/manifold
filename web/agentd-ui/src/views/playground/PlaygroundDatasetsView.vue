<template>
  <!-- Responsive layout: stack on small screens with a sized upload card; split columns on large screens -->
  <div
    :class="[
      'flex h-full min-h-0 flex-col gap-6 overflow-hidden',
      selectedDatasetId
        ? ''
        : 'lg:grid lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]',
    ]"
  >
    <!-- Upload card: cap height on small screens so it never pushes past the viewport; full height on large screens -->
    <section
      v-if="!selectedDatasetId"
      class="flex min-h-0 max-h-[50vh] flex-col overflow-hidden rounded-2xl border border-border/70 bg-surface p-4 lg:h-full lg:max-h-none"
    >
      <header class="mb-4">
        <h2 class="text-lg font-semibold">Upload Dataset</h2>
        <p class="text-sm text-subtle-foreground">
          Provide metadata and JSON rows for quick experiments.
        </p>
      </header>
      <div class="flex-1 overflow-auto overscroll-contain pr-1">
        <form class="grid gap-3 md:grid-cols-2" @submit.prevent="handleCreate">
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Name</span>
            <input
              v-model="form.name"
              required
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1"
              >Tags (comma separated)</span
            >
            <input
              v-model="form.tags"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            />
          </label>
          <label class="text-sm md:col-span-2">
            <span class="text-subtle-foreground mb-1">Description</span>
            <textarea
              v-model="form.description"
              rows="2"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
            ></textarea>
          </label>
          <label class="text-sm md:col-span-2">
            <span class="text-subtle-foreground mb-1">Rows (JSON array)</span>
            <textarea
              v-model="form.rows"
              rows="6"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 font-mono text-sm resize-none min-h-[12rem]"
              placeholder='[
  { "id": "sample-1", "inputs": { "question": "Hello" }, "expected": "Hi" }
]'
            ></textarea>
          </label>
          <div class="md:col-span-2 flex gap-3 items-center pb-2">
            <button
              type="submit"
              class="rounded border border-border/70 px-3 py-2 text-sm font-semibold"
            >
              Create dataset
            </button>
            <span v-if="createStatus" class="text-sm text-subtle-foreground">{{
              createStatus
            }}</span>
            <span v-if="createError" class="text-sm text-danger-foreground">{{
              createError
            }}</span>
          </div>
        </form>
      </div>
    </section>

    <!-- Right column (list/detail): consumes remaining height in stacked layout -->
    <div class="flex min-h-0 flex-1 flex-col lg:h-full">
      <section
        v-if="!selectedDatasetId"
        class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border/70 bg-surface p-4 lg:h-full"
      >
        <header
          class="mb-4 flex flex-col gap-2 md:flex-row md:items-center md:justify-between"
        >
          <div>
            <h2 class="text-lg font-semibold">Datasets</h2>
            <p class="text-sm text-subtle-foreground">
              Browse and select a dataset to inspect or edit.
            </p>
          </div>
          <button
            @click="store.loadDatasets"
            class="rounded border border-border/70 px-3 py-2 text-sm"
          >
            Refresh
          </button>
        </header>
        <div class="flex-1 overflow-auto overscroll-contain pr-1">
          <table class="min-w-full text-sm">
            <thead class="sticky top-0 bg-surface text-subtle-foreground">
              <tr>
                <th class="text-left px-3 py-2">Name</th>
                <th class="text-left px-3 py-2">Description</th>
                <th class="text-left px-3 py-2">Tags</th>
                <th class="text-left px-3 py-2">Created</th>
                <th class="text-right px-3 py-2">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="dataset in store.datasets"
                :key="dataset.id"
                class="border-t border-border/60 cursor-pointer hover:bg-surface-muted/30 focus-within:bg-surface-muted/30"
                :class="{
                  'bg-surface-muted/40': dataset.id === selectedDatasetId,
                }"
                tabindex="0"
                role="button"
                @click="selectDataset(dataset.id)"
                @keyup.enter.prevent="selectDataset(dataset.id)"
                @keyup.space.prevent="selectDataset(dataset.id)"
                :aria-pressed="dataset.id === selectedDatasetId"
              >
                <td class="px-3 py-2 font-medium">{{ dataset.name }}</td>
                <td class="px-3 py-2 max-w-[280px] truncate">
                  {{ dataset.description || "—" }}
                </td>
                <td class="px-3 py-2">{{ dataset.tags?.join(", ") || "—" }}</td>
                <td class="px-3 py-2 text-subtle-foreground">
                  {{ formatDate(dataset.createdAt) }}
                </td>
                <td class="px-3 py-2 text-right">
                  <button
                    class="rounded border border-danger/60 text-danger/60 px-2 py-1 text-xs"
                    @click.stop="deleteDataset(dataset.id)"
                  >
                    Delete
                  </button>
                </td>
              </tr>
              <tr v-if="store.datasetsLoading">
                <td
                  colspan="5"
                  class="px-3 py-3 text-center text-subtle-foreground"
                >
                  Loading…
                </td>
              </tr>
              <tr v-else-if="store.datasets.length === 0">
                <td
                  colspan="5"
                  class="px-3 py-3 text-center text-subtle-foreground"
                >
                  No datasets yet.
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div
          v-if="store.datasetsError"
          class="mt-4 rounded border border-danger/60 bg-danger/10 px-3 py-2 text-danger-foreground text-sm"
        >
          {{ store.datasetsError }}
        </div>
      </section>

      <section
        v-else
        class="grid min-h-0 flex-1 grid-rows-[auto,1fr] gap-4 overflow-hidden rounded-2xl border border-border/70 bg-surface p-4 lg:h-full"
      >
        <!-- Sticky header row -->
        <header
          class="sticky top-0 z-10 grid grid-cols-1 items-start gap-3 bg-surface/95 pb-2 backdrop-blur lg:grid-cols-[1fr_auto]"
        >
          <div class="flex items-start gap-3">
            <button
              class="inline-flex items-center gap-2 rounded-lg border border-border/70 px-3 py-2 text-sm"
              type="button"
              @click="clearSelection"
            >
              ← Back to datasets
            </button>
            <div class="min-w-0">
              <h2 class="truncate text-lg font-semibold">
                {{ selectedDataset?.name || "Dataset Details" }}
              </h2>
              <p class="text-sm text-subtle-foreground" v-if="selectedDataset">
                {{ formatDate(selectedDataset.createdAt) }} •
                {{ selectedRowCount }} row{{
                  selectedRowCount === 1 ? "" : "s"
                }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2 justify-end">
            <button
              class="rounded-lg border border-border/70 px-3 py-2 text-sm"
              type="button"
              @click="refreshSelected"
              :disabled="detailLoading"
            >
              Refresh
            </button>
            <button
              v-if="selectedDatasetId"
              class="rounded-lg border border-danger/60 px-3 py-2 text-sm text-danger/70 hover:text-danger/90"
              type="button"
              @click="deleteDataset(selectedDatasetId!)"
              :disabled="detailLoading"
            >
              Delete
            </button>
          </div>
        </header>

        <!-- Body row -->
        <div
          v-if="detailLoading"
          class="flex min-h-0 items-center justify-center text-sm text-subtle-foreground"
        >
          Loading dataset…
        </div>
        <div v-else class="min-h-0 flex-1 overflow-hidden">
          <div
            v-if="selectedDataset"
            class="grid h-full min-h-0 grid-cols-1 gap-4 lg:grid-cols-12"
          >
            <!-- Properties card (left) -->
            <div class="col-span-12 flex min-h-0 flex-col lg:col-span-4">
              <div
                class="flex h-full min-h-0 flex-col rounded-xl border border-border/60 bg-surface-muted/30"
              >
                <div
                  class="border-b border-border/60 px-4 py-3 text-sm font-medium"
                >
                  Properties
                </div>
                <div
                  class="min-h-0 space-y-3 overflow-auto overscroll-contain p-4 pr-3"
                >
                  <label class="text-sm">
                    <span class="text-subtle-foreground mb-1">Name</span>
                    <input
                      v-model="editForm.name"
                      required
                      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
                    />
                  </label>
                  <label class="text-sm">
                    <span class="text-subtle-foreground mb-1"
                      >Tags (comma separated)</span
                    >
                    <input
                      v-model="editForm.tags"
                      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
                    />
                  </label>
                  <label class="text-sm">
                    <span class="text-subtle-foreground mb-1">Description</span>
                    <textarea
                      v-model="editForm.description"
                      rows="3"
                      class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
                    ></textarea>
                  </label>
                </div>
                <!-- Sticky save bar for symmetry -->
                <div
                  class="sticky bottom-0 flex items-center gap-3 border-t border-border/60 bg-surface/90 p-3 backdrop-blur"
                >
                  <button
                    type="button"
                    class="rounded border border-border/70 px-3 py-2 text-sm font-semibold"
                    @click="handleUpdate"
                    :disabled="detailLoading || !!jsonEditorError"
                  >
                    Save changes
                  </button>
                  <span
                    v-if="detailStatus"
                    class="text-sm text-subtle-foreground"
                    >{{ detailStatus }}</span
                  >
                  <span
                    v-if="detailError"
                    class="text-sm text-danger-foreground"
                    >{{ detailError }}</span
                  >
                </div>
              </div>
            </div>

            <!-- Rows card (right) -->
            <div class="col-span-12 flex min-h-0 flex-col lg:col-span-8">
              <div
                class="flex h-full min-h-0 flex-col rounded-xl border border-border/60 bg-surface-muted/30"
              >
                <!-- Toolbar -->
                <div
                  class="flex items-center justify-between gap-3 border-b border-border/60 px-4 py-3"
                >
                  <span class="text-sm font-medium">Rows</span>
                  <nav
                    class="flex items-center gap-1 rounded-md border border-border/60"
                    role="tablist"
                    aria-label="Rows view"
                  >
                    <button
                      type="button"
                      role="tab"
                      :aria-selected="rowViewMode === 'table'"
                      class="px-2 py-1 text-xs"
                      :class="
                        rowViewMode === 'table'
                          ? 'bg-accent/10 text-accent'
                          : 'text-subtle-foreground'
                      "
                      @click="rowViewMode = 'table'"
                    >
                      Table
                    </button>
                    <button
                      type="button"
                      role="tab"
                      :aria-selected="rowViewMode === 'json'"
                      class="px-2 py-1 text-xs"
                      :class="
                        rowViewMode === 'json'
                          ? 'bg-accent/10 text-accent'
                          : 'text-subtle-foreground'
                      "
                      @click="rowViewMode = 'json'"
                    >
                      JSON
                    </button>
                  </nav>
                </div>

                <!-- Body -->
                <div class="min-h-0 flex-1 overflow-auto overscroll-contain">
                  <div
                    v-if="!previewRows.length"
                    class="m-4 rounded border border-border/60 p-4 text-xs text-subtle-foreground"
                  >
                    No rows available.
                  </div>
                  <template v-else>
                    <div v-if="rowViewMode === 'table'" class="min-h-0">
                      <table class="min-w-full text-xs">
                        <thead
                          class="bg-surface-muted/60 text-subtle-foreground sticky top-0"
                        >
                          <tr>
                            <th class="text-left px-3 py-2">ID</th>
                            <th class="text-left px-3 py-2">Split</th>
                            <th class="text-left px-3 py-2">Inputs</th>
                            <th class="text-left px-3 py-2">Expected</th>
                            <th class="text-left px-3 py-2">Meta</th>
                          </tr>
                        </thead>
                        <tbody>
                          <tr
                            v-for="row in previewRows"
                            :key="row.id"
                            class="border-t border-border/60"
                          >
                            <td class="align-top px-3 py-2 font-mono">
                              {{ row.id }}
                            </td>
                            <td
                              class="align-top px-3 py-2 text-subtle-foreground"
                            >
                              {{ row.split || "train" }}
                            </td>
                            <td class="align-top px-3 py-2">
                              <pre
                                class="whitespace-pre-wrap text-[11px] leading-tight"
                                >{{ formatForPreview(row.inputs) }}</pre
                              >
                            </td>
                            <td class="align-top px-3 py-2">
                              <pre
                                class="whitespace-pre-wrap text-[11px] leading-tight"
                                >{{ formatForPreview(row.expected) }}</pre
                              >
                            </td>
                            <td class="align-top px-3 py-2">
                              <pre
                                class="whitespace-pre-wrap text-[11px] leading-tight"
                                >{{ formatForPreview(row.meta) }}</pre
                              >
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                    <div v-else class="min-h-0">
                      <textarea
                        v-model="editRowsJson"
                        class="h-[min(60vh,32rem)] w-full resize-none bg-transparent px-4 py-3 font-mono text-[11px] leading-tight outline-none"
                        spellcheck="false"
                      ></textarea>
                    </div>
                    <div class="border-t border-border/60 px-4 py-2 text-xs">
                      <p v-if="jsonEditorError" class="text-danger-foreground">
                        {{ jsonEditorError }}
                      </p>
                      <p
                        v-else-if="hasMorePreview"
                        class="text-subtle-foreground"
                      >
                        Showing first {{ rowPreviewLimit }} rows of
                        {{ selectedRowCount }}.
                      </p>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="py-6 text-center text-sm text-danger-foreground">
            {{ detailError || "Dataset could not be loaded." }}
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import type { Dataset, DatasetRow } from "@/api/playground";
import { usePlaygroundStore } from "@/stores/playground";

const store = usePlaygroundStore();
const form = reactive({ name: "", description: "", tags: "", rows: "" });
const createStatus = ref("");
const createError = ref("");

const editForm = reactive({ name: "", description: "", tags: "" });
const editRowsJson = ref("");

const selectedDatasetId = ref<string | null>(null);
const selectedDataset = ref<Dataset | null>(null);
const detailLoading = ref(false);
const detailStatus = ref("");
const detailError = ref("");
const rowViewMode = ref<"table" | "json">("table");
let detailRequestSeq = 0;

const rowPreviewLimit = 50;
const baseRows = computed(() => selectedDataset.value?.rows ?? []);
const jsonEditorState = computed(() => {
  try {
    return { rows: prepareRows(editRowsJson.value), error: "" as string };
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
    const normalized = prepareRows(form.rows);
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

async function handleUpdate() {
  if (!selectedDatasetId.value) {
    return;
  }
  detailError.value = "";
  detailStatus.value = "";
  try {
    const normalized = prepareRows(editRowsJson.value);
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

function prepareRows(input: string): DatasetRow[] {
  const trimmed = input.trim();
  if (!trimmed) {
    return [];
  }
  let parsed: any;
  try {
    parsed = JSON.parse(trimmed);
  } catch (err) {
    throw new Error("Rows must be valid JSON");
  }
  if (!Array.isArray(parsed)) {
    throw new Error("Rows must be an array");
  }
  return parsed.map((row: any, idx: number) => normalizeRow(row, idx));
}

function normalizeRow(row: any, idx: number): DatasetRow {
  const id =
    typeof row?.id === "string" && row.id.trim().length > 0
      ? row.id
      : `row-${idx + 1}`;
  const inputs =
    row && typeof row === "object" && !Array.isArray(row) && row.inputs
      ? row.inputs
      : row;
  return {
    id,
    inputs,
    expected: row?.expected,
    meta: row?.meta,
    split:
      typeof row?.split === "string" && row.split.trim().length > 0
        ? row.split
        : "train",
  };
}

function parseTags(value: string): string[] {
  return value
    .split(",")
    .map((t) => t.trim())
    .filter(Boolean);
}

function formatRowsForEditor(rows: DatasetRow[]): string {
  if (!rows.length) {
    return "[]";
  }
  try {
    return JSON.stringify(rows, null, 2);
  } catch (err) {
    return "[]";
  }
}

function formatForPreview(value: unknown): string {
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
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}
</script>
