<template>
  <section class="flex min-h-0 min-w-0 flex-1 flex-col gap-4 overflow-hidden">
    <header
      class="halo-surface flex shrink-0 flex-wrap items-center gap-3 rounded-md !px-4 !py-3"
    >
      <div class="min-w-0 flex-1 basis-64">
        <h2 class="text-lg font-semibold leading-tight text-foreground">
          Memory Command Center
        </h2>
        <p class="text-xs text-faint-foreground">
          Unified health, graph memory, retrieval explainability, and guarded
          maintenance controls.
        </p>
      </div>
      <div
        class="flex min-w-0 flex-1 flex-wrap items-center justify-start gap-2 sm:justify-end"
      >
        <input
          v-model="tenantFilter"
          type="search"
          placeholder="Tenant"
          class="toolbar-input w-full sm:w-36"
        />
        <input
          v-model="sessionFilter"
          type="search"
          placeholder="Session"
          class="toolbar-input w-full sm:w-40"
        />
        <DropdownSelect
          v-model="graphTypeFilter"
          size="sm"
          class="w-full text-xs sm:w-36"
          :options="graphTypeOptions"
        />
        <button type="button" class="toolbar-btn" @click="refreshAll">
          Refresh
        </button>
      </div>
    </header>

    <div class="grid shrink-0 grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-6">
      <KpiTile
        label="Health"
        :value="healthLabel"
        :detail="healthDetail"
        :tone="healthTone"
      />
      <KpiTile
        label="Searches"
        :value="formatNumber(overview?.totals?.searches ?? 0)"
        :detail="`${formatDecimal(overview?.totals?.avgHitsPerSearch ?? 0)} hits/search`"
      />
      <KpiTile
        label="Search avg"
        :value="`${formatDecimal(overview?.latency?.avgMs ?? 0)} ms`"
        detail="Evolving memory latency"
      />
      <KpiTile
        label="Writes"
        :value="formatNumber(overview?.totals?.evolves ?? 0)"
        :detail="`${formatNumber(overview?.totals?.evolveErrors ?? 0)} errors`"
      />
      <KpiTile
        label="MAGMA queue"
        :value="formatNumber(overview?.magma?.queueDepth ?? 0)"
        :detail="`${formatNumber(overview?.magma?.failedTotal ?? 0)} failed · ${formatNumber(overview?.magma?.droppedTotal ?? 0)} dropped`"
        :tone="overview?.magma?.lastError ? 'danger' : 'default'"
      />
      <KpiTile
        label="Graph"
        :value="`${formatNumber(graphStats.nodes)} / ${formatNumber(graphStats.edges)}`"
        :detail="`${formatNumber(graphStats.reviewEdges)} review edges`"
        :tone="graphStats.reviewEdges > 0 ? 'warning' : 'default'"
      />
    </div>

    <div class="grid min-h-0 flex-1 gap-4 xl:grid-cols-[minmax(0,1.4fr)_24rem]">
      <section
        class="memory-graph-card halo-surface flex min-h-0 min-w-0 flex-col overflow-hidden rounded-md !p-0"
      >
        <div
          class="flex shrink-0 flex-col gap-3 border-b border-border/60 px-4 py-3 sm:flex-row sm:items-center sm:justify-between"
        >
          <div>
            <h3 class="text-sm font-semibold text-foreground">
              MAGMA Graph Memory
            </h3>
            <p class="text-xs text-faint-foreground">
              {{ graphData?.nodes?.length ?? 0 }} nodes ·
              {{ graphData?.edges?.length ?? 0 }} edges
            </p>
          </div>
          <input
            v-model="graphQuery"
            type="search"
            placeholder="Search graph"
            class="toolbar-input w-full sm:w-48"
            @keyup.enter.prevent="refreshGraph"
          />
        </div>

        <div
          v-if="graphLoading"
          class="flex flex-1 items-center justify-center text-sm text-faint-foreground"
        >
          Loading graph…
        </div>
        <div
          v-else-if="!flowNodes.length"
          class="flex flex-1 items-center justify-center border-t border-border/40 text-sm text-faint-foreground"
        >
          No graph memory nodes match the current filters.
        </div>
        <VueFlow
          v-else
          class="memory-flow min-h-0 flex-1"
          :nodes="flowNodes"
          :edges="flowEdges"
          :fit-view-on-init="true"
          :min-zoom="0.2"
          :max-zoom="1.8"
          @node-click="onNodeClick"
          @edge-click="onEdgeClick"
          @pane-click="clearSelection"
        >
          <Background
            pattern-color="rgb(var(--color-border) / 0.35)"
            :gap="22"
          />
          <MiniMap
            pannable
            zoomable
            class="!bg-surface/95 !border !border-border"
            mask-color="rgb(var(--color-background) / 0.45)"
          />
        </VueFlow>
      </section>

      <aside
        class="memory-side-stack flex min-h-0 flex-col gap-4 overflow-hidden"
      >
        <section
          class="memory-inspector-card halo-surface flex min-h-0 flex-1 flex-col overflow-hidden rounded-md !p-4"
        >
          <div class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <h3 class="shrink-0 text-sm font-semibold text-foreground">
              Inspector
            </h3>
            <div
              v-if="selectedNode"
              class="memory-inspector-body mt-3 min-h-0 flex-1 space-y-3 overflow-y-auto text-xs"
            >
              <PillLabel :label="selectedNode.type" />
              <h4 class="break-words text-sm font-semibold text-foreground">
                {{ selectedNode.label }}
              </h4>
              <p
                v-if="selectedNode.text"
                class="whitespace-pre-wrap text-subtle-foreground"
              >
                {{ selectedNode.text }}
              </p>
              <dl class="grid grid-cols-2 gap-2 text-faint-foreground">
                <InfoItem label="Tenant" :value="selectedNode.tenant || '—'" />
                <InfoItem
                  label="Session"
                  :value="selectedNode.session || '—'"
                />
                <InfoItem
                  label="Created"
                  :value="formatDate(selectedNode.createdAt)"
                />
                <InfoItem label="ID" :value="selectedNode.id" />
              </dl>
            </div>
            <div
              v-else-if="selectedEdge"
              class="memory-inspector-body mt-3 min-h-0 flex-1 space-y-3 overflow-y-auto text-xs"
            >
              <PillLabel :label="selectedEdge.graphType" />
              <h4 class="break-words text-sm font-semibold text-foreground">
                {{ selectedEdge.rel }}
              </h4>
              <dl class="grid grid-cols-2 gap-2 text-faint-foreground">
                <InfoItem label="Source" :value="selectedEdge.source" />
                <InfoItem label="Target" :value="selectedEdge.target" />
                <InfoItem
                  label="Weight"
                  :value="formatDecimal(selectedEdge.weight ?? 0)"
                />
                <InfoItem
                  label="Confidence"
                  :value="formatDecimal(selectedEdge.confidence ?? 0)"
                />
                <InfoItem
                  label="Review"
                  :value="selectedEdge.reviewState || 'approved'"
                />
                <InfoItem label="Reason" :value="selectedEdge.reason || '—'" />
              </dl>
              <div class="flex flex-wrap gap-2">
                <button
                  type="button"
                  class="toolbar-btn toolbar-btn-accent"
                  :disabled="actionBusy"
                  @click="approveSelectedEdge"
                >
                  Approve
                </button>
                <button
                  type="button"
                  class="toolbar-btn toolbar-btn-danger"
                  :disabled="actionBusy"
                  @click="retractSelectedEdge"
                >
                  Retract
                </button>
              </div>
            </div>
            <div
              v-else
              class="memory-inspector-body mt-3 min-h-0 flex-1 overflow-y-auto rounded-md border border-dashed border-border bg-surface-muted/40 p-4 text-sm text-faint-foreground"
            >
              Select a graph node or edge to inspect its details and available
              actions.
            </div>
          </div>
        </section>

        <section class="halo-surface shrink-0 rounded-md !p-4">
          <h3 class="text-sm font-semibold text-foreground">
            Operator Controls
          </h3>
          <div class="mt-3 grid grid-cols-2 gap-2">
            <button
              type="button"
              class="toolbar-btn"
              :disabled="actionBusy"
              @click="dryRunPrune"
            >
              Prune dry-run
            </button>
            <button
              type="button"
              class="toolbar-btn toolbar-btn-danger"
              :disabled="actionBusy"
              @click="runPrune"
            >
              Prune
            </button>
            <button
              type="button"
              class="toolbar-btn"
              :disabled="actionBusy"
              @click="drainQueue"
            >
              Drain queue
            </button>
            <button
              type="button"
              class="toolbar-btn"
              :disabled="actionBusy || !sessionFilter"
              @click="rebuildEmbeddings"
            >
              Rebuild embeddings
            </button>
          </div>
          <p v-if="actionMessage" class="mt-3 text-xs text-subtle-foreground">
            {{ actionMessage }}
          </p>
        </section>
      </aside>
    </div>

    <div
      class="grid min-h-[260px] shrink-0 gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_24rem]"
    >
      <section
        class="memory-scroll-card halo-surface flex min-h-0 flex-col overflow-hidden rounded-md !p-4"
      >
        <div class="flex items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-foreground">
              Retrieval Explain
            </h3>
            <p class="text-xs text-faint-foreground">
              Query to anchors, graph views, and injected context.
            </p>
          </div>
          <button
            type="button"
            class="toolbar-btn"
            :disabled="!explainQuery.trim()"
            @click="runExplain"
          >
            Explain
          </button>
        </div>
        <input
          v-model="explainQuery"
          type="search"
          placeholder="Ask a memory retrieval question"
          class="toolbar-input mt-3 w-full"
          @keyup.enter.prevent="runExplain"
        />
        <div
          class="mt-3 min-h-0 flex-1 overflow-y-auto rounded-md border border-border/60 bg-surface-muted/40 p-3 text-xs"
        >
          <div v-if="explainLoading" class="text-faint-foreground">
            Explaining retrieval…
          </div>
          <div v-else-if="explainData" class="space-y-3">
            <div class="flex flex-wrap gap-2">
              <PillLabel :label="`intent ${explainData.intent}`" />
              <PillLabel :label="`${explainData.anchorCount} anchors`" />
              <PillLabel :label="`${explainData.events.length} events`" />
            </div>
            <p class="whitespace-pre-wrap text-subtle-foreground">
              {{ explainData.context || "No context returned." }}
            </p>
          </div>
          <div v-else class="text-faint-foreground">
            Run a query to see MAGMA retrieval decisions.
          </div>
        </div>
      </section>

      <section
        class="memory-scroll-card halo-surface flex min-h-0 flex-col overflow-hidden rounded-md !p-4"
      >
        <h3 class="text-sm font-semibold text-foreground">Memory Timeline</h3>
        <div class="mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto pr-1">
          <div
            v-for="item in timelineItems"
            :key="item.id + item.time"
            class="rounded-md border border-border/60 bg-surface-muted/30 px-3 py-2 text-xs"
          >
            <div class="flex items-center justify-between gap-2">
              <span class="font-medium text-foreground">{{ item.title }}</span>
              <span
                class="shrink-0 text-[10px] uppercase tracking-wide text-faint-foreground"
                >{{ item.lane }}</span
              >
            </div>
            <p
              v-if="item.detail"
              class="mt-1 line-clamp-2 text-subtle-foreground"
            >
              {{ item.detail }}
            </p>
            <p class="mt-1 text-[10px] text-faint-foreground">
              {{ formatDate(item.time) }}
            </p>
          </div>
          <p v-if="!timelineItems.length" class="text-sm text-faint-foreground">
            No memory timeline events match the current filters.
          </p>
        </div>
      </section>

      <section
        class="memory-scroll-card halo-surface flex min-h-0 flex-col overflow-hidden rounded-md !p-4"
      >
        <h3 class="text-sm font-semibold text-foreground">Memory Lanes</h3>
        <div class="mt-3 max-h-36 space-y-3 overflow-y-auto pr-1">
          <div
            v-for="lane in overview?.lanes ?? []"
            :key="lane.id"
            class="rounded-md border border-border/60 bg-surface-muted/30 px-3 py-2 text-xs"
          >
            <div class="flex items-center justify-between gap-3">
              <span class="font-medium text-foreground">{{ lane.label }}</span>
              <span :class="['lane-status', lane.status]">{{
                lane.status
              }}</span>
            </div>
            <p class="mt-1 text-faint-foreground">{{ lane.detail }}</p>
          </div>
        </div>
        <div
          v-if="reviewEdges.length"
          class="mt-4 min-h-0 flex-1 overflow-y-auto pr-1"
        >
          <h4
            class="font-mono text-[11px] uppercase tracking-[0.12em] text-faint-foreground"
          >
            Review edges
          </h4>
          <button
            v-for="edge in reviewEdges.slice(0, 6)"
            :key="edge.id"
            type="button"
            class="mt-2 block w-full rounded-md border border-warning/30 bg-warning/10 px-3 py-2 text-left text-xs text-foreground"
            @click="selectReviewEdge(edge)"
          >
            {{ edge.graphType }} · {{ edge.rel }}
          </button>
        </div>
      </section>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, toRef } from "vue";
