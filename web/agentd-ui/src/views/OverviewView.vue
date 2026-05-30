<template>
  <section class="flex min-h-full flex-col gap-4">
    <header class="flex items-start justify-between gap-4">
      <div>
        <h2 class="font-display text-2xl leading-tight text-foreground">
          Situation Room
        </h2>
        <p class="mt-1 text-sm text-muted-foreground">
          Agents, throughput, recent work, and queue operations.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <label
          v-if="overviewMode === 'customize'"
          class="flex items-center gap-2 text-xs text-foreground"
        >
          <span>Time Range</span>
          <DropdownSelect
            v-model="dashboardTimeRange"
            size="sm"
            class="text-xs"
            :options="timeRangeDropdownOptions"
          />
        </label>
        <MSegmented
          v-model="overviewMode"
          :options="[
            { value: 'customize', label: 'Customize' },
            { value: 'queue-ops', label: 'Queue Ops' },
          ]"
        />
      </div>
    </header>

    <template v-if="overviewMode === 'customize'">
      <div
        class="halo-surface grid min-w-0 grid-cols-4 overflow-hidden px-0 py-2"
      >
        <div
          v-for="stat in overviewStats"
          :key="stat.label"
          class="min-w-0 border-l border-[rgb(var(--color-border))] px-4 py-1 first:border-l-0"
        >
          <p
            class="truncate font-mono text-[10px] uppercase tracking-[0.12em] text-faint-foreground"
          >
            {{ stat.label }}
          </p>
          <p
            :class="[
              'mt-1 font-display text-2xl font-semibold leading-none tabular-nums',
              stat.data ? 'text-[rgb(var(--data))]' : 'text-foreground',
            ]"
          >
            {{ stat.value }}
          </p>
        </div>
      </div>
      <DashboardGrid
        :layout="dashboardLayout"
        storage-key="overview-dashboard-layout"
        @layout-change="onLayoutChange"
      >
        <template #item-tokens>
          <TokenUsagePanel :time-range="dashboardTimeRange" />
        </template>

        <template #item-traces>
          <TracesPanel :time-range="dashboardTimeRange" />
        </template>

        <template #item-memory>
          <MemoryPanel />
        </template>

        <template #item-memory-metrics>
          <MemoryMetricsPanel :time-range="dashboardTimeRange" />
        </template>

        <template #item-logs>
          <LogsPanel
            :time-range="dashboardTimeRange"
            :selected-log-id="selectedLogId"
            @select-log="openLogDetail"
          />
        </template>

        <template #item-agents>
          <AgentsPanel :agents="agents" />
        </template>

        <template #item-runs>
          <RecentRunsPanel :runs="recentRuns" />
        </template>

        <template #item-throughput>
          <section class="flex h-full flex-col p-4 text-foreground">
            <header class="mb-2 flex items-start justify-between gap-4">
              <div class="min-w-0">
                <p
                  class="font-mono text-[10px] uppercase tracking-[0.18em] text-faint-foreground"
                >
                  Runs
                </p>
                <h2 class="font-display text-lg leading-tight text-foreground">
                  Throughput
                </h2>
                <p class="mt-1 text-xs leading-snug text-muted-foreground">
                  Rolling hourly buckets for runs started in the last 8 hours.
                </p>
              </div>
              <dl class="grid shrink-0 grid-cols-3 gap-3 text-right">
                <div>
                  <dt
                    class="font-mono text-[9px] uppercase tracking-[0.14em] text-faint-foreground"
                  >
                    Total
                  </dt>
                  <dd class="mt-1 text-lg font-semibold tabular-nums">
                    {{ throughputTotal }}
                  </dd>
                </div>
                <div>
                  <dt
                    class="font-mono text-[9px] uppercase tracking-[0.14em] text-faint-foreground"
                  >
                    Peak
                  </dt>
                  <dd
                    class="mt-1 text-lg font-semibold tabular-nums text-[rgb(var(--data))]"
                  >
                    {{ throughputPeak }}
                  </dd>
                </div>
                <div>
                  <dt
                    class="font-mono text-[9px] uppercase tracking-[0.14em] text-faint-foreground"
                  >
                    Latest
                  </dt>
                  <dd class="mt-1 text-lg font-semibold tabular-nums">
                    {{ throughputLatest }}
                  </dd>
                </div>
              </dl>
            </header>
            <div class="min-h-0 flex-1">
              <svg
                class="h-full w-full"
                viewBox="0 0 1080 220"
                preserveAspectRatio="none"
                role="img"
                aria-label="Run throughput"
              >
                <g
                  v-for="tick in chartYTicks"
                  :key="tick.label"
                  class="text-[9px]"
                >
                  <line
                    x1="44"
                    x2="1064"
                    :y1="tick.y"
                    :y2="tick.y"
                    stroke="rgb(var(--color-border) / 0.55)"
                    stroke-dasharray="3 5"
                    vector-effect="non-scaling-stroke"
                  />
                  <text
                    x="28"
                    :y="tick.y + 3"
                    text-anchor="end"
                    class="fill-[rgb(var(--color-faint-foreground))] font-mono"
                  >
                    {{ tick.label }}
                  </text>
                </g>
                <path
                  :d="sparkAreaPath"
                  fill="rgb(var(--data) / 0.10)"
                  stroke="none"
                />
                <path
                  :d="sparkPath"
                  fill="none"
                  stroke="rgb(var(--data))"
                  stroke-width="2"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  vector-effect="non-scaling-stroke"
                />
                <g v-for="point in chartPoints" :key="point.timestamp">
                  <title>
                    {{ point.value }} runs started at {{ point.timestamp }}
                  </title>
                  <circle
                    :cx="point.x"
                    :cy="point.y"
                    r="4"
                    fill="rgb(var(--data))"
                  />
                  <text
                    :x="point.x"
                    :y="point.valueLabelY"
                    text-anchor="middle"
                    class="fill-[rgb(var(--color-foreground))] font-mono text-[10px] font-semibold"
                  >
                    {{ point.value }}
                  </text>
                  <text
                    :x="point.x"
                    y="204"
                    text-anchor="middle"
                    class="fill-[rgb(var(--color-faint-foreground))] font-mono text-[9px]"
                  >
                    {{ point.label }}
                  </text>
                </g>
              </svg>
            </div>
          </section>
        </template>
      </DashboardGrid>
    </template>

    <DurableView v-else embedded class="min-h-0 flex-1" />

    <LogDetailDrawer
      :open="Boolean(selectedLogId)"
      :log-id="selectedLogId"
      :window="selectedLogWindow"
      @close="closeLogDetail"
    />
  </section>
