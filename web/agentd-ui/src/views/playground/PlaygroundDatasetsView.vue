<template>
  <div class="space-y-6">
    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header>
        <h2 class="text-lg font-semibold">Upload Dataset</h2>
        <p class="text-sm text-subtle-foreground">Provide metadata and JSON rows for quick experiments.</p>
      </header>
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
          <textarea v-model="form.rows" rows="6" class="w-full rounded border border-border/70 bg-surface-muted/60 px-3 py-2 font-mono text-sm" placeholder='[
  { "id": "sample-1", "inputs": { "question": "Hello" }, "expected": "Hi" }
]'></textarea>
        </label>
        <div class="md:col-span-2 flex gap-3 items-center">
          <button type="submit" class="rounded border border-border/70 px-3 py-2 text-sm font-semibold">Create dataset</button>
          <span v-if="createStatus" class="text-sm text-subtle-foreground">{{ createStatus }}</span>
          <span v-if="createError" class="text-sm text-danger-foreground">{{ createError }}</span>
        </div>
      </form>
    </section>

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">Datasets</h2>
          <p class="text-sm text-subtle-foreground">Recently uploaded datasets.</p>
        </div>
        <button @click="store.loadDatasets" class="rounded border border-border/70 px-3 py-2 text-sm">Refresh</button>
      </header>
      <table class="w-full text-sm">
        <thead class="text-subtle-foreground">
          <tr>
            <th class="text-left py-2">Name</th>
            <th class="text-left py-2">Description</th>
            <th class="text-left py-2">Tags</th>
            <th class="text-left py-2">Created</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="dataset in store.datasets" :key="dataset.id" class="border-t border-border/60">
            <td class="py-2 font-medium">{{ dataset.name }}</td>
            <td class="py-2 max-w-[280px] truncate">{{ dataset.description || '—' }}</td>
            <td class="py-2">{{ dataset.tags?.join(', ') || '—' }}</td>
            <td class="py-2 text-subtle-foreground">{{ formatDate(dataset.createdAt) }}</td>
          </tr>
          <tr v-if="store.datasetsLoading"><td colspan="4" class="py-3 text-center text-subtle-foreground">Loading…</td></tr>
          <tr v-else-if="store.datasets.length === 0"><td colspan="4" class="py-3 text-center text-subtle-foreground">No datasets yet.</td></tr>
        </tbody>
      </table>
      <div v-if="store.datasetsError" class="rounded border border-danger/60 bg-danger/10 px-3 py-2 text-danger-foreground text-sm">
        {{ store.datasetsError }}
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'

const store = usePlaygroundStore()
const form = reactive({ name: '', description: '', tags: '', rows: '' })
const createStatus = ref('')
const createError = ref('')

onMounted(async () => {
  if (!store.datasets.length) {
    await store.loadDatasets()
  }
})

async function handleCreate() {
  createError.value = ''
  try {
    const rows = form.rows ? JSON.parse(form.rows) : []
    if (!Array.isArray(rows)) {
      throw new Error('Rows must be an array')
    }
    const normalized = rows.map((row: any, idx: number) => ({
      id: row.id || `row-${idx + 1}`,
      inputs: row.inputs || row,
      expected: row.expected,
      meta: row.meta,
      split: row.split || 'train',
    }))
    const tags = form.tags
      .split(',')
      .map((t) => t.trim())
      .filter(Boolean)
    await store.addDataset({ name: form.name, description: form.description, tags }, normalized)
    createStatus.value = 'Dataset created.'
    form.name = ''
    form.description = ''
    form.tags = ''
    form.rows = ''
    setTimeout(() => (createStatus.value = ''), 3_000)
  } catch (err) {
    createError.value = extractErr(err)
  }
}

function extractErr(err: unknown): string {
  const anyErr = err as any
  if (anyErr?.response?.data?.error) return anyErr.response.data.error
  return anyErr?.message || 'Failed to create dataset.'
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
