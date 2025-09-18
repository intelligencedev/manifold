<template>
  <section class="space-y-10">
    <header class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-white">Runs</h1>
        <p class="text-sm text-slate-400">Inspect historical completions and replay transcripts.</p>
      </div>
      <div class="flex items-center gap-3">
        <input
          v-model="search"
          type="search"
          placeholder="Search by prompt or run id"
          class="w-full rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 focus:border-emerald-400 focus:outline-none focus:ring focus:ring-emerald-400/20 sm:w-72"
        />
        <button
          class="rounded-lg border border-slate-700 px-3 py-2 text-sm font-semibold text-slate-300 transition hover:border-slate-500"
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
  staleTime: 15_000
})

const filteredRuns = computed(() => {
  const runs = data.value ?? []
  const term = search.value.trim().toLowerCase()
  if (!term) return runs
  return runs.filter((run) =>
    [run.id, run.prompt, run.status].some((value) => value.toLowerCase().includes(term))
  )
})
</script>