</template>

<script setup lang="ts">
import { ref, computed, watch } from "vue";
import { useQuery } from "@tanstack/vue-query";
import { useRoute, useRouter } from "vue-router";
import DashboardGrid, {
  type GridItemConfig,
} from "@/components/DashboardGrid.vue";
import TokenUsagePanel from "@/components/observability/TokenUsagePanel.vue";
import TracesPanel from "@/components/observability/TracesPanel.vue";
import MemoryPanel from "@/components/observability/MemoryPanel.vue";
import MemoryMetricsPanel from "@/components/observability/MemoryMetricsPanel.vue";
import LogsPanel from "@/components/observability/LogsPanel.vue";
import LogDetailDrawer from "@/components/observability/LogDetailDrawer.vue";
import AgentsPanel from "@/components/overview/AgentsPanel.vue";
import RecentRunsPanel from "@/components/overview/RecentRunsPanel.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import MSegmented from "@/components/ui/MSegmented.vue";
import DurableView from "@/views/DurableView.vue";
import {
  fetchAgentRuns,
  fetchAgentStatus,
  listSpecialists,
} from "@/api/client";
import {
  TOKEN_METRIC_TIME_RANGES,
  type MetricsTimeRangeValue,
} from "@/composables/observability/useTokenMetrics";

type OverviewMode = "customize" | "queue-ops";

const route = useRoute();
const router = useRouter();
const selectedLogId = ref<string | null>(null);
const selectedLogWindow = ref<MetricsTimeRangeValue>("24h");
const overviewMode = ref<OverviewMode>(normalizeOverviewMode(route.query.tab));
const dashboardTimeRange = ref<MetricsTimeRangeValue>("24h");

