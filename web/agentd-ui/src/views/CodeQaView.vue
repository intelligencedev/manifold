<template>
  <section class="flex h-full min-h-0 flex-col gap-5 overflow-hidden">
    <!-- Slim functional page header. No hero, no marketing copy. -->
    <header class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="min-w-0">
        <h1 class="text-2xl font-semibold text-foreground">Code QA</h1>
        <p class="text-sm text-subtle-foreground">
          Gate runs and judge verdicts on candidate diffs.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <input
          v-model="search"
          type="search"
          placeholder="Search runs"
          class="w-full rounded-lg border border-border/70 bg-surface-muted/60 px-3 py-2 text-sm text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40 sm:w-64"
        />
        <button
          type="button"
          class="rounded-lg border border-accent/50 bg-accent/10 px-3 py-2 text-sm font-semibold text-accent transition hover:bg-accent/15"
          :aria-expanded="formOpen"
          @click="formOpen = !formOpen"
        >
          {{ formOpen ? "Close" : "New run" }}
        </button>
      </div>
    </header>

    <!-- Inline launch form: only visible on demand, single compact row. -->
    <form
      v-if="formOpen"
      class="grid gap-3 rounded-xl border border-border/60 bg-surface-muted/30 p-4 md:grid-cols-[2fr_2fr_1fr_1fr_auto]"
      @submit.prevent="launchRun"
    >
      <label class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-subtle-foreground">
        Project ID
        <input
          v-model="draft.project_id"
          type="text"
          placeholder="Optional"
          class="rounded-md border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-subtle-foreground">
        Repository path
        <input
          v-model="draft.repository_path"
          type="text"
          placeholder="Active workdir"
          class="rounded-md border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-subtle-foreground">
        Base
        <input
          v-model="draft.base_ref"
          type="text"
          placeholder="HEAD~1"
          class="rounded-md border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
        />
      </label>
      <label class="flex flex-col gap-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-subtle-foreground">
        Head
        <input
          v-model="draft.head_ref"
          type="text"
          placeholder="HEAD"
          class="rounded-md border border-border/60 bg-surface px-3 py-2 text-sm font-normal normal-case tracking-normal text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40"
        />
      </label>
      <div class="flex flex-col items-stretch justify-end gap-2 md:flex-row md:items-center">
        <label class="flex items-center gap-2 text-xs text-subtle-foreground">
          <input v-model="draft.include_repo_context" type="checkbox" class="h-4 w-4 rounded border-border/60" />
          Repo context
        </label>
        <button
          type="submit"
          :disabled="startMutation.isPending.value"
          class="rounded-md bg-accent px-4 py-2 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {{ startMutation.isPending.value ? "Starting…" : "Start run" }}
        </button>
      </div>
      <p v-if="startError" class="text-xs text-danger md:col-span-5">{{ startError }}</p>
    </form>

    <!-- Two-column working surface: runs rail + run detail. -->
    <div class="grid min-h-0 flex-1 gap-5 lg:grid-cols-[300px_minmax(0,1fr)]">
      <!-- Runs rail -->
      <aside class="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border/60 bg-surface-muted/20">
        <div class="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <h2 class="text-[11px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground">
            Runs
          </h2>
          <span class="text-xs text-faint-foreground tabular-nums">
            {{ filteredRuns.length }}<span v-if="filteredRuns.length !== runs.length"> / {{ runs.length }}</span>
          </span>
        </div>
        <div class="scrollbar-inset flex-1 space-y-1 overflow-y-auto p-2">
          <button
            v-for="run in filteredRuns"
            :key="run.run_id"
            type="button"
            class="w-full rounded-lg border px-3 py-2.5 text-left transition"
            :class="
              selectedRunId === run.run_id
                ? 'border-accent/50 bg-accent/10'
                : 'border-border/40 bg-surface-muted/30 hover:border-border hover:bg-surface-muted/55'
            "
            @click="selectRun(run.run_id)"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="truncate font-mono text-[11px] text-subtle-foreground">{{ shortId(run.run_id) }}</span>
              <StatusBadge :state="badgeState(run.status)">{{ run.status }}</StatusBadge>
            </div>
            <p class="mt-1 truncate text-sm font-medium text-foreground">
              {{ relativeRepository(run.repository) }}
            </p>
            <div class="mt-1 flex items-center justify-between text-[11px] text-faint-foreground">
              <span>{{ formatTime(run.started_at) }}</span>
              <span :class="actionTextClass(run.aggregate?.action)">
                {{ actionLabel(run.aggregate?.action) }}
              </span>
            </div>
          </button>
          <div
            v-if="!filteredRuns.length"
            class="rounded-lg border border-dashed border-border/60 px-3 py-8 text-center text-xs text-subtle-foreground"
          >
            {{ runs.length ? "No runs match your search." : "No runs yet — start one above." }}
          </div>
        </div>
      </aside>

      <!-- Run detail -->
      <div
        v-if="selectedRun"
        class="flex min-h-0 flex-col overflow-hidden rounded-xl border border-border/60 bg-surface-muted/20"
      >
        <!-- Detail header: identity + status + key metrics -->
        <div class="border-b border-border/60 px-5 py-4">
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <p class="font-mono text-[11px] text-faint-foreground">{{ selectedRun.run_id }}</p>
              <h2 class="truncate text-base font-semibold text-foreground">{{ selectedRunTitle }}</h2>
              <p class="mt-0.5 text-xs text-subtle-foreground">{{ selectedRunDescription }}</p>
            </div>
            <div class="flex items-center gap-2">
              <span
                class="inline-flex items-center rounded-full px-2.5 py-0.5 text-[11px] font-semibold uppercase tracking-wide"
                :class="actionPillClass(selectedRun.aggregate?.action)"
              >
                {{ actionLabel(selectedRun.aggregate?.action) }}
              </span>
              <StatusBadge :state="badgeState(selectedRun.status)">{{ selectedRun.status }}</StatusBadge>
            </div>
          </div>

          <dl class="mt-4 grid gap-4 sm:grid-cols-4">
            <div>
              <dt class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground">
                Quality Δ
              </dt>
              <dd
                class="mt-1 text-lg font-semibold tabular-nums"
                :class="deltaClass(selectedRun.aggregate?.quality_delta)"
              >
                {{ formatDelta(selectedRun.aggregate?.quality_delta) }}
              </dd>
            </div>
            <div>
              <dt class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground">
                Confidence
              </dt>
              <dd class="mt-1 text-lg font-semibold text-foreground tabular-nums">
                {{ formatPercent(selectedRun.aggregate?.confidence) }}
              </dd>
            </div>
            <div>
              <dt class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground">
                Gates
              </dt>
              <dd class="mt-1 text-lg font-semibold text-foreground tabular-nums">
                {{ gatePassCount }}<span class="text-faint-foreground">/{{ selectedRun.gates.length }}</span>
              </dd>
            </div>
            <div>
              <dt class="text-[10px] font-semibold uppercase tracking-[0.22em] text-subtle-foreground">
                Judges
              </dt>
              <dd class="mt-1 text-lg font-semibold text-foreground tabular-nums">
                {{ selectedRun.judges.length }}
              </dd>
            </div>
          </dl>

          <p v-if="selectedRun.aggregate?.rationale" class="mt-3 text-xs text-subtle-foreground">
            {{ selectedRun.aggregate.rationale }}
          </p>
        </div>

        <!-- Tab strip -->
        <nav
          class="flex items-center gap-1 border-b border-border/60 px-3 pt-2"
          role="tablist"
          aria-label="Run detail sections"
        >
          <button
            v-for="tab in detailTabs"
            :key="tab.id"
            type="button"
            role="tab"
            :aria-selected="detailTab === tab.id ? 'true' : 'false'"
            :tabindex="detailTab === tab.id ? 0 : -1"
            class="relative px-3 py-2 text-xs font-semibold uppercase tracking-[0.16em] transition"
            :class="
              detailTab === tab.id
                ? 'text-accent after:absolute after:inset-x-2 after:-bottom-px after:h-0.5 after:rounded-full after:bg-accent'
                : 'text-subtle-foreground hover:text-foreground'
            "
            @click="detailTab = tab.id"
          >
            {{ tab.label }}
            <span v-if="tab.count !== undefined" class="ml-1 text-[10px] text-faint-foreground tabular-nums">
              {{ tab.count }}
            </span>
          </button>
        </nav>

        <!-- Tab panels -->
        <div class="scrollbar-inset min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <!-- GATES -->
          <section v-if="detailTab === 'gates'" role="tabpanel" class="space-y-3">
            <div class="overflow-hidden rounded-lg border border-border/60">
              <table class="min-w-full divide-y divide-border/60 text-sm">
                <thead class="bg-surface-muted/50 text-left text-[11px] uppercase tracking-wide text-faint-foreground">
                  <tr>
                    <th class="px-3 py-2">Gate</th>
                    <th class="px-3 py-2">Ref</th>
                    <th class="px-3 py-2 text-right">Duration</th>
                    <th class="px-3 py-2">Result</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border/50">
                  <template v-for="gate in selectedRun.gates" :key="`${gate.ref}-${gate.name}`">
                    <tr class="hover:bg-surface-muted/30">
                      <td class="px-3 py-2 font-mono text-xs text-foreground">{{ gate.name }}</td>
                      <td class="px-3 py-2 font-mono text-xs text-subtle-foreground">{{ gate.ref || "HEAD" }}</td>
                      <td class="px-3 py-2 text-right text-xs text-subtle-foreground tabular-nums">
                        {{ gate.duration_ms }} ms
                      </td>
                      <td class="px-3 py-2">
                        <span
                          class="inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-semibold uppercase tracking-wide"
                          :class="gateResultClass(gate)"
                        >
                          <span class="h-1.5 w-1.5 rounded-full" :class="gateDotClass(gate)"></span>
                          {{ gateResultLabel(gate) }}
                        </span>
                      </td>
                    </tr>
                    <tr v-if="gate.stderr || gate.stdout" class="bg-surface-muted/20">
                      <td colspan="4" class="px-3 pb-3">
                        <pre class="max-h-40 overflow-auto rounded-md border border-border/50 bg-black/30 p-2 text-[11px] leading-5 text-slate-200/85">{{ gate.stderr || gate.stdout }}</pre>
                      </td>
                    </tr>
                  </template>
                  <tr v-if="!selectedRun.gates.length">
                    <td colspan="4" class="px-3 py-8 text-center text-xs text-subtle-foreground">
                      No gates recorded.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <!-- JUDGES -->
          <section v-else-if="detailTab === 'judges'" role="tabpanel" class="grid gap-3 md:grid-cols-2">
            <article
              v-for="judge in selectedRun.judges"
              :key="judge.judge_id"
              class="rounded-lg border border-border/60 bg-surface-muted/30 px-3 py-3"
            >
              <header class="flex items-center justify-between gap-2">
                <div class="min-w-0">
                  <p class="truncate text-sm font-semibold text-foreground">{{ judge.judge_id }}</p>
                  <p class="text-[11px] text-subtle-foreground">
                    {{ judge.verdict }} · {{ formatPercent(judge.confidence) }}
                  </p>
                </div>
                <span
                  class="rounded-full border border-border/60 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-subtle-foreground"
                >
                  {{ judge.swap_applied ? "swap" : "direct" }}
                </span>
              </header>
              <dl class="mt-3 grid grid-cols-2 gap-x-4 gap-y-1.5 text-xs">
                <template v-for="score in judgeScoreRows(judge.scores)" :key="score.label">
                  <dt class="truncate text-faint-foreground">{{ score.label }}</dt>
                  <dd class="text-right font-semibold tabular-nums" :class="deltaClass(score.raw)">
                    {{ score.value }}
                  </dd>
                </template>
              </dl>
              <p v-if="judge.blocking_concerns?.length" class="mt-2 text-[11px] text-warning">
                Blocking: {{ judge.blocking_concerns.join(", ") }}
              </p>
            </article>
            <div
              v-if="!selectedRun.judges.length"
              class="rounded-lg border border-dashed border-border/60 px-3 py-8 text-center text-xs text-subtle-foreground md:col-span-2"
            >
              No judges recorded.
            </div>
          </section>

          <!-- FILES -->
          <section v-else-if="detailTab === 'files'" role="tabpanel">
            <div class="overflow-hidden rounded-lg border border-border/60">
              <table class="min-w-full divide-y divide-border/60 text-sm">
                <thead class="bg-surface-muted/50 text-left text-[11px] uppercase tracking-wide text-faint-foreground">
                  <tr>
                    <th class="px-3 py-2">Path</th>
                    <th class="px-3 py-2">Status</th>
                    <th class="px-3 py-2">Related tests</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border/50">
                  <tr v-for="file in selectedRun.diff?.files || []" :key="file.path" class="hover:bg-surface-muted/30">
                    <td class="px-3 py-2 font-mono text-xs text-foreground">{{ file.path }}</td>
                    <td class="px-3 py-2 text-xs text-subtle-foreground">{{ file.status }}</td>
                    <td class="px-3 py-2 font-mono text-xs text-subtle-foreground">
                      {{ file.related_tests?.join(", ") || "—" }}
                    </td>
                  </tr>
                  <tr v-if="!selectedRun.diff?.files?.length">
                    <td colspan="3" class="px-3 py-8 text-center text-xs text-subtle-foreground">
                      No file changes recorded.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>

          <!-- DIFF -->
          <section v-else-if="detailTab === 'diff'" role="tabpanel" class="space-y-2">
            <div class="flex items-center justify-between">
              <p class="font-mono text-[11px] text-subtle-foreground">
                {{ selectedRun.diff?.base_ref || "HEAD~1" }} → {{ selectedRun.diff?.head_ref || "HEAD" }}
              </p>
              <span
                v-if="selectedRun.diff?.truncated"
                class="rounded-full border border-warning/40 bg-warning/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-warning"
              >
                truncated
              </span>
            </div>
            <pre
              class="scrollbar-inset overflow-auto rounded-md border border-border/60 bg-black/30 p-3 text-[11px] leading-5 text-slate-200/85"
            >{{ selectedRun.diff?.unified_diff || "No diff captured." }}</pre>
          </section>

          <!-- TIMELINE -->
          <section v-else-if="detailTab === 'timeline'" role="tabpanel">
            <ol class="space-y-2">
              <li
                v-for="event in liveEvents"
                :key="event.sequence"
                class="rounded-md border border-border/60 bg-surface-muted/30 px-3 py-2"
              >
                <div class="flex items-center justify-between gap-2">
                  <p class="text-[11px] font-semibold uppercase tracking-[0.16em] text-foreground">
                    {{ event.type.replaceAll("_", " ") }}
                  </p>
                  <span class="text-[10px] text-faint-foreground">{{ formatTime(event.occurred_at) }}</span>
                </div>
                <pre
                  v-if="event.payload && Object.keys(event.payload).length"
                  class="mt-1.5 max-h-36 overflow-auto rounded bg-black/24 p-2 text-[10px] leading-4 text-slate-200/80"
                >{{ JSON.stringify(event.payload, null, 2) }}</pre>
              </li>
              <li
                v-if="!liveEvents.length"
                class="rounded-md border border-dashed border-border/60 px-3 py-8 text-center text-xs text-subtle-foreground"
              >
                No events yet.
              </li>
            </ol>
          </section>
        </div>
      </div>

      <!-- Empty detail placeholder -->
      <div
        v-else
        class="flex min-h-0 items-center justify-center rounded-xl border border-dashed border-border/60 bg-surface-muted/20 px-6 text-sm text-subtle-foreground"
      >
        Select a run from the list, or start a new one.
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { useRoute, useRouter } from "vue-router";
import StatusBadge from "@/components/StatusBadge.vue";
import {
  fetchCodeQARun,
  fetchCodeQARunEvents,
  listCodeQARuns,
  startCodeQARun,
  streamCodeQARunEvents,
  type CodeQAAction,
  type CodeQARun,
  type CodeQARunEvent,
} from "@/api/codeqa";

type BadgeState =
  | "online"
  | "offline"
  | "degraded"
  | "queued"
  | "running"
  | "failed"
  | "completed";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();

type DetailTabId = "gates" | "judges" | "files" | "diff" | "timeline";

const search = ref("");
const startError = ref("");
const formOpen = ref(false);
const detailTab = ref<DetailTabId>("gates");
const liveEvents = ref<CodeQARunEvent[]>([]);
const draft = reactive({
  project_id: "",
  repository_path: "",
  base_ref: "",
  head_ref: "",
  include_repo_context: true,
});

let stopStreaming: (() => void) | null = null;

const runsQuery = useQuery({
  queryKey: ["codeqa-runs"],
  queryFn: () => listCodeQARuns(50),
  staleTime: 10_000,
  refetchInterval: 15_000,
});

const selectedRunId = computed(() => {
  const param = route.params.runId;
  if (typeof param === "string" && param.trim()) return param.trim();
  const runs = runsQuery.data.value ?? [];
  return runs[0]?.run_id ?? "";
});

const runQuery = useQuery({
  queryKey: ["codeqa-run", selectedRunId],
  queryFn: () => fetchCodeQARun(selectedRunId.value),
  enabled: computed(() => Boolean(selectedRunId.value)),
  staleTime: 5_000,
});

const eventsQuery = useQuery({
  queryKey: ["codeqa-run-events", selectedRunId],
  queryFn: () => fetchCodeQARunEvents(selectedRunId.value),
  enabled: computed(() => Boolean(selectedRunId.value)),
  staleTime: 0,
});

const startMutation = useMutation({
  mutationFn: startCodeQARun,
  onSuccess: async (result) => {
    startError.value = "";
    formOpen.value = false;
    await queryClient.invalidateQueries({ queryKey: ["codeqa-runs"] });
    await router.push({ name: "codeqa", params: { runId: result.run_id } });
  },
  onError: (error) => {
    startError.value = error instanceof Error ? error.message : "Failed to start Code QA run";
  },
});

const runs = computed(() => runsQuery.data.value ?? []);

const filteredRuns = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) return runs.value;
  return runs.value.filter((run) => {
    const haystack = [run.run_id, run.repository, run.aggregate?.action, run.status]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return haystack.includes(term);
  });
});

