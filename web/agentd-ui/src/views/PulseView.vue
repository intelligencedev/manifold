<script setup lang="ts">
import { ref, computed, watch, nextTick } from "vue";
import { useQuery, useMutation, useQueryClient } from "@tanstack/vue-query";
import GlassCard from "@/components/ui/GlassCard.vue";
import Pill from "@/components/ui/Pill.vue";
import {
  listMatrixRooms,
  fetchMatrixRoomMessages,
  listMatrixRoomTasks,
  createMatrixRoomTask,
  updateMatrixRoomTask,
  deleteMatrixRoomTask,
  setMatrixRoomTaskEnabled,
  runMatrixRoomTaskNow,
  type MatrixRoom,
  type MatrixTask,
  type MatrixMessage,
  type MatrixTaskUpsertInput,
  type MatrixTaskPatchInput,
} from "@/api/matrix";

const qc = useQueryClient();

// ── rooms ──────────────────────────────────────────────────────────────────
const { data: rooms, isPending: roomsLoading } = useQuery({
  queryKey: ["matrix-rooms"],
  queryFn: listMatrixRooms,
  refetchInterval: 10_000,
});

const selectedRoomId = ref<string | null>(null);
const selectedRoom = computed<MatrixRoom | undefined>(() =>
  rooms.value?.find((r) => r.roomId === selectedRoomId.value),
);

function selectRoom(id: string) {
  selectedRoomId.value = id;
  activeTab.value = "tasks";
}

// auto-select first room
watch(
  rooms,
  (list) => {
    if (!list?.length) {
      selectedRoomId.value = null;
      return;
    }
    if (
      !selectedRoomId.value ||
      !list.some((r) => r.roomId === selectedRoomId.value)
    ) {
      selectedRoomId.value = list[0].roomId;
    }
  },
  { immediate: true },
);

// ── tabs ───────────────────────────────────────────────────────────────────
const activeTab = ref<"transcript" | "tasks">("tasks");

// ── messages ───────────────────────────────────────────────────────────────
const transcriptEl = ref<HTMLElement | null>(null);

const { data: messages, isPending: messagesLoading } = useQuery({
  queryKey: computed(() => ["matrix-messages", selectedRoomId.value]),
  queryFn: () => fetchMatrixRoomMessages(selectedRoomId.value!, 100),
  enabled: computed(() => !!selectedRoomId.value),
  refetchInterval: 5_000,
});

// API returns newest-first; reverse so DOM is oldest→newest (scroll to bottom = latest)
const orderedMessages = computed(() => [...(messages.value ?? [])].reverse());

// scroll to bottom whenever the message list changes
watch(orderedMessages, async () => {
  await nextTick();
  if (transcriptEl.value) {
    transcriptEl.value.scrollTop = transcriptEl.value.scrollHeight;
  }
});

function formatTime(iso: string) {
  return new Date(iso).toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleDateString([], {
    month: "short",
    day: "numeric",
  });
}

function isImage(mime?: string) {
  return mime?.startsWith("image/");
}

// ── tasks ──────────────────────────────────────────────────────────────────
type MatrixTaskRunStatus = NonNullable<MatrixTask["activeRunStatus"]>;

const ACTIVE_PULSE_RUN_STATUSES = new Set<MatrixTaskRunStatus>([
  "queued",
  "running",
  "waiting",
]);

const fastTaskPolling = ref(false);

function isPulseRunActive(task: MatrixTask) {
  if (!task.activeRunId) return false;
  if (!task.activeRunStatus) return true;
  return ACTIVE_PULSE_RUN_STATUSES.has(task.activeRunStatus);
}

function taskStateLabel(task: MatrixTask) {
  if (!task.enabled) return "disabled";
  if (isPulseRunActive(task)) return task.activeRunStatus || "queued";
  if (task.due) return "due";
  if (task.lastRunStatus) return task.lastRunStatus;
  return "waiting";
}

function taskStateTone(
  task: MatrixTask,
): "accent" | "neutral" | "success" | "danger" | "warning" | "info" {
  const state = taskStateLabel(task);
  if (state === "running") return "accent";
  if (state === "queued" || state === "waiting") return "info";
  if (state === "due") return "warning";
  if (state === "completed") return "success";
  if (state === "failed") return "danger";
  return "neutral";
}

