<template>
  <WarppBaseNode
    :collapsed="collapsed"
    :min-width="collapsed ? WARPP_STEP_NODE_COLLAPSED.width : STEP_MIN_WIDTH"
    :min-height="collapsed ? WARPP_STEP_NODE_COLLAPSED.height : STEP_MIN_HEIGHT"
    :min-width-px="nodeMinWidthPx"
    :min-height-px="nodeMinHeightPx"
    :show-resizer="isDesignMode"
    :show-back="showBack"
    :root-class="rootClass"
    :selected="props.selected"
    @resize-end="onResizeEnd"
  >
    <template #front>
      <!-- Header -->
      <div class="flex items-start justify-between gap-2">
        <div class="flex-1">
          <div class="flex items-center gap-2">
            <button
              class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
              :aria-expanded="!collapsed"
              :title="collapsed ? 'Expand' : 'Collapse'"
              @click.prevent.stop="toggleCollapsed"
            >
              <svg
                class="h-3.5 w-3.5 transition-transform"
                :class="collapsed ? '-rotate-90' : 'rotate-0'"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <polyline points="6 9 12 15 18 9"></polyline>
              </svg>
            </button>
            <div class="text-sm font-semibold text-foreground select-none">
              {{ headerLabel }}
            </div>
          </div>
        </div>
        <div class="flex items-center gap-1">
          <span
            v-show="!collapsed"
            class="text-[10px] uppercase tracking-wide text-faint-foreground"
            >#{{ orderLabel }}</span
          >
          <button
            class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
            title="Advanced (promote to attribute)"
            aria-label="Advanced (promote to attribute)"
            @click.prevent.stop="toggleBack(true)"
          >
            <GearIcon class="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
      <!-- Step ID chip row (below header, always visible) -->
      <div class="mt-1 flex items-center justify-between gap-2">
        <button
          class="hidden sm:inline-flex max-w-[200px] items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-mono text-foreground/80 hover:bg-muted/60"
          :title="copied ? 'Copied!' : `Copy step id: ${props.id}`"
          @click.prevent.stop="copyStepId"
        >
          <svg
            v-if="!copied"
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="h-3.5 w-3.5"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
            <path
              d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
            ></path>
          </svg>
          <svg
            v-else
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 24 24"
            class="h-3.5 w-3.5 text-green-500"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <polyline points="20 6 9 17 4 12"></polyline>
          </svg>
          <span class="truncate">{{ props.id }}</span>
        </button>
      </div>

      <!-- Content -->
      <div class="mt-3" :class="collapsed ? 'hidden' : ''">
        <div class="space-y-2">
          <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
            Step Text
            <input
              v-model="stepText"
              type="text"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
              placeholder="Describe this step"
              :disabled="!isDesignMode"
              @keydown.meta.enter.prevent="applyChanges"
              @keydown.ctrl.enter.prevent="applyChanges"
            />
          </label>
          <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
            Guard
            <input
              v-model="guardText"
              type="text"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
              placeholder="Example: A.os != 'windows'"
              :disabled="!isDesignMode"
              @keydown.meta.enter.prevent="applyChanges"
              @keydown.ctrl.enter.prevent="applyChanges"
            />
          </label>
          <label
            class="flex items-center gap-2 text-[11px] text-muted-foreground"
          >
            <input
              v-model="publishResult"
              type="checkbox"
              class="accent-accent"
              :disabled="!isDesignMode"
            />
            Publish result
          </label>
        </div>

        <div v-if="showParamsSection" class="mt-3 space-y-2">
          <div class="text-[11px] font-semibold text-muted-foreground">
            Parameters
          </div>
          <ParameterFormField
            v-if="isDesignMode && parameterSchemaFiltered"
            :schema="parameterSchemaFiltered"
            :model-value="argsState"
            @update:model-value="onArgsUpdate"
          />
          <p
            v-else-if="isDesignMode && toolName"
            class="text-[11px] italic text-faint-foreground"
          >
            This tool has no configurable parameters.
          </p>
          <p
            v-else-if="isDesignMode"
            class="text-[11px] italic text-faint-foreground"
          >
            Select a tool to edit parameters.
          </p>
          <div v-else class="space-y-1 text-[11px] text-muted-foreground">
            <template v-if="runtimeArgs.length">
              <div
                v-for="([key, value], index) in runtimeArgs"
                :key="`${key}-${index}`"
                class="flex items-start gap-2"
              >
                <span class="min-w-[72px] font-semibold text-foreground">{{
                  key
                }}</span>
                <span
                  class="block flex-1 min-w-0 max-h-[6rem] overflow-auto whitespace-pre-wrap break-words text-foreground/80"
                  style="overflow-wrap: anywhere"
                >
                  {{ formatRuntimeValue(value) }}
                </span>
              </div>
            </template>
            <p
              v-else-if="runtimeStatus === 'pending'"
              class="italic text-faint-foreground"
            >
              Waiting for execution…
            </p>
            <p v-else class="italic text-faint-foreground">
              Run the workflow to see resolved values.
            </p>
            <p
              v-if="runtimeStatusMessage"
              class="italic text-faint-foreground break-words"
              style="overflow-wrap: anywhere"
            >
              {{ runtimeStatusMessage }}
            </p>
          </div>
          <p
            v-if="runtimeError && runtimeStatus !== 'pending'"
            class="rounded border border-danger/40 bg-danger/10 px-2 py-1 text-[10px] text-danger-foreground"
          >
            <span
              class="block max-h-[6rem] overflow-auto whitespace-pre-wrap break-words"
              style="overflow-wrap: anywhere"
              >{{ runtimeError }}</span
            >
          </p>
        </div>

        <div
          v-show="isDesignMode"
          class="mt-4 flex items-center justify-end gap-2"
        >
          <span
            v-if="isDirty"
            class="text-[10px] italic text-warning-foreground"
            >Unsaved</span
          >
          <button
            class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
            :disabled="!isDirty"
            @click="applyChanges"
            title="Apply changes (Cmd/Ctrl+Enter)"
          >
            Apply
          </button>
        </div>

        <div
          v-show="!isDesignMode && hasRuntimeDetails"
          class="mt-3 flex items-center justify-end"
        >
          <button
            type="button"
            class="text-[11px] font-medium text-accent underline decoration-dotted underline-offset-2 transition hover:text-accent-foreground"
            @click="viewRuntimeDetails"
          >
            View details
          </button>
        </div>
      </div>
    </template>

    <template #back>
      <!-- Back header -->
      <div class="flex items-start justify-between gap-2">
        <span class="text-[10px] uppercase tracking-wide text-faint-foreground"
          >Advanced • Promote to attribute (optional)</span
        >
        <button
          class="inline-flex h-5 w-5 items-center justify-center rounded hover:bg-muted/60 text-foreground/80"
          title="Back"
          aria-label="Back"
          @click.prevent.stop="toggleBack(false)"
        >
          <GearIcon class="h-3.5 w-3.5" />
        </button>
      </div>

      <!-- Back content -->
      <div class="mt-3" :class="collapsed ? 'hidden' : ''">
        <div class="space-y-2">
          <p class="text-[10px] text-faint-foreground">
            Prefer referencing prior step data with
            <code>{{ `\${A.${props.id}.json...}` }}</code
            >. Promote to an attribute when you want a short, stable name
            (useful for guards and reuse).
          </p>
          <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
            Output Attribute
            <input
              v-model="outputAttr"
              type="text"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
              placeholder="e.g. result"
              :disabled="!isDesignMode"
              @input="markDirty"
            />
          </label>
          <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
            Output From
            <input
              v-model="outputFrom"
              type="text"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
              placeholder="payload | json.<path> | delta.<key> | args.<key>"
              :disabled="!isDesignMode"
              @input="markDirty"
            />
          </label>
          <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
            Output Value
            <input
              v-model="outputValue"
              type="text"
              class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
              placeholder="Literal override"
              :disabled="!isDesignMode"
              @input="markDirty"
            />
          </label>

          <div
            v-show="isDesignMode"
            class="pt-1 flex items-center justify-end gap-2"
          >
            <span
              v-if="isDirty"
              class="text-[10px] italic text-warning-foreground"
              >Unsaved</span
            >
            <button
              class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
              :disabled="!isDirty"
              @click="applyChanges"
            >
              Apply
            </button>
          </div>
        </div>
      </div>
    </template>
  </WarppBaseNode>
