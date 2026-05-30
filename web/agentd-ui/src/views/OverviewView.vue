<template>
  <section class="flex min-h-full flex-col gap-5">
    <header class="flex items-start justify-between gap-4">
      <div>
        <h2 class="font-display text-2xl leading-tight text-foreground">
          Situation Room
        </h2>
        <p class="mt-1 text-sm text-muted-foreground">
          Live agents, today's throughput, and recent work.
        </p>
      </div>
      <div class="flex items-center gap-2">
        <button
          v-if="overviewMode === 'customize'"
          type="button"
          class="halo-focus inline-flex h-8 items-center gap-1.5 rounded-md border border-[rgb(var(--line-strong))] px-2.5 font-mono text-[11px] uppercase tracking-[0.08em] text-muted-foreground transition hover:bg-surface-muted hover:text-foreground"
          aria-label="Reset dashboard layout"
          title="Reset dashboard layout"
          @click="resetLayout"
        >
          <SolarRefreshIcon class="h-3.5 w-3.5" />
          Reset
        </button>
        <MSegmented
          v-model="overviewMode"
          :options="[
            { value: 'live', label: 'Live' },
            { value: 'customize', label: 'Customize' },
          ]"
        />
      </div>
    </header>

    <template v-if="overviewMode === 'live'">
      <MReadouts>
        <MReadout
          v-for="stat in overviewStats"
          :key="stat.label"
          :k="stat.label"
          :v="stat.value"
          :data="stat.data"
        />
      </MReadouts>

      <MSurface
        title="Throughput"
        eyebrow="Runs"
        description="Runs started in rolling hourly buckets."
      >
        <div class="h-[220px]">
          <svg
            class="h-full w-full overflow-visible"
            viewBox="0 0 720 180"
            role="img"
            aria-label="Run throughput"
          >
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
            />
            <g v-for="point in chartPoints" :key="point.label">
              <circle
                :cx="point.x"
                :cy="point.y"
                r="3"
                fill="rgb(var(--data))"
              />
              <text
                :x="point.x"
                y="176"
                text-anchor="middle"
                class="fill-[rgb(var(--color-faint-foreground))] font-mono text-[9px]"
              >
                {{ point.label }}
              </text>
            </g>
          </svg>
        </div>
      </MSurface>

      <div class="grid min-h-[320px] grid-cols-2 gap-5">
        <AgentsPanel :agents="agents" />
        <RecentRunsPanel :runs="recentRuns" />
      </div>
    </template>

    <div v-else class="min-h-0 flex-1 pb-6 pt-1">
      <DashboardGrid
        ref="dashboardGridRef"
        :layout="dashboardLayout"
        storage-key="overview-dashboard-layout"
        @layout-change="onLayoutChange"
      >
        <template #item-tokens>
          <TokenUsagePanel />
        </template>

        <template #item-traces>
          <TracesPanel />
        </template>

        <template #item-memory>
          <MemoryPanel />
        </template>

        <template #item-memory-metrics>
          <MemoryMetricsPanel />
        </template>

        <template #item-logs>
          <LogsPanel
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
      </DashboardGrid>
    </div>

    <LogDetailDrawer
      :open="Boolean(selectedLogId)"
      :log-id="selectedLogId"
      :window="selectedLogWindow"
      @close="closeLogDetail"
    />
  </section>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useQuery } from "@tanstack/vue-query";
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
import MReadout from "@/components/ui/MReadout.vue";
import MReadouts from "@/components/ui/MReadouts.vue";
import MSegmented from "@/components/ui/MSegmented.vue";
import MSurface from "@/components/ui/MSurface.vue";
import SolarRefreshIcon from "@/components/icons/SolarRefresh.vue";
import {
  fetchAgentRuns,
  fetchAgentStatus,
  listSpecialists,
} from "@/api/client";
import type { MetricsTimeRangeValue } from "@/composables/observability/useTokenMetrics";

const dashboardGridRef = ref<InstanceType<typeof DashboardGrid>>();
const selectedLogId = ref<string | null>(null);
const selectedLogWindow = ref<MetricsTimeRangeValue>("1h");
const overviewMode = ref("live");

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
  // Evolving memory metrics + inspector
  { i: "memory-metrics", x: 0, y: 9, w: 5, h: 5, minW: 4, minH: 4 },
  { i: "memory", x: 5, y: 9, w: 7, h: 5, minW: 4, minH: 4 },
  // Logs - full width
  { i: "logs", x: 0, y: 14, w: 12, h: 4, minW: 4, minH: 3 },
]);

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

const chartPoints = computed(() => {
  const now = new Date();
  const buckets = Array.from({ length: 8 }, (_, index) => {
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
      value,
    };
  });
  const max = Math.max(1, ...buckets.map((bucket) => bucket.value));
  return buckets.map((bucket, index) => {
    const x = 28 + index * (664 / Math.max(1, buckets.length - 1));
    const y = 150 - (bucket.value / max) * 120;
    return { ...bucket, x, y };
  });
});

const sparkPath = computed(() =>
  chartPoints.value
    .map((point, index) => `${index === 0 ? "M" : "L"} ${point.x} ${point.y}`)
    .join(" "),
);

const sparkAreaPath = computed(() => {
  const points = chartPoints.value;
  if (!points.length) return "";
  return `${sparkPath.value} L ${points[points.length - 1].x} 150 L ${points[0].x} 150 Z`;
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

function resetLayout() {
  dashboardGridRef.value?.resetLayout();
}

function openLogDetail(payload: { id: string; window: MetricsTimeRangeValue }) {
  selectedLogId.value = payload.id;
  selectedLogWindow.value = payload.window;
}

function closeLogDetail() {
  selectedLogId.value = null;
}
</script>