import { useQuery } from "@tanstack/vue-query";
import dagre from "dagre";
import { MarkerType, VueFlow, type Edge, type Node } from "@vue-flow/core";
import { Background } from "@vue-flow/background";
import { MiniMap } from "@vue-flow/minimap";
import DropdownSelect from "@/components/DropdownSelect.vue";
import type { MetricsTimeRangeValue } from "@/composables/observability/useTokenMetrics";
import {
  approveMagmaEdge,
  drainMagmaConsolidation,
  fetchMemoryObservabilityExplain,
  fetchMemoryObservabilityGraph,
  fetchMemoryObservabilityOverview,
  fetchMemoryObservabilityReviewEdges,
  fetchMemoryObservabilityTimeline,
  pruneMagmaMemory,
  rebuildEvolvingMemoryEmbeddings,
  retractMagmaEdge,
  type MagmaEdgeSelector,
  type MemoryObservabilityEdge,
  type MemoryObservabilityGraph,
  type MemoryObservabilityNode,
  type MemoryObservabilityOverview,
} from "@/api/memoryObservability";

const props = defineProps<{
  timeRange: MetricsTimeRangeValue;
}>();

const selectedRange = toRef(props, "timeRange");
const tenantFilter = ref("");
const sessionFilter = ref("");
const graphTypeFilter = ref("");
const graphQuery = ref("");
const explainQuery = ref("");
const selectedNode = ref<MemoryObservabilityNode | null>(null);
const selectedEdge = ref<MemoryObservabilityEdge | null>(null);
const actionBusy = ref(false);
const actionMessage = ref("");
const explainData = ref<Awaited<
  ReturnType<typeof fetchMemoryObservabilityExplain>