</template>

<script setup lang="ts">
import {
  computed,
  inject,
  ref,
  watch,
  onMounted,
  type CSSProperties,
} from "vue";
import { useVueFlow, type NodeProps } from "@vue-flow/core";
import type { OnResizeEnd } from "@vue-flow/node-resizer";

import WarppBaseNode from "./WarppBaseNode.vue";
import ParameterFormField from "@/components/flow/ParameterFormField.vue";
import type { StepNodeData } from "@/types/flow";
import type { WarppTool, WarppStepTrace } from "@/types/warpp";
import type { Ref } from "vue";
import GearIcon from "@/components/icons/Gear.vue";
import {
  WARPP_STEP_NODE_DIMENSIONS,
  WARPP_STEP_NODE_COLLAPSED,
} from "@/constants/warppNodes";

const props = defineProps<NodeProps<StepNodeData>>();

const { updateNodeData, updateNode } = useVueFlow();

const STEP_MIN_WIDTH = WARPP_STEP_NODE_DIMENSIONS.minWidth;
const STEP_MIN_HEIGHT = WARPP_STEP_NODE_DIMENSIONS.minHeight;
const nodeMinWidthPx = computed(() =>
  collapsed.value
    ? `${WARPP_STEP_NODE_COLLAPSED.width}px`
    : `${STEP_MIN_WIDTH}px`,
);
const nodeMinHeightPx = computed(() =>
  collapsed.value
    ? `${WARPP_STEP_NODE_COLLAPSED.height}px`
    : `${STEP_MIN_HEIGHT}px`,
);

