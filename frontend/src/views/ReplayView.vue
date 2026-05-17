<template>
  <section class="space-y-5">
    <div>
      <h1 class="text-xl font-semibold">Replay</h1>
      <p class="mt-0.5 text-sm text-white/50">Inspect the replayable event timeline for any run. <kbd class="rounded border border-border px-1 py-0.5 text-[10px]">j</kbd><kbd class="rounded border border-border px-1 py-0.5 text-[10px]">k</kbd> navigate events.</p>
    </div>

    <!-- Run selector -->
    <Card>
      <div class="flex items-center gap-3">
        <div class="relative flex-1">
          <select
            v-model="selectedRunId"
            class="w-full appearance-none rounded-lg border border-border bg-black/15 px-3 py-2.5 text-sm text-foreground focus:border-accent/40 focus:outline-none"
          >
            <option value="">— Select a run —</option>
            <option v-for="run in recentRuns" :key="run.id" :value="run.id">
              {{ run.id }} · {{ run.status }} · {{ truncatePrompt(run.prompt) }}
            </option>
          </select>
        </div>
        <span class="text-white/30">or</span>
        <Input v-model="manualRunId" placeholder="Paste run ID…" class="w-64" />
        <Button variant="primary" :loading="replay.loading" :disabled="!effectiveRunId" @click="load">Load</Button>
      </div>
      <ErrorBanner v-if="replay.error" :message="replay.error" class="mt-3" @dismiss="replay.error = ''" />
    </Card>

    <!-- Timeline -->
    <div v-if="replay.events.length" class="grid gap-4 xl:grid-cols-[360px_1fr]">
      <!-- Event list -->
      <Card title="Event timeline" :description="`${replay.events.length} events`" :no-padding="true">
        <template #header>
          <Badge :variant="statusVariant(replay.status)">{{ replay.status || 'loaded' }}</Badge>
        </template>
        <div ref="listEl" class="overflow-y-auto divide-y divide-border/30 focus:outline-none" style="max-height: 600px;" tabindex="0" @keydown="handleKey">
          <EventRow
            v-for="(ev, idx) in replay.events"
            :key="idx"
            :event="ev"
            :index="idx"
            :active="cursor.index.value === idx"
            @select="cursor.set(idx)"
          />
        </div>
        <!-- Scrubber -->
        <div class="border-t border-border/40 px-3 py-2 flex items-center gap-2">
          <button class="rounded px-2 py-1 text-xs text-white/40 hover:text-white" @click="cursor.prev">‹</button>
          <input
            type="range"
            :min="0"
            :max="Math.max(replay.events.length - 1, 0)"
            :value="cursor.index.value"
            class="flex-1 accent-accent"
            @input="cursor.set(Number(($event.target as HTMLInputElement).value))"
          />
          <button class="rounded px-2 py-1 text-xs text-white/40 hover:text-white" @click="cursor.next">›</button>
          <span class="tabular-nums text-[10px] text-white/30 w-16 text-right">{{ cursor.index.value + 1 }} / {{ replay.events.length }}</span>
        </div>
      </Card>

      <!-- Event detail -->
      <div class="space-y-4">
        <Card title="Event detail" :description="`Event ${cursor.index.value + 1}`">
          <EventDeltaView :event="currentEvent" />
        </Card>

        <!-- Diff view when two adjacent events selected -->
        <Card v-if="cursor.index.value > 0" title="Diff from previous" description="Changes between adjacent events">
          <div class="grid gap-3 xl:grid-cols-2">
            <div>
              <div class="mb-1 text-[10px] uppercase text-white/30">Previous ({{ cursor.index.value }})</div>
              <pre class="overflow-auto rounded-lg bg-black/20 p-3 text-[10px] text-red-300/70 leading-relaxed" style="max-height:240px; white-space:pre-wrap;">{{ diffLeft }}</pre>
            </div>
            <div>
              <div class="mb-1 text-[10px] uppercase text-white/30">Current ({{ cursor.index.value + 1 }})</div>
              <pre class="overflow-auto rounded-lg bg-black/20 p-3 text-[10px] text-emerald-300/70 leading-relaxed" style="max-height:240px; white-space:pre-wrap;">{{ diffRight }}</pre>
            </div>
          </div>
        </Card>
      </div>
    </div>

    <!-- Empty state -->
    <EmptyState v-else-if="!replay.loading" icon="⏱" title="Load a run to inspect its event timeline" description="Select a run from the dropdown or paste a run ID." />
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";
import Input from "@/components/ui/Input.vue";
import EventDeltaView from "@/components/replay/EventDeltaView.vue";
import EventRow from "@/components/replay/EventRow.vue";
import { fetchRuns } from "@/api/metrics";
import { useReplayStore } from "@/stores/replay";
import { useReplayCursor } from "@/composables/useReplayCursor";
import { useHotkeys } from "@/composables/useHotkeys";

const replay = useReplayStore();
const cursor = useReplayCursor(() => replay.events.length || 1);
const recentRuns = ref<any[]>([]);
const selectedRunId = ref("");
const manualRunId = ref("");
const listEl = ref<HTMLElement>();

const effectiveRunId = computed(() => manualRunId.value.trim() || selectedRunId.value);

const currentEvent = computed(() => replay.events[cursor.index.value] ?? null);

const diffLeft = computed(() => {
  if (cursor.index.value < 1) return "";
  try { return JSON.stringify(replay.events[cursor.index.value - 1], null, 2); } catch { return ""; }
});

const diffRight = computed(() => {
  if (!currentEvent.value) return "";
  try { return JSON.stringify(currentEvent.value, null, 2); } catch { return ""; }
});

onMounted(async () => {
  recentRuns.value = await fetchRuns().catch(() => []);
});

watch(cursor.index, () => {
  // Scroll active row into view
  const el = listEl.value;
  if (!el) return;
  const active = el.querySelector('[aria-current="true"]') as HTMLElement | null;
  active?.scrollIntoView({ block: "nearest" });
});

useHotkeys({
  j: () => cursor.next(),
  k: () => cursor.prev(),
});

function handleKey(e: KeyboardEvent) {
  if (e.key === "ArrowDown" || e.key === "j") { e.preventDefault(); cursor.next(); }
  if (e.key === "ArrowUp" || e.key === "k") { e.preventDefault(); cursor.prev(); }
}

async function load() {
  const id = effectiveRunId.value;
  if (!id) return;
  cursor.set(0);
  await replay.load(id);
}

function truncatePrompt(p: string) {
  return p?.length > 40 ? p.slice(0, 40) + "…" : p;
}

function statusVariant(s: string): "success" | "danger" | "muted" | "warning" {
  if (s === "completed") return "success";
  if (s === "failed") return "danger";
  if (s === "running") return "warning";
  return "muted";
}
</script>