const timeRangeDropdownOptions = TOKEN_METRIC_TIME_RANGES.map((option) => ({
  id: option.value,
  label: option.label,
  value: option.value,
}));

// Define default dashboard layout
// 12 columns grid, row height = 80px + 16px margin = 96px per row
const dashboardLayout = ref<GridItemConfig[]>([
  // Token Usage - wide, tall (takes up more space)
  { i: "tokens", x: 0, y: 0, w: 8, h: 4, minW: 4, minH: 3 },
  // Agents - sidebar
  { i: "agents", x: 8, y: 0, w: 4, h: 4, minW: 3, minH: 3 },
  // Traces - wide, tall
  { i: "traces", x: 0, y: 4, w: 8, h: 5, minW: 4, minH: 4 },
  // Recent Runs - sidebar
  { i: "runs", x: 8, y: 4, w: 4, h: 5, minW: 3, minH: 3 },
  // Throughput graph
  { i: "throughput", x: 0, y: 9, w: 12, h: 4, minW: 6, minH: 3 },
  // Evolving memory metrics + inspector
  { i: "memory-metrics", x: 0, y: 13, w: 5, h: 5, minW: 4, minH: 4 },
  { i: "memory", x: 5, y: 13, w: 7, h: 5, minW: 4, minH: 4 },
  // Logs - full width
  { i: "logs", x: 0, y: 18, w: 12, h: 4, minW: 4, minH: 3 },
]);

watch(
  () => route.query.tab,
  (tab) => {
    overviewMode.value = normalizeOverviewMode(tab);
  },
);

watch(overviewMode, (mode) => {
  if (normalizeOverviewMode(route.query.tab) === mode) return;
  void router.replace({
    query: {
      ...route.query,
      tab: mode === "queue-ops" ? mode : undefined,
    },
  });
});

watch(dashboardTimeRange, (range) => {
  selectedLogWindow.value = range;
});

const { data: agentData } = useQuery({
  queryKey: ["agent-status"],
  queryFn: fetchAgentStatus,
  staleTime: 30_000,
});

const { data: specialistsData } = useQuery({
  queryKey: ["specialists"],
  queryFn: listSpecialists,
  staleTime: 30_000,
});

const { data: runsData } = useQuery({
  queryKey: ["agent-runs"],
  queryFn: fetchAgentRuns,
  staleTime: 15_000,
});

const agents = computed(() => {
  const base = (agentData.value ?? []).slice();
  // If the orchestrator specialist is present in the specialists list, expose
  // it as a synthetic agent in the Overview. The backend exposes a synthetic
  // "orchestrator" specialist via /api/specialists; convert it to an
  // AgentStatus-like object for rendering here.
  const specs = specialistsData?.value ?? [];
  const orch = specs.find(
    (s: any) => String(s.name).toLowerCase().trim() === "orchestrator",
  );
  if (orch) {
    const exists = base.find(
      (a: any) =>
        String(a.id).toLowerCase().trim() ===
        String(orch.name).toLowerCase().trim(),
    );
    if (!exists) {
      base.unshift({
        id: orch.name || "orchestrator",
        name: orch.name || "orchestrator",
        state: orch.paused ? "offline" : "online",
        model: orch.model || "",
        updatedAt: new Date().toISOString(),
      });
    }
  }
  return base;
});
const runs = computed(() => runsData.value ?? []);

const specialistCount = computed(() => (specialistsData?.value ?? []).length);

const runsToday = computed(
  () => runs.value.filter((run) => isToday(run.createdAt)).length,
);

const runsSummary = computed(() => `${runsToday.value} started today`);

const overviewStats = computed(() => [
  {
    label: "Active Agents",
    value: agents.value.length.toLocaleString(),
    secondary: "Reporting status",
  },
  {
    label: "Runs Today",
    value: runsToday.value.toLocaleString(),
    secondary: runsSummary.value,
    data: true,
  },
  {
    label: "Recent Runs",
    value: recentRuns.value.length.toLocaleString(),
    secondary: "Past 24 hours",
    data: true,
  },
  {
    label: "Specialists",
    value: specialistCount.value.toLocaleString(),
    secondary: "Available roles",
  },
]);

