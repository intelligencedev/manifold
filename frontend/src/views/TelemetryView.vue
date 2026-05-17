<template>
  <section class="space-y-5">
    <div class="flex items-center justify-between">
      <h1 class="text-xl font-semibold">Telemetry</h1>
      <Button size="sm" variant="ghost" :loading="loading" @click="reload">Refresh</Button>
    </div>

    <!-- KPI row -->
    <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <Card title="Total runs" description="All time">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ allRuns.length }}</div>
        <SparkBar :values="runSparkValues" label="last 20 runs" bar-color="bg-accent/50" class="mt-3" />
      </Card>
      <Card title="Active" description="Running right now">
        <div class="mt-1 text-3xl font-semibold tabular-nums text-emerald-400">{{ activeRunCount }}</div>
      </Card>
      <Card title="Failed" description="Non-zero exits">
        <div class="mt-1 text-3xl font-semibold tabular-nums" :class="failedRunCount ? 'text-red-400' : ''">{{ failedRunCount }}</div>
      </Card>
      <Card title="Trust budgets" description="Autonomy ledger">
        <div class="mt-1 text-3xl font-semibold tabular-nums">{{ trust.budgets.length }}</div>
      </Card>
    </div>

    <div class="grid gap-5 xl:grid-cols-[1fr_400px]">
      <!-- Runs table -->
      <Card title="Run history" :description="`${allRuns.length} runs`" :no-padding="true">
        <div class="overflow-x-auto">
          <table class="w-full text-xs">
            <thead>
              <tr class="border-b border-border/50 text-left text-[10px] uppercase tracking-wider text-white/35">
                <th class="px-4 py-2.5">ID</th>
                <th class="px-4 py-2.5">Status</th>
                <th class="px-4 py-2.5 hidden sm:table-cell">Prompt</th>
                <th class="px-4 py-2.5 text-right">Tokens</th>
                <th class="px-4 py-2.5 hidden lg:table-cell text-right">Started</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="run in pagedRuns"
                :key="run.id"
                class="border-b border-border/30 hover:bg-white/3 transition-colors"
              >
                <td class="px-4 py-3 font-mono text-[11px] text-white/50">{{ shortId(run.id) }}</td>
                <td class="px-4 py-3"><Badge :variant="statusVariant(run.status)">{{ run.status }}</Badge></td>
                <td class="px-4 py-3 max-w-xs truncate text-white/60 hidden sm:table-cell">{{ run.prompt }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-white/50">{{ run.tokens ? fmtNum(run.tokens) : '—' }}</td>
                <td class="px-4 py-3 text-right text-white/35 hidden lg:table-cell">{{ run.createdAt ? reltime(run.createdAt) : '—' }}</td>
              </tr>
            </tbody>
          </table>
          <div v-if="allRuns.length === 0" class="px-4 py-10 text-center text-xs text-white/30">No run history available.</div>
        </div>
        <!-- Pagination -->
        <div v-if="totalPages > 1" class="flex items-center justify-between border-t border-border/40 px-4 py-3 text-xs text-white/40">
          <span>Page {{ page + 1 }} of {{ totalPages }}</span>
          <div class="flex gap-2">
            <Button size="sm" variant="ghost" :disabled="page === 0" @click="page--">Prev</Button>
            <Button size="sm" variant="ghost" :disabled="page >= totalPages - 1" @click="page++">Next</Button>
          </div>
        </div>
      </Card>

      <!-- Trust ledger -->
      <div class="space-y-4">
        <Card title="Trust ledger" description="Per-specialist autonomy budgets">
          <template #header>
            <Button size="sm" variant="ghost" @click="trust.refresh">Sync</Button>
          </template>
          <div v-if="trust.budgets.length === 0" class="py-6 text-center text-xs text-white/30">No trust budgets configured.</div>
          <div v-else class="space-y-4">
            <div v-for="budget in trust.budgets" :key="budget.name" class="space-y-1.5">
              <div class="flex items-center justify-between">
                <span class="text-xs font-medium">{{ budget.name }}</span>
                <span class="tabular-nums text-[11px] text-white/40">{{ budget.spent }} / {{ budget.unlimited ? '∞' : budget.quota }}</span>
              </div>
              <div class="h-1.5 w-full overflow-hidden rounded-full bg-white/10">
                <div
                  class="h-1.5 rounded-full transition-all duration-500"
                  :class="spendRatio(budget) > 0.8 ? 'bg-red-400' : spendRatio(budget) > 0.5 ? 'bg-amber-400' : 'bg-emerald-400'"
                  :style="{ width: `${Math.min(100, spendRatio(budget) * 100)}%` }"
                />
              </div>
              <div class="flex justify-end gap-2">
                <Button size="sm" variant="ghost" @click="openRefill(budget.name)">Refill</Button>
              </div>
            </div>
          </div>
        </Card>

        <!-- Token breakdown -->
        <Card title="Token usage" description="By model (recent window)">
          <div v-if="tokenRows.length === 0" class="text-center text-xs text-white/30 py-4">No token data available.</div>
          <div v-else class="space-y-2">
            <div v-for="row in tokenRows" :key="row.model ?? row.provider" class="flex items-center justify-between text-xs">
              <span class="text-white/60 truncate">{{ row.model || row.provider || 'unknown' }}</span>
              <span class="tabular-nums text-white/50 ml-2 shrink-0">{{ fmtNum(row.totalTokens ?? row.total_tokens ?? 0) }}</span>
            </div>
          </div>
        </Card>
      </div>
    </div>

    <!-- Refill modal -->
    <Modal :open="refillModal.open" title="Refill trust budget" @close="refillModal.open = false">
      <p class="mb-4 text-sm text-white/60">Set quota for <strong class="text-white/80">{{ refillModal.name }}</strong></p>
      <Input v-model="refillModal.quota" type="number" label="New quota" placeholder="100" />
      <template #footer>
        <Button variant="ghost" @click="refillModal.open = false">Cancel</Button>
        <Button variant="primary" :loading="refillModal.loading" @click="submitRefill">Refill</Button>
      </template>
    </Modal>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import Badge from "@/components/ui/Badge.vue";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import Input from "@/components/ui/Input.vue";
import Modal from "@/components/ui/Modal.vue";
import SparkBar from "@/components/charts/SparkBar.vue";
import { fetchRuns, fetchTokenMetrics } from "@/api/metrics";
import { useTrustStore } from "@/stores/trust";

const trust = useTrustStore();
const loading = ref(false);
const allRuns = ref<any[]>([]);
const tokenData = ref<any>(null);
const page = ref(0);
const PAGE_SIZE = 15;

const pagedRuns = computed(() => allRuns.value.slice(page.value * PAGE_SIZE, (page.value + 1) * PAGE_SIZE));
const totalPages = computed(() => Math.ceil(allRuns.value.length / PAGE_SIZE));
const activeRunCount = computed(() => allRuns.value.filter((r) => r.status === "running").length);
const failedRunCount = computed(() => allRuns.value.filter((r) => r.status === "failed").length);
const runSparkValues = computed(() => allRuns.value.slice(-20).map((r) => (r.status === "completed" ? 1 : r.status === "failed" ? 0.5 : 0.1)));
const tokenRows = computed(() => {
  if (!tokenData.value) return [];
  if (Array.isArray(tokenData.value)) return tokenData.value;
  if (Array.isArray(tokenData.value?.models)) return tokenData.value.models;
  if (Array.isArray(tokenData.value?.totals)) return tokenData.value.totals;
  return [];
});

const refillModal = ref({ open: false, name: "", quota: "100", loading: false });

onMounted(() => reload());

async function reload() {
  loading.value = true;
  try {
    [allRuns.value, tokenData.value] = await Promise.all([
      fetchRuns().catch(() => []),
      fetchTokenMetrics().catch(() => null),
    ]);
    await trust.refresh();
  } finally {
    loading.value = false;
  }
}

function openRefill(name: string) {
  refillModal.value = { open: true, name, quota: "100", loading: false };
}

async function submitRefill() {
  const quota = parseInt(refillModal.value.quota, 10);
  if (!quota || quota < 1) return;
  refillModal.value.loading = true;
  try {
    await trust.refill(refillModal.value.name, quota);
    refillModal.value.open = false;
  } finally {
    refillModal.value.loading = false;
  }
}

function spendRatio(b: any) {
  if (b.unlimited || !b.quota) return 0;
  return Math.min(1, b.spent / b.quota);
}

function statusVariant(status: string): "success" | "danger" | "muted" | "warning" {
  if (status === "running") return "success";
  if (status === "failed") return "danger";
  if (status === "completed") return "muted";
  return "warning";
}

function shortId(id: string) {
  return id?.length > 14 ? id.slice(0, 6) + "…" + id.slice(-4) : id;
}

function fmtNum(n: number) {
  return new Intl.NumberFormat().format(n);
}

function reltime(iso: string) {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60_000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3_600_000) return `${Math.round(diff / 60_000)}m ago`;
  return `${Math.round(diff / 3_600_000)}h ago`;
}
</script>
