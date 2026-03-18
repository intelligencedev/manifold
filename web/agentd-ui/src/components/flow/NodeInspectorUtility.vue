<template>
  <div class="min-w-0 space-y-3 overflow-x-hidden">
    <div class="flex items-center justify-between">
      <div class="text-xs text-subtle-foreground">Configure utility</div>
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground"
        >Unsaved</span
      >
      <span
        v-else-if="showAppliedFeedback"
        class="text-[10px] italic text-emerald-400"
        >Applied</span
      >
    </div>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      Display Label
      <input
        v-model="labelText"
        type="text"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
        placeholder="Optional heading"
        :disabled="!isDesignMode || hydratingRef"
      />
    </label>

    <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
      <span>Textbox Content</span>
      <div class="relative">
        <textarea
          ref="contentTextareaEl"
          v-model="contentText"
          rows="4"
          class="w-full rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto resize-none whitespace-pre-wrap break-words"
          :class="isExpressionContent ? 'border-accent/60 bg-accent/5' : ''"
          placeholder="Enter static text or use ={{$run.input.query}} bindings"
          :disabled="!isDesignMode || hydratingRef"
          @wheel.stop
        />
        <button
          v-if="hasUpstream"
          type="button"
          class="absolute top-1 right-1 inline-flex h-5 items-center gap-0.5 rounded px-1 text-[10px] font-mono transition"
          :class="contentPickerOpen ? 'bg-accent text-accent-foreground' : 'bg-muted/80 text-muted-foreground hover:bg-accent/40 hover:text-foreground'"
          title="Insert reference from upstream node"
          @click.prevent.stop="toggleContentPicker"
        >
          {x}
        </button>
        <ExpressionPicker
          v-if="hasUpstream"
          :open="contentPickerOpen"
          :node-id="props.nodeId"
          :anchor="contentTextareaEl"
          @select="onPickExpression"
          @close="contentPickerOpen = false"
        />
      </div>
    </label>

    <label
      v-if="isAgentResponse"
      class="flex min-w-0 flex-col gap-1 text-[11px] text-muted-foreground"
    >
      Render Mode
      <DropdownSelect
        v-model="renderMode"
        size="xs"
        class="w-full min-w-0 text-[11px]"
        :disabled="!isDesignMode || hydratingRef"
        :options="[
          { id: 'raw', label: 'Raw text', value: 'raw' },
          { id: 'markdown', label: 'Markdown', value: 'markdown' },
          { id: 'html', label: 'HTML', value: 'html' },
        ]"
      />
      <p class="text-[10px] text-faint-foreground">
        Choose how the response should be rendered inside the node.
      </p>
    </label>

    <div class="pt-1 flex items-center justify-end gap-2">
      <button
        class="rounded px-2 py-1 text-[11px] font-medium transition"
        :class="
          showAppliedFeedback
            ? 'bg-emerald-500 text-white shadow-[0_0_0_1px_rgba(16,185,129,0.3)]'
            : 'bg-accent text-accent-foreground'
        "
        :disabled="(!isDirty && !showAppliedFeedback) || !isDesignMode"
        @click="applyChanges"
      >
        {{ showAppliedFeedback ? 'Applied' : 'Apply' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, provide, ref, watch, type Ref } from "vue";
import { useVueFlow } from "@vue-flow/core";
import type { Edge } from "@vue-flow/core";
import type { StepNodeData } from "@/types/flow";
import type { FlowEditorStep } from "@/types/flowEditor";
import DropdownSelect from "@/components/DropdownSelect.vue";
import ExpressionPicker from "@/components/flow/ExpressionPicker.vue";

const TOOL_NAME_FALLBACK = "utility_textbox";
const AGENT_RESPONSE_TOOL = "agent_response";
type RenderMode = "raw" | "markdown" | "html";

const props = defineProps<{ nodeId: string; data: StepNodeData }>();
const { updateNodeData } = useVueFlow();
const modeRef = inject<Ref<"design" | "run">>(
  "flowEditorMode",
  ref<"design" | "run">("design"),
);
const hydratingRef = inject<Ref<boolean>>("flowEditorHydrating", ref(false));
const edgesRef = inject<Ref<Edge[]>>("flowEditorEdges", ref([]));

provide("flowEditorNodeId", props.nodeId);

const isDesignMode = computed(() => modeRef.value === "design");
const labelText = ref("");
const contentText = ref("");
const contentTextareaEl = ref<HTMLTextAreaElement | null>(null);
const contentPickerOpen = ref(false);
const renderMode = ref<RenderMode>("markdown");
const isDirty = ref(false);
const showAppliedFeedback = ref(false);
let appliedFeedbackTimer: ReturnType<typeof setTimeout> | null = null;

const hasUpstream = computed(() =>
  edgesRef.value.some((e) => e.target === props.nodeId),
);
const isExpressionContent = computed(() =>
  looksLikeExpression(contentText.value),
);

const toolName = computed(
  () => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK,
);
const isAgentResponse = computed(() => toolName.value === AGENT_RESPONSE_TOOL);

function parseRenderMode(value: unknown): RenderMode {
  const mode = typeof value === "string" ? (value as RenderMode) : "markdown";
  return mode === "raw" || mode === "html" || mode === "markdown"
    ? mode
    : "markdown";
}

function looksLikeExpression(value: unknown): boolean {
  if (typeof value !== "string") return false;
  return value.trim().startsWith("=");
}

let suppress = false;
watch(
  () => props.data?.step,
  (step) => {
    suppress = true;
    const args = (step?.tool?.args ?? {}) as Record<string, unknown>;
    labelText.value = String(args.label ?? step?.text ?? "");
    contentText.value = String(args.text ?? "");
    renderMode.value = parseRenderMode(args.render_mode);
    isDirty.value = false;
    suppress = false;
  },
  { immediate: true, deep: true },
);

watch(
  [labelText, contentText, renderMode],
  () => {
    if (suppress || hydratingRef.value || !isDesignMode.value) return;
    clearAppliedFeedback();
    isDirty.value = true;
  },
);

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return;
  const nextStep: FlowEditorStep = {
    ...(props.data?.step ?? ({} as FlowEditorStep)),
    id: props.nodeId,
    text: labelText.value.trim() || prettifyName(toolName.value),
    publish_result: Boolean(props.data?.step?.publish_result),
    tool: {
      name: toolName.value,
      args: buildArgs(),
    },
  };
  updateNodeData(props.nodeId, {
    ...(props.data ?? { order: 0, kind: "utility" }),
    step: cloneStep(nextStep),
  });
  isDirty.value = false;
  triggerAppliedFeedback();
}

