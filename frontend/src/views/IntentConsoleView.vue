<template>
  <section class="space-y-5">
    <div>
      <h1 class="text-xl font-semibold">Intent Console</h1>
      <p class="mt-0.5 text-sm text-white/50">Compose operator intent and run agents directly.</p>
    </div>

    <div class="grid gap-5 xl:grid-cols-[1fr_320px]">
      <!-- Compose panel -->
      <div class="space-y-4">
        <Card title="Prompt" description="Describe the task, goal, and constraints.">
          <template #header>
            <div class="flex flex-wrap items-center gap-2">
              <select
                v-model="routeTarget"
                class="min-w-36 rounded-lg border border-border bg-black/15 px-2.5 py-1.5 text-xs text-white/70 focus:border-accent/40 focus:outline-none"
              >
                <option value="">Orchestrator</option>
                <optgroup label="Specialists">
                  <option v-for="sp in specialists" :key="String(sp.name)" :value="String(sp.name)">{{ sp.name }}</option>
                </optgroup>
                <optgroup label="Teams">
                  <option v-for="t in teams" :key="String(t.name)" :value="`team:${t.name}`">{{ t.name }}</option>
                </optgroup>
              </select>
              <select
                v-model="selectedProjectId"
                class="min-w-44 rounded-lg border border-border bg-black/15 px-2.5 py-1.5 text-xs text-white/70 focus:border-accent/40 focus:outline-none"
              >
                <option value="">No project</option>
                <option v-for="project in projects.projects" :key="project.id" :value="project.id">
                  {{ project.name || project.id }}
                </option>
              </select>
            </div>
          </template>

          <textarea
            v-model="prompt"
            class="min-h-[200px] w-full resize-none rounded-lg border border-border bg-black/15 p-3 text-sm text-foreground placeholder:text-white/25 focus:border-accent/40 focus:outline-none focus:ring-1 focus:ring-accent/20"
            placeholder="Describe the objective, constraints, and desired end state…"
            @keydown.meta.enter.prevent="run"
            @keydown.ctrl.enter.prevent="run"
          />

          <div class="mt-3 flex items-center justify-between">
            <p class="text-xs text-white/30"><kbd class="rounded border border-border px-1">⌘↵</kbd> submit</p>
            <div class="flex gap-2">
              <Button variant="ghost" size="sm" :disabled="running" @click="clearOutput">Clear</Button>
              <Button variant="primary" size="md" :loading="running" :disabled="!prompt.trim()" @click="run">Run agent</Button>
            </div>
          </div>
        </Card>

        <!-- Streaming output -->
        <Card title="Output" description="Live agent response stream">
          <div
            ref="outputEl"
            class="min-h-[180px] rounded-lg border border-border/50 bg-black/20 p-3 font-mono text-xs text-white/70 leading-relaxed overflow-auto"
            style="max-height: 520px; white-space: pre-wrap; word-break: break-word;"
          >
            <span v-if="!outputText && !running" class="text-white/25 not-italic">Response will appear here…</span>
            <span v-html="outputHtml" />
            <span v-if="running" class="ml-0.5 inline-block h-3 w-1.5 bg-accent animate-pulse" aria-hidden="true" />
          </div>
          <ErrorBanner v-if="runError" :message="runError" class="mt-3" @dismiss="runError = ''" />
        </Card>
      </div>

      <!-- Sidebar: workflows + tools -->
      <div class="space-y-4">
        <Card title="Workflows" :description="`${workflows.length} available`">
          <div class="space-y-1.5">
            <button
              v-for="wf in workflows"
              :key="wf.id"
              class="w-full rounded-lg border border-border/50 bg-black/10 px-3 py-2.5 text-left text-xs hover:border-border transition-colors"
              @click="prompt = `Run workflow: ${wf.name || wf.id}`"
            >
              <div class="font-medium text-white/80">{{ wf.name || wf.id }}</div>
              <div v-if="wf.description" class="mt-0.5 text-white/40 line-clamp-2">{{ wf.description }}</div>
            </button>
            <EmptyState v-if="workflows.length === 0" icon="🔀" title="No workflows" description="Create flow v2 workflows to appear here." class="py-6" />
          </div>
        </Card>

        <Card title="Available tools" :description="`${tools.length} tools`">
          <div class="max-h-64 overflow-y-auto space-y-1">
            <div v-for="tool in tools" :key="tool.name ?? tool.id" class="flex items-center gap-2 px-1 py-1 text-xs text-white/50">
              <span class="h-1 w-1 rounded-full bg-accent/40 shrink-0" />
              {{ tool.name || tool.id }}
            </div>
          </div>
        </Card>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { listFlowTools, listWorkflows } from "@/api/flow";
