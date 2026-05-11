<template>
  <section class="flex h-full min-h-0 gap-4 overflow-hidden">
    <aside class="flex min-h-0 w-[320px] min-w-[300px] flex-col gap-4 overflow-hidden">
      <Panel
        title="Matrix Gateway"
        eyebrow="Dedicated View"
        description="Configured rooms, transcript retention, and hidden Manifold sessions for Matrix traffic."
        class="min-h-0 flex-1"
      >
        <div class="mb-4 grid grid-cols-2 gap-3">
          <MetricCard
            label="Rooms"
            :value="rooms.length"
            secondary="Configured in agentd"
          />
          <MetricCard
            label="Messages"
            :value="totalMessageCount"
            secondary="Stored Matrix transcript"
          />
        </div>

        <div
          v-if="roomsQuery.isLoading.value"
          class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
        >
          Loading Matrix rooms...
        </div>
        <div
          v-else-if="roomsQuery.isError.value"
          class="rounded-[18px] border border-danger/50 bg-danger/10 p-4 text-sm text-danger-foreground"
        >
          Failed to load Matrix rooms.
        </div>
        <div
          v-else-if="!rooms.length"
          class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
        >
          No Matrix rooms are configured.
        </div>
        <div v-else class="flex min-h-0 flex-col gap-3 overflow-auto pr-1">
          <GlassCard
            v-for="room in rooms"
            :key="room.roomId"
            interactive
            class="cursor-pointer"
            :class="
              room.roomId === selectedRoomId
                ? 'ring-2 ring-accent/60 ring-offset-2 ring-offset-background'
                : ''
            "
            @click="selectedRoomId = room.roomId"
          >
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="truncate text-sm font-semibold text-foreground">
                  {{ room.roomId }}
                </div>
                <p class="mt-1 text-xs text-subtle-foreground">
                  Default target: {{ room.defaultTarget || "None" }}
                </p>
              </div>
              <Pill :tone="room.enabledTaskCount > 0 ? 'accent' : 'neutral'" size="sm">
                {{ room.enabledTaskCount }}/{{ room.taskCount }} active
              </Pill>
            </div>

            <div class="mt-3 flex flex-wrap gap-2">
              <Pill :tone="room.allowUnmentioned ? 'success' : 'warning'" size="sm">
                {{ room.allowUnmentioned ? "Unmentioned allowed" : "Mention only" }}
              </Pill>
              <Pill tone="neutral" size="sm">
                Retention {{ room.messageRetention }}
              </Pill>
              <Pill tone="neutral" size="sm">
                {{ room.stats.messageCount }} messages
              </Pill>
            </div>

            <div class="mt-4 grid grid-cols-2 gap-2 text-xs text-subtle-foreground">
              <div>
                <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                  Last activity
                </span>
                <span>{{ formatDateTime(room.stats.lastActivityAt) }}</span>
              </div>
              <div>
                <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                  Hidden session
                </span>
                <span class="truncate">{{ room.sessionId }}</span>
              </div>
            </div>
          </GlassCard>
        </div>
      </Panel>
    </aside>

    <div class="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-hidden">
      <Panel
        v-if="selectedRoom"
        title="Room Overview"
        eyebrow="Matrix Room"
        :description="selectedRoom.defaultTarget ? `Primary route target: ${selectedRoom.defaultTarget}` : 'No default route target configured.'"
      >
        <template #actions>
          <button
            type="button"
            class="rounded-full border border-white/12 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-accent/40 hover:text-accent"
            @click="refreshSelectedRoom"
          >
            Refresh
          </button>
        </template>

        <div class="grid grid-cols-4 gap-3">
          <MetricCard
            label="Stored Messages"
            :value="selectedRoom.stats.messageCount"
            :secondary="formatDateTime(selectedRoom.stats.lastActivityAt)"
          />
          <MetricCard
            label="Pulse Routes"
            :value="selectedRoom.routes.length"
            :secondary="`${selectedRoom.enabledTaskCount} tasks enabled`"
          />
          <MetricCard
            label="Session"
            :value="selectedSession?.name || 'Matrix Room'"
            :secondary="selectedRoom.sessionId"
          />
          <MetricCard
            label="Max Concurrent"
            :value="selectedRoom.maxConcurrent || 1"
            :secondary="selectedRoom.systemPromptRef || 'No system prompt ref'"
          />
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <Pill :tone="selectedRoom.allowUnmentioned ? 'success' : 'warning'">
            {{ selectedRoom.allowUnmentioned ? 'Room accepts unmentioned prompts' : 'Room requires mentions' }}
          </Pill>
          <Pill tone="neutral">Retention {{ selectedRoom.messageRetention }} messages</Pill>
          <Pill v-if="selectedRoom.stats.lastSender" tone="info">
            Last sender {{ selectedRoom.stats.lastSender }}
          </Pill>
          <Pill v-for="route in selectedRoom.routes" :key="`${route.routeTarget}-${route.revision}`" :tone="route.lastPulseError ? 'danger' : route.enabled ? 'success' : 'neutral'">
            {{ route.routeTarget || 'default' }}
          </Pill>
        </div>

        <div
          v-if="Object.keys(selectedRoom.mentions || {}).length"
          class="mt-4 grid grid-cols-2 gap-2 rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm"
        >
          <div
            v-for="(target, alias) in selectedRoom.mentions"
            :key="alias"
            class="flex items-center justify-between gap-3 rounded-full border border-white/10 bg-black/10 px-3 py-2"
          >
            <span class="font-medium text-foreground">{{ alias }}</span>
            <span class="text-subtle-foreground">{{ target }}</span>
          </div>
        </div>
      </Panel>

      <div class="flex min-h-0 min-w-0 flex-1 gap-4 overflow-hidden">
        <Panel
          title="Transcript"
          eyebrow="Last 100 Messages"
          description="Inbound and outbound Matrix traffic stored independently from the normal chat view."
          class="min-h-0 min-w-0 flex-1"
        >
          <div
            v-if="messagesQuery.isLoading.value"
            class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
          >
            Loading transcript...
          </div>
          <div
            v-else-if="messagesQuery.isError.value"
            class="rounded-[18px] border border-danger/50 bg-danger/10 p-4 text-sm text-danger-foreground"
          >
            Failed to load transcript.
          </div>
          <div
            v-else-if="!orderedMessages.length"
            class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
          >
            No stored messages for this room yet.
          </div>
          <div v-else class="scrollbar-inset flex max-h-full flex-col gap-3 overflow-auto pr-1">
            <article
              v-for="message in orderedMessages"
              :key="message.id"
              class="rounded-[20px] border p-4"
              :class="
                message.direction === 'outbound'
                  ? 'border-accent/25 bg-accent/8'
                  : 'border-white/10 bg-surface-muted/30'
              "
            >
              <div class="mb-2 flex items-center justify-between gap-3 text-xs text-subtle-foreground">
                <div class="flex items-center gap-2">
                  <Pill :tone="message.direction === 'outbound' ? 'accent' : 'neutral'" size="sm">
                    {{ message.direction }}
                  </Pill>
                  <span class="truncate">
                    {{ message.sender || message.target || 'Matrix gateway' }}
                  </span>
                  <span class="text-faint-foreground">{{ message.msgType }}</span>
                </div>
                <time>{{ formatDateTime(message.createdAt) }}</time>
              </div>

              <p class="whitespace-pre-wrap text-sm leading-relaxed text-foreground">
                {{ message.body || '(no body)' }}
              </p>

              <div v-if="message.mediaUrl" class="mt-3 space-y-2">
                <img
                  v-if="message.msgType === 'm.image'"
                  :src="message.mediaUrl"
                  alt="Matrix attachment"
                  class="max-h-72 rounded-[18px] border border-white/10 object-contain"
                />
                <a
                  :href="message.mediaUrl"
                  target="_blank"
                  rel="noreferrer"
                  class="inline-flex text-xs font-semibold text-accent transition hover:text-accent/80"
                >
                  Open attachment
                </a>
              </div>
            </article>
          </div>
        </Panel>

        <aside class="flex min-h-0 w-[420px] min-w-[380px] flex-col gap-4 overflow-hidden">
          <Panel
            :title="editingTaskId ? 'Edit Pulse Task' : 'New Pulse Task'"
            eyebrow="Scheduler"
            description="Create room-specific scheduled prompts without exposing these Matrix sessions in the main chat list."
          >
            <form class="space-y-3" @submit.prevent="saveTask">
              <div class="grid gap-3">
                <label class="space-y-1 text-sm">
                  <span class="text-subtle-foreground">Title</span>
                  <input
                    v-model.trim="taskDraft.title"
                    type="text"
                    placeholder="Daily digest"
                    class="w-full rounded-2xl border border-white/10 bg-surface-muted/30 px-4 py-3 text-sm text-foreground outline-none transition focus:border-accent/50"
                  />
                </label>

                <label class="space-y-1 text-sm">
                  <span class="text-subtle-foreground">Prompt</span>
                  <textarea
                    v-model.trim="taskDraft.prompt"
                    rows="4"
                    placeholder="Summarize the last activity and call out blockers."
                    class="w-full rounded-2xl border border-white/10 bg-surface-muted/30 px-4 py-3 text-sm text-foreground outline-none transition focus:border-accent/50"
                  ></textarea>
                </label>

                <div class="grid grid-cols-[1fr_130px] gap-3">
                  <label class="space-y-1 text-sm">
                    <span class="text-subtle-foreground">Route target</span>
                    <input
                      v-model.trim="taskDraft.routeTarget"
                      list="matrix-route-targets"
                      placeholder="Use room default"
                      class="w-full rounded-2xl border border-white/10 bg-surface-muted/30 px-4 py-3 text-sm text-foreground outline-none transition focus:border-accent/50"
                    />
                  </label>

                  <label class="space-y-1 text-sm">
                    <span class="text-subtle-foreground">Interval (sec)</span>
                    <input
                      v-model.number="taskDraft.intervalSeconds"
                      type="number"
                      min="60"
                      step="60"
                      class="w-full rounded-2xl border border-white/10 bg-surface-muted/30 px-4 py-3 text-sm text-foreground outline-none transition focus:border-accent/50"
                    />
                  </label>
                </div>

                <label class="inline-flex items-center gap-2 text-sm text-subtle-foreground">
                  <input
                    v-model="taskDraft.enabled"
                    type="checkbox"
                    class="h-4 w-4 rounded border-white/20 bg-transparent"
                  />
                  Task enabled
                </label>
              </div>

              <datalist id="matrix-route-targets">
                <option v-for="option in routeTargetOptions" :key="option" :value="option"></option>
              </datalist>

              <div class="flex items-center gap-2">
                <button
                  type="submit"
                  class="rounded-full border border-accent/40 bg-accent/10 px-4 py-2 text-xs font-semibold text-accent transition hover:bg-accent/20 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="saveMutation.isPending.value || !selectedRoomId"
                >
                  {{ saveMutation.isPending.value ? 'Saving...' : editingTaskId ? 'Save task' : 'Create task' }}
                </button>
                <button
                  type="button"
                  class="rounded-full border border-white/10 px-4 py-2 text-xs font-semibold text-subtle-foreground transition hover:border-white/20 hover:text-foreground"
                  @click="resetTaskDraft"
                >
                  Clear
                </button>
              </div>
            </form>
          </Panel>

          <Panel
            title="Pulse Tasks"
            eyebrow="Room Schedule"
            description="Cards reflect stored pulse tasks and the route they will dispatch through."
            class="min-h-0 flex-1"
          >
            <div
              v-if="tasksQuery.isLoading.value"
              class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
            >
              Loading tasks...
            </div>
            <div
              v-else-if="tasksQuery.isError.value"
              class="rounded-[18px] border border-danger/50 bg-danger/10 p-4 text-sm text-danger-foreground"
            >
              Failed to load tasks.
            </div>
            <div
              v-else-if="!tasks.length"
              class="rounded-[18px] border border-white/10 bg-surface-muted/30 p-4 text-sm text-faint-foreground"
            >
              No pulse tasks exist for this room.
            </div>
            <div v-else class="scrollbar-inset flex max-h-full flex-col gap-3 overflow-auto pr-1">
              <GlassCard v-for="task in tasks" :key="task.id" class="border border-white/10">
                <div class="flex items-start justify-between gap-3">
                  <div class="min-w-0">
                    <div class="truncate text-sm font-semibold text-foreground">
                      {{ task.title }}
                    </div>
                    <p class="mt-1 text-xs text-subtle-foreground">
                      {{ task.routeTarget || selectedRoom?.defaultTarget || 'No route target' }}
                    </p>
                  </div>
                  <Pill :tone="task.enabled ? task.due ? 'warning' : 'success' : 'neutral'" size="sm">
                    {{ task.enabled ? task.due ? 'Due now' : 'Enabled' : 'Paused' }}
                  </Pill>
                </div>

                <p class="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-subtle-foreground">
                  {{ task.prompt }}
                </p>

                <div class="mt-4 grid grid-cols-2 gap-2 text-xs text-subtle-foreground">
                  <div>
                    <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                      Interval
                    </span>
                    <span>{{ task.intervalHuman || formatInterval(task.intervalSeconds) }}</span>
                  </div>
                  <div>
                    <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                      Next run
                    </span>
                    <span>{{ formatRelativeOrDate(task.nextRunAt) }}</span>
                  </div>
                  <div>
                    <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                      Last run
                    </span>
                    <span>{{ formatRelativeOrDate(task.lastRunAt) }}</span>
                  </div>
                  <div>
                    <span class="block text-[10px] uppercase tracking-[0.18em] text-faint-foreground">
                      Room state
                    </span>
                    <span>{{ task.roomEnabled ? 'Route enabled' : 'Route paused' }}</span>
                  </div>
                </div>

                <div
                  v-if="task.lastResultSummary"
                  class="mt-3 rounded-[16px] border border-white/10 bg-black/10 p-3 text-xs text-subtle-foreground"
                >
                  {{ task.lastResultSummary }}
                </div>

                <div class="mt-4 flex flex-wrap gap-2">
                  <button
                    type="button"
                    class="rounded-full border border-white/10 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-white/20 hover:text-foreground"
                    @click="startEditingTask(task)"
                  >
                    Edit
                  </button>
                  <button
                    type="button"
                    class="rounded-full border border-accent/30 px-3 py-1.5 text-xs font-semibold text-accent transition hover:bg-accent/10 disabled:cursor-not-allowed disabled:opacity-60"
                    :disabled="runNowMutation.isPending.value"
                    @click="runTaskNow(task.id)"
                  >
                    Run now
                  </button>
                  <button
                    type="button"
                    class="rounded-full border border-white/10 px-3 py-1.5 text-xs font-semibold text-subtle-foreground transition hover:border-white/20 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                    :disabled="toggleMutation.isPending.value"
                    @click="toggleTask(task)"
                  >
                    {{ task.enabled ? 'Disable' : 'Enable' }}
                  </button>
                  <button
                    type="button"
                    class="rounded-full border border-danger/50 px-3 py-1.5 text-xs font-semibold text-danger transition hover:bg-danger/10 disabled:cursor-not-allowed disabled:opacity-60"
                    :disabled="deleteMutation.isPending.value"
                    @click="removeTask(task.id)"
                  >
                    Delete
                  </button>
                </div>
              </GlassCard>
            </div>
          </Panel>
        </aside>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import GlassCard from "@/components/ui/GlassCard.vue";
