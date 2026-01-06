<template>
  <div
    class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg flex h-full flex-col overflow-hidden"
  >
    <header class="flex items-center justify-between gap-2">
      <div>
        <p class="text-xs uppercase tracking-wide text-subtle-foreground">
          Agents
        </p>
        <h2 class="text-base font-semibold text-foreground">Status</h2>
      </div>
      <Pill tone="neutral" size="sm">{{
        agents.length ? `${agents.length} total` : "None"
      }}</Pill>
    </header>

    <p v-if="!agents.length" class="mt-4 text-xs text-faint-foreground">
      No agents reported from the backend yet.
    </p>

    <ul v-else class="mt-4 space-y-2 overflow-y-auto pr-1 text-xs">
      <li
        v-for="agent in agents"
        :key="agent.id"
        class="flex items-center justify-between gap-2 rounded-[14px] border border-white/10 bg-surface-muted/40 px-3 py-2"
      >
        <div class="min-w-0">
          <p class="truncate text-xs font-semibold text-foreground">
            {{ agent.name || agent.id }}
          </p>
          <p class="mt-0.5 truncate text-[11px] text-faint-foreground">
            {{ agent.model || "Model not set" }}
          </p>
        </div>
        <div class="flex flex-col items-end gap-1">
          <Pill :tone="agentTone(agent.state)" size="sm">{{
            agent.state || "unknown"
          }}</Pill>
          <span class="text-[10px] text-faint-foreground">
            {{ formatRelativeTime(agent.updatedAt) }}
          </span>
        </div>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import Pill from "@/components/ui/Pill.vue";

type Agent = {
  id: string | number;
  name?: string;
  model?: string;
  state?: string;
  updatedAt?: string;
};

const props = defineProps<{ agents: Agent[] }>();
const agents = computed(() => props.agents ?? []);

const agentTone = (state?: string) => {
  if (state === "online") return "success";
  if (state === "degraded") return "warning";
  if (state === "offline") return "danger";
  return "neutral";
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