const selectedRun = computed<CodeQARun | null>(() => runQuery.data.value ?? null);
const selectedRunTitle = computed(() =>
  selectedRun.value ? relativeRepository(selectedRun.value.repository) : "",
);
const selectedRunDescription = computed(() => {
  if (!selectedRun.value) return "";
  return `${selectedRun.value.diff?.base_ref || "HEAD~1"} → ${selectedRun.value.diff?.head_ref || "HEAD"} · started ${formatTime(selectedRun.value.started_at)}`;
});

const gatePassCount = computed(
  () => selectedRun.value?.gates.filter((gate) => gate.ok || gate.skipped).length ?? 0,
);

const detailTabs = computed(() => [
  { id: "gates" as DetailTabId, label: "Gates", count: selectedRun.value?.gates.length ?? 0 },
  { id: "judges" as DetailTabId, label: "Judges", count: selectedRun.value?.judges.length ?? 0 },
  {
    id: "files" as DetailTabId,
    label: "Files",
    count: selectedRun.value?.diff?.files?.length ?? 0,
  },
  { id: "diff" as DetailTabId, label: "Diff", count: undefined },
  { id: "timeline" as DetailTabId, label: "Timeline", count: liveEvents.value.length },
]);

watch(
  () => eventsQuery.data.value,
  (payload) => {
    liveEvents.value = payload?.events ? [...payload.events] : [];
  },
  { immediate: true },
);

