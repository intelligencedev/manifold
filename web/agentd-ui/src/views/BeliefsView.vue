<template>
  <section class="flex h-full min-h-0 flex-col gap-3 overflow-hidden">
    <header class="flex items-end justify-between gap-4 border-b border-border/70 pb-3">
      <div class="min-w-0">
        <p class="text-xs font-semibold uppercase tracking-[0.14em] text-subtle-foreground">
          Shared Belief Memory
        </p>
        <h1 class="mt-1 text-2xl font-semibold leading-tight text-foreground">
          Belief operations
        </h1>
      </div>
      <form class="flex min-w-[640px] items-center gap-2" @submit.prevent="loadSearch">
        <input
          v-model="query"
          class="min-h-[38px] flex-1 rounded-md border border-border bg-surface px-3 text-sm outline-none transition focus:border-accent"
          placeholder="Search belief statements"
        />
        <select
          v-model="status"
          class="min-h-[38px] rounded-md border border-border bg-surface px-3 text-sm outline-none transition focus:border-accent"
        >
          <option value="active">Active</option>
          <option value="superseded">Superseded</option>
          <option value="retracted">Retracted</option>
          <option value="">All</option>
        </select>
        <button
          class="min-h-[38px] rounded-md bg-accent px-4 text-sm font-semibold text-accent-foreground transition hover:bg-accent/90 disabled:opacity-50"
          type="submit"
          :disabled="loadingSearch"
        >
          Search
        </button>
        <button
          class="min-h-[38px] rounded-md border border-border px-3 text-sm font-semibold transition hover:bg-surface-muted"
          type="button"
          @click="loadCandidates"
        >
          Candidates {{ candidates.length }}
        </button>
      </form>
    </header>

    <div class="grid min-h-0 flex-1 grid-cols-[minmax(360px,0.95fr)_minmax(520px,1.25fr)] gap-3 overflow-hidden">
      <aside class="min-h-0 overflow-hidden rounded-lg border border-border/70 bg-surface">
        <div class="flex items-center justify-between border-b border-border/70 px-3 py-2">
          <h2 class="text-sm font-semibold">Results</h2>
          <span class="text-xs text-subtle-foreground">{{ results.length }}</span>
        </div>
        <div class="h-full min-h-0 overflow-y-auto pb-10">
          <button
            v-for="result in results"
            :key="result.belief.id"
            type="button"
            class="block w-full border-b border-border/50 px-3 py-3 text-left transition hover:bg-surface-muted/70"
            :class="selected?.belief.id === result.belief.id ? 'bg-surface-muted/80' : ''"
            @click="selectBelief(result)"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="truncate text-sm font-medium text-foreground">
                {{ result.belief.statement }}
              </span>
              <span class="shrink-0 rounded bg-background px-2 py-0.5 text-xs font-semibold text-subtle-foreground">
                {{ confidence(result.belief.confidence) }}
              </span>
            </div>
            <div class="mt-2 flex items-center gap-2 text-xs text-subtle-foreground">
              <span>{{ result.belief.kind || 'fact' }}</span>
              <span>{{ result.belief.enforcement || 'none' }}</span>
              <span>{{ result.belief.status }}</span>
              <span>for {{ result.belief.evidenceFor }}</span>
              <span>against {{ result.belief.evidenceAgainst }}</span>
            </div>
          </button>
          <p v-if="!loadingSearch && results.length === 0" class="px-3 py-8 text-sm text-subtle-foreground">
            No beliefs match the current filters.
          </p>
        </div>
      </aside>

      <main class="grid min-h-0 grid-rows-[minmax(190px,auto)_1fr] gap-3 overflow-hidden">
        <section class="rounded-lg border border-border/70 bg-surface p-4">
          <div v-if="selected" class="space-y-4">
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0">
                <p class="text-xs font-semibold uppercase tracking-[0.14em] text-subtle-foreground">
                  {{ selected.scope?.kind || 'scope' }} / {{ selected.scope?.path || selected.belief.scopeId }}
                </p>
                <h2 class="mt-1 text-lg font-semibold leading-snug text-foreground">
                  {{ selected.belief.statement }}
                </h2>
              </div>
              <button
                class="rounded-md border border-destructive/40 px-3 py-2 text-sm font-semibold text-destructive transition hover:bg-destructive/10 disabled:opacity-50"
                type="button"
                :disabled="selected.belief.status === 'retracted' || retracting"
                @click="retractSelected"
              >
                Retract
              </button>
            </div>
            <div class="grid grid-cols-4 gap-2 text-sm">
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Confidence</p>
                <p class="mt-1 font-semibold">{{ confidence(selected.belief.confidence) }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Evidence for</p>
                <p class="mt-1 font-semibold">{{ selected.belief.evidenceFor }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Evidence against</p>
                <p class="mt-1 font-semibold">{{ selected.belief.evidenceAgainst }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Status</p>
                <p class="mt-1 font-semibold capitalize">{{ selected.belief.status }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Kind</p>
                <p class="mt-1 font-semibold">{{ selected.belief.kind || 'fact' }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Enforcement</p>
                <p class="mt-1 font-semibold">{{ selected.belief.enforcement || 'none' }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Review</p>
                <p class="mt-1 font-semibold">{{ selected.belief.reviewState || 'auto_active' }}</p>
              </div>
              <div class="rounded-md bg-background/80 p-3">
                <p class="text-xs text-subtle-foreground">Source quality</p>
                <p class="mt-1 font-semibold">{{ confidence(selected.belief.sourceQuality) }}</p>
              </div>
            </div>
          </div>
          <p v-else class="text-sm text-subtle-foreground">
            Select a belief to inspect evidence, promotion history, and prompt influence.
          </p>
        </section>

        <section class="grid min-h-0 grid-cols-2 gap-3 overflow-hidden">
          <div class="min-h-0 overflow-hidden rounded-lg border border-border/70 bg-surface">
            <div class="border-b border-border/70 px-3 py-2">
              <h3 class="text-sm font-semibold">Evidence</h3>
            </div>
            <div class="h-full overflow-y-auto px-3 py-2 pb-10 text-sm">
              <div v-for="item in detail?.evidence || []" :key="item.id" class="border-b border-border/50 py-2">
                <div class="flex items-center justify-between gap-2">
                  <span class="font-medium">{{ item.sourceKind }}</span>
                  <span class="text-xs text-subtle-foreground">{{ item.polarity }} · {{ item.weight }}</span>
                </div>
                <p class="mt-1 text-subtle-foreground">{{ item.note || item.sourceId }}</p>
              </div>
              <p v-if="!detail?.evidence?.length" class="py-6 text-subtle-foreground">No evidence loaded.</p>
            </div>
          </div>

          <div class="min-h-0 overflow-hidden rounded-lg border border-border/70 bg-surface">
            <div class="border-b border-border/70 px-3 py-2">
              <h3 class="text-sm font-semibold">Promotion history</h3>
            </div>
            <div class="h-full overflow-y-auto px-3 py-2 pb-10 text-sm">
              <div v-for="item in detail?.promotions || []" :key="item.id" class="border-b border-border/50 py-2">
                <div class="font-medium">{{ confidence(item.confidenceBefore) }} → {{ confidence(item.confidenceAfter) }}</div>
                <p class="mt-1 text-subtle-foreground">{{ item.reason || `${item.fromScope} to ${item.toScope}` }}</p>
              </div>
              <p v-if="!detail?.promotions?.length" class="py-6 text-subtle-foreground">No promotions loaded.</p>
            </div>
          </div>
        </section>
      </main>
    </div>

    <footer class="grid grid-cols-[1fr_1fr] gap-3 border-t border-border/70 pt-3">
      <form class="flex items-center gap-2" @submit.prevent="loadInfluence">
        <input
          v-model="influenceQuery"
          class="min-h-[36px] flex-1 rounded-md border border-border bg-surface px-3 text-sm outline-none transition focus:border-accent"
          placeholder="Preview prompt influence for a query"
        />
        <button class="rounded-md border border-border px-3 py-2 text-sm font-semibold transition hover:bg-surface-muted" type="submit">
          Trace
        </button>
      </form>
      <div class="flex items-center justify-end gap-2">
        <input
          v-model="projectId"
          class="min-h-[36px] w-56 rounded-md border border-border bg-surface px-3 text-sm outline-none transition focus:border-accent"
          placeholder="Project ID"
        />
        <button class="rounded-md border border-border px-3 py-2 text-sm font-semibold transition hover:bg-surface-muted" type="button" @click="loadPolicies">
          Review policies
        </button>
      </div>
    </footer>

    <div v-if="influence || policies.length || candidates.length" class="max-h-72 overflow-y-auto rounded-lg border border-border/70 bg-background/90 p-3 text-xs text-subtle-foreground">
      <pre v-if="influence" class="whitespace-pre-wrap font-mono">{{ influence.prompt.text || 'No prompt context selected.' }}</pre>
      <ul v-if="influence && influence.results && influence.results.length" class="mt-2 space-y-1">
        <li v-for="(entry, idx) in influence.results" :key="idx" class="flex items-start gap-2">
          <span
            class="rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase"
            :class="entry.source === 'rag' ? 'bg-amber-200/60 text-amber-900' : 'bg-emerald-200/60 text-emerald-900'"
          >{{ entry.source === 'rag' ? 'RAG' : 'BELIEF' }}</span>
          <span class="flex-1">{{ entry.result.belief.statement }}</span>
        </li>
      </ul>
      <div v-if="policies.length" class="mt-2 space-y-2">
        <div v-for="policy in policies" :key="policy.id" class="border-t border-border/50 pt-2">
          <span class="font-semibold text-foreground">{{ policy.kind }} / {{ policy.mode || policy.severity }}</span>
          <span class="ml-2">{{ policy.statement }}</span>
        </div>
      </div>
      <div v-if="candidates.length" class="mt-2 space-y-2">
        <div v-for="candidate in candidates" :key="candidate.id" class="border-t border-border/50 pt-2">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <span class="font-semibold text-foreground">
                {{ candidate.kind }} / {{ candidate.enforcement }} / {{ confidence(candidate.confidence) }}
              </span>
              <p class="mt-1 text-subtle-foreground">{{ candidate.statement || candidate.rejectionReason }}</p>
              <p class="mt-1 text-[11px] text-subtle-foreground">
                {{ candidate.validationStatus }} · {{ candidate.reviewState }} · {{ candidate.model || 'unknown model' }}
              </p>
            </div>
            <div class="flex shrink-0 items-center gap-1">
              <button
                class="rounded border border-border px-2 py-1 font-semibold text-foreground transition hover:bg-surface-muted disabled:opacity-50"
                type="button"
                :disabled="reviewingCandidate === candidate.id || !!candidate.acceptedBeliefId"
                @click="acceptCandidate(candidate.id)"
              >
                Accept
              </button>
              <button
                class="rounded border border-destructive/40 px-2 py-1 font-semibold text-destructive transition hover:bg-destructive/10 disabled:opacity-50"
                type="button"
                :disabled="reviewingCandidate === candidate.id || candidate.reviewState === 'operator_rejected'"
                @click="rejectCandidate(candidate.id)"
              >
                Reject
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue";
import {
  acceptBeliefCandidate,
  fetchBeliefDetail,
  fetchBeliefCandidates,
  fetchBeliefInfluence,
  fetchBeliefPolicies,
  rejectBeliefCandidate,
  retractBelief,
  searchBeliefs,
  type BeliefCandidate,
  type BeliefDetail,
  type BeliefInfluenceTrace,
  type BeliefSearchResult,
  type PolicyRecord,
} from "@/api/beliefs";

const query = ref("");
const status = ref("active");
const results = ref<BeliefSearchResult[]>([]);
const selected = ref<BeliefSearchResult | null>(null);
const detail = ref<BeliefDetail | null>(null);
const loadingSearch = ref(false);
const retracting = ref(false);
const influenceQuery = ref("");
const influence = ref<BeliefInfluenceTrace | null>(null);
const projectId = ref("");
const policies = ref<PolicyRecord[]>([]);
const candidates = ref<BeliefCandidate[]>([]);
const reviewingCandidate = ref("");

onMounted(() => {
  void loadSearch();
  void loadCandidates();
});

async function loadSearch() {
  loadingSearch.value = true;
  try {
    results.value = await searchBeliefs({
      q: query.value || undefined,
      status: status.value || undefined,
      limit: 50,
    });
    if (!selected.value && results.value.length) {
      await selectBelief(results.value[0]);
    }
  } finally {
    loadingSearch.value = false;
  }
}

async function selectBelief(result: BeliefSearchResult) {
  selected.value = result;
  detail.value = await fetchBeliefDetail(result.belief.id);
}

async function retractSelected() {
  if (!selected.value) return;
  retracting.value = true;
  try {
    const updated = await retractBelief(selected.value.belief.id, "operator retraction from admin UI");
    selected.value = { ...selected.value, belief: updated };
    detail.value = detail.value ? { ...detail.value, belief: updated } : null;
  } finally {
    retracting.value = false;
  }
}

async function loadInfluence() {
  influence.value = await fetchBeliefInfluence({
    q: influenceQuery.value || query.value || undefined,
    project_id: projectId.value || undefined,
    limit: 8,
  });
}

async function loadPolicies() {
  policies.value = await fetchBeliefPolicies({
    project_id: projectId.value || undefined,
  });
}

async function loadCandidates() {
  candidates.value = await fetchBeliefCandidates({
    review_state: "needs_review",
    limit: 20,
  });
}

async function acceptCandidate(id: string) {
  reviewingCandidate.value = id;
  try {
    await acceptBeliefCandidate(id);
    await Promise.all([loadCandidates(), loadSearch()]);
  } finally {
    reviewingCandidate.value = "";
  }
}

async function rejectCandidate(id: string) {
  reviewingCandidate.value = id;
  try {
    await rejectBeliefCandidate(id);
    await loadCandidates();
  } finally {
    reviewingCandidate.value = "";
  }
}

function confidence(value: number) {
  return `${Math.round((value || 0) * 100)}%`;
}
</script>
