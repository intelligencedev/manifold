<template>
  <section class="flex h-full min-h-0 flex-col overflow-hidden">
    <header class="relative overflow-hidden rounded-[32px] border border-white/12 bg-[radial-gradient(circle_at_top_left,rgba(111,180,255,0.18),transparent_40%),radial-gradient(circle_at_bottom_right,rgba(255,190,92,0.16),transparent_36%),rgba(8,12,18,0.92)] px-6 py-6 shadow-[0_24px_80px_rgba(0,0,0,0.34)]">
      <div class="absolute inset-y-0 right-0 hidden w-1/3 bg-[linear-gradient(180deg,rgba(255,255,255,0.08),transparent)] md:block"></div>
      <div class="relative z-10 grid gap-6 lg:grid-cols-[minmax(0,1.3fr)_minmax(320px,0.9fr)]">
        <div class="space-y-4">
          <p class="text-[11px] font-semibold uppercase tracking-[0.28em] text-sky-100/70">
            Code Quality Control Room
          </p>
          <div class="max-w-3xl space-y-3">
            <h1 class="text-3xl font-semibold tracking-tight text-white md:text-4xl">
              Compare a candidate diff against hard evidence first, then let the judge ensemble argue over the remainder.
            </h1>
            <p class="max-w-2xl text-sm leading-6 text-slate-200/78 md:text-base">
              Build, vet, test, and formatting signals dominate. The live timeline streams the evaluation stages as the run moves from diff packaging to gate results to the final accept, reject, or human-review decision.
            </p>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <div class="border-t border-white/12 pt-3">
              <p class="text-[10px] uppercase tracking-[0.22em] text-slate-300/60">Recent runs</p>
              <p class="mt-2 text-2xl font-semibold text-white">{{ runs.length }}</p>
              <p class="mt-1 text-xs text-slate-300/72">Stored evaluations</p>
            </div>
            <div class="border-t border-white/12 pt-3">
              <p class="text-[10px] uppercase tracking-[0.22em] text-slate-300/60">Running now</p>
              <p class="mt-2 text-2xl font-semibold text-white">{{ runningCount }}</p>
              <p class="mt-1 text-xs text-slate-300/72">Live SSE updates</p>
            </div>
            <div class="border-t border-white/12 pt-3">
              <p class="text-[10px] uppercase tracking-[0.22em] text-slate-300/60">Latest action</p>
              <p class="mt-2 text-2xl font-semibold text-white">{{ latestActionLabel }}</p>
              <p class="mt-1 text-xs text-slate-300/72">Most recent stored recommendation</p>
            </div>
          </div>
        </div>

        <form class="grid gap-3 rounded-[26px] border border-white/12 bg-white/6 p-4 backdrop-blur-md" @submit.prevent="launchRun">
          <div>
            <label class="mb-1 block text-[11px] font-semibold uppercase tracking-[0.22em] text-slate-200/72">Project ID</label>
            <input v-model="draft.project_id" type="text" placeholder="Optional project workspace" class="w-full rounded-2xl border border-white/10 bg-black/18 px-3 py-2.5 text-sm text-white placeholder:text-slate-400/70 focus:border-sky-300/40 focus:outline-none" />
          </div>
          <div>
            <label class="mb-1 block text-[11px] font-semibold uppercase tracking-[0.22em] text-slate-200/72">Repository Path</label>
            <input v-model="draft.repository_path" type="text" placeholder="Defaults to active workdir" class="w-full rounded-2xl border border-white/10 bg-black/18 px-3 py-2.5 text-sm text-white placeholder:text-slate-400/70 focus:border-sky-300/40 focus:outline-none" />
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-[11px] font-semibold uppercase tracking-[0.22em] text-slate-200/72">Base ref</label>
              <input v-model="draft.base_ref" type="text" placeholder="HEAD~1" class="w-full rounded-2xl border border-white/10 bg-black/18 px-3 py-2.5 text-sm text-white placeholder:text-slate-400/70 focus:border-sky-300/40 focus:outline-none" />
            </div>
            <div>
              <label class="mb-1 block text-[11px] font-semibold uppercase tracking-[0.22em] text-slate-200/72">Head ref</label>
              <input v-model="draft.head_ref" type="text" placeholder="HEAD" class="w-full rounded-2xl border border-white/10 bg-black/18 px-3 py-2.5 text-sm text-white placeholder:text-slate-400/70 focus:border-sky-300/40 focus:outline-none" />
            </div>
          </div>
          <label class="flex items-center gap-3 rounded-2xl border border-white/10 bg-black/14 px-3 py-3 text-sm text-slate-100/86">
            <input v-model="draft.include_repo_context" type="checkbox" class="h-4 w-4 rounded border-white/20 bg-transparent text-sky-300 focus:ring-sky-300/30" />
            Include AGENTS.md and README context in the bundle
          </label>
          <div class="flex items-center justify-between gap-3 pt-1">
            <p v-if="startError" class="text-sm text-rose-300">{{ startError }}</p>
            <p v-else class="text-xs text-slate-300/72">Runs start asynchronously and stream back stage events.</p>
            <button type="submit" class="rounded-full bg-white px-4 py-2 text-sm font-semibold text-slate-900 transition hover:bg-sky-100 disabled:cursor-not-allowed disabled:opacity-60" :disabled="startMutation.isPending.value">
              {{ startMutation.isPending.value ? "Launching…" : "Start Code QA" }}
            </button>
          </div>
        </form>
      </div>
    </header>

    <div class="mt-5 grid min-h-0 flex-1 gap-5 lg:grid-cols-[360px_minmax(0,1fr)]">
      <Panel title="Stored Runs" description="Choose a run to inspect the live timeline, gate breakdown, and judge ensemble verdicts." eyebrow="History" class="min-h-0 overflow-hidden">
        <div class="mb-4 flex items-center gap-3">
          <input v-model="search" type="search" placeholder="Search by run id, repo, or action" class="w-full rounded-full border border-border/70 bg-surface-muted/70 px-4 py-2.5 text-sm text-foreground placeholder:text-faint-foreground focus:border-accent focus:outline-none focus:ring-2 focus:ring-ring/40" />
        </div>
        <div class="max-h-[calc(100vh-26rem)] space-y-2 overflow-y-auto pr-1">
          <button v-for="run in filteredRuns" :key="run.run_id" type="button" class="group w-full rounded-[22px] border px-4 py-3 text-left transition" :class="selectedRunId === run.run_id ? 'border-accent/50 bg-accent/10 shadow-[0_16px_34px_rgba(0,0,0,0.18)]' : 'border-border/60 bg-surface-muted/35 hover:border-border hover:bg-surface-muted/60'" @click="selectRun(run.run_id)">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0 space-y-2">
                <p class="truncate font-mono text-xs text-faint-foreground">{{ run.run_id }}</p>
                <p class="line-clamp-2 text-sm font-medium text-foreground">{{ relativeRepository(run.repository) }}</p>
                <div class="flex flex-wrap items-center gap-2 text-xs text-subtle-foreground">
                  <span>{{ run.diff?.base_ref || 'HEAD~1' }} → {{ run.diff?.head_ref || 'HEAD' }}</span>
                  <span>{{ formatTime(run.started_at) }}</span>
                </div>
              </div>
              <StatusBadge :state="run.status">{{ run.status }}</StatusBadge>
            </div>
            <div class="mt-3 flex items-center justify-between text-xs">
              <span class="rounded-full px-2.5 py-1 font-semibold uppercase tracking-wide" :class="actionClass(run.aggregate?.action)">{{ actionLabel(run.aggregate?.action) }}</span>
              <span class="text-subtle-foreground">Δ {{ formatDelta(run.aggregate?.quality_delta) }}</span>
            </div>
          </button>
          <div v-if="!filteredRuns.length" class="rounded-[22px] border border-dashed border-border/60 px-4 py-8 text-center text-sm text-subtle-foreground">
            No Code QA runs match the current filter.
          </div>
        </div>
      </Panel>

      <div class="min-h-0 overflow-y-auto pb-2">
        <Panel v-if="selectedRun" :title="selectedRunTitle" :description="selectedRunDescription" eyebrow="Run Detail">
          <template #actions>
            <StatusBadge :state="selectedRun.status">{{ selectedRun.status }}</StatusBadge>
          </template>

          <div class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_320px]">
            <div class="space-y-6">
              <section class="grid gap-4 md:grid-cols-3">
                <div class="border-t border-border/60 pt-3">
                  <p class="text-[10px] uppercase tracking-[0.22em] text-faint-foreground">Recommended action</p>
                  <p class="mt-2 text-2xl font-semibold text-foreground">{{ actionLabel(selectedRun.aggregate?.action) }}</p>
                  <p class="mt-1 text-xs text-subtle-foreground">{{ selectedRun.aggregate?.rationale || 'No rationale recorded.' }}</p>
                </div>
                <div class="border-t border-border/60 pt-3">
                  <p class="text-[10px] uppercase tracking-[0.22em] text-faint-foreground">Quality delta</p>
                  <p class="mt-2 text-2xl font-semibold text-foreground">{{ formatDelta(selectedRun.aggregate?.quality_delta) }}</p>
                  <p class="mt-1 text-xs text-subtle-foreground">Threshold-aware aggregate across judges</p>
                </div>
                <div class="border-t border-border/60 pt-3">
                  <p class="text-[10px] uppercase tracking-[0.22em] text-faint-foreground">Confidence</p>
                  <p class="mt-2 text-2xl font-semibold text-foreground">{{ formatPercent(selectedRun.aggregate?.confidence) }}</p>
                  <p class="mt-1 text-xs text-subtle-foreground">Mean ensemble confidence after evidence penalties</p>
                </div>
              </section>

              <section class="grid gap-4 lg:grid-cols-2">
                <div class="space-y-3">
                  <div class="flex items-center justify-between">
                    <h2 class="text-sm font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Deterministic gates</h2>
                    <span class="text-xs text-faint-foreground">{{ selectedRun.gates.length }} checks</span>
                  </div>
                  <div class="space-y-2">
                    <article v-for="gate in selectedRun.gates" :key="`${gate.ref}-${gate.name}`" class="rounded-[20px] border border-border/60 bg-surface-muted/35 px-4 py-3">
                      <div class="flex items-start justify-between gap-3">
                        <div>
                          <p class="text-sm font-semibold text-foreground">{{ gate.name }}</p>
                          <p class="text-xs text-subtle-foreground">{{ gate.ref || 'HEAD' }} · {{ gate.duration_ms }} ms</p>
                        </div>
                        <StatusBadge :state="gate.ok ? 'completed' : 'failed'">{{ gate.ok ? 'ok' : 'failed' }}</StatusBadge>
                      </div>
                      <p v-if="gate.hard_fail" class="mt-2 text-xs font-medium text-rose-300">Hard fail: this gate can force rejection before judge preference is considered.</p>
                      <pre v-if="gate.stderr || gate.stdout" class="mt-3 max-h-36 overflow-auto rounded-2xl bg-black/30 p-3 text-[11px] leading-5 text-slate-200/82">{{ gate.stderr || gate.stdout }}</pre>
                    </article>
                  </div>
                </div>

                <div class="space-y-3">
                  <div class="flex items-center justify-between">
                    <h2 class="text-sm font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Judge ensemble</h2>
                    <span class="text-xs text-faint-foreground">{{ selectedRun.judges.length }} verdicts</span>
                  </div>
                  <div class="space-y-2">
                    <article v-for="judge in selectedRun.judges" :key="judge.judge_id" class="rounded-[20px] border border-border/60 bg-surface-muted/35 px-4 py-3">
                      <div class="flex items-start justify-between gap-3">
                        <div>
                          <p class="text-sm font-semibold text-foreground">{{ judge.judge_id }}</p>
                          <p class="text-xs text-subtle-foreground">{{ judge.verdict }} · {{ formatPercent(judge.confidence) }} confidence</p>
                        </div>
                        <span class="rounded-full border border-border/60 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wide text-subtle-foreground">{{ judge.swap_applied ? 'swap' : 'direct' }}</span>
                      </div>
                      <dl class="mt-3 grid gap-2 sm:grid-cols-2">
                        <div v-for="score in judgeScoreRows(judge.scores)" :key="score.label" class="flex items-center justify-between rounded-full bg-black/18 px-3 py-1.5 text-xs">
                          <dt class="text-faint-foreground">{{ score.label }}</dt>
                          <dd class="font-semibold text-foreground">{{ score.value }}</dd>
                        </div>
                      </dl>
                      <p v-if="judge.blocking_concerns?.length" class="mt-3 text-xs text-amber-300">Blocking concerns: {{ judge.blocking_concerns.join(', ') }}</p>
                    </article>
                  </div>
                </div>
              </section>

              <section class="space-y-3">
                <div class="flex items-center justify-between">
                  <h2 class="text-sm font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Changed files</h2>
                  <span class="text-xs text-faint-foreground">{{ selectedRun.diff?.files?.length || 0 }} files</span>
                </div>
                <div class="overflow-hidden rounded-[24px] border border-border/60 bg-surface-muted/30">
                  <table class="min-w-full divide-y divide-border/60 text-sm">
                    <thead class="bg-surface-muted/60 text-left text-[11px] uppercase tracking-wide text-faint-foreground">
                      <tr>
                        <th class="px-4 py-3">Path</th>
                        <th class="px-4 py-3">Status</th>
                        <th class="px-4 py-3">Related tests</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-border/50">
                      <tr v-for="file in selectedRun.diff?.files || []" :key="file.path">
                        <td class="px-4 py-3 font-mono text-xs text-foreground">{{ file.path }}</td>
                        <td class="px-4 py-3 text-subtle-foreground">{{ file.status }}</td>
                        <td class="px-4 py-3 text-subtle-foreground">{{ file.related_tests?.join(', ') || '—' }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section>
            </div>

            <aside class="space-y-4">
              <section class="rounded-[24px] border border-border/60 bg-surface-muted/35 p-4">
                <div class="flex items-center justify-between">
                  <h2 class="text-sm font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Live timeline</h2>
                  <span class="text-xs text-faint-foreground">{{ liveEvents.length }} events</span>
                </div>
                <ol class="mt-4 space-y-3">
                  <li v-for="event in liveEvents" :key="event.sequence" class="relative border-l border-border/60 pl-4">
                    <span class="absolute left-[-5px] top-1.5 h-2.5 w-2.5 rounded-full bg-accent"></span>
                    <p class="text-xs font-semibold uppercase tracking-[0.16em] text-foreground">{{ event.type.replaceAll('_', ' ') }}</p>
                    <p class="mt-1 text-xs text-subtle-foreground">{{ formatTime(event.occurred_at) }}</p>
                    <pre v-if="event.payload" class="mt-2 overflow-auto rounded-2xl bg-black/24 p-2 text-[11px] leading-5 text-slate-200/80">{{ JSON.stringify(event.payload, null, 2) }}</pre>
                  </li>
                </ol>
              </section>

              <section class="rounded-[24px] border border-border/60 bg-surface-muted/35 p-4">
                <h2 class="text-sm font-semibold uppercase tracking-[0.2em] text-subtle-foreground">Bundle excerpt</h2>
                <p class="mt-2 text-xs leading-5 text-subtle-foreground">
                  {{ selectedRun.diff?.base_ref || 'HEAD~1' }} → {{ selectedRun.diff?.head_ref || 'HEAD' }}
                  <span v-if="selectedRun.diff?.truncated"> · truncated to policy limits</span>
                </p>
                <pre class="mt-3 max-h-[24rem] overflow-auto rounded-[22px] bg-black/30 p-3 text-[11px] leading-5 text-slate-200/82">{{ selectedRun.diff?.unified_diff || 'No diff captured.' }}</pre>
              </section>
            </aside>
          </div>
        </Panel>

        <Panel v-else title="No Run Selected" description="Start a new Code QA run or choose one from the history rail to inspect the current result set." eyebrow="Awaiting Selection">
          <div class="rounded-[26px] border border-dashed border-border/60 px-6 py-12 text-center text-sm text-subtle-foreground">
            No Code QA run is selected yet.
          </div>
        </Panel>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from "vue";
import { useMutation, useQuery, useQueryClient } from "@tanstack/vue-query";
import { useRoute, useRouter } from "vue-router";
import Panel from "@/components/ui/Panel.vue";
import StatusBadge from "@/components/StatusBadge.vue";
import {
  fetchCodeQARun,
  fetchCodeQARunEvents,
  listCodeQARuns,
  startCodeQARun,
  streamCodeQARunEvents,
  type CodeQARun,
  type CodeQARunEvent,
  type CodeQAAction,
} from "@/api/codeqa";

const route = useRoute();
const router = useRouter();
const queryClient = useQueryClient();
const search = ref("");
const startError = ref("");
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
    await queryClient.invalidateQueries({ queryKey: ["codeqa-runs"] });
    await router.push({ name: "codeqa", params: { runId: result.run_id } });
  },
  onError: (error) => {
    startError.value = error instanceof Error ? error.message : "Failed to start Code QA run";
  },
});