watch(
  () => [selectedRunId.value, selectedRun.value?.status] as const,
  async ([runId, status]) => {
    stopStreaming?.();
    stopStreaming = null;
    if (!runId || !status || status !== "running") {
      return;
    }
    stopStreaming = streamCodeQARunEvents(
      runId,
      (event) => {
        if (!liveEvents.value.some((existing) => existing.sequence === event.sequence)) {
          liveEvents.value = [...liveEvents.value, event].sort((a, b) => a.sequence - b.sequence);
        }
        if (event.type === "run_completed" || event.type === "run_failed") {
          stopStreaming?.();
          stopStreaming = null;
          queryClient.invalidateQueries({ queryKey: ["codeqa-run", selectedRunId] });
          queryClient.invalidateQueries({ queryKey: ["codeqa-run-events", selectedRunId] });
          queryClient.invalidateQueries({ queryKey: ["codeqa-runs"] });
        }
      },
      () => {
        queryClient.invalidateQueries({ queryKey: ["codeqa-run-events", selectedRunId] });
      },
    );
  },
  { immediate: true },
);

onBeforeUnmount(() => {
  stopStreaming?.();
});

async function launchRun() {
  await startMutation.mutateAsync({
    project_id: draft.project_id.trim() || undefined,
    repository_path: draft.repository_path.trim() || undefined,
    base_ref: draft.base_ref.trim() || undefined,
    head_ref: draft.head_ref.trim() || undefined,
    include_repo_context: draft.include_repo_context,
  });
}

