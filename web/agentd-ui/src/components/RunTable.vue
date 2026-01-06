<template>
  <div
    class="overflow-hidden rounded-2xl border border-border/70 bg-surface shadow-lg"
  >
    <table class="min-w-full divide-y divide-border/60">
      <thead class="bg-surface/80">
        <tr>
          <th
            class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
          >
            Run ID
          </th>
          <th
            class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
          >
            Prompt
          </th>
          <th
            class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
          >
            Tokens
          </th>
          <th
            class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
          >
            Started
          </th>
          <th
            class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wide text-subtle-foreground"
          >
            Status
          </th>
        </tr>
      </thead>
      <tbody class="divide-y divide-border/60">
        <tr v-for="run in runs" :key="run.id" class="hover:bg-surface-muted/60">
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