import MetricCard from "@/components/ui/MetricCard.vue";
import Panel from "@/components/ui/Panel.vue";
import Pill from "@/components/ui/Pill.vue";
import { listSpecialists, listTeams } from "@/api/client";
import {
  createMatrixRoomTask,
  deleteMatrixRoomTask,
  fetchMatrixRoomMessages,
  fetchMatrixRoomSession,
  listMatrixRooms,
  listMatrixRoomTasks,
  runMatrixRoomTaskNow,
  setMatrixRoomTaskEnabled,
  updateMatrixRoomTask,
  type MatrixMessage,
  type MatrixRoom,
  type MatrixTask,
} from "@/api/matrix";

const queryClient = useQueryClient();
const selectedRoomId = ref("");
const actionError = ref("");
const editingTaskId = ref("");
const taskDraft = reactive({
  title: "",
  prompt: "",
  routeTarget: "",
  intervalSeconds: 900,
  enabled: true,
});

const roomsQuery = useQuery({
  queryKey: ["matrix", "rooms"],
  queryFn: listMatrixRooms,
  staleTime: 5_000,
  refetchInterval: 15_000,
});

const tasksQuery = useQuery({
  queryKey: ["matrix", "tasks", selectedRoomId],
  queryFn: () => listMatrixRoomTasks(selectedRoomId.value),
  enabled: computed(() => Boolean(selectedRoomId.value)),
  staleTime: 2_000,
  refetchInterval: 15_000,
});