function selectRun(runId: string) {
  router.push({ name: "codeqa", params: { runId } });
}

function shortId(runId: string) {
  return runId.length > 10 ? `${runId.slice(0, 8)}…` : runId;
}

function badgeState(status: string): BadgeState {
  switch (status) {
    case "running":
    case "queued":
    case "failed":
    case "completed":
      return status;
    default:
      return "queued";
  }
}

function formatTime(value?: string) {
  if (!value) return "—";
  return new Date(value).toLocaleString();
}

function formatPercent(value?: number) {
  if (typeof value !== "number") return "—";
  return `${Math.round(value * 100)}%`;
}

function formatDelta(value?: number) {
  if (typeof value !== "number") return "—";
  const prefix = value > 0 ? "+" : "";
  return `${prefix}${value.toFixed(2)}`;
}

function actionLabel(action?: CodeQAAction) {
  switch (action) {
    case "accept":
      return "Accept";
    case "reject":
      return "Reject";
    case "human_review":
      return "Human Review";
    case "revert_candidate":
      return "Revert";
    default:
      return "Pending";
  }
}

function actionPillClass(action?: CodeQAAction) {
  switch (action) {
    case "accept":
      return "border border-success/40 bg-success/10 text-success";
    case "reject":
      return "border border-danger/40 bg-danger/10 text-danger";
    case "human_review":
      return "border border-warning/40 bg-warning/10 text-warning";
    case "revert_candidate":
      return "border border-warning/40 bg-warning/10 text-warning";
    default:
      return "border border-border/60 bg-surface-muted/60 text-subtle-foreground";
  }
}