> | null>(null);
const explainLoading = ref(false);

const graphTypeOptions = [
  { id: "all", label: "All graphs", value: "" },
  { id: "semantic", label: "Semantic", value: "semantic" },
  { id: "temporal", label: "Temporal", value: "temporal" },
  { id: "causal", label: "Causal", value: "causal" },
  { id: "entity", label: "Entity", value: "entity" },
];

const queryParams = computed(() => ({
  window: selectedRange.value,
  tenant: tenantFilter.value || undefined,
  sessionId: sessionFilter.value || undefined,
  graphType: graphTypeFilter.value || undefined,
  q: graphQuery.value || undefined,
}));

const overviewQuery = useQuery({
  queryKey: [
    "memory-observability-overview",
    selectedRange,
    tenantFilter,
    sessionFilter,
  ],
  queryFn: () => fetchMemoryObservabilityOverview(queryParams.value),
  refetchInterval: 15_000,
});

const graphQueryResult = useQuery({
  queryKey: [
    "memory-observability-graph",
    tenantFilter,
    sessionFilter,
    graphTypeFilter,
    graphQuery,
  ],
  queryFn: () => fetchMemoryObservabilityGraph(queryParams.value),
  refetchInterval: 20_000,
});

const timelineQuery = useQuery({
  queryKey: ["memory-observability-timeline", tenantFilter, sessionFilter],
  queryFn: () => fetchMemoryObservabilityTimeline(queryParams.value),
  refetchInterval: 20_000,
});

