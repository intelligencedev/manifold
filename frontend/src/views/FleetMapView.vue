<template>
  <div class="space-y-5">
    <!-- KPI bar -->
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Card title="Total runs" :description="runningRuns.length + ' running'">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ fleet.runs.length }}</div>
      </Card>
      <Card title="Specialists" :description="(fleet.state?.specialists.length ?? 0) + ' registered'">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ fleet.state?.specialists.length ?? 0 }}</div>
      </Card>
      <Card title="Teams" :description="(fleet.state?.teams.length ?? 0) + ' configured'">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ fleet.state?.teams.length ?? 0 }}</div>
      </Card>
      <Card title="Delegation edges" :description="(fleet.state?.active_delegation_edges.length ?? 0) + ' active'">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ fleet.state?.active_delegation_edges.length ?? 0 }}</div>
      </Card>
    </div>

    <!-- WebGL Fleet Map + Event Feed -->
    <div class="grid gap-4 xl:grid-cols-[1fr_340px]">

      <!-- Topology card wrapping the WebGL canvas -->
      <Card :no-padding="true">
        <template #header>
          <!-- This slot lands outside the canvas, in the Card header row -->
        </template>
        <div class="flex items-center justify-between px-4 pt-4 pb-3">
          <div>
            <h2 class="text-sm font-semibold">Fleet topology</h2>
            <p class="text-xs text-white/40">WebGL force-directed graph · {{ filteredGraphNodes.length }} of {{ graphNodes.length }} objects visible</p>
          </div>
          <Button size="sm" variant="ghost" :loading="loading" @click="reload">Refresh</Button>
        </div>

        <div v-if="graphNodes.length > 0" class="grid gap-3 px-4 pb-3 lg:grid-cols-[minmax(220px,1fr)_auto]">
          <label class="relative block">
            <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs text-white/30">⌕</span>
            <input
              v-model="objectFilter"
              type="search"
              class="h-9 w-full rounded-lg border border-border bg-black/20 pl-8 pr-3 text-sm text-foreground placeholder:text-white/30 focus:border-accent/50 focus:outline-none focus:ring-1 focus:ring-accent/30"
              placeholder="Filter objects by name, model, status, or id"
            />
          </label>
          <div class="flex flex-wrap items-center gap-1.5">
            <button
              v-for="option in objectKindOptions"
              :key="option.kind"
              :aria-pressed="visibleObjectKinds[option.kind]"
              class="h-9 rounded-lg border px-2.5 text-xs transition-colors"
              :class="visibleObjectKinds[option.kind]
                ? 'border-accent/45 bg-accent/10 text-accent'
                : 'border-border/50 bg-black/10 text-white/45 hover:border-border hover:text-white/70'"
              @click="toggleObjectKind(option.kind)"
            >
              {{ option.label }}
            </button>
            <Button v-if="activeFilterCount > 0" size="sm" variant="ghost" @click="clearFilters">Clear</Button>
          </div>
        </div>

        <!-- Loading skeletons -->
        <div v-if="loading && !fleet.state" class="px-4 pb-4">
          <Skeleton variant="block" class="h-[480px]" />
        </div>

        <!-- Empty state -->
        <div v-else-if="graphNodes.length === 0" class="px-4 pb-4">
          <EmptyState
            icon="🤖"
            title="No specialists registered"
            description="Create specialists in the classic UI, then return here to see the fleet topology."
            class="h-[480px]"
          />
        </div>

        <div v-else-if="filteredGraphNodes.length === 0" class="px-4 pb-4">
          <EmptyState
            icon="⌕"
            title="No matching objects"
            description="Adjust the search or object type filters to restore topology nodes."
            class="h-[480px]"
          />
        </div>

        <!-- WebGL Canvas -->
        <div v-else class="px-4 pb-4">
          <FleetCanvas
            :nodes="filteredGraphNodes"
            :edges="filteredGraphEdges"
            :height="480"
          />
        </div>
      </Card>

      <!-- Event feed sidebar -->
      <Card title="Event feed" description="Real-time SSE stream" class="flex flex-col">
        <template #header>
          <Badge :variant="fleet.connected ? 'success' : 'muted'" dot>
            {{ fleet.connected ? 'live' : 'offline' }}
          </Badge>
        </template>

        <div class="flex-1 overflow-y-auto" style="max-height: 540px">
          <TransitionGroup name="event-list" tag="div" class="space-y-0.5">
            <div
              v-for="(ev, idx) in recentEvents"
              :key="`${ev.kind}-${ev.at}-${idx}`"
              class="flex items-start gap-2 rounded-md px-2 py-1.5 text-xs hover:bg-white/4"
            >
              <Badge :variant="eventVariant(ev.kind)" class="mt-0.5 shrink-0 w-[56px] justify-center">
                {{ shortKind(ev.kind) }}
              </Badge>
              <div class="min-w-0 flex-1">
                <div class="truncate text-white/70">
                  {{ ev.message || ev.title || ev.agent || ev.specialist || shortId(ev.run_id ?? '') }}
                </div>
                <div v-if="ev.at" class="text-[10px] text-white/25">{{ reltime(ev.at) }}</div>
              </div>
            </div>
          </TransitionGroup>
          <EmptyState v-if="recentEvents.length === 0" icon="📡" title="Waiting for events" class="py-8 text-xs" />
        </div>
      </Card>
    </div>

    <!-- Specialist roster -->
    <Card v-if="specialists.length" title="Specialist roster" description="All registered specialists in this session">
      <div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        <div
          v-for="sp in specialists"
          :key="String(sp.name)"
          class="flex items-center gap-3 rounded-lg border border-border/50 bg-black/10 px-3 py-2.5"
        >
          <span
            class="flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-bold"
            :class="sp.paused ? 'bg-amber-500/20 text-amber-400' : 'bg-accent/20 text-accent'"
          >
            {{ String(sp.name ?? '?')[0].toUpperCase() }}
          </span>
          <div class="min-w-0">
            <div class="truncate text-xs font-medium">{{ sp.name }}</div>
            <div class="text-[10px] text-white/40">{{ sp.model || sp.provider }}</div>
          </div>
          <Badge v-if="sp.paused" variant="warning" class="ml-auto shrink-0">paused</Badge>
          <Badge v-else variant="success" dot class="ml-auto shrink-0">online</Badge>
        </div>
      </div>
    </Card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import Skeleton from "@/components/ui/Skeleton.vue";
