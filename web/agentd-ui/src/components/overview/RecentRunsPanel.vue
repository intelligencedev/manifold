<template>
  <div
    class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg flex h-full flex-col overflow-hidden"
  >
    <header class="flex items-center justify-between gap-2">
      <div>
        <p class="text-xs uppercase tracking-wide text-subtle-foreground">
          Recent Runs
        </p>
        <h2 class="text-base font-semibold text-foreground">Past 24 hours</h2>
      </div>
      <Pill tone="neutral" size="sm">{{
        runs.length ? `${runs.length} shown` : "None"
      }}</Pill>
    </header>

    <p v-if="!runs.length" class="mt-4 text-xs text-faint-foreground">
      No recent runs in the last 24 hours.
    </p>

    <ul v-else class="mt-4 space-y-2 overflow-y-auto pr-1 text-xs">
      <li
        v-for="run in runs"
        :key="run.id"
        class="rounded-[14px] border border-white/10 bg-surface-muted/40 px-3 py-2"
      >
        <p class="line-clamp-2 text-xs font-semibold text-foreground">
          {{ run.prompt || "Untitled run" }}
        </p>
        <div
          class="mt-2 flex items-center justify-between gap-2 text-[11px] text-faint-foreground"
        >
          <Pill :tone="runTone(run.status)" size="sm">{{
            run.status || "unknown"
          }}</Pill>
          <span>{{ formatRelativeTime(run.createdAt) }}</span>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import Pill from "@/components/ui/Pill.vue";

type Run = {
  id: string | number;
  status?: string;
  prompt?: string;
  createdAt?: string;
};

const props = defineProps<{ runs: Run[] }>();
const runs = computed(() => props.runs ?? []);

const runTone = (status?: string) => {
  if (status === "completed") return "success";
  if (status === "running") return "accent";
  return "danger";
};

const formatRelativeTime = (value?: string) => {
  if (!value) return "";
  const date = new Date(value);
  const now = new Date();
  const diffSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (!Number.isFinite(diffSeconds)) return "";
  if (diffSeconds < 45) return "just now";

  const minutes = Math.floor(diffSeconds / 60);
  if (minutes < 60) return `${minutes} min${minutes === 1 ? "" : "s"} ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours} h${hours === 1 ? "" : "s"} ago`;

  const days = Math.floor(hours / 24);
  return `${days} d${days === 1 ? "" : "s"} ago`;
};
</script>