const reviewQuery = useQuery({
  queryKey: ["memory-observability-review-edges", tenantFilter, sessionFilter],
  queryFn: () => fetchMemoryObservabilityReviewEdges(queryParams.value),
  refetchInterval: 20_000,
});

const overview = computed(() => {
  const data = overviewQuery.data.value;
  return isOverviewResponse(data) ? data : null;
});
const graphData = computed(() => {
  const data = graphQueryResult.data.value;
  return isGraphResponse(data) ? data : null;
});
const graphLoading = computed(() => graphQueryResult.isLoading.value);
const graphStats = computed(
  () =>
    overview.value?.graph ??
    graphData.value?.graph ?? {
      nodes: 0,
      edges: 0,
      events: 0,
      entities: 0,
      reviewEdges: 0,
    },
);
const timelineItems = computed(() => {
  const items = timelineQuery.data.value?.items;
  return Array.isArray(items) ? items : [];
});
const reviewEdges = computed(() => {
  const edges = reviewQuery.data.value?.edges;
  return Array.isArray(edges) ? edges : [];
});

const flowNodes = computed<Node[]>(() =>
  layoutNodes(graphData.value?.nodes ?? [], graphData.value?.edges ?? []),
);
const flowEdges = computed<Edge[]>(() =>
  (graphData.value?.edges ?? []).map(toFlowEdge),
);