const toolsRef = inject<Ref<WarppTool[]>>("warppTools", ref<WarppTool[]>([]));
const hydratingRef = inject<Ref<boolean>>("warppHydrating", ref(false));
const modeRef = inject<Ref<"design" | "run">>(
  "warppMode",
  ref<"design" | "run">("design"),
);
const runTraceRef = inject<Ref<Record<string, WarppStepTrace>>>(
  "warppRunTrace",
  ref<Record<string, WarppStepTrace>>({}),
);
const runningRef = inject<Ref<boolean>>("warppRunning", ref(false));
const openResultModal = inject<(stepId: string, title: string) => void>(
  "warppOpenResultModal",
  () => {},
);

const toolOptions = computed(() => {
  const options = [...(toolsRef?.value ?? [])];
  const current = props.data?.step?.tool?.name;
  if (current && !options.some((tool) => tool.name === current)) {
    options.push({ name: current });
  }
  return options;
});

const stepText = ref("");
const guardText = ref("");
const publishResult = ref(false);
const toolName = ref("");
const argsState = ref<Record<string, unknown>>({});
const isDirty = ref(false);
const collapsed = ref(true);
const rootClass = computed(() => [
  collapsed.value
    ? "min-w-[160px] min-h-[72px]"
    : "min-w-[320px] min-h-[260px] h-full",
  "transition-colors duration-150 ease-out",
]);
const showBack = ref(false);
const outputAttr = ref("");
const outputFrom = ref("");
const outputValue = ref("");
const copied = ref(false);

