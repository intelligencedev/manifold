<template>
  <div class="grid h-full min-h-0 gap-6 lg:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)]">
    <section class="flex min-h-0 flex-col rounded-2xl border border-border/70 bg-surface p-4">
      <header class="mb-4">
        <h2 class="text-lg font-semibold">Upload Dataset</h2>
        <p class="text-sm text-subtle-foreground">Provide metadata and JSON rows for quick experiments.</p>
      </header>
      <div class="flex-1 overflow-auto pr-1">
        <form class="grid gap-3 md:grid-cols-2" @submit.prevent="handleCreate">
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Name</span>
            <input v-model="form.name" required class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
          </label>
          <label class="text-sm">
            <span class="text-subtle-foreground mb-1">Tags (comma separated)</span>
            <input v-model="form.tags" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
          </label>
          <label class="text-sm md:col-span-2">
            <span class="text-subtle-foreground mb-1">Description</span>
            <textarea v-model="form.description" rows="2" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
          </label>
          <label class="text-sm md:col-span-2">
            <span class="text-subtle-foreground mb-1">Rows (JSON array)</span>
            <textarea
              v-model="form.rows"
              rows="6"
              class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 font-mono text-sm"
              placeholder='[
  { "id": "sample-1", "inputs": { "question": "Hello" }, "expected": "Hi" }
]'
            ></textarea>
          </label>
          <div class="md:col-span-2 flex gap-3 items-center pb-2">
            <button type="submit" class="rounded border border-border/70 px-3 py-2 text-sm font-semibold">Create dataset</button>
            <span v-if="createStatus" class="text-sm text-subtle-foreground">{{ createStatus }}</span>
            <span v-if="createError" class="text-sm text-danger-foreground">{{ createError }}</span>
          </div>
        </form>
      </div>
    </section>

    <div class="flex min-h-0 flex-col">
      <section
        v-if="!selectedDatasetId"
        class="flex min-h-0 flex-col rounded-2xl border border-border/70 bg-surface p-4"
      >
        <header class="mb-4 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
          <div>
            <h2 class="text-lg font-semibold">Datasets</h2>
            <p class="text-sm text-subtle-foreground">Browse and select a dataset to inspect or edit.</p>
          </div>
          <button @click="store.loadDatasets" class="rounded border border-border/70 px-3 py-2 text-sm">Refresh</button>
        </header>
        <div class="flex-1 overflow-auto pr-1">
          <table class="min-w-full text-sm">
            <thead class="sticky top-0 bg-surface text-subtle-foreground">
              <tr>
                <th class="text-left px-3 py-2">Name</th>
                <th class="text-left px-3 py-2">Description</th>
                <th class="text-left px-3 py-2">Tags</th>
                <th class="text-left px-3 py-2">Created</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="dataset in store.datasets"
                :key="dataset.id"
                class="border-t border-border/60 cursor-pointer hover:bg-surface-muted/30 focus-within:bg-surface-muted/30"
                :class="{ 'bg-surface-muted/40': dataset.id === selectedDatasetId }"
                tabindex="0"
                role="button"
                @click="selectDataset(dataset.id)"
                @keyup.enter.prevent="selectDataset(dataset.id)"
                @keyup.space.prevent="selectDataset(dataset.id)"
                :aria-pressed="dataset.id === selectedDatasetId"
              >
                <td class="px-3 py-2 font-medium">{{ dataset.name }}</td>
                <td class="px-3 py-2 max-w-[280px] truncate">{{ dataset.description || '—' }}</td>
                <td class="px-3 py-2">{{ dataset.tags?.join(', ') || '—' }}</td>
                <td class="px-3 py-2 text-subtle-foreground">{{ formatDate(dataset.createdAt) }}</td>
              </tr>
              <tr v-if="store.datasetsLoading"><td colspan="4" class="px-3 py-3 text-center text-subtle-foreground">Loading…</td></tr>
              <tr v-else-if="store.datasets.length === 0"><td colspan="4" class="px-3 py-3 text-center text-subtle-foreground">No datasets yet.</td></tr>
            </tbody>
          </table>
        </div>
        <div v-if="store.datasetsError" class="mt-4 rounded border border-danger/60 bg-danger/10 px-3 py-2 text-danger-foreground text-sm">
          {{ store.datasetsError }}
        </div>
      </section>

      <section
        v-else
        class="flex min-h-0 flex-col rounded-2xl border border-border/70 bg-surface p-4"
      >
        <header class="mb-4 flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-wrap items-center gap-3">
            <button
              class="inline-flex items-center gap-2 rounded border border-border/70 px-3 py-2 text-sm"
              type="button"
              @click="clearSelection"
            >
              ← Back to datasets
            </button>
            <div>
              <h2 class="text-lg font-semibold">{{ selectedDataset?.name || 'Dataset Details' }}</h2>
              <p class="text-sm text-subtle-foreground" v-if="selectedDataset">
                {{ formatDate(selectedDataset.createdAt) }} • {{ selectedRowCount }} row{{ selectedRowCount === 1 ? '' : 's' }}
              </p>
            </div>
          </div>
          <button
            class="rounded border border-border/70 px-3 py-2 text-sm"
            type="button"
            @click="refreshSelected"
            :disabled="detailLoading"
          >
            Refresh
          </button>
        </header>
        <div v-if="detailLoading" class="flex-1 py-6 text-center text-sm text-subtle-foreground">
          Loading dataset…
        </div>
        <div v-else class="flex-1 overflow-auto pr-1">
          <div
            v-if="selectedDataset"
            class="grid gap-6 lg:grid-cols-[minmax(0,360px),minmax(0,1fr)]"
          >
            <div class="space-y-3">
              <label class="text-sm">
                <span class="text-subtle-foreground mb-1">Name</span>
                <input
                  v-model="editForm.name"
                  required
                  class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2"
                />
              </label>
              <label class="text-sm">
                <span class="text-subtle-foreground mb-1">Tags (comma separated)</span>
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
              <label class="text-sm block">
                <div class="flex items-center justify-between text-subtle-foreground mb-1">
                  <span>Rows (JSON array)</span>
                  <span class="text-xs">{{ selectedRowCount }} row{{ selectedRowCount === 1 ? '' : 's' }}</span>
                </div>
                <textarea
                  v-model="editForm.rows"
                  class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 font-mono text-sm h-64 min-h-[16rem] resize-none overflow-auto"
                ></textarea>
              </label>
              <div class="flex gap-3 items-center pt-1">
                <button
                  type="button"
                  class="rounded border border-border/70 px-3 py-2 text-sm font-semibold"
                  @click="handleUpdate"
                >
                  Save changes
                </button>
                <span v-if="detailStatus" class="text-sm text-subtle-foreground">{{ detailStatus }}</span>
                <span v-if="detailError" class="text-sm text-danger-foreground">{{ detailError }}</span>
              </div>
            </div>
            <div class="flex min-h-[16rem] flex-col space-y-3">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium">Rows preview</span>
                <div class="flex gap-2 text-xs">
                  <button
                    type="button"
                    class="rounded border px-2 py-1 transition-colors"
                    :class="rowViewMode === 'table' ? 'border-accent text-accent bg-accent/10' : 'border-border/60 text-subtle-foreground'"
                    @click="rowViewMode = 'table'"
                  >
                    Table
                  </button>
                  <button
                    type="button"
                    class="rounded border px-2 py-1 transition-colors"
                    :class="rowViewMode === 'json' ? 'border-accent text-accent bg-accent/10' : 'border-border/60 text-subtle-foreground'"
                    @click="rowViewMode = 'json'"
                  >
                    JSON
                  </button>
                </div>
              </div>
              <div v-if="!previewRows.length" class="text-xs text-subtle-foreground border border-border/60 rounded p-4">
                No rows available.
              </div>
              <template v-else>
                <div
                  v-if="rowViewMode === 'table'"
                  class="flex-1 border border-border/60 rounded overflow-hidden"
                >
                  <div class="h-full overflow-auto">
                    <table class="min-w-full text-xs">
                      <thead class="bg-surface-muted/60 text-subtle-foreground sticky top-0">
                        <tr>
                          <th class="text-left px-3 py-2">ID</th>
                          <th class="text-left px-3 py-2">Split</th>
                          <th class="text-left px-3 py-2">Inputs</th>
                          <th class="text-left px-3 py-2">Expected</th>
                          <th class="text-left px-3 py-2">Meta</th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr v-for="row in previewRows" :key="row.id" class="border-t border-border/60">
                          <td class="align-top px-3 py-2 font-mono">{{ row.id }}</td>
                          <td class="align-top px-3 py-2 text-subtle-foreground">{{ row.split || 'train' }}</td>
                          <td class="align-top px-3 py-2">
                            <pre class="whitespace-pre-wrap text-[11px] leading-tight">{{ formatForPreview(row.inputs) }}</pre>
                          </td>
                          <td class="align-top px-3 py-2">
                            <pre class="whitespace-pre-wrap text-[11px] leading-tight">{{ formatForPreview(row.expected) }}</pre>
                          </td>
                          <td class="align-top px-3 py-2">
                            <pre class="whitespace-pre-wrap text-[11px] leading-tight">{{ formatForPreview(row.meta) }}</pre>
                          </td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
                <div
                  v-else
                  class="flex-1 border border-border/60 rounded bg-surface-muted/40 overflow-auto"
                >
                  <pre class="text-[11px] leading-tight whitespace-pre px-3 py-3">{{ formatRowsForEditor(previewRows) }}</pre>
                </div>
                <p v-if="hasMorePreview" class="text-xs text-subtle-foreground">
                  Showing first {{ rowPreviewLimit }} rows of {{ selectedRowCount }}.
                </p>
              </template>
            </div>
          </div>
          <div v-else class="py-6 text-center text-sm text-danger-foreground">
            {{ detailError || 'Dataset could not be loaded.' }}
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import type { Dataset, DatasetRow } from '@/api/playground'
import { usePlaygroundStore } from '@/stores/playground'

