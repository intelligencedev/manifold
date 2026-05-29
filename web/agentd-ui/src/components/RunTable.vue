<template>
  <div class="overflow-hidden rounded-lg border border-border bg-surface">
    <table class="min-w-full">
      <thead>
        <tr>
          <th
            class="border-b border-border px-4 py-3 text-left font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-faint-foreground"
          >
            Run ID
          </th>
          <th
            class="border-b border-border px-4 py-3 text-left font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-faint-foreground"
          >
            Prompt
          </th>
          <th
            class="border-b border-border px-4 py-3 text-left font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-faint-foreground"
          >
            Tokens
          </th>
          <th
            class="border-b border-border px-4 py-3 text-left font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-faint-foreground"
          >
            Started
          </th>
          <th
            class="border-b border-border px-4 py-3 text-left font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-faint-foreground"
          >
            Status
          </th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="run in runs" :key="run.id" class="border-b border-[rgb(var(--line-soft))] last:border-b-0 hover:bg-surface-muted">
          <td
            class="whitespace-nowrap px-4 py-3 text-sm font-mono text-muted-foreground"
          >
            {{ run.id }}
          </td>
          <td class="max-w-xl px-4 py-3 text-sm text-muted-foreground">
            <span class="line-clamp-2">{{ run.prompt }}</span>
          </td>
          <td
            class="whitespace-nowrap px-4 py-3 text-sm text-subtle-foreground"
          >
            {{ run.tokens ?? "—" }}
          </td>
          <td
            class="whitespace-nowrap px-4 py-3 text-sm text-subtle-foreground"
          >
            <time :datetime="run.createdAt">{{
              formatDate(run.createdAt)
            }}</time>
          </td>
          <td class="whitespace-nowrap px-4 py-3 text-sm">
            <StatusBadge :state="run.status">{{ run.status }}</StatusBadge>
          </td>
        </tr>
        <tr v-if="!runs.length">
          <td
            colspan="5"
            class="px-4 py-8 text-center text-sm text-subtle-foreground"
          >
            No runs yet.
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import type { AgentRun } from "@/api/client";
import StatusBadge from "./StatusBadge.vue";

defineProps<{ runs: AgentRun[] }>();

function formatDate(value: string) {
  const date = new Date(value);
  return date.toLocaleString();
}
</script>
