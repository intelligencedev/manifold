<template>
  <section
    class="flex h-full flex-col gap-4 overflow-hidden rounded-lg border border-border/70 bg-surface p-6"
  >
    <header class="flex flex-wrap items-center justify-between gap-3 shrink-0">
      <div>
        <h2 class="text-sm font-semibold text-foreground">Memory Inspector</h2>
        <p class="mt-0.5 text-xs text-subtle-foreground">
          Introspect chat summaries and evolving experiences.
        </p>
      </div>
      <div
        class="grid flex-1 grid-cols-2 gap-2 text-xs sm:flex-none sm:max-w-md"
      >
        <DropdownSelect
          v-model="selectedSessionId"
          size="sm"
          class="min-w-0 text-xs"
          :options="sessionDropdownOptions"
        />
        <input
          v-model="evolvingQuery"
          type="search"
          placeholder="Search evolving memory…"
          class="halo-surface min-w-0 rounded border border-border bg-surface px-2 py-1 text-xs text-foreground"
          @keyup.enter.prevent="refreshEvolving()"
        />
      </div>
    </header>

    <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden">
      <!-- Chat Summary - full width, shrink-0 to maintain size -->
      <div class="flex shrink-0 flex-col gap-2">
        <h3
          class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
        >
          Chat summary
        </h3>
        <div
          v-if="!selectedSessionId"
          class="rounded-4 border border-dashed border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          Select a session to inspect its summary and memory plan.
        </div>
        <div
          v-else-if="sessionLoading"
          class="rounded-4 border border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          Loading session memory…
        </div>
        <div
          v-else-if="sessionError"
          class="rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
        >
          {{ sessionError }}
        </div>
        <div
          v-else-if="sessionMissing"
          class="rounded-4 border border-dashed border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          No chat summary is available for this session yet.
        </div>
        <div
          v-else-if="sessionDebug"
          class="flex flex-col gap-3 rounded-4 border border-border bg-surface-muted/40 p-3 text-xs"
        >
          <p class="text-foreground">
            {{ preview(sessionDebug.summary || "No summary yet.", 240) }}
          </p>
          <p class="text-[11px] text-faint-foreground">
            Summarized {{ sessionDebug.summarizedCount }} messages; tail size
            {{
              sessionDebug.plan.totalMessages -
              sessionDebug.plan.tailStartIndex
            }}.
          </p>
          <div class="grid grid-cols-2 gap-2 text-[11px] text-faint-foreground">
            <div>
              <p>
                Mode:
                <span class="font-mono">{{ sessionDebug.plan.mode }}</span>
              </p>
              <p>Context: {{ sessionDebug.plan.contextWindowTokens }} tokens</p>
              <p>
                Target util:
                {{ (sessionDebug.plan.targetUtilizationPct * 100).toFixed(0) }}%
              </p>
            </div>
            <div>
              <p>
                History est:
                {{ sessionDebug.plan.estimatedHistoryTokens }} tokens
              </p>
              <p>
                Tail est: {{ sessionDebug.plan.estimatedTailTokens }} tokens
              </p>
              <p>Tail start index: {{ sessionDebug.plan.tailStartIndex }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Evolving Memory - full width, flex-1 to expand and fill remaining space -->
      <div class="flex min-h-0 flex-1 flex-col gap-2">
        <h3
          class="shrink-0 font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
        >
          Evolving memory
        </h3>
        <div
          v-if="!selectedSessionId"
          class="rounded-4 border border-dashed border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          Select a session to inspect its evolving memory.
        </div>
        <div
          v-else-if="evolvingLoading"
          class="rounded-4 border border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          Loading evolving memory…
        </div>
        <div
          v-else-if="evolvingError"
          class="rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
        >
          {{ evolvingError }}
        </div>
        <div
          v-else-if="!evolvingDebug || !evolvingDebug.enabled"
          class="rounded-4 border border-dashed border-border bg-surface-muted/40 px-3 py-2 text-xs text-subtle-foreground"
        >
          Evolving memory is disabled. Enable
          <code>evolvingMemory.enabled</code> in config to persist experiences.
        </div>
        <div
          v-else
          class="flex min-h-0 flex-1 flex-col gap-2 rounded-4 border border-border bg-surface-muted/40 p-3 text-xs"
        >
          <p class="shrink-0 text-faint-foreground">
            {{ evolvingDebug.totalEntries }} entries · window
            {{ evolvingDebug.windowSize }} · topK
            {{ evolvingDebug.topK }}
          </p>
          <p
            v-if="deleteError"
            class="shrink-0 rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-[11px] text-danger"
          >
            {{ deleteError }}
          </p>
          <div class="min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
            <div
              v-for="e in evolvingEntries"
              :key="'mem-' + e.id"
              class="rounded-4 border border-border bg-surface px-3 py-2 cursor-pointer transition-colors hover:bg-surface-muted/60"
              @click="toggleExpanded(e.id)"
            >
              <div class="flex items-start justify-between gap-2">
                <p
                  :class="[
                    'text-[11px] font-semibold text-foreground',
                    isExpanded(e.id) ? 'whitespace-pre-wrap' : 'truncate',
                  ]"
                >
                  {{ isExpanded(e.id) ? e.input : preview(e.input) }}
                </p>
                <div class="flex shrink-0 items-center gap-1">
                  <button
                    type="button"
                    class="rounded-3 p-1 text-subtle-foreground transition-colors hover:bg-danger/10 hover:text-danger disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="isDeleting(e.id)"
                    title="Delete memory"
                    aria-label="Delete memory"
                    @click.stop="deleteMemoryEntry(e)"
                  >
                    <SolarTrashIcon
                      v-if="!isDeleting(e.id)"
                      class="h-3.5 w-3.5"
                    />
                    <span
                      v-else
                      class="block h-3.5 w-3.5 animate-spin rounded-full border border-current border-t-transparent"
                      aria-hidden="true"
                    />
                  </button>
                  <button
                    type="button"
                    class="rounded-3 p-1 text-subtle-foreground transition-colors hover:bg-surface-muted hover:text-foreground"
                    :aria-label="isExpanded(e.id) ? 'Collapse' : 'Expand'"
                    @click.stop="toggleExpanded(e.id)"
                  >
                    <svg
                      class="h-4 w-4 transition-transform"
                      :class="{ 'rotate-180': isExpanded(e.id) }"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M19 9l-7 7-7-7"
                      />
                    </svg>
                  </button>
                </div>
              </div>
              <p
                :class="[
                  'mt-1 text-[11px] text-subtle-foreground',
                  isExpanded(e.id) ? 'whitespace-pre-wrap' : 'line-clamp-2',
                ]"
              >
                {{
                  isExpanded(e.id)
                    ? e.summary || e.output || ""
                    : preview(e.summary || e.output, 200)
                }}
              </p>
              <p
                v-if="e.score != null"
                class="mt-1 text-[10px] text-faint-foreground"
              >
                score
                {{ (explanationFor(e.id)?.finalScore ?? e.score).toFixed(3) }}
                <span v-if="explanationFor(e.id)">
                  · sim {{ explanationFor(e.id)!.similarity.toFixed(3) }} ·
                  quality {{ explanationFor(e.id)!.qualityWeight.toFixed(2) }} ·
                  decay
                  {{ explanationFor(e.id)!.decay.toFixed(2) }}
                </span>
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useMutation, useQuery } from "@tanstack/vue-query";
import DropdownSelect from "@/components/DropdownSelect.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import {
  deleteEvolvingMemory,
  fetchEvolvingMemory,
  fetchEvolvingMemoryExplain,
  fetchMemorySessionDebug,
  fetchMemorySessions,
  type EvolvingMemoryDebug,
  type MemorySessionDebug,
  type EvolvingMemoryEntry,
  type MemoryScoreExplanation,
  type ScoredEvolvingMemoryEntry,
} from "@/api/memory";