const runs = computed(() => runsQuery.data.value ?? []);
const runningCount = computed(() => runs.value.filter((run) => run.status === "running").length);
const latestActionLabel = computed(() => actionLabel(runs.value[0]?.aggregate?.action));
const filteredRuns = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) return runs.value;
  return runs.value.filter((run) => {
    const haystack = [
      run.run_id,
      run.repository,
      run.aggregate?.action,
      run.status,
    ]
      .filter(Boolean)
      .join(" ")
      .toLowerCase();
    return haystack.includes(term);
  });
});
const selectedRun = computed<CodeQARun | null>(() => runQuery.data.value ?? null);
const selectedRunTitle = computed(() => {
  if (!selectedRun.value) return "";
  return relativeRepository(selectedRun.value.repository);
});
const selectedRunDescription = computed(() => {
  if (!selectedRun.value) return "";
  return `${selectedRun.value.diff?.base_ref || 'HEAD~1'} → ${selectedRun.value.diff?.head_ref || 'HEAD'} · started ${formatTime(selectedRun.value.started_at)}`;
});

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

function actionClass(action?: CodeQAAction) {
  switch (action) {
    case "accept":
      return "border border-emerald-400/30 bg-emerald-400/10 text-emerald-300";
    case "reject":
      return "border border-rose-400/30 bg-rose-400/10 text-rose-300";
    case "human_review":
      return "border border-amber-400/30 bg-amber-400/10 text-amber-300";
    case "revert_candidate":
      return "border border-orange-400/30 bg-orange-400/10 text-orange-300";
    default:
      return "border border-border/60 bg-surface-muted/60 text-subtle-foreground";
  }
}

function relativeRepository(value?: string) {
  if (!value) return "No repository";
  return value.split("/").slice(-4).join("/");
}

function judgeScoreRows(scores: Record<string, number>) {
  return Object.entries(scores)
    .map(([label, value]) => ({
      label: label.replaceAll("_", " "),
      value: formatDelta(value),
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}
</script>