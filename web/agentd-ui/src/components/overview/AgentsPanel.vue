<template>
  <div class="halo-surface flex h-full flex-col overflow-hidden p-5">
    <header class="flex items-center justify-between gap-2">
      <div>
        <p class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground">
          Agents
        </p>
        <h2 class="font-display text-xl font-semibold text-foreground">Status</h2>
      </div>
      <MBadge tone="neutral">{{
        agents.length ? `${agents.length} total` : "None"
      }}</MBadge>
    </header>

    <p v-if="!agents.length" class="mt-4 text-xs text-faint-foreground">
      No agents reported from the backend yet.
    </p>

    <div v-else class="mt-4 overflow-hidden rounded-lg border border-border">
      <MRow
        v-for="agent in agents"
        :key="agent.id"
      >
        <template #avatar>
          {{ initials(agent.name || String(agent.id)) }}
        </template>
        <template #title>
          <p class="truncate">
            {{ agent.name || agent.id }}
          </p>
        </template>
        <template #meta>
          <p class="truncate">
            {{ agent.model || "Model not set" }}
          </p>
        </template>
        <template #status>
          <div class="flex flex-col items-end gap-1">
            <MStatus
              :state="statusState(agent.state)"
              :label="agent.state || 'unknown'"
              :pulse="agent.state === 'online'"
            />
            <span class="font-mono text-[10px] text-faint-foreground">
            {{ formatRelativeTime(agent.updatedAt) }}
            </span>
          </div>
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

type Agent = {
  id: string | number;
  name?: string;
  model?: string;
  state?: string;
  updatedAt?: string;
};

const props = defineProps<{ agents: Agent[] }>();
const agents = computed(() => props.agents ?? []);

const statusState = (state?: string): "run" | "ok" | "warn" | "danger" | "idle" => {
  if (state === "online") return "ok";
  if (state === "degraded") return "warn";
  if (state === "offline") return "danger";
  return "idle";
};

const initials = (value: string) => {
  return value
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase())
    .join("");
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
