<template>
  <div class="space-y-3">
    <div class="flex items-center justify-between">
      <div class="text-xs text-subtle-foreground">Configure utility</div>
      <span v-if="isDirty" class="text-[10px] italic text-warning-foreground"
        >Unsaved</span
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
      <div class="flex items-center justify-between gap-2">
        <span>Textbox Content</span>
        <div class="inline-flex rounded border border-border/60 bg-surface-muted p-0.5">
          <button
            type="button"
            class="rounded px-2 py-0.5 text-[10px]"
            :class="contentMode === 'literal' ? 'bg-accent text-accent-foreground' : 'text-subtle-foreground'"
            :disabled="!isDesignMode || hydratingRef"
            @click="setContentMode('literal')"
          >
            Literal
          </button>
          <button
            type="button"
            class="rounded px-2 py-0.5 text-[10px]"
            :class="contentMode === 'binding' ? 'bg-accent text-accent-foreground' : 'text-subtle-foreground'"
            :disabled="!isDesignMode || hydratingRef"
            @click="setContentMode('binding')"
          >
            Binding
          </button>
        </div>
      </div>
      <textarea
        v-model="contentText"
        rows="4"
        class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground overflow-auto w-full h-[92px] resize-none whitespace-pre-wrap break-words"
        :placeholder="contentPlaceholder"
        :disabled="!isDesignMode || hydratingRef"
      />
      <p class="text-[10px] text-faint-foreground">
        {{ contentHelpText }}
      </p>
    </label>

    <label
      v-if="isAgentResponse"
      class="flex flex-col gap-1 text-[11px] text-muted-foreground"
    >
      Render Mode
      <DropdownSelect
        v-model="renderMode"
        size="xs"
        class="text-[11px]"
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

    <details class="mt-1" v-if="isDesignMode">
      <summary class="cursor-pointer text-[11px] text-subtle-foreground">
        Advanced (promote to attribute)
      </summary>
      <div class="mt-2 space-y-2">
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Attribute
          <input
            v-model="outputAttr"
            type="text"
            class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
            :placeholder="`Defaults to ${defaultAttributeHint}`"
          />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output From
          <input
            v-model="outputFrom"
            type="text"
            class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
            placeholder="payload | json.<path> | delta.<key> | inputs.<key>"
          />
        </label>
        <label class="flex flex-col gap-1 text-[11px] text-muted-foreground">
          Output Value
          <input
            v-model="outputValue"
            type="text"
            class="rounded border border-border/60 bg-surface-muted px-2 py-1 text-[11px] text-foreground"
            placeholder="Literal override"
          />
        </label>
        <p class="text-[10px] text-faint-foreground">
          When left blank the value is published as
          <code>{{ defaultAttributeHint }}</code
          >.
        </p>
      </div>
    </details>

    <div class="pt-1 flex items-center justify-end gap-2">
      <button
        class="rounded bg-accent px-2 py-1 text-[11px] font-medium text-accent-foreground transition disabled:opacity-40"
        :disabled="!isDirty || !isDesignMode"
        @click="applyChanges"
      >
        Apply
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, inject, ref, watch, type Ref } from "vue";
import { useVueFlow } from "@vue-flow/core";
import type { StepNodeData } from "@/types/flow";
import type { FlowEditorStep } from "@/types/flowEditor";
import DropdownSelect from "@/components/DropdownSelect.vue";

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

const isDesignMode = computed(() => modeRef.value === "design");
const labelText = ref("");
const contentText = ref("");
const contentMode = ref<"literal" | "binding">("literal");
const renderMode = ref<RenderMode>("markdown");
const outputAttr = ref("");
const outputFrom = ref("");
const outputValue = ref("");
const isDirty = ref(false);
const bindingExample = "={{$run.input.query}}";
const contentPlaceholder = computed(() =>
  contentMode.value === "binding" ? bindingExample : "Enter static text",
);
const contentHelpText = computed(() =>
  contentMode.value === "binding"
    ? `Binding mode stores a Flow expression such as ${bindingExample}.`
    : "Literal mode stores plain text exactly as written.",
);

const toolName = computed(
  () => props.data?.step?.tool?.name ?? TOOL_NAME_FALLBACK,
);
const isAgentResponse = computed(() => toolName.value === AGENT_RESPONSE_TOOL);
const defaultAttributeHint = computed(() => `${props.nodeId}_text`);

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
    contentMode.value = looksLikeExpression(args.text) ? "binding" : "literal";
    renderMode.value = parseRenderMode(args.render_mode);
    outputAttr.value =
      typeof args.output_attr === "string" ? (args.output_attr as string) : "";
    outputFrom.value =
      typeof args.output_from === "string" ? (args.output_from as string) : "";
    outputValue.value =
      typeof args.output_value === "string"
        ? (args.output_value as string)
        : "";
    isDirty.value = false;
    suppress = false;
  },
  { immediate: true, deep: true },
);

watch(
  [labelText, contentText, outputAttr, outputFrom, outputValue, renderMode],
  () => {
    if (suppress || hydratingRef.value || !isDesignMode.value) return;
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
}

function buildArgs(): Record<string, unknown> {
  const args: Record<string, unknown> = {};
  const label = labelText.value.trim();
  const text = contentText.value;
  const attr = outputAttr.value.trim();
  const from = outputFrom.value.trim();
  const val = outputValue.value.trim();
  if (label) args.label = label;
  if (text) args.text = text;
  if (isAgentResponse.value) args.render_mode = renderMode.value;
  if (attr) args.output_attr = attr;
  if (from) args.output_from = from;
  if (val) args.output_value = val;
  return args;
}

function setContentMode(mode: "literal" | "binding") {
  if (!isDesignMode.value || hydratingRef.value) return;
  if (contentMode.value === mode) return;
  contentMode.value = mode;
  isDirty.value = true;
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
</script>