function taskRunButtonLabel(task: MatrixTask) {
  if (isPulseRunActive(task)) {
    if (task.activeRunStatus === "running") return "Running…";
    if (task.activeRunStatus === "waiting") return "Waiting…";
    return "Queued…";
  }
  return "Run now";
}

function taskDurableRunId(task: MatrixTask) {
  return task.activeRunId || task.lastRunId || "";
}

const { data: tasks, isPending: tasksLoading } = useQuery({
  queryKey: computed(() => ["matrix-tasks", selectedRoomId.value]),
  queryFn: () => listMatrixRoomTasks(selectedRoomId.value!),
  enabled: computed(() => !!selectedRoomId.value),
  refetchInterval: computed(() => (fastTaskPolling.value ? 2_000 : 10_000)),
});

watch(
  tasks,
  (list) => {
    fastTaskPolling.value = !!list?.some(isPulseRunActive);
  },
  { immediate: true },
);

// task form state
const showNewTask = ref(false);
const editingTaskId = ref<string | null>(null);

const blankDraft = (): MatrixTaskUpsertInput => ({
  title: "",
  prompt: "",
  scheduleType: "interval",
  intervalSeconds: 3600,
  specificTime: "09:00",
  specificAt: defaultSpecificAt(),
  enabled: true,
});

const draft = ref<MatrixTaskUpsertInput>(blankDraft());
const editDraft = ref<MatrixTaskPatchInput>({});

function startEditTask(task: MatrixTask) {
  editingTaskId.value = task.id;
  editDraft.value = {
    title: task.title,
    prompt: task.prompt,
    scheduleType: task.scheduleType || "interval",
    intervalSeconds: task.intervalSeconds,
    specificTime: task.specificTime || "09:00",
    specificAt: task.specificAt
      ? toLocalDatetimeInput(task.specificAt)
      : defaultSpecificAt(),
    routeTarget: task.routeTarget,
    enabled: task.enabled,
  };
}
function cancelEdit() {
  editingTaskId.value = null;
  editDraft.value = {};
}

function invalidateTasks() {
  qc.invalidateQueries({ queryKey: ["matrix-tasks", selectedRoomId.value] });
  qc.invalidateQueries({ queryKey: ["matrix-rooms"] });
}

const createMutation = useMutation({
  mutationFn: () =>
    createMatrixRoomTask(
      selectedRoomId.value!,
      taskPayload(draft.value) as MatrixTaskUpsertInput,
    ),
  onSuccess: () => {
    showNewTask.value = false;
    draft.value = blankDraft();
    invalidateTasks();
  },
});

const patchMutation = useMutation({
  mutationFn: ({ id, patch }: { id: string; patch: MatrixTaskPatchInput }) =>
    updateMatrixRoomTask(
      selectedRoomId.value!,
      id,
      taskPayload(patch) as MatrixTaskPatchInput,
    ),
  onSuccess: () => {
    cancelEdit();
    invalidateTasks();
  },
});

const deleteMutation = useMutation({
  mutationFn: (id: string) => deleteMatrixRoomTask(selectedRoomId.value!, id),
  onSuccess: invalidateTasks,
});

const enableMutation = useMutation({
  mutationFn: (id: string) =>
    setMatrixRoomTaskEnabled(selectedRoomId.value!, id, true),
  onSuccess: invalidateTasks,
});

const disableMutation = useMutation({
  mutationFn: (id: string) =>
    setMatrixRoomTaskEnabled(selectedRoomId.value!, id, false),
  onSuccess: invalidateTasks,
});

const runMutation = useMutation({
  mutationFn: (id: string) => runMatrixRoomTaskNow(selectedRoomId.value!, id),
  onSuccess: invalidateTasks,
});

// interval helpers
const INTERVAL_PRESETS = [
  { label: "1 min", value: 60 },
  { label: "5 min", value: 300 },
  { label: "15 min", value: 900 },
  { label: "30 min", value: 1800 },
  { label: "1 hr", value: 3600 },
  { label: "6 hr", value: 21600 },
  { label: "24 hr", value: 86400 },
];

