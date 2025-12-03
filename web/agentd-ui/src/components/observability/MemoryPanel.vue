<template>
  <section class="ap-panel ap-hover flex flex-col gap-4 rounded-5 bg-surface p-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-sm font-semibold text-foreground">Memory</h2>
        <p class="mt-0.5 text-xs text-subtle-foreground">
          Introspect chat summaries and evolving experiences.
        </p>
      </div>
      <div class="flex items-center gap-2 text-xs">
        <select
          v-model="selectedSessionId"
          class="ap-input min-w-[120px] rounded border border-border bg-surface px-2 py-1 text-xs text-foreground"
        >
          <option value="">Select session…</option>
          <option
            v-for="s in sessions"
            :key="s.id"
            :value="s.id"
          >
            {{ s.name || s.id }}
          </option>
        </select>
        <input
          v-model="evolvingQuery"
          type="search"
          placeholder="Search evolving memory…"
          class="ap-input w-40 rounded border border-border bg-surface px-2 py-1 text-xs text-foreground"
          @keyup.enter.prevent="refreshEvolving"
        />
      </div>
    </header>

    <div class="grid gap-4 md:grid-cols-2 min-h-0">
      <div class="flex min-h-0 flex-col gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">
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
          v-else-if="sessionDebug"
          class="flex min-h-0 flex-col gap-3 rounded-4 border border-border bg-surface-muted/40 p-3 text-xs"
        >
          <p class="whitespace-pre-wrap text-foreground">
            {{ sessionDebug.summary || 'No summary yet.' }}
          </p>
          <p class="text-[11px] text-faint-foreground">
            Summarized {{ sessionDebug.summarizedCount }} messages; tail size
            {{ sessionDebug.plan.totalMessages - sessionDebug.plan.tailStartIndex }}.
          </p>
          <div class="grid grid-cols-2 gap-2 text-[11px] text-faint-foreground">
            <div>
              <p>Mode: <span class="font-mono">{{ sessionDebug.plan.mode }}</span></p>
              <p>Context: {{ sessionDebug.plan.contextWindowTokens }} tokens</p>
              <p>Target util: {{ (sessionDebug.plan.targetUtilizationPct * 100).toFixed(0) }}%</p>
            </div>
            <div>
              <p>History est: {{ sessionDebug.plan.estimatedHistoryTokens }} tokens</p>
              <p>Tail est: {{ sessionDebug.plan.estimatedTailTokens }} tokens</p>
              <p>Tail start index: {{ sessionDebug.plan.tailStartIndex }}</p>
            </div>
          </div>
        </div>
      </div>

      <div class="flex min-h-0 flex-col gap-2">
        <h3 class="text-xs font-semibold uppercase tracking-wide text-subtle-foreground">
          Evolving memory
        </h3>
        <div
          v-if="evolvingLoading"
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
          Evolving memory is disabled. Enable <code>evolvingMemory.enabled</code> in config to persist experiences.
        </div>
        <div
          v-else
          class="flex min-h-0 flex-col gap-2 rounded-4 border border-border bg-surface-muted/40 p-3 text-xs"
        >
          <p class="text-faint-foreground">
            {{ evolvingDebug.totalEntries }} entries · window {{ evolvingDebug.windowSize }} · topK
            {{ evolvingDebug.topK }}
          </p>
          <div class="max-h-56 space-y-2 overflow-y-auto pr-1">
            <div
              v-for="entry in (evolvingDebug.retrieved?.length ? evolvingDebug.retrieved : evolvingDebug.recentWindow || [])"
              :key="'mem-' + (entry.entry ? entry.entry.id : entry.id)"
              class="rounded-4 border border-border bg-surface px-3 py-2"
            >
              <p class="text-[11px] font-semibold text-foreground truncate">
                {{ preview(entry.entry ? entry.entry.input : entry.input) }}
              </p>
              <p class="mt-1 text-[11px] text-subtle-foreground line-clamp-2">
                {{ preview(entry.entry ? entry.entry.summary || entry.entry.output : entry.summary || entry.output, 200) }}
              </p>
              <p v-if="entry.score != null" class="mt-1 text-[10px] text-faint-foreground">
                score {{ (entry.score as number).toFixed(3) }}
              </p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useQuery } from '@tanstack/vue-query'
import {
  fetchEvolvingMemory,
  fetchMemorySessionDebug,
  fetchMemorySessions,
  type EvolvingMemoryDebug,
  type MemorySessionDebug,
} from '@/api/memory'

const sessions = ref<{ id: string; name: string }[]>([])
const selectedSessionId = ref('')
const evolvingQuery = ref('')

const sessionDebug = ref<MemorySessionDebug | null>(null)
const sessionLoading = ref(false)
const sessionError = ref('')

const evolvingDebug = ref<EvolvingMemoryDebug | null>(null)
const evolvingLoading = ref(false)
const evolvingError = ref('')

const { refetch: refetchSessions } = useQuery({
  queryKey: ['memory-sessions'],
  queryFn: fetchMemorySessions,
  staleTime: 30_000,
  onSuccess(data) {
    sessions.value = data
  },
})

async function refreshSessionDebug() {
  sessionError.value = ''
  sessionDebug.value = null
  if (!selectedSessionId.value) return
  sessionLoading.value = true
  try {
    sessionDebug.value = await fetchMemorySessionDebug(selectedSessionId.value)
  } catch (err: any) {
    sessionError.value = err?.message || 'Failed to load session memory'
  } finally {
    sessionLoading.value = false
  }
}

async function refreshEvolving() {
  evolvingError.value = ''
  evolvingLoading.value = true
  try {
    evolvingDebug.value = await fetchEvolvingMemory(evolvingQuery.value.trim() || undefined)
  } catch (err: any) {
    evolvingError.value = err?.message || 'Failed to load evolving memory'
  } finally {
    evolvingLoading.value = false
  }
}

onMounted(async () => {
  await refetchSessions()
  await refreshEvolving()
})

watch(selectedSessionId, () => {
  void refreshSessionDebug()
})

const preview = (text?: string, limit = 120) => {
  if (!text) return ''
  return text.length > limit ? text.slice(0, limit) + '…' : text
}

const hasMemory = computed(() => !!evolvingDebug.value || !!sessionDebug.value)

defineExpose({ hasMemory })
</script>

