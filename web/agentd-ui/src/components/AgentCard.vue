<template>
  <div
    class="flex flex-col rounded-2xl border border-border/70 bg-surface p-6 shadow-lg"
  >
    <div class="flex items-center justify-between">
      <div>
        <p class="text-base font-medium text-muted-foreground">
          {{ agent.name }}
        </p>
        <p class="text-sm text-subtle-foreground">Model {{ agent.model }}</p>
      </div>
      <StatusBadge :state="agent.state">{{ agent.state }}</StatusBadge>
    </div>
    <div
      class="mt-6 flex items-center justify-between text-sm text-subtle-foreground"
    >
      <p>ID: {{ agent.id }}</p>
      <p>
        Updated
        <time :datetime="agent.updatedAt">{{ relativeUpdated }}</time>
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import type { AgentStatus } from "@/api/client";
import StatusBadge from "./StatusBadge.vue";

const props = defineProps<{ agent: AgentStatus }>();

const relativeUpdated = computed(() => {
  const updated = new Date(props.agent.updatedAt);
  const now = new Date();
  const diff = Math.abs(now.getTime() - updated.getTime());
  const minutes = Math.round(diff / 60000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return `${days}d ago`;
});
</script>