const store = usePlaygroundStore()
const form = reactive({ name: '', description: '', tags: '', rows: '' })
const createStatus = ref('')
const createError = ref('')

const editForm = reactive({ name: '', description: '', tags: '', rows: '' })
const selectedDatasetId = ref<string | null>(null)
const selectedDataset = ref<Dataset | null>(null)
const detailLoading = ref(false)
const detailStatus = ref('')
const detailError = ref('')
const rowViewMode = ref<'table' | 'json'>('table')
let detailRequestSeq = 0

const rowPreviewLimit = 50
const selectedRows = computed(() => selectedDataset.value?.rows ?? [])
const selectedRowCount = computed(() => selectedRows.value.length)
const previewRows = computed(() => selectedRows.value.slice(0, rowPreviewLimit))
const hasMorePreview = computed(() => selectedRows.value.length > rowPreviewLimit)

onMounted(async () => {
  if (!store.datasets.length) {
    await store.loadDatasets()
  }
})

watch(selectedDatasetId, (id) => {
  detailStatus.value = ''
  detailError.value = ''
  void loadSelectedDataset(id)
})

watch(selectedDataset, () => {
  rowViewMode.value = 'table'
})

async function loadSelectedDataset(id: string | null, options: { force?: boolean } = {}) {
  if (!id) {
    selectedDataset.value = null
    detailLoading.value = false
    resetEditForm()
    return
  }
  detailLoading.value = true
  const requestId = ++detailRequestSeq
  try {
    const dataset = await store.ensureDataset(id, options)
    if (selectedDatasetId.value !== id || requestId !== detailRequestSeq) {
      return
    }
    if (!dataset) {
      selectedDataset.value = null
      detailError.value = 'Dataset not found.'
      resetEditForm()
      return
    }
    selectedDataset.value = {
      ...dataset,
      rows: dataset.rows ?? [],
    }
    populateEditForm(dataset)
  } catch (err) {
    if (selectedDatasetId.value === id && requestId === detailRequestSeq) {
      detailError.value = extractErr(err, 'Failed to load dataset.')
      selectedDataset.value = null
      resetEditForm()
    }
  } finally {
    if (selectedDatasetId.value === id && requestId === detailRequestSeq) {
      detailLoading.value = false
    }
  }
}

