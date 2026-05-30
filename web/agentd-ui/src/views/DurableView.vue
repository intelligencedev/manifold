<template>
  <section class="flex h-full min-h-0 flex-col overflow-hidden">
    <header class="flex items-start justify-between gap-6 px-6 py-3">
      <div class="min-w-0">
        <p
          class="text-[10px] font-semibold uppercase tracking-[0.24em] text-subtle-foreground"
        >
          Durable Workflows
        </p>
        <h1 class="mt-1 text-2xl font-semibold leading-tight text-foreground">
          Queue Operations
        </h1>
        <p class="mt-1 max-w-3xl text-sm text-subtle-foreground">
          Postgres-backed task queues, leases, waits, retries, and checkpointed
          runs.
        </p>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <Chip muted>Auto-refresh</Chip>
        <AppButton size="sm" :loading="isRefreshing" @click="refreshAll">
          Refresh
        </AppButton>
      </div>
    </header>

    <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-3 pb-3">
      <section class="grid grid-cols-6 gap-3">
        <div
          v-for="stat in statusTotals"
          :key="stat.key"
          class="halo-surface rounded-4 border border-border/70 p-4"
        >
          <p
            class="text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
          >
            {{ stat.label }}
          </p>
          <p
            :class="[
              'mt-2 text-2xl font-semibold leading-none tabular-nums',
              stat.tone,
            ]"
          >
            {{ formatNumber(stat.value) }}
          </p>
        </div>
      </section>

      <section
        class="halo-surface flex min-h-[220px] flex-col rounded-4 border border-border/70"
      >
        <div
          class="flex items-center justify-between gap-4 border-b border-border/60 pb-3"
        >
          <div>
            <p
              class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
            >
              Queues
            </p>
            <p class="mt-1 text-sm text-faint-foreground">
              {{ queueSummary }}
            </p>
          </div>
          <div class="text-right text-xs text-subtle-foreground">
            <p>{{ formatNumber(totalTaskCount) }} total tasks</p>
            <p class="mt-1 text-faint-foreground">
              {{ formatNumber(activeWorkCount) }} active
            </p>
          </div>
        </div>

        <div class="mt-3 min-h-0 flex-1 overflow-auto">
          <div
            class="grid grid-cols-[minmax(190px,1.6fr)_repeat(6,minmax(86px,1fr))] gap-2 border-b border-border/60 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
          >
            <span>Queue</span>
            <span class="text-right">Queued</span>
            <span class="text-right">Running</span>
            <span class="text-right">Waiting</span>
            <span class="text-right">Completed</span>
            <span class="text-right">Failed</span>
            <span class="text-right">Cancelled</span>
          </div>
          <div
            v-if="queuesLoading"
            class="mt-3 rounded-3 border border-border/60 bg-surface-muted/30 p-4 text-sm text-subtle-foreground"
          >
            Loading queue health...
          </div>
          <div
            v-else-if="queues.length === 0"
            class="mt-3 rounded-3 border border-border/60 bg-surface-muted/30 p-4 text-sm text-subtle-foreground"
          >
            No durable queues have recorded tasks yet.
          </div>
          <template v-else>
            <button
              v-for="queue in queues"
              :key="queue.queue"
              type="button"
              :class="[
                'mt-2 grid w-full grid-cols-[minmax(190px,1.6fr)_repeat(6,minmax(86px,1fr))] gap-2 rounded-3 border px-3 py-2.5 text-sm transition',
                selectedQueue === queue.queue
                  ? 'border-accent/60 bg-accent/10'
                  : 'border-border/60 bg-surface-muted/25 hover:border-border hover:bg-surface-muted/45',
              ]"
              @click="toggleQueue(queue.queue)"
            >
              <span
                class="min-w-0 truncate text-left font-medium text-foreground"
              >
                {{ queue.queue }}
              </span>
              <span class="text-right tabular-nums text-info">
                {{ formatNumber(queue.queued) }}
              </span>
              <span class="text-right tabular-nums text-accent">
                {{ formatNumber(queue.running) }}
              </span>
              <span class="text-right tabular-nums text-warning">
                {{ formatNumber(queue.waiting) }}
              </span>
              <span class="text-right tabular-nums text-success">
                {{ formatNumber(queue.completed) }}
              </span>
              <span class="text-right tabular-nums text-danger">
                {{ formatNumber(queue.failed) }}
              </span>
              <span class="text-right tabular-nums text-subtle-foreground">
                {{ formatNumber(queue.cancelled) }}
              </span>
            </button>
          </template>
        </div>
      </section>

      <section
        class="halo-surface flex min-h-0 flex-1 flex-col rounded-4 border border-border/70"
      >
        <div
          class="flex flex-wrap items-end justify-between gap-3 border-b border-border/60 pb-3"
        >
          <div>
            <p
              class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
            >
              Task Listing
            </p>
            <p class="mt-1 text-sm text-faint-foreground">
              {{ tasks.length }} visible from the latest query
            </p>
          </div>
          <div class="flex flex-wrap items-end gap-2">
            <label class="grid gap-1 text-xs text-subtle-foreground">
              <span>Queue</span>
              <select
                v-model="selectedQueue"
                class="min-h-9 rounded-3 border border-border/70 bg-surface-muted/55 px-3 text-sm text-foreground outline-none transition focus:border-accent"
              >
                <option value="">All queues</option>
                <option v-for="queue in queueNames" :key="queue" :value="queue">
                  {{ queue }}
                </option>
              </select>
            </label>
            <label class="grid gap-1 text-xs text-subtle-foreground">
              <span>Status</span>
              <select
                v-model="selectedStatus"
                class="min-h-9 rounded-3 border border-border/70 bg-surface-muted/55 px-3 text-sm text-foreground outline-none transition focus:border-accent"
              >
                <option value="">All statuses</option>
                <option
                  v-for="status in taskStatuses"
                  :key="status"
                  :value="status"
                >
                  {{ status }}
                </option>
              </select>
            </label>
            <label class="grid gap-1 text-xs text-subtle-foreground">
              <span>Name</span>
              <input
                v-model.trim="taskNameFilter"
                type="search"
                placeholder="Task name"
                class="min-h-9 w-56 rounded-3 border border-border/70 bg-surface-muted/55 px-3 text-sm text-foreground outline-none transition placeholder:text-faint-foreground focus:border-accent"
              />
            </label>
          </div>
        </div>

        <div class="mt-3 min-h-0 flex-1 overflow-auto">
          <div
            class="grid grid-cols-[minmax(260px,1.5fr)_minmax(150px,0.8fr)_minmax(120px,0.7fr)_90px_minmax(155px,0.8fr)_minmax(155px,0.8fr)_80px] gap-3 border-b border-border/60 pb-2 text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
          >
            <span>Task</span>
            <span>Queue</span>
            <span>Status</span>
            <span class="text-right">Attempt</span>
            <span>Available</span>
            <span>Updated</span>
            <span class="text-right">Action</span>
          </div>
          <div
            v-if="tasksLoading"
            class="mt-3 rounded-3 border border-border/60 bg-surface-muted/30 p-4 text-sm text-subtle-foreground"
          >
            Loading durable tasks...
          </div>
          <div
            v-else-if="tasks.length === 0"
            class="mt-3 rounded-3 border border-border/60 bg-surface-muted/30 p-4 text-sm text-subtle-foreground"
          >
            No tasks match the current filters.
          </div>
          <template v-else>
            <button
              v-for="task in tasks"
              :key="task.id"
              type="button"
              class="mt-2 grid w-full grid-cols-[minmax(260px,1.5fr)_minmax(150px,0.8fr)_minmax(120px,0.7fr)_90px_minmax(155px,0.8fr)_minmax(155px,0.8fr)_80px] gap-3 rounded-3 border border-border/60 bg-surface-muted/25 px-3 py-2.5 text-left text-sm transition hover:border-border hover:bg-surface-muted/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/70"
              @click="openTask(task)"
            >
              <span class="min-w-0">
                <span class="block truncate font-medium text-foreground">
                  {{ task.name }}
                </span>
                <span class="mt-1 block truncate text-xs text-faint-foreground">
                  {{ task.id }}
                </span>
              </span>
              <span class="min-w-0 truncate text-subtle-foreground">
                {{ task.queue }}
              </span>
              <span>
                <Chip :class="['capitalize', statusTone(task.status)]">
                  {{ task.status }}
                </Chip>
              </span>
              <span class="text-right tabular-nums text-foreground">
                {{ task.attempt }}
              </span>
              <span class="min-w-0 truncate text-subtle-foreground">
                {{ formatDate(task.available_at) }}
              </span>
              <span class="min-w-0 truncate text-subtle-foreground">
                {{ formatDate(task.updated_at) }}
              </span>
              <span class="text-right text-xs font-semibold text-accent">
                View
              </span>
            </button>
          </template>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { useRoute, useRouter } from "vue-router";