function humanInterval(seconds: number) {
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.round(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)}h`;
  return `${Math.round(seconds / 86400)}d`;
}

function toLocalDatetimeInput(iso: string) {
  const date = new Date(iso);
  const offsetMs = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16);
}

function defaultSpecificAt() {
  return toLocalDatetimeInput(new Date(Date.now() + 60 * 60_000).toISOString());
}

function scheduleLabel(task: MatrixTask) {
  return (
    task.scheduleLabel ||
    (task.scheduleType === "interval"
      ? humanInterval(task.intervalSeconds)
      : task.scheduleType)
  );
}

function taskPayload(input: MatrixTaskUpsertInput | MatrixTaskPatchInput) {
  const payload: MatrixTaskUpsertInput | MatrixTaskPatchInput = {
    routeTarget: input.routeTarget,
    title: input.title,
    prompt: input.prompt,
    scheduleType: input.scheduleType,
    enabled: input.enabled,
  };
  if (!payload.scheduleType || payload.scheduleType === "interval") {
    payload.scheduleType = "interval";
    payload.intervalSeconds = input.intervalSeconds || 3600;
  } else if (payload.scheduleType === "daily_time") {
    payload.specificTime = input.specificTime || "09:00";
  } else {
    payload.specificAt = input.specificAt || defaultSpecificAt();
  }
  return payload;
}

const mutErr = ref<string | null>(null);

async function submitCreate() {
  mutErr.value = null;
  try {
    await createMutation.mutateAsync();
  } catch (e: unknown) {
    mutErr.value = e instanceof Error ? e.message : "Failed";
  }
}

async function submitPatch(id: string) {
  mutErr.value = null;
  try {
    await patchMutation.mutateAsync({ id, patch: editDraft.value });
  } catch (e: unknown) {
    mutErr.value = e instanceof Error ? e.message : "Failed";
  }
}

async function doDelete(id: string) {
  if (!confirm("Delete this task?")) return;
  mutErr.value = null;
  try {
    await deleteMutation.mutateAsync(id);
  } catch (e: unknown) {
    mutErr.value = e instanceof Error ? e.message : "Failed";
  }
}
</script>

<template>
  <section class="flex h-full min-h-0 flex-row overflow-hidden">
    <!-- ── left: room list ─────────────────────────────────────────────── -->
    <aside
      class="flex w-64 shrink-0 flex-col border-r border-border/50 overflow-hidden"
    >
      <div
        class="flex items-center justify-between px-4 py-3 border-b border-border/40"
      >
        <span
          class="text-xs font-semibold uppercase tracking-widest text-subtle-foreground"
          >Rooms</span
        >
        <span class="text-[11px] text-faint-foreground">{{
          rooms?.length ?? 0
        }}</span>
      </div>

      <div
        v-if="roomsLoading"
        class="flex flex-1 items-center justify-center text-xs text-faint-foreground"
      >
        Loading…
      </div>

      <nav v-else class="scrollbar-inset flex-1 overflow-y-auto py-2">
        <button
          v-for="room in rooms"
          :key="room.roomId"
          type="button"
          :class="[
            'w-full text-left px-4 py-3 transition-colors rounded-none border-l-2',
            selectedRoomId === room.roomId
              ? 'border-accent bg-accent/8 text-foreground'
              : 'border-transparent text-subtle-foreground hover:bg-surface-muted/40 hover:text-foreground',
          ]"
          @click="selectRoom(room.roomId)"
        >
          <!-- room id truncated -->
          <p class="truncate text-xs font-semibold">
            {{ room.roomId.replace(/^!/, "").split(":")[0] }}
          </p>
          <p class="truncate text-[11px] text-faint-foreground mt-0.5">
            {{ room.roomId.split(":")[1] ?? "" }}
          </p>
          <div class="mt-1.5 flex items-center gap-2">
            <Pill tone="neutral" size="sm"
              >{{ room.stats.messageCount }} msgs</Pill
            >
            <Pill v-if="room.enabledTaskCount" tone="accent" size="sm">
              {{ room.enabledTaskCount }} tasks
            </Pill>
          </div>
        </button>

        <p
          v-if="!rooms?.length"
          class="px-4 py-6 text-center text-xs text-faint-foreground"
        >
          No rooms configured.
        </p>
      </nav>
    </aside>

    <!-- ── right: room detail ──────────────────────────────────────────── -->
    <div
      v-if="!selectedRoom"
      class="flex flex-1 items-center justify-center text-sm text-faint-foreground"
    >
      Select a room
    </div>

    <div v-else class="flex min-w-0 flex-1 flex-col overflow-hidden">
      <!-- room header -->
      <header
        class="flex shrink-0 items-start justify-between gap-6 border-b border-border/40 px-6 py-3"
      >
        <div class="min-w-0">
          <h2 class="truncate text-sm font-semibold text-foreground">
            {{ selectedRoom.roomId }}
          </h2>
          <div class="mt-1 flex flex-wrap items-center gap-2">
            <Pill tone="neutral" size="sm"
              >target: {{ selectedRoom.defaultTarget || "—" }}</Pill
            >
            <Pill
              :tone="selectedRoom.allowUnmentioned ? 'success' : 'neutral'"
              size="sm"
            >
              {{
                selectedRoom.allowUnmentioned ? "all messages" : "mentions only"
              }}
            </Pill>
            <Pill tone="neutral" size="sm"
              >{{ selectedRoom.stats.messageCount }} messages</Pill
            >
            <Pill
              v-if="selectedRoom.stats.lastActivityAt"
              tone="neutral"
              size="sm"
            >
              last: {{ formatDate(selectedRoom.stats.lastActivityAt) }}
            </Pill>
          </div>
        </div>

        <!-- tab switcher -->
        <div
          class="flex shrink-0 items-center gap-1 rounded-full border border-border/50 bg-surface-muted/30 p-0.5"
        >
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold transition"
            :class="
              activeTab === 'transcript'
                ? 'bg-surface text-foreground shadow-sm'
                : 'text-subtle-foreground hover:text-foreground'
            "
            @click="activeTab = 'transcript'"
          >
            Transcript
          </button>
          <button
            type="button"
            class="rounded-full px-3 py-1 text-xs font-semibold transition"
            :class="
              activeTab === 'tasks'
                ? 'bg-surface text-foreground shadow-sm'
                : 'text-subtle-foreground hover:text-foreground'
            "
            @click="activeTab = 'tasks'"
          >
            Tasks
            <span
              v-if="selectedRoom.taskCount"
              class="ml-1 text-[10px] tabular-nums text-faint-foreground"
              >{{ selectedRoom.taskCount }}</span
            >
          </button>
        </div>
      </header>

      <!-- ── transcript tab ──────────────────────────────────────────── -->
      <div
        v-show="activeTab === 'transcript'"
        class="flex min-h-0 flex-1 flex-col overflow-hidden"
      >
        <div
          v-if="messagesLoading"
          class="flex flex-1 items-center justify-center text-xs text-faint-foreground"
        >
          Loading messages…
        </div>

        <div
          v-else-if="!messages?.length"
          class="flex flex-1 items-center justify-center text-xs text-faint-foreground"
        >
          No messages yet.
        </div>

        <div
          v-else
          ref="transcriptEl"
          class="scrollbar-inset flex-1 overflow-y-auto px-6 py-4 space-y-2"
        >
          <div
            v-for="msg in orderedMessages"
            :key="msg.id"
            :class="[
              'flex gap-3',
              msg.direction === 'outbound' ? 'flex-row-reverse' : 'flex-row',
            ]"
          >
            <!-- avatar dot -->
            <div
              :class="[
                'mt-1 h-2 w-2 shrink-0 rounded-full',
                msg.direction === 'outbound'
                  ? 'bg-accent'
                  : 'bg-subtle-foreground/40',
              ]"
            />

            <div
              :class="[
                'max-w-[70%] min-w-0',
                msg.direction === 'outbound' ? 'items-end' : 'items-start',
                'flex flex-col gap-0.5',
              ]"
            >
              <!-- meta row -->
              <div
                :class="[
                  'flex items-center gap-2 text-[11px] text-faint-foreground',
                  msg.direction === 'outbound' ? 'flex-row-reverse' : '',
                ]"
              >
                <span class="font-medium">{{
                  msg.sender || (msg.direction === "outbound" ? "bot" : "user")
                }}</span>
                <span>{{ formatTime(msg.createdAt) }}</span>
              </div>

              <!-- bubble -->
              <div
                :class="[
                  'rounded-lg px-3 py-2 text-sm leading-relaxed',
                  msg.direction === 'outbound'
                    ? 'bg-accent/15 text-foreground border border-accent/20 rounded-tr-sm'
                    : 'bg-surface-muted/60 text-foreground border border-border/40 rounded-tl-sm',
                ]"
              >
                <!-- inline image -->
                <img
                  v-if="msg.mediaUrl && isImage(msg.mediaMime)"
                  :src="msg.mediaUrl"
                  :alt="msg.body || 'image'"
                  class="mb-1.5 max-h-48 max-w-full rounded-lg object-contain"
                />
                <p class="whitespace-pre-wrap break-words">{{ msg.body }}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- ── tasks tab ───────────────────────────────────────────────── -->
      <div
        v-show="activeTab === 'tasks'"
        class="scrollbar-inset flex-1 overflow-y-auto px-6 py-4"
      >
        <!-- error banner -->
        <div
          v-if="mutErr"
          class="mb-4 rounded-md border border-danger/50 bg-danger/10 px-4 py-2 text-xs text-danger-foreground"
        >
          {{ mutErr }}
        </div>

        <!-- new task button / form -->
        <div class="mb-5">
          <button
            v-if="!showNewTask"
            type="button"
            class="rounded-full border border-accent/40 px-4 py-1.5 text-xs font-semibold text-accent transition hover:bg-accent/10"
            @click="
              showNewTask = true;
              draft = blankDraft();
            "
          >
            + New task
          </button>

          <GlassCard v-else :padded="false" class="mb-4 p-4">
            <p
              class="mb-3 text-xs font-semibold uppercase tracking-widest text-subtle-foreground"
            >
              New pulse task
            </p>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <label
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Title
                <input
                  v-model="draft.title"
                  type="text"
                  placeholder="e.g. Daily digest"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                />
              </label>

              <label
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Schedule
                <select
                  v-model="draft.scheduleType"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option value="interval">Interval</option>
                  <option value="daily_time">Daily at time</option>
                  <option value="once_at">Once at date/time</option>
                </select>
              </label>

              <label
                v-if="draft.scheduleType === 'interval'"
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Interval
                <select
                  v-model.number="draft.intervalSeconds"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                >
                  <option
                    v-for="p in INTERVAL_PRESETS"
                    :key="p.value"
                    :value="p.value"
                  >
                    {{ p.label }}
                  </option>
                </select>
              </label>

              <label
                v-else-if="draft.scheduleType === 'daily_time'"
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Local time
                <input
                  v-model="draft.specificTime"
                  type="time"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                />
              </label>

              <label
                v-else
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Local date/time
                <input
                  v-model="draft.specificAt"
                  type="datetime-local"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                />
              </label>

              <label
                class="col-span-full flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Prompt
                <textarea
                  v-model="draft.prompt"
                  rows="3"
                  placeholder="What should the bot say or do on each pulse?"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40 resize-none"
                />
              </label>

              <label
                class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
              >
                Route target
                <span
                  class="font-normal normal-case tracking-normal text-faint-foreground"
                  >(optional)</span
                >
                <input
                  v-model="draft.routeTarget"
                  type="text"
                  placeholder="specialist slug"
                  class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                />
              </label>

              <div class="flex items-center gap-2 self-end">
                <label
                  class="flex cursor-pointer items-center gap-2 text-xs text-subtle-foreground"
                >
                  <input
                    v-model="draft.enabled"
                    type="checkbox"
                    class="h-4 w-4 rounded border-border/60"
                  />
                  Enabled
                </label>
              </div>
            </div>

            <div class="mt-4 flex items-center gap-2">
              <button
                type="button"
                :disabled="
                  createMutation.isPending.value ||
                  !draft.title ||
                  !draft.prompt
                "
                class="rounded-lg bg-accent px-4 py-1.5 text-xs font-semibold text-accent-foreground transition hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-50"
                @click="submitCreate"
              >
                {{ createMutation.isPending.value ? "Saving…" : "Create" }}
              </button>
              <button
                type="button"
                class="rounded-lg border border-border/60 px-4 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:text-foreground"
                @click="showNewTask = false"
              >
                Cancel
              </button>
            </div>
          </GlassCard>
        </div>

        <!-- tasks list -->
        <div v-if="tasksLoading" class="text-xs text-faint-foreground">
          Loading tasks…
        </div>

        <p v-else-if="!tasks?.length" class="text-xs text-faint-foreground">
          No pulse tasks configured for this room.
        </p>

        <div v-else class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
          <GlassCard
            v-for="task in tasks"
            :key="task.id"
            :padded="false"
            class="flex flex-col gap-0 overflow-hidden"
          >
            <!-- edit form -->
            <template v-if="editingTaskId === task.id">
              <div class="p-4">
                <p
                  class="mb-3 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                >
                  Edit task
                </p>
                <div class="grid grid-cols-1 gap-3">
                  <label
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Title
                    <input
                      v-model="editDraft.title"
                      type="text"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    />
                  </label>
                  <label
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Schedule
                    <select
                      v-model="editDraft.scheduleType"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    >
                      <option value="interval">Interval</option>
                      <option value="daily_time">Daily at time</option>
                      <option value="once_at">Once at date/time</option>
                    </select>
                  </label>
                  <label
                    v-if="editDraft.scheduleType === 'interval'"
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Interval
                    <select
                      v-model.number="editDraft.intervalSeconds"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    >
                      <option
                        v-for="p in INTERVAL_PRESETS"
                        :key="p.value"
                        :value="p.value"
                      >
                        {{ p.label }}
                      </option>
                    </select>
                  </label>
                  <label
                    v-else-if="editDraft.scheduleType === 'daily_time'"
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Local time
                    <input
                      v-model="editDraft.specificTime"
                      type="time"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    />
                  </label>
                  <label
                    v-else
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Local date/time
                    <input
                      v-model="editDraft.specificAt"
                      type="datetime-local"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    />
                  </label>
                  <label
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Prompt
                    <textarea
                      v-model="editDraft.prompt"
                      rows="3"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40 resize-none"
                    />
                  </label>
                  <label
                    class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-widest text-subtle-foreground"
                  >
                    Route target
                    <input
                      v-model="editDraft.routeTarget"
                      type="text"
                      placeholder="specialist slug"
                      class="rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
                    />
                  </label>
                </div>
                <div class="mt-4 flex items-center gap-2">
                  <button
                    type="button"
                    :disabled="patchMutation.isPending.value"
                    class="rounded-lg bg-accent px-3 py-1.5 text-xs font-semibold text-accent-foreground transition hover:bg-accent/90 disabled:opacity-50"
                    @click="submitPatch(task.id)"
                  >
                    {{ patchMutation.isPending.value ? "Saving…" : "Save" }}
                  </button>
                  <button
                    type="button"
                    class="rounded-lg border border-border/60 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:text-foreground"
                    @click="cancelEdit"
                  >
                    Cancel
                  </button>
                </div>
              </div>
            </template>

            <!-- card view -->
            <template v-else>
              <!-- top: status + title -->
              <div class="flex items-start gap-3 p-4 pb-2">
                <div class="mt-0.5 flex flex-col items-center gap-1">
                  <div
                    :class="[
                      'h-2.5 w-2.5 rounded-full',
                      isPulseRunActive(task)
                        ? 'animate-pulse bg-accent text-accent'
                        : task.enabled
                        ? 'bg-success text-success'
                        : 'bg-subtle-foreground/30',
                    ]"
                    :title="taskStateLabel(task)"
                  />
                </div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-sm font-semibold text-foreground">
                    {{ task.title }}
                  </p>
                  <p
                    class="mt-0.5 line-clamp-2 text-[11px] text-subtle-foreground leading-relaxed"
                  >
                    {{ task.prompt }}
                  </p>
                </div>
              </div>

              <!-- metadata row -->
              <div class="flex flex-wrap items-center gap-1.5 px-4 py-2">
                <Pill tone="neutral" size="sm">{{ scheduleLabel(task) }}</Pill>
                <Pill v-if="task.routeTarget" tone="info" size="sm"
                  >→ {{ task.routeTarget }}</Pill
                >
                <Pill :tone="taskStateTone(task)" size="sm">
                  {{ taskStateLabel(task) }}
                </Pill>
              </div>

              <!-- timing row -->
              <div
                class="grid grid-cols-2 gap-x-3 border-t border-border/30 px-4 py-2 text-[11px]"
              >
                <div>
                  <p class="text-faint-foreground">Last run</p>
                  <p class="font-medium text-subtle-foreground">
                    {{ task.lastRunHuman || "never" }}
                  </p>
                </div>
                <div>
                  <p class="text-faint-foreground">Next run</p>
                  <p class="font-medium text-subtle-foreground">
                    {{ task.nextRunHuman || "—" }}
                  </p>
                </div>
              </div>

              <!-- last result -->
              <div
                v-if="task.lastResultSummary"
                class="border-t border-border/30 px-4 py-2"
              >
                <p
                  class="line-clamp-2 text-[11px] text-faint-foreground italic"
                >
                  {{ task.lastResultSummary }}
                </p>
              </div>

              <div
                v-if="taskDurableRunId(task)"
                class="border-t border-border/30 px-4 py-2"
              >
                <RouterLink
                  :to="{
                    name: 'durable-task',
                    params: { taskId: taskDurableRunId(task) },
                  }"
                  class="truncate text-[11px] font-semibold text-accent transition hover:text-accent/80"
                >
                  Durable run: {{ taskDurableRunId(task) }}
                </RouterLink>
              </div>

              <!-- action row -->
              <div
                class="flex items-center gap-1.5 border-t border-border/30 px-4 py-2.5"
              >
                <button
                  v-if="task.enabled"
                  type="button"
                  class="rounded-md border border-border/50 px-2.5 py-1 text-[11px] font-semibold text-subtle-foreground transition hover:border-warning/50 hover:text-warning"
                  :disabled="disableMutation.isPending.value"
                  @click="disableMutation.mutate(task.id)"
                >
                  Disable
                </button>
                <button
                  v-else
                  type="button"
                  class="rounded-md border border-border/50 px-2.5 py-1 text-[11px] font-semibold text-subtle-foreground transition hover:border-success/50 hover:text-success"
                  :disabled="enableMutation.isPending.value"
                  @click="enableMutation.mutate(task.id)"
                >
                  Enable
                </button>

                <button
                  type="button"
                  class="rounded-md border border-border/50 px-2.5 py-1 text-[11px] font-semibold text-subtle-foreground transition hover:border-accent/50 hover:text-accent"
                  :disabled="
                    runMutation.isPending.value ||
                    isPulseRunActive(task) ||
                    !task.enabled
                  "
                  @click="runMutation.mutate(task.id)"
                >
                  {{ taskRunButtonLabel(task) }}
                </button>

                <div class="flex-1" />

                <button
                  type="button"
                  class="rounded-md border border-border/50 px-2.5 py-1 text-[11px] font-semibold text-subtle-foreground transition hover:border-border hover:text-foreground"
                  @click="startEditTask(task)"
                >
                  Edit
                </button>

                <button
                  type="button"
                  class="rounded-md border border-border/50 px-2.5 py-1 text-[11px] font-semibold text-subtle-foreground transition hover:border-danger/50 hover:text-danger"
                  :disabled="deleteMutation.isPending.value"
                  @click="doDelete(task.id)"
                >
                  Delete
                </button>
              </div>
            </template>
          </GlassCard>
        </div>
      </div>
    </div>
  </section>
</template>
