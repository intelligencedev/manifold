<template>
  <div class="space-y-6">
    <section class="grid gap-4 md:grid-cols-3">
      <article class="rounded-2xl border border-border/70 bg-surface p-4">
        <h2 class="text-sm text-subtle-foreground">Prompts</h2>
        <p class="mt-2 text-3xl font-semibold">{{ store.promptCount }}</p>
        <RouterLink to="/playground/prompts" class="mt-4 inline-flex text-sm text-accent hover:underline">Manage prompts →</RouterLink>
      </article>
      <article class="rounded-2xl border border-border/70 bg-surface p-4">
        <h2 class="text-sm text-subtle-foreground">Datasets</h2>
        <p class="mt-2 text-3xl font-semibold">{{ store.datasetCount }}</p>
        <RouterLink to="/playground/datasets" class="mt-4 inline-flex text-sm text-accent hover:underline">Upload data →</RouterLink>
      </article>
      <article class="rounded-2xl border border-border/70 bg-surface p-4">
        <h2 class="text-sm text-subtle-foreground">Experiments</h2>
        <p class="mt-2 text-3xl font-semibold">{{ store.experimentCount }}</p>
        <RouterLink to="/playground/experiments" class="mt-4 inline-flex text-sm text-accent hover:underline">Run experiments →</RouterLink>
      </article>
    </section>

    <section class="rounded-2xl border border-border/70 bg-surface p-4 space-y-4">
      <header class="flex items-center justify-between">
        <div>
          <h2 class="text-lg font-semibold">Recent Prompts</h2>
          <p class="text-sm text-subtle-foreground">Latest prompt definitions.</p>
        </div>
        <RouterLink to="/playground/prompts" class="text-sm text-accent hover:underline">View all</RouterLink>
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
          <tr v-for="prompt in recentPrompts" :key="prompt.id" class="border-t border-border/60">
            <td class="py-2 font-medium">
              <RouterLink :to="`/playground/prompts/${prompt.id}`" class="text-accent hover:underline">{{ prompt.name }}</RouterLink>
            </td>
            <td class="py-2 truncate max-w-[240px]">{{ prompt.description || '—' }}</td>
            <td class="py-2">{{ prompt.tags?.join(', ') || '—' }}</td>
            <td class="py-2 text-subtle-foreground">{{ formatDate(prompt.createdAt) }}</td>
          </tr>
          <tr v-if="store.promptsLoading"><td colspan="4" class="py-3 text-center text-subtle-foreground">Loading…</td></tr>
          <tr v-else-if="recentPrompts.length === 0"><td colspan="4" class="py-3 text-center text-subtle-foreground">No prompts yet.</td></tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { computed, onMounted } from 'vue'
import { usePlaygroundStore } from '@/stores/playground'

const store = usePlaygroundStore()

onMounted(async () => {
  if (!store.prompts.length) {
    await store.loadPrompts()
  }
  if (!store.datasets.length) {
    await store.loadDatasets()
  }
  if (!store.experiments.length) {
    await store.loadExperiments()
  }
})

const recentPrompts = computed(() => store.prompts.slice(0, 5))

function formatDate(value?: string) {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}
</script>
