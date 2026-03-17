<template>
  <Teleport to="body">
    <div
      v-if="open"
      ref="pickerEl"
      class="expression-picker fixed z-[9999] w-72 max-h-72 overflow-y-auto rounded-lg border border-border/70 bg-surface shadow-xl text-[11px]"
      :style="floatingStyle"
      @mousedown.stop
      @pointerdown.stop
    >
    <div
      class="sticky top-0 z-10 flex items-center justify-between border-b border-border/60 bg-surface px-3 py-2"
    >
      <span class="font-semibold text-foreground">Insert reference</span>
      <button
        class="text-faint-foreground hover:text-foreground"
        title="Close"
        @click="$emit('close')"
      >
        ✕
      </button>
    </div>

    <div v-if="upstreamEntries.length === 0 && !hasTriggerInputs" class="px-3 py-4 text-center text-faint-foreground">
      No upstream nodes connected.
      <p class="mt-1 text-[10px]">Connect a node to this one to reference its output.</p>
    </div>

    <!-- Workflow input section -->
    <div v-if="hasTriggerInputs" class="border-b border-border/40">
      <button
        class="flex w-full items-center gap-1.5 px-3 py-2 text-left font-semibold text-muted-foreground hover:bg-muted/40"
        @click="toggleSection('__trigger__')"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          class="h-3 w-3 transition-transform"
          :class="expandedSections.has('__trigger__') ? 'rotate-90' : ''"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="9 18 15 12 9 6"></polyline>
        </svg>
        <span>Workflow Input</span>
        <span class="ml-auto text-[10px] font-normal text-faint-foreground">$run.input</span>
      </button>
      <div v-show="expandedSections.has('__trigger__')" class="pb-1">
        <button
          class="flex w-full items-center gap-2 px-5 py-1.5 text-left text-foreground/90 hover:bg-accent/20 hover:text-foreground"
          @click="select('={{$run.input}}')"
        >
          <span class="font-mono text-[10px] text-accent">$run.input</span>
          <span class="ml-auto text-[10px] text-faint-foreground">full input</span>
        </button>
        <button
          class="flex w-full items-center gap-2 px-5 py-1.5 text-left text-foreground/90 hover:bg-accent/20 hover:text-foreground"
          @click="select('={{$run.input.query}}')"
        >
          <span class="font-mono text-[10px] text-accent">$run.input.query</span>
          <span class="ml-auto text-[10px] text-faint-foreground">query field</span>
        </button>
      </div>
    </div>

    <!-- Upstream node sections -->
    <div
      v-for="entry in upstreamEntries"
      :key="entry.nodeId"
      class="border-b border-border/40 last:border-b-0"
    >
      <button
        class="flex w-full items-center gap-1.5 px-3 py-2 text-left font-semibold text-muted-foreground hover:bg-muted/40"
        @click="toggleSection(entry.nodeId)"
      >
        <svg
          xmlns="http://www.w3.org/2000/svg"
          viewBox="0 0 24 24"
          class="h-3 w-3 transition-transform"
          :class="expandedSections.has(entry.nodeId) ? 'rotate-90' : ''"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <polyline points="9 18 15 12 9 6"></polyline>
        </svg>
        <span class="truncate">{{ entry.label }}</span>
        <span class="ml-auto shrink-0 text-[10px] font-normal text-faint-foreground">{{
          entry.toolName || "step"
        }}</span>
      </button>
      <div v-show="expandedSections.has(entry.nodeId)" class="pb-1">
        <!-- Full output reference -->
        <button
          class="flex w-full items-center gap-2 px-5 py-1.5 text-left text-foreground/90 hover:bg-accent/20 hover:text-foreground"
          @click="select(`={{$node.${entry.nodeId}.output}}`)"
        >
          <span class="font-mono text-[10px] text-accent">output</span>
          <span class="ml-auto text-[10px] text-faint-foreground">full output</span>
        </button>
        <!-- Known output fields from runtime trace -->
        <button
          v-for="field in entry.outputFields"
          :key="field.key"
          class="flex w-full items-center gap-2 px-5 py-1.5 text-left text-foreground/90 hover:bg-accent/20 hover:text-foreground"
          @click="select(`={{$node.${entry.nodeId}.output.${field.key}}}`)"
        >
          <span class="font-mono text-[10px] text-accent">output.{{ field.key }}</span>
          <span
            v-if="field.preview"
            class="ml-auto max-w-[100px] truncate text-[10px] text-faint-foreground"
            :title="field.preview"
          >
            {{ field.preview }}
          </span>
        </button>
        <!-- If no fields discovered, show hint -->
        <p
          v-if="entry.outputFields.length === 0"
          class="px-5 py-1 text-[10px] italic text-faint-foreground"
        >
          Run the flow to discover output fields.
        </p>
      </div>
    </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { inject, ref, computed, watch, nextTick, onMounted, onBeforeUnmount, type Ref } from "vue";
import type { Edge } from "@vue-flow/core";
import type { FlowEditorTool, FlowEditorStepTrace } from "@/types/flowEditor";

defineOptions({ name: "ExpressionPicker" });

interface UpstreamOutputField {
  key: string;
  preview?: string;
}

interface UpstreamEntry {
  nodeId: string;
  label: string;
  toolName: string;
  outputFields: UpstreamOutputField[];
}

const props = defineProps<{
  open: boolean;
  nodeId: string;
  /** Anchor element to position near */
  anchor?: HTMLElement | null;
}>();