function selectDataset(id: string) {
  if (selectedDatasetId.value === id) {
    return
  }
  selectedDatasetId.value = id
}

function refreshSelected() {
  if (!selectedDatasetId.value) {
    return
  }
  void loadSelectedDataset(selectedDatasetId.value, { force: true })
}

function clearSelection() {
  selectedDatasetId.value = null
  selectedDataset.value = null
  detailStatus.value = ''
  detailError.value = ''
  resetEditForm()
}

async function handleCreate() {
  createError.value = ''
  try {
    const normalized = prepareRows(form.rows)
    const tags = parseTags(form.tags)
    await store.addDataset({ name: form.name, description: form.description, tags }, normalized)
    createStatus.value = 'Dataset created.'
    form.name = ''
    form.description = ''
    form.tags = ''
    form.rows = ''
    setTimeout(() => (createStatus.value = ''), 3_000)
  } catch (err) {
    createError.value = extractErr(err, 'Failed to create dataset.')
  }
}

async function handleUpdate() {
  if (!selectedDatasetId.value) {
    return
  }
  detailError.value = ''
  detailStatus.value = ''
  try {
    const normalized = prepareRows(editForm.rows)
    const tags = parseTags(editForm.tags)
    const dataset = await store.saveDataset(selectedDatasetId.value, {
      name: editForm.name,
      description: editForm.description,
      tags,
    }, normalized)
    if (selectedDatasetId.value !== dataset.id) {
      return
    }
    selectedDataset.value = dataset
    populateEditForm(dataset)
    detailStatus.value = 'Dataset updated.'
    setTimeout(() => {
      detailStatus.value = ''
    }, 3_000)
  } catch (err) {
    detailError.value = extractErr(err, 'Failed to update dataset.')
  }
}