import FleetCanvas from "@/components/fleet/FleetCanvas.vue";
import type { GraphEdge, GraphNode, NodeKind } from "@/components/fleet/types";
import { useFleetStore } from "@/stores/fleet";

const fleet = useFleetStore();
const loading = ref(false);
const objectFilter = ref("");
const objectKindOptions: Array<{ kind: NodeKind; label: string }> = [
  { kind: "orchestrator", label: "Orchestrator" },
  { kind: "specialist", label: "Specialists" },
  { kind: "team", label: "Teams" },
  { kind: "run", label: "Runs" },
];
const visibleObjectKinds = ref<Record<NodeKind, boolean>>({
  orchestrator: true,
  specialist: true,
  team: true,
  run: true,
});

// ─── Graph data derivation ────────────────────────────────────────────────────

const graphNodes = computed<GraphNode[]>(() => {
  const nodes: GraphNode[] = [];

  // Always include orchestrator as the centre node
  nodes.push({
    id: "orchestrator",
    label: "Orchestrator",
    kind: "orchestrator",
    status: "online",
  });

  // Specialists
  for (const sp of fleet.state?.specialists ?? []) {
    nodes.push({
      id: `sp:${sp.name}`,
      label: String(sp.name ?? "?"),
      kind: "specialist",
      status: sp.paused ? "paused" : "online",
      sublabel: String(sp.model ?? sp.provider ?? ""),
    });
  }

  // Teams
  for (const team of fleet.state?.teams ?? []) {
    nodes.push({
      id: `team:${team.name}`,
      label: String(team.name ?? "?"),
      kind: "team",
      status: "online",
      sublabel: `${(team.members as string[] | undefined)?.length ?? 0} members`,
    });
  }

  // Active and recently-finished runs
  for (const run of fleet.runs) {
    if (run.status === "running" || run.status === "failed") {
      nodes.push({
        id: run.id,
        label: run.id.length > 10 ? run.id.slice(0, 8) + "…" : run.id,
        kind: "run",
        status: run.status as GraphNode["status"],
        sublabel: run.prompt.length > 30 ? run.prompt.slice(0, 28) + "…" : run.prompt,
      });
    }
  }

  return nodes;
});