function actionTextClass(action?: CodeQAAction) {
  switch (action) {
    case "accept":
      return "text-success";
    case "reject":
      return "text-danger";
    case "human_review":
    case "revert_candidate":
      return "text-warning";
    default:
      return "text-subtle-foreground";
  }
}

function relativeRepository(value?: string) {
  if (!value) return "No repository";
  return value.split("/").slice(-4).join("/");
}

function gateResultLabel(gate: { ok: boolean; hard_fail?: boolean; skipped?: boolean }) {
  if (gate.skipped) return "skipped";
  if (gate.ok) return "ok";
  return gate.hard_fail ? "hard fail" : "failed";
}

function gateResultClass(gate: { ok: boolean; hard_fail?: boolean; skipped?: boolean }) {
  if (gate.skipped) return "border border-border/60 bg-surface-muted/60 text-subtle-foreground";
  if (gate.ok) return "border border-success/40 bg-success/10 text-success";
  if (gate.hard_fail) return "border border-danger/40 bg-danger/10 text-danger";
  return "border border-warning/40 bg-warning/10 text-warning";
}

function gateDotClass(gate: { ok: boolean; hard_fail?: boolean; skipped?: boolean }) {
  if (gate.skipped) return "bg-subtle-foreground";
  if (gate.ok) return "bg-success";
  if (gate.hard_fail) return "bg-danger";
  return "bg-warning";
}

function deltaClass(value: number | undefined) {
  if (typeof value !== "number" || value === 0) return "text-foreground";
  return value > 0 ? "text-success" : "text-danger";
}

function judgeScoreRows(scores: Record<string, number>) {
  return Object.entries(scores)
    .map(([label, value]) => ({
      label: label.replaceAll("_", " "),
      value: formatDelta(value),
      raw: value,
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}
</script>