const healthLabel = computed(() => {
  if (!overview.value?.config?.memoryEnabled) return "Disabled";
  if (overview.value.magma?.lastError) return "Attention";
  if ((overview.value.magma?.queueDepth ?? 0) > 0) return "Working";
  return "Healthy";
});
const healthTone = computed(() => {
  if (!overview.value?.config?.memoryEnabled) return "muted";
  if (overview.value.magma?.lastError) return "danger";
  if (
    (overview.value.graph?.reviewEdges ?? 0) > 0 ||
    (overview.value.magma?.queueDepth ?? 0) > 0
  )
    return "warning";
  return "success";
});
const healthDetail = computed(
  () =>
    overview.value?.magma?.lastError ||
    overview.value?.source ||
    "No telemetry",
);

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

function isOverviewResponse(
  value: unknown,
): value is MemoryObservabilityOverview {
  return isRecord(value) && isRecord(value.config) && isRecord(value.totals);
}

function isGraphResponse(value: unknown): value is MemoryObservabilityGraph {
  return (
    isRecord(value) && Array.isArray(value.nodes) && Array.isArray(value.edges)
  );
}

function layoutNodes(
  nodes: MemoryObservabilityNode[],
  edges: MemoryObservabilityEdge[],
): Node[] {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: "LR",
    nodesep: 60,
    ranksep: 100,
    marginx: 40,
    marginy: 40,
  });
  for (const node of nodes) graph.setNode(node.id, { width: 180, height: 64 });
  for (const edge of edges) graph.setEdge(edge.source, edge.target);
  try {
    dagre.layout(graph);
  } catch {
    // Fall through to deterministic grid positions.
  }
  return nodes.map((node, index) => {
    const pos = graph.node(node.id) as { x: number; y: number } | undefined;
    return {
      id: node.id,
      type: "default",
      position: pos
        ? { x: pos.x - 90, y: pos.y - 32 }
        : { x: (index % 4) * 220, y: Math.floor(index / 4) * 120 },
      label: node.label,
      data: { node },
      class: `memory-node memory-node-${node.type}`,
      style: nodeStyle(node.type),
    };
  });
}

function toFlowEdge(edge: MemoryObservabilityEdge): Edge {
  const reviewNeeded = edge.reviewState && edge.reviewState !== "approved";
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    label: edge.rel,
    type: "smoothstep",
    animated: Boolean(reviewNeeded),
    data: { edge },
    markerEnd: { type: MarkerType.ArrowClosed },
    style: {
      stroke: edgeColor(edge.graphType),
      strokeWidth: 1.5 + Math.min(3, edge.weight || edge.confidence || 0),
      strokeDasharray: reviewNeeded ? "6 4" : undefined,
    },
    labelStyle: { fill: "rgb(var(--color-faint-foreground))", fontSize: 10 },
  };
}

function nodeStyle(type: string) {
  const color =
    type === "entity"
      ? "var(--color-warning)"
      : type === "event"
        ? "var(--data)"
        : "var(--color-accent)";
  return {
    border: `1px solid rgb(${color} / 0.45)`,
    background: `rgb(${color} / 0.10)`,
    color: "rgb(var(--color-foreground))",
    borderRadius: "8px",
    fontSize: "12px",
    width: "180px",
  };
}

function edgeColor(type: string) {
  switch (type) {
    case "causal":
      return "rgb(var(--color-danger))";
    case "temporal":
      return "rgb(var(--color-warning))";
    case "entity":
      return "rgb(var(--color-accent))";
    default:
      return "rgb(var(--data))";
  }
}

