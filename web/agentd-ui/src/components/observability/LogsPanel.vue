<template>
  <div
    class="rounded-2xl border border-border/70 bg-surface p-6 shadow-lg flex h-full flex-col overflow-hidden"
  >
    <div class="flex flex-wrap items-start justify-between gap-4 mb-4">
      <div>
        <h2 class="text-lg font-semibold text-foreground">System Logs</h2>
        <p class="text-xs text-faint-foreground">
          Real-time application logs (Admin only)
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          @click="refreshLogs"
          :disabled="loading"
          class="rounded-lg border border-border/60 bg-muted/10 px-3 py-1.5 text-xs text-foreground hover:bg-muted/20 disabled:opacity-50 transition-colors"
        >
          {{ loading ? "Loading..." : "Refresh" }}
        </button>
        <button
          @click="clearLogs"
          class="rounded-lg border border-border/60 bg-muted/10 px-3 py-1.5 text-xs text-foreground hover:bg-muted/20 transition-colors"
        >
          Clear
        </button>
      </div>
    </div>

    <div
      class="flex-1 overflow-auto rounded-lg border border-border/60 bg-surface-muted/40 p-3 font-mono text-xs"
    >
      <div v-if="loading && logs.length === 0" class="text-faint-foreground">
        Loading logs...
      </div>
      <div v-else-if="error" class="text-danger">
        {{ error }}
      </div>
      <div v-else-if="logs.length === 0" class="text-faint-foreground">
        No logs available
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="(log, index) in logs"
          :key="index"
          :class="[
            'flex gap-3 leading-relaxed hover:bg-muted/10 px-2 py-1 rounded',
            getLogLevelClass(log.level),
          ]"
        >
          <span class="text-faint-foreground shrink-0">{{
            formatTimestamp(log.timestamp)
          }}</span>
          <span
            class="font-semibold shrink-0 uppercase"
            :class="getLevelColorClass(log.level)"
          >
            {{ log.level }}
          </span>
          <span class="text-foreground break-all">{{ log.message }}</span>
        </div>
      </div>
    </div>

    <div
      class="mt-3 flex items-center justify-between text-xs text-faint-foreground"
    >
      <span>{{ logs.length }} log{{ logs.length === 1 ? "" : "s" }}</span>
      <span v-if="lastUpdated"
        >Last updated: {{ formatRelativeTime(lastUpdated) }}</span
      >
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { fetchLogs, type LogEntry } from "@/api/client";

const logs = ref<LogEntry[]>([]);
const loading = ref(false);
const error = ref<string | null>(null);
const lastUpdated = ref<Date | null>(null);

let refreshInterval: number | null = null;

async function refreshLogs() {
  loading.value = true;
  error.value = null;
  try {
    const data = await fetchLogs({ limit: 100 });
    logs.value = data;
    lastUpdated.value = new Date();
  } catch (err: any) {
    error.value =
      err.response?.data?.message || err.message || "Failed to fetch logs";
  } finally {
    loading.value = false;
  }
}

function clearLogs() {
  logs.value = [];
}

function getLogLevelClass(level: string) {
  const normalized = level.toLowerCase();
  if (normalized === "error" || normalized === "fatal") return "bg-danger/5";
  if (normalized === "warn" || normalized === "warning") return "bg-warning/5";
  if (normalized === "info") return "bg-info/5";
  return "";
}

function getLevelColorClass(level: string) {
  const normalized = level.toLowerCase();
  if (normalized === "error" || normalized === "fatal") return "text-danger";
  if (normalized === "warn" || normalized === "warning") return "text-warning";
  if (normalized === "info") return "text-info";
  if (normalized === "debug") return "text-subtle-foreground";
  return "text-foreground";
}

function formatTimestamp(timestamp: string | number): string {
  const date = new Date(timestamp);
  return date.toLocaleTimeString("en-US", {
    hour12: false,
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatRelativeTime(date: Date): string {
  const now = new Date();
  const diffSeconds = Math.floor((now.getTime() - date.getTime()) / 1000);

  if (diffSeconds < 10) return "just now";
  if (diffSeconds < 60) return `${diffSeconds}s ago`;

  const minutes = Math.floor(diffSeconds / 60);
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

onMounted(() => {
  refreshLogs();
  // Auto-refresh every 10 seconds
  refreshInterval = setInterval(refreshLogs, 10000);
});

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval);
  }
});
</script>
