<template>
  <section class="flex h-full min-h-0 flex-col overflow-hidden">
    <header class="flex items-start justify-between gap-6 px-6 py-3">
      <div class="flex min-w-0 items-start gap-3">
        <AppButton size="sm" variant="ghost" @click="goBack"> Back </AppButton>
        <div class="min-w-0">
          <p
            class="text-[10px] font-semibold uppercase tracking-[0.24em] text-subtle-foreground"
          >
            Durable Task
          </p>
          <h1
            class="mt-1 truncate text-2xl font-semibold leading-tight text-foreground"
          >
            {{ task?.name ?? taskId }}
          </h1>
          <p class="mt-1 truncate text-sm text-subtle-foreground">
            {{ taskId }}
          </p>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-2">
        <Chip v-if="task" :class="['capitalize', statusTone(task.status)]">
          {{ task.status }}
        </Chip>
        <Chip muted>Auto-refresh</Chip>
        <AppButton size="sm" :loading="isRefreshing" @click="refreshAll">
          Refresh
        </AppButton>
      </div>
    </header>

    <div class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden px-3 pb-3">
      <div
        v-if="taskErrorMessage || eventsErrorMessage"
        class="rounded-4 border border-danger/40 bg-danger/10 px-4 py-3 text-sm text-danger"
      >
        {{ taskErrorMessage || eventsErrorMessage }}
      </div>

      <TaskInspector
        class="min-h-0 flex-1"
        :task="task"
        :events="events"
        :events-page="eventData ?? null"
        :events-loading="eventsLoading"
        :cancel-loading="cancelLoading"
        :retry-loading="retryLoading"
        :show-close="false"
        :empty-message="
          taskLoading ? 'Loading task details...' : 'Task unavailable.'
        "
        @cancel="cancelSelectedTask"
        @retry="retrySelectedTask"
        @events-older="loadOlderEvents"
        @events-newer="loadNewerEvents"
        @events-latest="loadLatestEvents"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, shallowRef, watch } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { useRoute, useRouter } from "vue-router";
import {
  cancelDurableTask,
  fetchDurableTask,
  fetchDurableTaskEvents,
  retryDurableTask,
  type DurableTaskEventsParams,
  type DurableTaskStatus,
} from "@/api/durable";
import TaskInspector from "@/components/durable/TaskInspector.vue";
import AppButton from "@/components/ui/AppButton.vue";
import Chip from "@/components/ui/Chip.vue";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();
const eventPageLimit = 200;

const taskId = computed(() => routeParam(route.params.taskId));
const eventPageCursor = shallowRef<Omit<DurableTaskEventsParams, "limit">>({});
const eventQueryParams = computed<DurableTaskEventsParams>(() => ({
  ...eventPageCursor.value,
  limit: eventPageLimit,
}));

const {
  data: taskData,
  isLoading: taskLoading,
  error: taskError,
  refetch: refetchTask,
} = useQuery({
  queryKey: computed(() => ["durable", "task", taskId.value]),
  queryFn: () => fetchDurableTask(taskId.value),
  enabled: computed(() => Boolean(taskId.value)),
  refetchInterval: 5_000,
});

const {
  data: eventData,
  isLoading: eventsLoading,
  error: eventsError,
  refetch: refetchEvents,
} = useQuery({
  queryKey: computed(() => [
    "durable",
    "task-events",
    taskId.value,
    eventQueryParams.value,
  ]),
  queryFn: () => fetchDurableTaskEvents(taskId.value, eventQueryParams.value),
  enabled: computed(() => Boolean(taskId.value)),
  refetchInterval: 5_000,
});

watch(taskId, () => {
  eventPageCursor.value = {};
});

const cancelMutation = useMutation({
  mutationFn: cancelDurableTask,
  onSuccess: () => {
    void queryClient.invalidateQueries({ queryKey: ["durable"] });
  },
});

const retryMutation = useMutation({
  mutationFn: ({
    taskId,
    resetCheckpoints,
  }: {
    taskId: string;
    resetCheckpoints: boolean;
  }) => retryDurableTask(taskId, resetCheckpoints),
  onSuccess: () => {
    void queryClient.invalidateQueries({ queryKey: ["durable"] });
  },
});

const task = computed(() => taskData.value ?? null);
const events = computed(() => eventData.value?.events ?? []);

const cancelLoading = computed(() => cancelMutation.isPending.value);
const retryLoading = computed(() => retryMutation.isPending.value);
const isRefreshing = computed(
  () =>
    taskLoading.value ||
    eventsLoading.value ||
    cancelLoading.value ||
    retryLoading.value,
);

const taskErrorMessage = computed(() => errorMessage(taskError.value));
const eventsErrorMessage = computed(() => errorMessage(eventsError.value));

async function refreshAll() {
  await Promise.all([refetchTask(), refetchEvents()]);
}

function goBack() {
  if (route.query.tab === "queue-ops") {
    void router.push({ name: "overview", query: route.query });
    return;
  }
  void router.push({ name: "durable", query: route.query });
}

function cancelSelectedTask(taskId: string) {
  cancelMutation.mutate(taskId);
}

function retrySelectedTask(taskId: string, resetCheckpoints: boolean) {
  retryMutation.mutate({ taskId, resetCheckpoints });
}

function loadOlderEvents() {
  const firstSequence = eventData.value?.first_sequence;
  if (!firstSequence) return;
  eventPageCursor.value = { before: firstSequence };
}

function loadNewerEvents() {
  const lastSequence = eventData.value?.last_sequence;
  if (!lastSequence) return;
  eventPageCursor.value = { after: lastSequence };
}

function loadLatestEvents() {
  eventPageCursor.value = {};
}

function routeParam(value: unknown) {
  if (Array.isArray(value)) {
    return typeof value[0] === "string" ? value[0] : "";
  }
  return typeof value === "string" ? value : "";
}

function errorMessage(value: unknown) {
  if (!value) return "";
  if (value instanceof Error) return value.message;
  return "Unable to load durable task details.";
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
</script>
