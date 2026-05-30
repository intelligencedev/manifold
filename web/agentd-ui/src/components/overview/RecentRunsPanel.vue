<template>
  <div class="halo-surface flex h-full flex-col overflow-hidden p-5">
    <header class="flex items-center justify-between gap-2">
      <div>
        <p class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground">
          Recent Runs
        </p>
        <h2 class="font-display text-xl font-semibold text-foreground">Past 24 hours</h2>
      </div>
      <MBadge tone="neutral">{{
        runs.length ? `${runs.length} shown` : "None"
      }}</MBadge>
    </header>

    <p v-if="!runs.length" class="mt-4 text-xs text-faint-foreground">
      No recent runs in the last 24 hours.
    </p>

    <div v-else class="mt-4 overflow-hidden rounded-lg border border-border">
      <MRow
        v-for="run in runs"
        :key="run.id"
      >
        <template #avatar>
          RN
        </template>
        <template #title>
          <p class="line-clamp-2">
            {{ run.prompt || "Untitled run" }}
          </p>
        </template>
        <template #meta>
          <span class="font-mono text-[10px] uppercase tracking-[0.08em]">
            {{ formatRelativeTime(run.createdAt) }}
          </span>
        </template>
        <template #status>
          <MStatus
            :state="runState(run.status)"
            :label="run.status || 'unknown'"
            :pulse="run.status === 'running'"
          />
        </template>
      </MRow>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import MBadge from "@/components/ui/MBadge.vue";
import MRow from "@/components/ui/MRow.vue";
import MStatus from "@/components/ui/MStatus.vue";

type Run = {
  id: string | number;
  status?: string;
  prompt?: string;
  createdAt?: string;
};

const props = defineProps<{ runs: Run[] }>();
const runs = computed(() => props.runs ?? []);

const runState = (status?: string): "run" | "ok" | "warn" | "danger" | "idle" => {
  if (status === "completed") return "ok";
  if (status === "running") return "run";
  if (status === "failed") return "danger";
  return "idle";
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