const selectedSessionId = ref("");
const evolvingQuery = ref("");
const expandedEntries = ref<Set<string>>(new Set());
const deletingMemoryIds = ref<Set<string>>(new Set());

const sessionDebug = ref<MemorySessionDebug | null>(null);
const sessionLoading = ref(false);
const sessionError = ref("");
const sessionMissing = ref(false);

const evolvingDebug = ref<EvolvingMemoryDebug | null>(null);
const evolvingExplanations = ref<MemoryScoreExplanation[]>([]);
const evolvingLoading = ref(false);
const evolvingError = ref("");
const deleteError = ref("");

const { data: sessionsData, refetch: refetchSessions } = useQuery({
  queryKey: ["memory-sessions"],
  queryFn: fetchMemorySessions,
  staleTime: 30_000,
});

// Compute sessions from the query data - Vue Query v5 removed onSuccess callback
const sessions = computed(() => sessionsData.value ?? []);

const deleteMemoryMutation = useMutation({
  mutationFn: ({ id, sessionId }: { id: string; sessionId: string }) =>
    deleteEvolvingMemory(id, sessionId),
});

const sessionDropdownOptions = computed(() => [
  { id: "", label: "Select session…", value: "" },
  ...sessions.value.map((s) => ({
    id: s.id,
    label: s.name || s.id,
    value: s.id,
  })),
]);

async function refreshSessionDebug() {
  sessionError.value = "";
  sessionMissing.value = false;
  sessionDebug.value = null;
  if (!selectedSessionId.value) return;
  sessionLoading.value = true;
  try {
    sessionDebug.value = await fetchMemorySessionDebug(selectedSessionId.value);
  } catch (err: any) {
    if (err?.response?.status === 404) {
      sessionMissing.value = true;
      return;
    }
    sessionError.value = err?.message || "Failed to load session memory";
  } finally {
    sessionLoading.value = false;
  }
}