const messagesQuery = useQuery({
  queryKey: ["matrix", "messages", selectedRoomId],
  queryFn: () => fetchMatrixRoomMessages(selectedRoomId.value, 100),
  enabled: computed(() => Boolean(selectedRoomId.value)),
  staleTime: 0,
  refetchInterval: 10_000,
});

const sessionQuery = useQuery({
  queryKey: ["matrix", "session", selectedRoomId],
  queryFn: () => fetchMatrixRoomSession(selectedRoomId.value),
  enabled: computed(() => Boolean(selectedRoomId.value)),
  staleTime: 5_000,
});

const specialistsQuery = useQuery({
  queryKey: ["matrix", "specialists"],
  queryFn: listSpecialists,
  staleTime: 30_000,
});

const teamsQuery = useQuery({
  queryKey: ["matrix", "teams"],
  queryFn: listTeams,
  staleTime: 30_000,
});

const saveMutation = useMutation({
  mutationFn: async () => {
    if (!selectedRoomId.value) {
      throw new Error("Select a room before saving a task.");
    }
    const payload = {
      title: taskDraft.title,
      prompt: taskDraft.prompt,
      routeTarget: taskDraft.routeTarget || undefined,
      intervalSeconds: Number(taskDraft.intervalSeconds || 0),
      enabled: taskDraft.enabled,
    };
    if (editingTaskId.value) {
      return updateMatrixRoomTask(selectedRoomId.value, editingTaskId.value, payload);
    }
    return createMatrixRoomTask(selectedRoomId.value, payload);
  },
  onSuccess: async () => {
    actionError.value = "";
    resetTaskDraft();
    await invalidateSelectedRoom();
  },
  onError: (error) => {
    actionError.value = error instanceof Error ? error.message : "Failed to save Matrix task.";
  },
});