const orderLabel = computed(() => (props.data?.order ?? 0) + 1);
const isDesignMode = computed(() => modeRef.value === "design");
const runtimeTrace = computed(() => {
  const rec = runTraceRef.value;
  if (!rec || typeof rec !== "object") return undefined;
  return rec[props.id];
});
const runtimeArgs = computed(() => {
  const trace = runtimeTrace.value;
  if (!trace?.renderedArgs) {
    return [] as Array<[string, unknown]>;
  }
  return Object.entries(trace.renderedArgs as Record<string, unknown>).filter(
    ([key]) => !OUTPUT_KEYS.has(key),
  );
});
const runtimeError = computed(() => runtimeTrace.value?.error);
const runtimeStatus = computed(() => {
  if (runtimeTrace.value?.status) return runtimeTrace.value.status;
  if (modeRef.value === "run" && runningRef.value && !runtimeTrace.value)
    return "pending";
  return undefined;
});
const runtimeStatusMessage = computed(() => {
  const trace = runtimeTrace.value;
  if (!trace) return undefined;
  switch (trace.status) {
    case "skipped":
      return "Guard prevented execution.";
    case "noop":
      return "Step has no tool configured.";
    case "error":
      return "Step encountered an error.";
    default:
      return undefined;
  }
});
const hasRuntimeDetails = computed(() => Boolean(runtimeTrace.value));

const currentTool = computed(
  () => toolOptions.value.find((tool) => tool.name === toolName.value) ?? null,
);
const parameterSchema = computed(() => currentTool.value?.parameters ?? null);
const OUTPUT_KEYS = new Set(["output_attr", "output_from", "output_value"]);

const parameterSchemaFiltered = computed(() => {
  const schema = parameterSchema.value as any;
  if (!schema || typeof schema !== "object") return schema;
  const cloned: any = { ...schema };
  if (schema.properties && typeof schema.properties === "object") {
    cloned.properties = { ...schema.properties };
    for (const k of Object.keys(cloned.properties)) {
      if (OUTPUT_KEYS.has(k)) delete cloned.properties[k];
    }
    if (Object.keys(cloned.properties).length === 0) {
      // No visible fields left; hide the parameters form entirely.
      return null;
    }
  }
  if (Array.isArray(schema.required)) {
    cloned.required = schema.required.filter(
      (k: string) => !OUTPUT_KEYS.has(k),
    );
  }
  return cloned;
});

const showParamsSection = computed(() => {
  if (isDesignMode.value) return Boolean(parameterSchemaFiltered.value);
  // In run mode we show runtime args if there are any non-output keys
  return (
    runtimeArgs.value.length > 0 ||
    Boolean(runtimeStatusMessage.value) ||
    Boolean(runtimeError.value)
  );
});

const headerLabel = computed(() => currentTool.value?.name ?? "Workflow Step");

let suppressCommit = false;
let suppressToolReset = false;

// In run mode we want full scrollable values, no truncation

watch(
  () => props.data?.step,
  (nextStep) => {
    suppressCommit = true;
    suppressToolReset = true;
    stepText.value = nextStep?.text ?? "";
    guardText.value = nextStep?.guard ?? "";
    publishResult.value = Boolean(nextStep?.publish_result);
    toolName.value = nextStep?.tool?.name ?? "";
    argsState.value = cloneArgs(nextStep?.tool?.args);
    // Strip output config keys from the front-side args editor state
    if (argsState.value && typeof argsState.value === "object") {
      for (const k of OUTPUT_KEYS) {
        if (k in (argsState.value as Record<string, unknown>)) {
          delete (argsState.value as Record<string, unknown>)[k];
        }
      }
    }
    const a = (nextStep?.tool?.args ?? {}) as Record<string, unknown>;
    outputAttr.value =
      typeof a.output_attr === "string" ? (a.output_attr as string) : "";
    outputFrom.value =
      typeof a.output_from === "string" ? (a.output_from as string) : "";
    outputValue.value =
      typeof a.output_value === "string" ? (a.output_value as string) : "";
    suppressCommit = false;
  },
  { immediate: true, deep: true },
);

watch(
  [
    stepText,
    guardText,
    publishResult,
    toolName,
    outputAttr,
    outputFrom,
    outputValue,
  ],
  () => markDirty(),
);
watch(argsState, () => markDirty(), { deep: true });

function markDirty() {
  if (suppressCommit || hydratingRef.value || !isDesignMode.value) return;
  isDirty.value = true;
}

function onArgsUpdate(value: unknown) {
  if (value && typeof value === "object" && !Array.isArray(value))
    argsState.value = value as Record<string, unknown>;
  else argsState.value = {};
  markDirty();
}

