<template>
  <div class="space-y-6">
    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-4">
      <header>
        <h2 class="text-lg font-semibold">Create Prompt</h2>
        <p class="text-sm text-subtle-foreground">Define a prompt template and optional metadata.</p>
      </header>
      <form class="grid gap-3 md:grid-cols-2" @submit.prevent="handleCreate">
        <label class="flex flex-col text-sm md:col-span-1">
          <span class="text-subtle-foreground mb-1">Name</span>
          <input v-model="form.name" required class="rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
        </label>
        <label class="flex flex-col text-sm md:col-span-1">
          <span class="text-subtle-foreground mb-1">Tags (comma separated)</span>
          <input v-model="form.tags" placeholder="sales,onboarding" class="rounded border border-border/70 bg-surface-muted/60 px-3 py-2" />
        </label>
        <label class="flex flex-col text-sm md:col-span-2">
          <span class="text-subtle-foreground mb-1">Description</span>
          <textarea v-model="form.description" rows="2" class="rounded border border-border/70 bg-surface-muted/60 px-3 py-2"></textarea>
        </label>
        <div class="md:col-span-2 flex gap-3">
          <button type="submit" class="rounded border border-border/70 px-3 py-2 text-sm font-semibold">Create</button>
          <span v-if="createStatus" class="text-sm text-subtle-foreground">{{ createStatus }}</span>
        </div>
      </form>
    </section>

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-3">
      <header class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">Prompts</h2>
          <p class="text-sm text-subtle-foreground">Manage existing prompts and versions.</p>
        </div>
        <input v-model="search" placeholder="Filter by name or tag" class="rounded border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm w-64" />
      </header>

      <table class="w-full text-sm">
        <thead class="text-subtle-foreground">
          <tr>
            <th class="text-left py-2">Name</th>
            <th class="text-left py-2">Description</th>
            <th class="text-left py-2">Tags</th>
            <th class="text-left py-2">Created</th>
            <th class="text-right py-2 pr-2">Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="prompt in filteredPrompts" :key="prompt.id" class="border-t border-border/60">
            <td class="py-2 font-medium">
              <RouterLink :to="`/playground/prompts/${prompt.id}`" class="text-accent hover:underline">{{ prompt.name }}</RouterLink>
            </td>
            <td class="py-2 max-w-[280px] truncate">{{ prompt.description || '—' }}</td>
            <td class="py-2">{{ prompt.tags?.join(', ') || '—' }}</td>
            <td class="py-2 text-subtle-foreground">{{ formatDate(prompt.createdAt) }}</td>
            <td class="py-2 pr-2 text-right">
              <button class="rounded border border-danger/60 text-danger-foreground px-2 py-1 text-xs"
                @click="confirmDeletePrompt(prompt.id)">Delete</button>
            </td>
          </tr>
          <tr v-if="store.promptsLoading"><td colspan="5" class="py-3 text-center text-subtle-foreground">Loading…</td></tr>
          <tr v-else-if="filteredPrompts.length === 0"><td colspan="5" class="py-3 text-center text-subtle-foreground">No prompts found.</td></tr>
        </tbody>
      </table>
      <div v-if="store.promptsError" class="rounded border border-danger/60 bg-danger/10 px-3 py-2 text-danger-foreground text-sm">
        {{ store.promptsError }}
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { computed, onMounted, reactive, ref } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'

const store = usePlaygroundStore()
const form = reactive({ name: '', description: '', tags: '' })
const createStatus = ref('')
const search = ref('')
const deleting = ref<string | null>(null)

onMounted(async () => {
  if (!store.prompts.length) {
    await store.loadPrompts()
  }
})

const filteredPrompts = computed(() => {
  if (!search.value) return store.prompts
  const term = search.value.toLowerCase()
  return store.prompts.filter((p) => {
    return (
      p.name.toLowerCase().includes(term) ||
      (p.description && p.description.toLowerCase().includes(term)) ||
      (p.tags && p.tags.some((t) => t.toLowerCase().includes(term)))
    )
  })
})

async function handleCreate() {
  await store.addPrompt(form)
  createStatus.value = 'Prompt created.'
  form.name = ''
  form.description = ''
  form.tags = ''
  setTimeout(() => (createStatus.value = ''), 3_000)
}

async function confirmDeletePrompt(id: string) {
  if (deleting.value) return
  const ok = window.confirm('Delete this prompt and all its versions?')
  if (!ok) return
  try {
    deleting.value = id
    await store.removePrompt(id)
  } finally {
    deleting.value = null
  }
}

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