const toggleMutation = useMutation({
  mutationFn: async (task: MatrixTask) => {
    if (!selectedRoomId.value) {
      throw new Error("Select a room before updating a task.");
    }
    return setMatrixRoomTaskEnabled(selectedRoomId.value, task.id, !task.enabled);
  },
  onSuccess: async () => {
    actionError.value = "";
    await invalidateSelectedRoom();
  },
  onError: (error) => {
    actionError.value = error instanceof Error ? error.message : "Failed to toggle Matrix task.";
  },
});

const runNowMutation = useMutation({
  mutationFn: async (taskId: string) => {
    if (!selectedRoomId.value) {
      throw new Error("Select a room before triggering a task.");
    }
    return runMatrixRoomTaskNow(selectedRoomId.value, taskId);
  },
  onSuccess: async () => {
    actionError.value = "";
    await invalidateSelectedRoom();
  },
  onError: (error) => {
    actionError.value = error instanceof Error ? error.message : "Failed to mark the Matrix task due.";
  },
});

const deleteMutation = useMutation({
  mutationFn: async (taskId: string) => {
    if (!selectedRoomId.value) {
      throw new Error("Select a room before deleting a task.");
    }
    return deleteMatrixRoomTask(selectedRoomId.value, taskId);
  },
  onSuccess: async () => {
    actionError.value = "";
    if (editingTaskId.value) {
      resetTaskDraft();
    }
    await invalidateSelectedRoom();
  },
  onError: (error) => {
    actionError.value = error instanceof Error ? error.message : "Failed to delete Matrix task.";
  },
});