function commit() {
  if (hydratingRef.value || !isDesignMode.value) {
    return;
  }
  const toolPayload = buildToolPayload(toolName.value, argsState.value);
  // Merge output config into args
  if (toolPayload) {
    const merged: Record<string, unknown> = { ...(toolPayload.args ?? {}) };
    const oa = outputAttr.value.trim();
    const of = outputFrom.value.trim();
    const ov = outputValue.value.trim();
    if (oa) merged.output_attr = oa;
    if (of) merged.output_from = of;
    if (ov) merged.output_value = ov;
    if (Object.keys(merged).length) toolPayload.args = merged;
  }
  const nextStep = {
    ...(props.data?.step ?? {}),
    id: props.id,
    text: stepText.value,
    guard: guardText.value.trim() ? guardText.value.trim() : undefined,
    publish_result: publishResult.value,
    tool: toolPayload,
  };
  // Skip update if nothing changed (shallow compare key fields + JSON fallback for args)
  const prev = props.data?.step;
  if (prev) {
    const same =
      prev.text === nextStep.text &&
      prev.guard === nextStep.guard &&
      Boolean(prev.publish_result) === Boolean(nextStep.publish_result) &&
      (prev.tool?.name || "") === (nextStep.tool?.name || "") &&
      JSON.stringify(prev.tool?.args || {}) ===
        JSON.stringify(nextStep.tool?.args || {});
    if (same) {
      return;
    }
  }
  updateNodeData(props.id, {
    ...(props.data ?? { order: 0 }),
    step: cloneStep(nextStep),
  });
}

function applyChanges() {
  if (!isDesignMode.value || !isDirty.value) return;
  commit();
  isDirty.value = false;
}

function toggleCollapsed() {
  applyCollapsedStyle(!collapsed.value);
}

function toggleBack(v?: boolean) {
  showBack.value = typeof v === "boolean" ? v : !showBack.value;
}

function onResizeEnd(event: OnResizeEnd) {
  if (!isDesignMode.value) return;
  const widthPx = `${Math.round(event.params.width)}px`;
  const heightPx = `${Math.round(event.params.height)}px`;
  updateNode(props.id, (node) => {
    const baseStyle: CSSProperties =
      typeof node.style === "function"
        ? ((node.style(node) as CSSProperties) ?? {})
        : { ...(node.style ?? {}) };
    return {
      style: {
        ...baseStyle,
        width: widthPx,
        height: heightPx,
      },
    };
  });
  isDirty.value = true;
}

function formatRuntimeValue(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value;
  if (typeof value === "number" || typeof value === "boolean")
    return String(value);
  try {
    return JSON.stringify(value);
  } catch (err) {
    console.warn("Failed to stringify runtime value", err);
    return String(value);
  }
}

function viewRuntimeDetails() {
  if (!runtimeTrace.value) return;
  openResultModal(props.id, headerLabel.value);
}

async function copyStepId() {
  try {
    await navigator.clipboard.writeText(props.id);
    copied.value = true;
    setTimeout(() => (copied.value = false), 1200);
  } catch (err) {
    // best-effort: fall back to prompt
    window.prompt("Copy step id", props.id);
  }
}

// Global expand/collapse signals injected from FlowView
const collapseAllSeq = inject<Ref<number>>("warppCollapseAllSeq", ref(0));
const expandAllSeq = inject<Ref<number>>("warppExpandAllSeq", ref(0));
const lastCollapseSeen = ref(0);
const lastExpandSeen = ref(0);
watch(collapseAllSeq, (v) => {
  if (typeof v === "number" && v !== lastCollapseSeen.value) {
    lastCollapseSeen.value = v;
    applyCollapsedStyle(true);
  }
});
watch(expandAllSeq, (v) => {
  if (typeof v === "number" && v !== lastExpandSeen.value) {
    lastExpandSeen.value = v;
    applyCollapsedStyle(false);
  }
});

function buildToolPayload(name: string, args: Record<string, unknown>) {
  if (!name) {
    return undefined;
  }
  const pruned = pruneArgs(args);
  if (
    !pruned ||
    (typeof pruned === "object" && Object.keys(pruned).length === 0)
  ) {
    return { name };
  }
  return { name, args: pruned as Record<string, unknown> };
}

