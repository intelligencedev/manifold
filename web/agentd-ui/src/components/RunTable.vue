<template>
  <div class="overflow-hidden rounded-2xl border border-slate-800 bg-slate-900/60 shadow-lg">
    <table class="min-w-full divide-y divide-slate-800">
      <thead class="bg-slate-900/80">
        <tr>
          <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
            Run ID
          </th>
          <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
            Prompt
          </th>
          <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
            Tokens
          </th>
          <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
            Started
          </th>
          <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-slate-500">
            Status
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-slate-800">
        <tr v-for="run in runs" :key="run.id" class="hover:bg-slate-900/60">
          <td class="whitespace-nowrap px-4 py-3 text-sm font-mono text-slate-300">{{ run.id }}</td>
          <td class="max-w-xl px-4 py-3 text-sm text-slate-300">
            <span class="line-clamp-2">{{ run.prompt }}</span>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-400">
            {{ run.tokens ?? '—' }}
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-sm text-slate-400">
            <time :datetime="run.createdAt">{{ formatDate(run.createdAt) }}</time>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-sm">
            <StatusBadge :state="run.status">{{ run.status }}</StatusBadge>
          </td>
        </tr>
        <tr v-if="!runs.length">
          <td colspan="5" class="px-4 py-8 text-center text-sm text-slate-500">
            No runs yet.
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import type { AgentRun } from '@/api/client'
import StatusBadge from './StatusBadge.vue'

const props = defineProps<{ runs: AgentRun[] }>()

function formatDate(value: string) {
  const date = new Date(value)
  return date.toLocaleString()
}
</script>