import {
  fetchDurableQueues,
  listDurableTasks,
  type DurableTask,
  type DurableTaskStatus,
} from "@/api/durable";
import AppButton from "@/components/ui/AppButton.vue";
import Chip from "@/components/ui/Chip.vue";

const props = withDefaults(
  defineProps<{
    embedded?: boolean;
  }>(),
  {
    embedded: false,
  },
);

const route = useRoute();
const router = useRouter();

const taskStatuses: DurableTaskStatus[] = [
  "queued",
  "running",
  "waiting",
  "completed",
  "failed",
  "cancelled",
];

const selectedQueue = ref(queryString(route.query.queue));
const selectedStatus = ref<DurableTaskStatus | "">(
  taskStatuses.includes(queryString(route.query.status) as DurableTaskStatus)
    ? (queryString(route.query.status) as DurableTaskStatus)
    : "",
);
const taskNameFilter = ref(queryString(route.query.name));

const taskFilters = computed(() => ({
  queue: selectedQueue.value,
  status: selectedStatus.value,
  name: taskNameFilter.value.trim(),
  limit: 75,
}));

const listingQuery = computed(() => {
  const query: Record<string, string> = {};
  if (props.embedded) query.tab = "queue-ops";
  if (selectedQueue.value) query.queue = selectedQueue.value;
  if (selectedStatus.value) query.status = selectedStatus.value;
  if (taskNameFilter.value.trim()) query.name = taskNameFilter.value.trim();
  return query;
});

