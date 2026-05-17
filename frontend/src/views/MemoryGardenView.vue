<template>
  <section class="space-y-5">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-semibold">Memory Garden</h1>
      <Button size="sm" variant="ghost" :loading="loading" @click="reload">Refresh</Button>
    </div>

    <!-- KPI cards -->
    <div class="grid grid-cols-3 gap-3">
      <Card title="Sessions" description="Conversation history">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ memory.sessions.length }}</div>
      </Card>
      <Card title="Beliefs" description="Active belief records">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ memory.beliefs.length }}</div>
      </Card>
      <Card title="Transit" description="Shared-memory records">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ memory.transit.length }}</div>
      </Card>
    </div>

    <div class="grid gap-5 xl:grid-cols-3">
      <!-- Sessions panel -->
      <Card title="Chat sessions" :no-padding="true">
        <div class="divide-y divide-border/30">
          <div
            v-for="session in memory.sessions"
            :key="session.id"
            class="flex cursor-pointer items-start gap-3 px-4 py-3 hover:bg-white/3 transition-colors"
            :class="selectedSession?.id === session.id ? 'bg-white/5' : ''"
            role="button"
            tabindex="0"
            @click="selectedSession = selectedSession?.id === session.id ? null : session"
            @keydown.enter="selectedSession = session"
          >
            <div class="min-w-0 flex-1">
              <div class="truncate text-xs font-medium text-white/80">{{ session.name || session.id }}</div>
              <div v-if="session.updatedAt || session.updated_at" class="text-[10px] text-white/35">{{ reltime(session.updatedAt ?? session.updated_at) }}</div>
            </div>
            <Badge v-if="selectedSession?.id === session.id" variant="accent">open</Badge>
          </div>
          <EmptyState v-if="memory.sessions.length === 0" icon="💬" title="No sessions" class="py-8" />
        </div>
      </Card>

      <!-- Beliefs panel -->
      <Card title="Beliefs" :description="`${filteredBeliefs.length} records`" :no-padding="true">
        <template #header>
          <Input v-model="beliefQuery" placeholder="Search beliefs…" class="w-48 text-xs" @input="debouncedBeliefSearch" />
        </template>
        <div class="divide-y divide-border/30">
          <div
            v-for="item in filteredBeliefs.slice(0, 30)"
            :key="item.belief?.id ?? item.id"
            class="px-4 py-3 text-xs"
          >
            <div class="mb-1 flex items-start justify-between gap-2">
              <p class="text-white/75 leading-relaxed">{{ item.belief?.statement ?? item.statement }}</p>
              <Badge v-if="item.belief?.status" :variant="beliefStatusVariant(item.belief.status)" class="shrink-0">{{ item.belief.status }}</Badge>
            </div>
            <div v-if="item.belief?.confidence != null" class="flex items-center gap-2 mt-1">
              <div class="h-1 flex-1 overflow-hidden rounded-full bg-white/10">
                <div class="h-1 rounded-full bg-accent transition-all" :style="{ width: `${Math.round(item.belief.confidence * 100)}%` }" />
              </div>
              <span class="tabular-nums text-[10px] text-white/35">{{ Math.round(item.belief.confidence * 100) }}%</span>
            </div>
          </div>
          <EmptyState v-if="filteredBeliefs.length === 0" icon="🧠" title="No beliefs" class="py-8" />
        </div>
      </Card>

      <!-- Transit panel -->
      <Card title="Transit records" :description="`${filteredTransit.length} keys`" :no-padding="true">
        <template #header>
          <Input v-model="transitQuery" placeholder="Search keys…" class="w-40 text-xs" />
        </template>
        <div class="divide-y divide-border/30">
          <div
            v-for="item in filteredTransit.slice(0, 30)"
            :key="item.keyName ?? item.id"
            class="px-4 py-3"
          >
            <div class="font-mono text-[11px] text-accent/80 truncate">{{ item.keyName ?? item.id }}</div>
            <p v-if="item.description" class="mt-0.5 text-[10px] text-white/45 line-clamp-2">{{ item.description }}</p>
            <div v-if="item.updatedAt ?? item.updated_at" class="mt-1 text-[10px] text-white/25">{{ reltime(item.updatedAt ?? item.updated_at) }}</div>
          </div>
          <EmptyState v-if="filteredTransit.length === 0" icon="📦" title="No transit records" class="py-8" />
        </div>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Input from "@/components/ui/Input.vue";
import { searchBeliefs } from "@/api/beliefs";
import { useMemoryStore } from "@/stores/memory";

const memory = useMemoryStore();
const loading = ref(false);
const selectedSession = ref<any>(null);
const beliefQuery = ref("");
const transitQuery = ref("");

const filteredBeliefs = computed(() => {
  if (!beliefQuery.value.trim()) return memory.beliefs;
  const q = beliefQuery.value.toLowerCase();
  return memory.beliefs.filter((b: any) => (b.belief?.statement ?? b.statement ?? "").toLowerCase().includes(q));
});

const filteredTransit = computed(() => {
  if (!transitQuery.value.trim()) return memory.transit;
  const q = transitQuery.value.toLowerCase();
  return memory.transit.filter((t: any) => (t.keyName ?? t.id ?? "").toLowerCase().includes(q));
});

onMounted(() => reload());

let beliefDebounce: ReturnType<typeof setTimeout>;
function debouncedBeliefSearch() {
  clearTimeout(beliefDebounce);
  beliefDebounce = setTimeout(async () => {
    if (beliefQuery.value.trim()) {
      const results = await searchBeliefs(beliefQuery.value).catch(() => []);
      memory.beliefs = results;
    } else {
      memory.refresh();
    }
  }, 300);
}

async function reload() {
  loading.value = true;
  try { await memory.refresh(); } finally { loading.value = false; }
}

function beliefStatusVariant(status: string): "success" | "danger" | "muted" {
  if (status === "active") return "success";
  if (status === "retracted") return "danger";
  return "muted";
}

function reltime(iso: string) {
  if (!iso) return "";
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.round(diff / 3_600_000)}h ago`;
  return new Date(iso).toLocaleDateString();
}
</script>