function pruneArgs(value: unknown): unknown {
  if (value === undefined || value === null) {
    return undefined;
  }
  if (Array.isArray(value)) {
    const prunedArray = value
      .map((item) => pruneArgs(item))
      .filter((item) => item !== undefined);
    return prunedArray.length ? prunedArray : undefined;
  }
  if (typeof value === "object") {
    const result: Record<string, unknown> = {};
    Object.entries(value as Record<string, unknown>).forEach(([key, val]) => {
      const pruned = pruneArgs(val);
      if (pruned !== undefined) {
        result[key] = pruned;
      }
    });
    return Object.keys(result).length ? result : undefined;
  }
  return value;
}

function cloneArgs(input: Record<string, unknown> | undefined) {
  if (!input) {
    return {};
  }
  try {
    return JSON.parse(JSON.stringify(input)) as Record<string, unknown>;
  } catch (err) {
    console.warn("Failed to clone args", err);
    return { ...input };
  }
}

function cloneStep(step: Record<string, unknown>) {
  try {
    return JSON.parse(JSON.stringify(step));
  } catch (err) {
    console.warn("Failed to clone step", err);
    return { ...step };
  }
}

// Remember last expanded size per node so we can restore on expand
const prevExpandedSize = new Map<string, { w: number; h: number }>();

function px(n: number) {
  return `${Math.round(n)}px`;
}

function applyCollapsedStyle(next: boolean) {
  const nodeId = props.id;
  // Update style dimensions so Vue Flow hit-testing matches visible size
  updateNode(nodeId, (node) => {
    const baseStyle: CSSProperties =
      typeof node.style === "function"
        ? ((node.style(node) as CSSProperties) ?? {})
        : { ...(node.style ?? {}) };

    if (next) {
      // store current explicit size to restore later
      const currW =
        typeof (baseStyle as any).width === "string"
          ? parseFloat((baseStyle as any).width as string)
          : undefined;
      const currH =
        typeof (baseStyle as any).height === "string"
          ? parseFloat((baseStyle as any).height as string)
          : undefined;
      if (currW && currH) prevExpandedSize.set(nodeId, { w: currW, h: currH });
      return {
        style: {
          ...baseStyle,
          width: px(WARPP_STEP_NODE_COLLAPSED.width),
          height: px(WARPP_STEP_NODE_COLLAPSED.height),
          minWidth: px(WARPP_STEP_NODE_COLLAPSED.width),
          minHeight: px(WARPP_STEP_NODE_COLLAPSED.height),
        },
      };
    }

    // expanding: try to restore previous explicit size, else defaults
    const restored = prevExpandedSize.get(nodeId);
    const targetW = restored?.w ?? WARPP_STEP_NODE_DIMENSIONS.defaultWidth;
    const targetH = restored?.h ?? WARPP_STEP_NODE_DIMENSIONS.defaultHeight;
    return {
      style: {
        ...baseStyle,
        width: px(targetW),
        height: px(targetH),
        minWidth: px(WARPP_STEP_NODE_DIMENSIONS.minWidth),
        minHeight: px(WARPP_STEP_NODE_DIMENSIONS.minHeight),
      },
    };
  });

  // Reflect state locally and on node data (ui-only)
  collapsed.value = next;
  const nextData = { ...(props.data ?? { order: 0 }), collapsed: next };
  updateNodeData(props.id, nextData);
}

// Sync with externally provided ui flag if present
watch(
  () => (props.data as any)?.collapsed,
  (next) => {
    if (typeof next === "boolean") {
      applyCollapsedStyle(next);
    }
  },
  { immediate: false },
);

// Apply initial collapsed dimensions so hitbox matches visual state
onMounted(() => {
  // default collapsed unless explicitly set false on data
  const initial =
    typeof (props.data as any)?.collapsed === "boolean"
      ? (props.data as any).collapsed
      : true;
  applyCollapsedStyle(initial);
});
</script>
