<template>
  <aside
    class="glass-surface flex h-full min-h-0 flex-col rounded-4 border border-border/70"
  >
    <!-- Full-width header -->
    <header
      class="flex items-start justify-between gap-3 border-b border-border/60 px-4 py-3"
    >
      <div class="min-w-0">
        <p
          class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
        >
          Task Inspector
        </p>
        <h2 class="mt-1 truncate text-lg font-semibold text-foreground">
          {{ task?.name ?? "No task selected" }}
        </h2>
        <p v-if="task" class="mt-1 truncate text-xs text-faint-foreground">
          {{ task.id }}
        </p>
      </div>
      <button
        v-if="showClose"
        type="button"
        class="rounded-full border border-transparent px-2.5 py-1 text-xs font-semibold text-subtle-foreground transition hover:border-border/70 hover:bg-surface-muted/55 hover:text-foreground"
        @click="$emit('close')"
      >
        Close
      </button>
    </header>

    <!-- Empty state -->
    <div
      v-if="!task"
      class="flex min-h-0 flex-1 items-center justify-center px-4 text-center text-sm text-subtle-foreground"
    >
      {{ emptyMessage }}
    </div>

    <!-- Two-column layout -->
    <template v-else>
      <div class="flex min-h-0 flex-1 overflow-hidden">
        <!-- Left column: meta + lifecycle + payloads -->
        <div
          class="flex min-w-[260px] max-w-xs flex-col overflow-y-auto border-r border-border/60"
        >
          <!-- Meta cards -->
          <section class="border-b border-border/60 px-4 py-3">
            <div class="grid grid-cols-2 gap-2">
              <div class="rounded-3 border border-border/60 bg-surface-muted/35 p-3">
                <p
                  class="text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
                >
                  Status
                </p>
                <Chip :class="['mt-2 capitalize', statusTone(task.status)]">
                  {{ task.status }}
                </Chip>
              </div>
              <div class="rounded-3 border border-border/60 bg-surface-muted/35 p-3">
                <p
                  class="text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
                >
                  Attempts
                </p>
                <p class="mt-2 text-xl font-semibold text-foreground tabular-nums">
                  {{ task.attempt }}
                </p>
              </div>
              <div class="rounded-3 border border-border/60 bg-surface-muted/35 p-3">
                <p
                  class="text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
                >
                  Queue
                </p>
                <p class="mt-2 truncate text-sm font-medium text-foreground">
                  {{ task.queue }}
                </p>
              </div>
              <div class="rounded-3 border border-border/60 bg-surface-muted/35 p-3">
                <p
                  class="text-[10px] font-semibold uppercase tracking-[0.18em] text-faint-foreground"
                >
                  Updated
                </p>
                <p class="mt-2 truncate text-sm font-medium text-foreground">
                  {{ formatDate(task.updated_at) }}
                </p>
              </div>
            </div>
          </section>

          <!-- Lifecycle -->
          <section class="border-b border-border/60 px-4 py-3">
            <div class="flex items-center justify-between gap-3">
              <div>
                <p
                  class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
                >
                  Lifecycle
                </p>
                <p class="mt-1 text-xs text-faint-foreground">
                  Created {{ formatDate(task.created_at) }}
                </p>
              </div>
              <AppButton
                v-if="!isTerminal"
                size="xs"
                variant="danger"
                :loading="cancelLoading"
                @click="$emit('cancel', task.id)"
              >
                Cancel
              </AppButton>
              <div v-else-if="canRetry" class="flex items-center gap-2">
                <label
                  class="flex items-center gap-2 text-xs text-subtle-foreground"
                >
                  <input
                    v-model="resetCheckpoints"
                    type="checkbox"
                    class="h-3.5 w-3.5 rounded border-border bg-surface-muted accent-accent"
                  />
                  Reset checkpoints
                </label>
                <AppButton
                  size="xs"
                  variant="accent"
                  :loading="retryLoading"
                  @click="$emit('retry', task.id, resetCheckpoints)"
                >
                  Retry
                </AppButton>
              </div>
            </div>
            <dl class="mt-3 grid grid-cols-2 gap-2 text-xs">
              <div
                class="rounded-3 border border-border/60 bg-surface-muted/25 p-2.5"
              >
                <dt class="text-faint-foreground">Available</dt>
                <dd class="mt-1 truncate font-medium text-foreground">
                  {{ formatDate(task.available_at) }}
                </dd>
              </div>
              <div
                class="rounded-3 border border-border/60 bg-surface-muted/25 p-2.5"
              >
                <dt class="text-faint-foreground">Completed</dt>
                <dd class="mt-1 truncate font-medium text-foreground">
                  {{
                    task.completed_at ? formatDate(task.completed_at) : "Pending"
                  }}
                </dd>
              </div>
            </dl>
          </section>

          <!-- Payloads -->
          <section class="px-4 py-3">
            <p
              class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
            >
              Payloads
            </p>
            <div class="mt-3 grid gap-2">
              <details
                v-for="block in payloadBlocks"
                :key="block.label"
                class="rounded-3 border border-border/60 bg-surface-muted/25 p-3"
              >
                <summary class="cursor-pointer text-sm font-medium text-foreground">
                  {{ block.label }}
                </summary>
                <pre
                  class="mt-3 max-h-36 overflow-auto rounded-3 border border-border/50 bg-background/45 p-2 text-[11px] leading-relaxed text-subtle-foreground"
                  >{{ block.value }}</pre
                >
              </details>
              <p
                v-if="payloadBlocks.length === 0"
                class="text-xs text-faint-foreground"
              >
                No payload data.
              </p>
            </div>
          </section>
        </div>

        <!-- Right column: Event Timeline -->
        <div class="flex min-w-0 flex-1 flex-col overflow-hidden">
          <!-- Sticky timeline header -->
          <div
            class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-surface-muted/60 px-4 py-3 backdrop-blur-sm"
          >
            <p
              class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground"
            >
              Event Timeline
            </p>
            <span class="text-xs text-faint-foreground tabular-nums">
              {{ events.length }} events
            </span>
          </div>

          <!-- Scrollable timeline list -->
          <div class="min-h-0 flex-1 overflow-y-auto px-4 py-3">
            <div
              v-if="eventsLoading"
              class="rounded-3 border border-border/60 bg-surface-muted/25 p-3 text-sm text-subtle-foreground"
            >
              Loading timeline...
            </div>
            <div
              v-else-if="events.length === 0"
              class="rounded-3 border border-border/60 bg-surface-muted/25 p-3 text-sm text-subtle-foreground"
            >
              No events recorded yet.
            </div>
            <ol v-else class="space-y-2">
              <li
                v-for="event in events"
                :key="event.id"
                class="rounded-3 border border-border/60 bg-surface-muted/25 p-3"
              >
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-medium text-foreground">
                      {{ event.name }}
                    </p>
                    <p class="mt-1 text-xs text-faint-foreground">
                      #{{ event.sequence }} - {{ formatDate(event.occurred_at) }}
                    </p>
                  </div>
                  <Chip muted>event</Chip>
                </div>
                <pre
                  v-if="hasPayload(event.payload)"
                  class="mt-2 max-h-28 overflow-auto rounded-3 border border-border/50 bg-background/45 p-2 text-[11px] leading-relaxed text-subtle-foreground"
                  >{{ pretty(event.payload) }}</pre
                >
              </li>
            </ol>
          </div>
        </div>
      </div>
    </template>
  </aside>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type {
  DurableEvent,
  DurableTask,
  DurableTaskStatus,
} from "@/api/durable";
import AppButton from "@/components/ui/AppButton.vue";
import Chip from "@/components/ui/Chip.vue";