function selectReviewEdge(edge: MemoryObservabilityEdge) {
  selectedEdge.value = edge;
  selectedNode.value = null;
}

function onNodeClick(event: { node: Node }) {
  selectedNode.value = event.node.data?.node as MemoryObservabilityNode;
  selectedEdge.value = null;
}

function onEdgeClick(event: { edge: Edge }) {
  selectedEdge.value = event.edge.data?.edge as MemoryObservabilityEdge;
  selectedNode.value = null;
}

function clearSelection() {
  selectedNode.value = null;
  selectedEdge.value = null;
}

async function refreshAll() {
  await Promise.all([
    overviewQuery.refetch(),
    graphQueryResult.refetch(),
    timelineQuery.refetch(),
    reviewQuery.refetch(),
  ]);
}

async function refreshGraph() {
  await graphQueryResult.refetch();
}

async function runExplain() {
  const q = explainQuery.value.trim();
  if (!q) return;
  explainLoading.value = true;
  try {
    explainData.value = await fetchMemoryObservabilityExplain({
      q,
      tenant: tenantFilter.value || undefined,
      maxNodes: 12,
      maxHops: 2,
    });
  } catch (err: any) {
    actionMessage.value = err?.message || "Retrieval explain failed.";
  } finally {
    explainLoading.value = false;
  }
}

async function runAction(
  label: string,
  fn: () => Promise<{ message: string }>,
) {
  actionBusy.value = true;
  actionMessage.value = `${label}…`;
  try {
    const result = await fn();
    actionMessage.value = result.message || `${label} complete.`;
    await refreshAll();
  } catch (err: any) {
    actionMessage.value =
      err?.response?.data?.error || err?.message || `${label} failed.`;
  } finally {
    actionBusy.value = false;
  }
}

async function dryRunPrune() {
  await runAction("Prune dry-run", () => pruneMagmaMemory({ dryRun: true }));
}

async function runPrune() {
  if (
    !window.confirm("Run MAGMA lifecycle pruning with the configured policy?")
  )
    return;
  await runAction("Prune", () => pruneMagmaMemory({ dryRun: false }));
}

async function drainQueue() {
  if (!window.confirm("Drain up to 25 queued MAGMA consolidation events now?"))
    return;
  await runAction("Drain queue", () => drainMagmaConsolidation(25));
}

async function rebuildEmbeddings() {
  if (!sessionFilter.value.trim()) return;
  if (
    !window.confirm(
      `Rebuild evolving memory embeddings for session ${sessionFilter.value}?`,
    )
  )
    return;
  await runAction("Rebuild embeddings", () =>
    rebuildEvolvingMemoryEmbeddings({ sessionId: sessionFilter.value.trim() }),
  );
}

function selectedEdgeSelector(): MagmaEdgeSelector | null {
  if (!selectedEdge.value) return null;
  return {
    source: selectedEdge.value.source,
    graphType: selectedEdge.value.graphType,
    rel: selectedEdge.value.rel,
    target: selectedEdge.value.target,
  };
}

async function approveSelectedEdge() {
  const selector = selectedEdgeSelector();
  if (!selector) return;
  await runAction("Approve edge", () =>
    approveMagmaEdge({ selector, reviewer: "ui" }),
  );
}

async function retractSelectedEdge() {
  const selector = selectedEdgeSelector();
  if (!selector) return;
  const reason =
    window.prompt("Reason for retracting this edge?", "operator_retraction") ||
    "";
  if (!reason.trim()) return;
  await runAction("Retract edge", () => retractMagmaEdge({ selector, reason }));
  selectedEdge.value = null;
}

function formatNumber(value: number) {
  return Number(value || 0).toLocaleString();
}

function formatDecimal(value: number) {
  return Number(value || 0).toFixed(value >= 10 ? 0 : 2);
}