const {
  data: queueData,
  isLoading: queuesLoading,
  refetch: refetchQueues,
} = useQuery({
  queryKey: ["durable", "queues"],
  queryFn: fetchDurableQueues,
  refetchInterval: 5_000,
});

const {
  data: taskData,
  isLoading: tasksLoading,
  refetch: refetchTasks,
} = useQuery({
  queryKey: computed(() => ["durable", "tasks", taskFilters.value]),
  queryFn: () => listDurableTasks(taskFilters.value),
  refetchInterval: 5_000,
});

const queues = computed(() => queueData.value ?? []);
const tasks = computed(() => taskData.value ?? []);

const queueNames = computed(() => {
  const names = new Set<string>();
  for (const queue of queues.value) names.add(queue.queue);
  for (const task of tasks.value) names.add(task.queue);
  return Array.from(names).sort((a, b) => a.localeCompare(b));
});

const totals = computed(() =>
  queues.value.reduce(
    (acc, queue) => {
      acc.queued += queue.queued;
      acc.running += queue.running;
      acc.waiting += queue.waiting;
      acc.completed += queue.completed;
      acc.failed += queue.failed;
      acc.cancelled += queue.cancelled;
      return acc;
    },
    {
      queued: 0,
      running: 0,
      waiting: 0,
      completed: 0,
      failed: 0,
      cancelled: 0,
    },
  ),
);

const statusTotals = computed(() => [
  {
    key: "queued",
    label: "Queued",
    value: totals.value.queued,
    tone: "text-info",
  },
  {
    key: "running",
    label: "Running",
    value: totals.value.running,
    tone: "text-accent",
  },
  {
    key: "waiting",
    label: "Waiting",
    value: totals.value.waiting,
    tone: "text-warning",
  },
  {
    key: "completed",
    label: "Completed",
    value: totals.value.completed,
    tone: "text-success",
  },
  {
    key: "failed",
    label: "Failed",
    value: totals.value.failed,
    tone: "text-danger",
  },
  {
    key: "cancelled",
    label: "Cancelled",
    value: totals.value.cancelled,
    tone: "text-subtle-foreground",
  },
]);

const totalTaskCount = computed(() =>
  statusTotals.value.reduce((sum, stat) => sum + stat.value, 0),
);

const activeWorkCount = computed(
  () => totals.value.queued + totals.value.running + totals.value.waiting,
);

const queueSummary = computed(() => {
  if (queues.value.length === 0) return "No queues reporting";
  return `${queues.value.length} queues reporting`;
});

const isRefreshing = computed(() => queuesLoading.value || tasksLoading.value);

function toggleQueue(queue: string) {
  selectedQueue.value = selectedQueue.value === queue ? "" : queue;
}

async function refreshAll() {
  await Promise.all([refetchQueues(), refetchTasks()]);
}

function openTask(task: DurableTask) {
  void router.push({
    name: "durable-task",
    params: { taskId: task.id },
    query: listingQuery.value,
  });
}

function statusTone(status: DurableTaskStatus) {
  switch (status) {
    case "completed":
      return "border-success/50 bg-success/10 text-success";
    case "failed":
    case "cancelled":
      return "border-danger/50 bg-danger/10 text-danger";
    case "running":
      return "border-accent/50 bg-accent/10 text-accent";
    case "waiting":
      return "border-warning/50 bg-warning/10 text-warning";
    case "queued":
    default:
      return "border-info/50 bg-info/10 text-info";
  }
}

function formatNumber(value: number) {
  return new Intl.NumberFormat().format(value);
}

function formatDate(value?: string) {
  if (!value) return "Never";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  }).format(date);
}

function queryString(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === "string" ? value[0] : "";
  }
  return typeof value === "string" ? value : "";
}
</script>