const rooms = computed(() => roomsQuery.data.value ?? []);
const tasks = computed(() => tasksQuery.data.value ?? []);
const orderedMessages = computed<MatrixMessage[]>(() => {
  const items = messagesQuery.data.value ?? [];
  return items.slice().reverse();
});
const selectedRoom = computed<MatrixRoom | null>(
  () => rooms.value.find((room) => room.roomId === selectedRoomId.value) ?? null,
);
const selectedSession = computed(() => sessionQuery.data.value?.session ?? null);
const totalMessageCount = computed(() =>
  rooms.value.reduce((sum, room) => sum + (room.stats.messageCount || 0), 0),
);

const routeTargetOptions = computed(() => {
  const values = new Set<string>();
  if (selectedRoom.value?.defaultTarget) values.add(selectedRoom.value.defaultTarget);
  Object.values(selectedRoom.value?.mentions || {}).forEach((value) => {
    if (value) values.add(value);
  });
  tasks.value.forEach((task) => {
    if (task.routeTarget) values.add(task.routeTarget);
  });
  (specialistsQuery.data.value ?? []).forEach((specialist) => {
    if (specialist.name) values.add(specialist.name);
  });
  (teamsQuery.data.value ?? []).forEach((team) => {
    if (team.name) values.add(team.name);
  });
  return Array.from(values).sort((left, right) => left.localeCompare(right));
});