const emit = defineEmits<{
  (e: "select", expression: string): void;
  (e: "close"): void;
}>();

const pickerEl = ref<HTMLElement | null>(null);

// Inject graph context from FlowView / FlowStepNode
const edges = inject<Ref<Edge[]>>("flowEditorEdges", ref([]));
const nodes = inject<Ref<any[]>>("flowEditorNodes", ref([]));
const tools = inject<Ref<FlowEditorTool[]>>("flowEditorTools", ref([]));
const runTrace = inject<Ref<Record<string, FlowEditorStepTrace>>>(
  "flowEditorRunTrace",
  ref({}),
);
const activeWorkflow = inject<Ref<any>>("flowEditorActiveWorkflow", ref(null));

const expandedSections = ref(new Set<string>());

const hasTriggerInputs = computed(() => {
  const wf = activeWorkflow.value;
  return Boolean(wf?.trigger);
});

/**
 * Walk the edge graph backwards to collect all upstream node IDs
 * (direct predecessors only for clarity, but we include transitive ancestors).
 */
function getUpstreamNodeIds(targetId: string): string[] {
  const visited = new Set<string>();
  const queue = [targetId];
  while (queue.length > 0) {
    const current = queue.shift()!;
    for (const edge of edges.value) {
      if (edge.target === current && !visited.has(edge.source)) {
        visited.add(edge.source);
        queue.push(edge.source);
      }
    }
  }
  return Array.from(visited);
}

const upstreamEntries = computed<UpstreamEntry[]>(() => {
  const upstreamIds = getUpstreamNodeIds(props.nodeId);
  const toolMap = new Map<string, FlowEditorTool>();
  for (const t of tools.value) toolMap.set(t.name, t);

  const trace: Record<string, FlowEditorStepTrace> =
    runTrace.value && typeof runTrace.value === "object" ? runTrace.value : {};

  // Build entries in topological order (reverse of discovery = upstream first)
  return upstreamIds.reverse().map((id) => {
    const node = nodes.value.find((n: any) => n.id === id);
    const stepData = node?.data;
    const toolName = stepData?.step?.tool?.name ?? "";
    const stepText = stepData?.step?.text ?? "";
    const label = stepText || id;

    // Discover output fields from runtime trace
    const stepTrace = trace[id];
    const outputFields: UpstreamOutputField[] = [];
    if (stepTrace) {
      // payload fields
      if (stepTrace.payload && typeof stepTrace.payload === "object") {
        for (const [key, val] of Object.entries(
          stepTrace.payload as Record<string, unknown>,
        )) {
          outputFields.push({
            key,
            preview: formatPreview(val),
          });
        }
      }
      // delta fields
      if (stepTrace.delta && typeof stepTrace.delta === "object") {
        for (const [key, val] of Object.entries(stepTrace.delta)) {
          if (!outputFields.some((f) => f.key === key)) {
            outputFields.push({
              key,
              preview: formatPreview(val),
            });
          }
        }
      }
    }

    return { nodeId: id, label, toolName, outputFields };
  });
});

// Auto-expand the first upstream node when the picker opens
watch(
  () => props.open,
  async (isOpen) => {
    if (isOpen) {
      expandedSections.value = new Set<string>();
      if (upstreamEntries.value.length > 0) {
        // Expand the most recent upstream (last in the list = direct predecessor)
        const directPredecessor = upstreamEntries.value[upstreamEntries.value.length - 1];
        if (directPredecessor) {
          expandedSections.value.add(directPredecessor.nodeId);
        }
      } else if (hasTriggerInputs.value) {
        expandedSections.value.add("__trigger__");
      }
      await nextTick();
      updatePosition();
    }
  },
);

function toggleSection(id: string) {
  const next = new Set(expandedSections.value);
  if (next.has(id)) next.delete(id);
  else next.add(id);
  expandedSections.value = next;
}

function select(expression: string) {
  emit("select", expression);
}

function formatPreview(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value.length > 40 ? value.slice(0, 40) + "…" : value;
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return `[${value.length} items]`;
  if (typeof value === "object") return "{…}";
  return "";
}

// Floating position calculated from anchor bounding rect
const floatingPos = ref<{ top: number; left: number }>({ top: 0, left: 0 });

const floatingStyle = computed(() => ({
  top: `${floatingPos.value.top}px`,
  left: `${floatingPos.value.left}px`,
}));

function updatePosition() {
  if (!props.anchor) return;
  const rect = props.anchor.getBoundingClientRect();
  const pickerWidth = 288; // w-72 = 18rem = 288px
  const pickerMaxHeight = 288; // max-h-72

  let top = rect.bottom + 4;
  let left = rect.right - pickerWidth;

  // Keep within viewport
  if (left < 8) left = 8;
  if (left + pickerWidth > window.innerWidth - 8) {
    left = window.innerWidth - pickerWidth - 8;
  }
  if (top + pickerMaxHeight > window.innerHeight - 8) {
    top = rect.top - pickerMaxHeight - 4;
    if (top < 8) top = 8;
  }

  floatingPos.value = { top, left };
}

// Click-outside handler
function onClickOutside(event: MouseEvent) {
  if (!props.open) return;
  if (pickerEl.value && !pickerEl.value.contains(event.target as Node)) {
    emit("close");
  }
}

onMounted(() => {
  document.addEventListener("mousedown", onClickOutside, true);
});
onBeforeUnmount(() => {
  document.removeEventListener("mousedown", onClickOutside, true);
});
</script>