function formatDate(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString([], {
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

const KpiTile = defineComponent({
  name: "KpiTile",
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    detail: { type: String, required: true },
    tone: { type: String, default: "default" },
  },
  setup(props) {
    return () =>
      h("div", { class: ["kpi-tile", props.tone] }, [
        h("p", { class: "kpi-label" }, props.label),
        h("p", { class: "kpi-value" }, props.value),
        h("p", { class: "kpi-detail" }, props.detail),
      ]);
  },
});

const InfoItem = defineComponent({
  name: "InfoItem",
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(props) {
    return () =>
      h(
        "div",
        {
          class:
            "min-w-0 rounded border border-border/50 bg-surface-muted/30 p-2",
        },
        [
          h(
            "dt",
            {
              class:
                "font-mono text-[10px] uppercase tracking-wide text-faint-foreground",
            },
            props.label,
          ),
          h("dd", { class: "mt-1 break-words text-foreground" }, props.value),
        ],
      );
  },
});

const PillLabel = defineComponent({
  name: "PillLabel",
  props: {
    label: { type: String, required: true },
  },
  setup(props) {
    return () =>
      h(
        "span",
        {
          class:
            "inline-flex rounded-full border border-border/70 bg-surface-muted/60 px-2 py-0.5 font-mono text-[10px] uppercase tracking-wide text-faint-foreground",
        },
        props.label,
      );
  },
});
</script>

<style scoped>
.toolbar-input {
  min-height: 2rem;
  border-radius: 0.375rem;
  border: 1px solid rgb(var(--color-border));
  background: rgb(var(--color-surface));
  padding: 0.375rem 0.625rem;
  font-size: 0.75rem;
  color: rgb(var(--color-foreground));
}

.toolbar-btn {
  min-height: 2rem;
  border-radius: 0.375rem;
  border: 1px solid rgb(var(--color-border) / 0.75);
  background: rgb(var(--color-surface-muted) / 0.35);
  padding: 0.375rem 0.625rem;
  font-size: 0.75rem;
  font-weight: 600;
  color: rgb(var(--color-foreground));
  transition:
    background-color 140ms ease,
    border-color 140ms ease,
    opacity 140ms ease;
}

.toolbar-btn:hover:not(:disabled) {
  background: rgb(var(--color-surface-muted) / 0.75);
}

.toolbar-btn:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.toolbar-btn-accent {
  border-color: rgb(var(--color-accent) / 0.45);
  background: rgb(var(--color-accent) / 0.14);
}

.toolbar-btn-danger {
  border-color: rgb(var(--color-danger) / 0.45);
  background: rgb(var(--color-danger) / 0.12);
}

.memory-graph-card {
  height: clamp(28rem, 52vh, 42rem);
}

.memory-side-stack {
  height: clamp(28rem, 52vh, 42rem);
}

.memory-inspector-card {
  max-height: calc(100% - 8.75rem);
}

.memory-inspector-body {
  overscroll-behavior: contain;
  padding-right: 0.25rem;
}

.memory-scroll-card {
  height: clamp(20rem, 38vh, 24rem);
}

.kpi-tile {
  min-width: 0;
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.65);
  background: rgb(var(--color-surface));
  padding: 0.75rem;
}

.kpi-label {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 0.625rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: rgb(var(--color-faint-foreground));
}

.kpi-value {
  margin-top: 0.375rem;
  font-size: 1.25rem;
  font-weight: 700;
  line-height: 1;
  color: rgb(var(--color-foreground));
}

.kpi-detail {
  margin-top: 0.375rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.6875rem;
  color: rgb(var(--color-faint-foreground));
}

.kpi-tile.success .kpi-value {
  color: rgb(var(--color-success));
}

.kpi-tile.warning .kpi-value {
  color: rgb(var(--color-warning));
}

.kpi-tile.danger .kpi-value {
  color: rgb(var(--color-danger));
}

.kpi-tile.muted .kpi-value {
  color: rgb(var(--color-faint-foreground));
}

.lane-status {
  border-radius: 999px;
  border: 1px solid rgb(var(--color-border) / 0.6);
  padding: 0.125rem 0.45rem;
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 0.625rem;
  text-transform: uppercase;
  color: rgb(var(--color-faint-foreground));
}

.lane-status.online {
  border-color: rgb(var(--color-success) / 0.4);
  color: rgb(var(--color-success));
}

.lane-status.working,
.lane-status.attention {
  border-color: rgb(var(--color-warning) / 0.4);
  color: rgb(var(--color-warning));
}

.memory-flow {
  background: rgb(var(--color-surface-muted) / 0.22);
}

:deep(.vue-flow__node) {
  box-shadow: none;
}

:deep(.vue-flow__edge-textbg) {
  fill: rgb(var(--color-surface));
}
</style>