const chartBuckets = computed(() => {
  const now = new Date();
  return Array.from({ length: 8 }, (_, index) => {
    const bucketStart = new Date(now);
    bucketStart.setMinutes(0, 0, 0);
    bucketStart.setHours(bucketStart.getHours() - (7 - index));
    const bucketEnd = new Date(bucketStart);
    bucketEnd.setHours(bucketEnd.getHours() + 1);
    const value = runs.value.filter((run) => {
      const created = new Date(run.createdAt);
      return created >= bucketStart && created < bucketEnd;
    }).length;
    return {
      label: bucketStart.toLocaleTimeString([], {
        hour: "numeric",
      }),
      timestamp: bucketStart.toLocaleTimeString([], {
        hour: "numeric",
        minute: "2-digit",
      }),
      value,
    };
  });
});

const chartMax = computed(() =>
  Math.max(1, ...chartBuckets.value.map((bucket) => bucket.value)),
);

const chartPoints = computed(() => {
  const max = chartMax.value;
  const top = 26;
  const bottom = 174;
  const height = bottom - top;
  return chartBuckets.value.map((bucket, index) => {
    const x = 52 + index * (996 / Math.max(1, chartBuckets.value.length - 1));
    const y = bottom - (bucket.value / max) * height;
    return {
      ...bucket,
      x,
      y,
      valueLabelY: Math.max(14, y - 10),
    };
  });
});

const chartYTicks = computed(() => {
  const max = chartMax.value;
  const midpoint = Math.ceil(max / 2);
  const values = Array.from(new Set([max, midpoint, 0]));
  const top = 26;
  const bottom = 174;
  const height = bottom - top;
  return values.map((value) => ({
    label: value.toLocaleString(),
    y: bottom - (value / max) * height,
  }));
});

const throughputTotal = computed(() =>
  chartBuckets.value.reduce((sum, bucket) => sum + bucket.value, 0),
);

const throughputPeak = computed(() =>
  Math.max(0, ...chartBuckets.value.map((bucket) => bucket.value)),
);

const throughputLatest = computed(
  () => chartBuckets.value[chartBuckets.value.length - 1]?.value ?? 0,
);

const sparkPath = computed(() => buildSmoothPath(chartPoints.value));

const sparkAreaPath = computed(() => {
  const points = chartPoints.value;
  if (!points.length || !sparkPath.value) return "";
  const baseline = 174;
  return `${sparkPath.value} L ${points[points.length - 1].x} ${baseline} L ${points[0].x} ${baseline} Z`;
});

const recentRuns = computed(() =>
  runs.value
    .filter((run) => isToday(run.createdAt))
    .slice()
    .sort(
      (a, b) =>
        new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
    )
    .slice(0, 5),
);

type ChartPoint = {
  x: number;
  y: number;
};

function buildSmoothPath(points: ChartPoint[]) {
  if (points.length === 0) return "";
  if (points.length === 1) return `M ${points[0].x} ${points[0].y}`;

  const commands = [`M ${points[0].x} ${points[0].y}`];
  for (let index = 0; index < points.length - 1; index += 1) {
    const current = points[index];
    const next = points[index + 1];
    const controlOffset = (next.x - current.x) * 0.45;
    commands.push(
      `C ${current.x + controlOffset} ${current.y}, ${next.x - controlOffset} ${next.y}, ${next.x} ${next.y}`,
    );
  }
  return commands.join(" ");
}

function isToday(value: string) {
  const date = new Date(value);
  const now = new Date();
  return (
    date.getDate() === now.getDate() &&
    date.getMonth() === now.getMonth() &&
    date.getFullYear() === now.getFullYear()
  );
}

function onLayoutChange(newLayout: GridItemConfig[]) {
  // Layout changes are automatically saved via DashboardGrid component
  console.log("Dashboard layout updated:", newLayout);
}

function openLogDetail(payload: { id: string; window: MetricsTimeRangeValue }) {
  selectedLogId.value = payload.id;
  selectedLogWindow.value = payload.window;
}

function closeLogDetail() {
  selectedLogId.value = null;
}

function normalizeOverviewMode(value: unknown): OverviewMode {
  const raw = Array.isArray(value) ? value[0] : value;
  return raw === "queue-ops" ? "queue-ops" : "customize";
}
</script>