async function refreshEvolving(options: { showLoading?: boolean } = {}) {
  evolvingError.value = "";
  if (!selectedSessionId.value) {
    evolvingDebug.value = null;
    evolvingExplanations.value = [];
    evolvingLoading.value = false;
    return;
  }
  if (options.showLoading !== false) {
    evolvingLoading.value = true;
  }
  try {
    const query = evolvingQuery.value.trim();
    const debug = await fetchEvolvingMemory(
      query || undefined,
      selectedSessionId.value,
    );
    evolvingDebug.value = debug;
    if (query) {
      const explain = await fetchEvolvingMemoryExplain(
        query,
        selectedSessionId.value,
      );
      evolvingExplanations.value = explain.explanations ?? [];
    } else {
      evolvingExplanations.value = [];
    }
  } catch (err: any) {
    evolvingError.value = err?.message || "Failed to load evolving memory";
  } finally {
    if (options.showLoading !== false) {
      evolvingLoading.value = false;
    }
  }
}

onMounted(async () => {
  await refetchSessions();
});

watch(
  sessions,
  (nextSessions) => {
    if (nextSessions.length === 0) {
      if (selectedSessionId.value) {
        selectedSessionId.value = "";
      }
      return;
    }

    const selectedStillExists = nextSessions.some(
      (session) => session.id === selectedSessionId.value,
    );
    if (!selectedStillExists) {
      selectedSessionId.value = nextSessions[0]?.id || "";
    }
  },
  { immediate: true },
);

watch(selectedSessionId, () => {
  void refreshSessionDebug();
  void refreshEvolving();
});

const preview = (text?: string, limit = 120) => {
  if (!text) return "";
  return text.length > limit ? text.slice(0, limit) + "…" : text;
};

const toggleExpanded = (id: string) => {
  if (expandedEntries.value.has(id)) {
    expandedEntries.value.delete(id);
  } else {
    expandedEntries.value.add(id);
  }
};

const isExpanded = (id: string) => expandedEntries.value.has(id);

const isDeleting = (id: string) => deletingMemoryIds.value.has(id);

function removeEvolvingEntry(id: string) {
  const debug = evolvingDebug.value;
  if (!debug) return;

  const recentWindow = (debug.recentWindow || []).filter(
    (entry) => entry.id !== id,
  );
  const retrieved = debug.retrieved?.filter((item) => item.entry.id !== id);
  const removedFromRecent =
    recentWindow.length !== (debug.recentWindow || []).length;
  const removedFromRetrieved =
    (retrieved?.length ?? 0) !== (debug.retrieved?.length ?? 0);

  evolvingDebug.value = {
    ...debug,
    totalEntries:
      removedFromRecent || removedFromRetrieved
        ? Math.max(0, debug.totalEntries - 1)
        : debug.totalEntries,
    recentWindow,
    retrieved,
  };
  evolvingExplanations.value = evolvingExplanations.value.filter(
    (explanation) => explanation.entry.id !== id,
  );
}

function deleteErrorMessage(err: any) {
  const data = err?.response?.data;
  if (typeof data === "string" && data.trim()) return data.trim();
  return err?.message || "Failed to delete memory";
}

async function deleteMemoryEntry(entry: NormalizedEvolvingEntry) {
  const id = entry.id.trim();
  if (!id || !selectedSessionId.value || isDeleting(id)) return;
  if (!window.confirm("Delete this memory? This cannot be undone.")) return;

  deleteError.value = "";
  deletingMemoryIds.value.add(id);
  try {
    await deleteMemoryMutation.mutateAsync({
      id,
      sessionId: selectedSessionId.value,
    });
    removeEvolvingEntry(id);
    expandedEntries.value.delete(id);
    await refreshEvolving({ showLoading: false });
  } catch (err: any) {
    deleteError.value = deleteErrorMessage(err);
  } finally {
    deletingMemoryIds.value.delete(id);
  }
}

const explanationByID = computed(() => {
  const lookup = new Map<string, MemoryScoreExplanation>();
  for (const explanation of evolvingExplanations.value) {
    lookup.set(explanation.entry.id, explanation);
  }
  return lookup;
});

const explanationFor = (id: string) => explanationByID.value.get(id);

const hasMemory = computed(() => !!evolvingDebug.value || !!sessionDebug.value);

type NormalizedEvolvingEntry = EvolvingMemoryEntry & { score: number | null };

const evolvingEntries = computed<NormalizedEvolvingEntry[]>(() => {
  const dbg = evolvingDebug.value;
  if (!dbg) return [];
  const list = (
    dbg.retrieved && dbg.retrieved.length
      ? dbg.retrieved
      : dbg.recentWindow || []
  ) as Array<ScoredEvolvingMemoryEntry | EvolvingMemoryEntry>;
  return list.map((item) => {
    if ("entry" in item) {
      const se = item as ScoredEvolvingMemoryEntry;
      return { ...se.entry, score: se.score };
    }
    const ee = item as EvolvingMemoryEntry;
    return { ...ee, score: null };
  });
});

defineExpose({ hasMemory });
</script>