function triggerAppliedFeedback() {
  showAppliedFeedback.value = true;
  if (appliedFeedbackTimer) clearTimeout(appliedFeedbackTimer);
  appliedFeedbackTimer = setTimeout(() => {
    showAppliedFeedback.value = false;
    appliedFeedbackTimer = null;
  }, 1400);
}

function clearAppliedFeedback() {
  showAppliedFeedback.value = false;
  if (appliedFeedbackTimer) {
    clearTimeout(appliedFeedbackTimer);
    appliedFeedbackTimer = null;
  }
}

function toggleContentPicker() {
  contentPickerOpen.value = !contentPickerOpen.value;
}

function onPickExpression(expression: string) {
  const current = contentText.value;
  contentText.value = current ? `${current}\n${expression}` : expression;
  isDirty.value = true;
}

function buildArgs(): Record<string, unknown> {
  const args: Record<string, unknown> = {};
  const label = labelText.value.trim();
  const text = contentText.value;
  if (label) args.label = label;
  if (text) args.text = text;
  if (isAgentResponse.value) args.render_mode = renderMode.value;
  return args;
}

function prettifyName(name: string): string {
  if (name === AGENT_RESPONSE_TOOL) return "Agent Response";
  if (!name.startsWith("utility_")) return name;
  return name
    .slice("utility_".length)
    .replace(/[_-]+/g, " ")
    .replace(/\b\w/g, (ch) => ch.toUpperCase());
}

function cloneStep(step: FlowEditorStep) {
  try {
    return JSON.parse(JSON.stringify(step)) as FlowEditorStep;
  } catch {
    return { ...step };
  }
}

onBeforeUnmount(() => {
  clearAppliedFeedback();
});
</script>