function populateEditForm(dataset: Dataset) {
  editForm.name = dataset.name
  editForm.description = dataset.description ?? ''
  editForm.tags = dataset.tags?.join(', ') ?? ''
  editForm.rows = formatRowsForEditor(dataset.rows ?? [])
}

function resetEditForm() {
  editForm.name = ''
  editForm.description = ''
  editForm.tags = ''
  editForm.rows = ''
}

function prepareRows(input: string): DatasetRow[] {
  const trimmed = input.trim()
  if (!trimmed) {
    return []
  }
  let parsed: any
  try {
    parsed = JSON.parse(trimmed)
  } catch (err) {
    throw new Error('Rows must be valid JSON')
  }
  if (!Array.isArray(parsed)) {
    throw new Error('Rows must be an array')
  }
  return parsed.map((row: any, idx: number) => normalizeRow(row, idx))
}

function normalizeRow(row: any, idx: number): DatasetRow {
  const id = typeof row?.id === 'string' && row.id.trim().length > 0 ? row.id : `row-${idx + 1}`
  const inputs = row && typeof row === 'object' && !Array.isArray(row) && row.inputs
    ? row.inputs
    : row
  return {
    id,
    inputs,
    expected: row?.expected,
    meta: row?.meta,
    split: typeof row?.split === 'string' && row.split.trim().length > 0 ? row.split : 'train',
  }
}

function parseTags(value: string): string[] {
  return value
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)
}

function formatRowsForEditor(rows: DatasetRow[]): string {
  if (!rows.length) {
    return '[]'
  }
  try {
    return JSON.stringify(rows, null, 2)
  } catch (err) {
    return '[]'
  }
}

function formatForPreview(value: unknown): string {
  if (typeof value === 'string') {
    return value
  }
  try {
    return JSON.stringify(value, null, 2)
  } catch (err) {
    return String(value)
  }
}

function extractErr(err: unknown, fallback: string): string {
  const anyErr = err as any
  if (anyErr?.response?.data?.error) return anyErr.response.data.error
  return anyErr?.message || fallback
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