watch(
  rooms,
  (nextRooms) => {
    if (!nextRooms.length) {
      selectedRoomId.value = "";
      return;
    }
    const stillExists = nextRooms.some((room) => room.roomId === selectedRoomId.value);
    if (!stillExists) {
      selectedRoomId.value = nextRooms[0].roomId;
    }
  },
  { immediate: true },
);

watch(selectedRoomId, () => {
  resetTaskDraft();
});

function resetTaskDraft() {
  editingTaskId.value = "";
  taskDraft.title = "";
  taskDraft.prompt = "";
  taskDraft.routeTarget = selectedRoom.value?.defaultTarget || "";
  taskDraft.intervalSeconds = 900;
  taskDraft.enabled = true;
}

function startEditingTask(task: MatrixTask) {
  editingTaskId.value = task.id;
  taskDraft.title = task.title;
  taskDraft.prompt = task.prompt;
  taskDraft.routeTarget = task.routeTarget || selectedRoom.value?.defaultTarget || "";
  taskDraft.intervalSeconds = task.intervalSeconds;
  taskDraft.enabled = task.enabled;
}

async function saveTask() {
  if (!taskDraft.title.trim() || !taskDraft.prompt.trim()) {
    actionError.value = "Task title and prompt are required.";
    return;
  }
  await saveMutation.mutateAsync();
}

async function toggleTask(task: MatrixTask) {
  await toggleMutation.mutateAsync(task);
}

async function runTaskNow(taskId: string) {
  await runNowMutation.mutateAsync(taskId);
}

async function removeTask(taskId: string) {
  if (!window.confirm("Delete this Matrix pulse task?")) return;
  await deleteMutation.mutateAsync(taskId);
}

async function refreshSelectedRoom() {
  await invalidateSelectedRoom();
}

async function invalidateSelectedRoom() {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: ["matrix", "rooms"] }),
    queryClient.invalidateQueries({ queryKey: ["matrix", "tasks", selectedRoomId] }),
    queryClient.invalidateQueries({ queryKey: ["matrix", "messages", selectedRoomId] }),
    queryClient.invalidateQueries({ queryKey: ["matrix", "session", selectedRoomId] }),
  ]);
}

function formatDateTime(value?: string) {
  if (!value) return "No activity yet";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function formatRelativeOrDate(value?: string) {
  if (!value) return "Not scheduled yet";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const deltaSeconds = Math.round((date.getTime() - Date.now()) / 1000);
  const absSeconds = Math.abs(deltaSeconds);
  if (absSeconds < 60) return deltaSeconds >= 0 ? "in under a minute" : "less than a minute ago";
  if (absSeconds < 3600) {
    const minutes = Math.round(absSeconds / 60);
    return deltaSeconds >= 0 ? `in ${minutes}m` : `${minutes}m ago`;
  }
  if (absSeconds < 86400) {
    const hours = Math.round(absSeconds / 3600);
    return deltaSeconds >= 0 ? `in ${hours}h` : `${hours}h ago`;
  }
  return date.toLocaleString();
}

function formatInterval(seconds: number) {
  if (seconds < 3600) return `${Math.round(seconds / 60)} min`;
  if (seconds < 86400) return `${Math.round(seconds / 3600)} hr`;
  return `${Math.round(seconds / 86400)} day`;
}
</script>