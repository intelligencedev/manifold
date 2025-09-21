<template>
  <section class="space-y-10">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-foreground">Runs</h1>
        <p class="text-sm text-subtle-foreground">
          Inspect historical completions and replay transcripts.
        </p>
      </div>
      <div class="flex items-center gap-3">
        <input
          v-model="search"
          type="search"
          placeholder="Search by prompt or run id"
          class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40 sm:w-72"
        />
        <button
          class="rounded-lg border border-border/70 px-3 py-2 text-sm font-semibold text-muted-foreground transition hover:border-border"
        >
          Export
        </button>
      </div>
    </header>

    <RunTable :runs="filteredRuns" />
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import { fetchAgentRuns } from '@/api/client'
import RunTable from '@/components/RunTable.vue'

const search = ref('')

const { data } = useQuery({
  queryKey: ['agent-runs'],
  queryFn: fetchAgentRuns,
  staleTime: 15_000,
})

const filteredRuns = computed(() => {
  const runs = data.value ?? []
  const term = search.value.trim().toLowerCase()
  if (!term) return runs
  return runs.filter((run) =>
    [run.id, run.prompt, run.status].some((value) => value.toLowerCase().includes(term)),
  )
})
</script>