const props = withDefaults(
  defineProps<{
    task: DurableTask | null;
    events: DurableEvent[];
    eventsLoading?: boolean;
    cancelLoading?: boolean;
    retryLoading?: boolean;
    showClose?: boolean;
    emptyMessage?: string;
  }>(),
  {
    showClose: true,
    emptyMessage:
      "Select a task from the operations table to inspect state, timeline, and payloads.",
  },
);

defineEmits<{
  close: [];
  cancel: [taskId: string];
  retry: [taskId: string, resetCheckpoints: boolean];
}>();

const resetCheckpoints = ref(false);

const isTerminal = computed(() => {
  const status = props.task?.status;
  return (
    status === "completed" || status === "failed" || status === "cancelled"
  );
});

const canRetry = computed(() => {
  const status = props.task?.status;
  return status === "failed" || status === "cancelled";
});

watch(
  () => props.task?.id,
  () => {
    resetCheckpoints.value = false;
  },
);

const payloadBlocks = computed(() => {
  if (!props.task) return [];
  return [
    { label: "Params", value: pretty(props.task.params ?? {}) },
    { label: "Headers", value: pretty(props.task.headers ?? {}) },
    { label: "Result", value: pretty(props.task.result ?? {}) },
    { label: "Failure", value: pretty(props.task.failure ?? {}) },
  ].filter((block) => block.value !== "{}");
});

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

function hasPayload(value: unknown) {
  return Boolean(
    value && typeof value === "object" && Object.keys(value).length > 0,
  );
}

function pretty(value: unknown) {
  return JSON.stringify(value ?? {}, null, 2);
}
</script>