import { useFleetStore } from "@/stores/fleet";
import { useProjectsStore } from "@/stores/projects";
import { useStreamingFetch } from "@/composables/useStreamingFetch";
import Button from "@/components/ui/Button.vue";
import Card from "@/components/ui/Card.vue";
import EmptyState from "@/components/ui/EmptyState.vue";
import ErrorBanner from "@/components/ui/ErrorBanner.vue";

const fleet = useFleetStore();
const projects = useProjectsStore();
const specialists = computed(() => fleet.state?.specialists ?? []);
const teams = computed(() => fleet.state?.teams ?? []);

const prompt = ref("");
const routeTarget = ref("");
const selectedProjectId = ref("");
const running = ref(false);
const runError = ref("");
const outputText = ref("");
const outputEl = ref<HTMLElement>();
const sessionId = ref(createSessionID());

const outputHtml = computed(() => {
  // Convert markdown-ish bold/code spans to HTML for basic readability
  return outputText.value
    .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
    .replace(/\*\*(.+?)\*\*/g, "<strong>$1</strong>")
    .replace(/`([^`]+)`/g, '<code class="rounded bg-white/8 px-1">$1</code>');
});

const workflows = ref<any[]>([]);
const tools = ref<any[]>([]);

onMounted(async () => {
  const [loadedWorkflows, loadedTools] = await Promise.all([
    listWorkflows().catch(() => []),
    listFlowTools().catch(() => []),
    projects.refresh().catch(() => {}),
  ]);
  workflows.value = loadedWorkflows;
  tools.value = loadedTools;
  if (!fleet.state) fleet.refresh().catch(() => {});
});

watch(outputText, async () => {
  await nextTick();
  if (outputEl.value) outputEl.value.scrollTop = outputEl.value.scrollHeight;
});

async function run() {
  if (!prompt.value.trim() || running.value) return;
  running.value = true;
  runError.value = "";
  outputText.value = "";

  // Build route query string
  let url = "/agent/run";
  const params = new URLSearchParams();
  if (routeTarget.value.startsWith("team:")) {
    params.set("team", routeTarget.value.slice(5));
  } else if (routeTarget.value) {
    params.set("specialist", routeTarget.value);
  }
  if (params.toString()) url += "?" + params.toString();

  const body: Record<string, string> = {
    prompt: prompt.value.trim(),
    session_id: sessionId.value,
  };
  if (selectedProjectId.value) {
    body.project_id = selectedProjectId.value;
  }

  try {
    await useStreamingFetch(url, body, (event: any) => {
      if (event.type === "delta" || event.type === "agent_delta") {
        outputText.value += (event.data ?? "");
      } else if (event.type === "final" || event.type === "agent_final") {
        outputText.value = event.data ?? outputText.value;
        running.value = false;
      } else if (event.type === "error" || event.type === "agent_error") {
        runError.value = event.data ?? "Run failed";
        running.value = false;
      }
    });
  } catch (err: unknown) {
    runError.value = err instanceof Error ? err.message : "Network error";
  } finally {
    running.value = false;
  }
}

function clearOutput() {
  outputText.value = "";
  runError.value = "";
}

function createSessionID() {
  if (typeof crypto !== "undefined" && "randomUUID" in crypto) {
    return crypto.randomUUID();
  }
  return `00000000-0000-4000-8000-${Math.random().toString(16).slice(2, 14).padEnd(12, "0")}`;
}
</script>