const graphEdges = computed<GraphEdge[]>(() => {
  const edges: GraphEdge[] = [];
  const nodeIds = new Set(graphNodes.value.map((n) => n.id));

  // Orchestrator → each specialist (membership)
  for (const sp of fleet.state?.specialists ?? []) {
    edges.push({
      source: "orchestrator",
      target: `sp:${sp.name}`,
      kind: "membership",
    });
  }

  // Orchestrator → each team (membership)
  for (const team of fleet.state?.teams ?? []) {
    edges.push({
      source: "orchestrator",
      target: `team:${team.name}`,
      kind: "membership",
    });
    // Team → member specialists
    for (const member of (team.members as string[] | undefined) ?? []) {
      if (nodeIds.has(`sp:${member}`)) {
        edges.push({
          source: `team:${team.name}`,
          target: `sp:${member}`,
          kind: "membership",
        });
      }
    }
  }

  // Active delegation edges from fleet state
  for (const edge of fleet.state?.active_delegation_edges ?? []) {
    const agentId = `sp:${edge.agent}`;
    const runId = String((edge as any).run_id ?? (edge as any).call_id ?? "");

    const source = nodeIds.has(agentId) ? agentId : "orchestrator";
    const target = nodeIds.has(runId) ? runId : null;

    if (target) {
      edges.push({
        source,
        target,
        kind: "delegation",
        animated: true,
        label: String((edge as any).agent ?? ""),
      });
    }
  }

  // Orchestrator → running runs (when no explicit delegation edge exists)
  const delegatedRunIds = new Set(
    edges.filter((e) => e.kind === "delegation").map((e) => e.target)
  );
  for (const run of fleet.runs) {
    if (run.status === "running" && nodeIds.has(run.id) && !delegatedRunIds.has(run.id)) {
      edges.push({
        source: "orchestrator",
        target: run.id,
        kind: "active",
        animated: true,
      });
    }
  }

  return edges;
});

const normalizedObjectFilter = computed(() => objectFilter.value.trim().toLowerCase());

const activeFilterCount = computed(() => {
  const hiddenKinds = objectKindOptions.filter((option) => !visibleObjectKinds.value[option.kind]).length;
  return hiddenKinds + (normalizedObjectFilter.value ? 1 : 0);
});

const filteredGraphNodes = computed<GraphNode[]>(() => {
  const query = normalizedObjectFilter.value;
  return graphNodes.value.filter((node) => {
    if (!visibleObjectKinds.value[node.kind]) return false;
    if (!query) return true;
    return searchableNodeText(node).includes(query);
  });
});

const filteredGraphEdges = computed<GraphEdge[]>(() => {
  const visibleIDs = new Set(filteredGraphNodes.value.map((node) => node.id));
  return graphEdges.value.filter((edge) => visibleIDs.has(edge.source) && visibleIDs.has(edge.target));
});

// ─── Computed helpers ─────────────────────────────────────────────────────────

const specialists = computed(() => fleet.state?.specialists ?? []);
const runningRuns = computed(() => fleet.runs.filter((r) => r.status === "running"));
const recentEvents = computed(() => [...fleet.events].reverse().slice(0, 50));

// ─── Lifecycle ────────────────────────────────────────────────────────────────

async function reload() {
  loading.value = true;
  try { await fleet.refresh(); } finally { loading.value = false; }
}

function searchableNodeText(node: GraphNode) {
  return [
    node.id,
    node.label,
    node.sublabel ?? "",
    node.kind,
    node.status,
  ].join(" ").toLowerCase();
}

function toggleObjectKind(kind: NodeKind) {
  visibleObjectKinds.value = {
    ...visibleObjectKinds.value,
    [kind]: !visibleObjectKinds.value[kind],
  };
}

function clearFilters() {
  objectFilter.value = "";
  visibleObjectKinds.value = {
    orchestrator: true,
    specialist: true,
    team: true,
    run: true,
  };
}

onMounted(async () => {
  loading.value = true;
  try { await fleet.refresh(); } finally { loading.value = false; }
  fleet.start();
});

// Auto-refresh when SSE offline
let pollTimer: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  pollTimer = setInterval(() => {
    if (!fleet.connected) fleet.refresh().catch(() => {});
  }, 10_000);
});
onUnmounted(() => {
  fleet.stop();
  if (pollTimer) clearInterval(pollTimer);
});

// ─── Formatting helpers ───────────────────────────────────────────────────────

function eventVariant(kind: string): "success" | "danger" | "info" | "warning" | "accent" | "muted" {
  if (kind === "run_started") return "info";
  if (kind === "run_finished") return "success";
  if (kind === "run_failed" || kind === "error") return "danger";
  if (kind === "input_request") return "warning";
  if (kind === "delegation") return "accent";
  return "muted";
}

function shortKind(kind: string) {
  const m: Record<string, string> = {
    run_started: "▶ start",
    run_finished: "✓ done",
    run_failed: "✗ fail",
    tool_start: "↑ tool",
    tool_result: "↓ tool",
    delegation: "→ agent",
    input_request: "? input",
    input_answered: "✓ input",
    error: "⚡ err",
  };
  return m[kind] ?? kind.replace(/_/g, " ");
}

function shortId(id: string) {
  if (!id) return "—";
  return id.length > 10 ? id.slice(0, 6) + "…" + id.slice(-3) : id;
}

function reltime(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return `${Math.round(diff / 3_600_000)}h ago`;
}
</script>

<style scoped>
.event-list-enter-active { transition: all 200ms ease; }
.event-list-enter-from  { opacity: 0; transform: translateY(-4px); }
</style>
