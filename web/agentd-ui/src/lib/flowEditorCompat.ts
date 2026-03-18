import type {
  FlowV2Edge,
  FlowV2InputBinding,
  FlowV2Node,
  FlowV2PutWorkflowRequest,
  FlowV2Tool,
  FlowV2Workflow,
  FlowV2WorkflowCanvas,
  FlowV2WorkflowSummary,
} from "@/types/flowV2";
import type { FlowEditorTool, FlowEditorWorkflow } from "@/types/flowEditor";

export type WorkflowListEntry = {
  intent: string;
  description?: string;
  keywords?: string[];
  project_id?: string;
};

export function flowToolToEditorTool(tool: FlowV2Tool): FlowEditorTool {
  return {
    name: tool.name,
    description: tool.description,
    parameters: tool.parameters,
  };
}

export function workflowToListEntry(workflow: Pick<
  FlowEditorWorkflow,
  "intent" | "description" | "keywords" | "project_id"
>): WorkflowListEntry {
  return {
    intent: workflow.intent,
    description: workflow.description,
    keywords: workflow.keywords,
    project_id: workflow.project_id,
  };
}

export function flowSummaryToListEntry(
  workflow: FlowV2WorkflowSummary,
): WorkflowListEntry {
  return {
    intent: workflow.id,
    description: workflow.description || workflow.name || workflow.id,
  };
}

export function flowV2ToEditorWorkflow(
  workflow: FlowV2Workflow,
  canvas?: FlowV2WorkflowCanvas,
): FlowEditorWorkflow {
  const incoming: Record<string, string[]> = {};
  for (const edge of workflow.edges ?? []) {
    const src = String(edge?.source?.node_id ?? "").trim();
    const target = String(edge?.target?.node_id ?? "").trim();
    if (!src || !target) continue;
    if (!incoming[target]) incoming[target] = [];
    incoming[target].push(src);
  }

  const steps = (workflow.nodes ?? []).map((node) => {
    const args: Record<string, unknown> = {};
    for (const [key, binding] of Object.entries(node.inputs ?? {})) {
      if (!binding || typeof binding !== "object") continue;
      if (typeof binding.expression === "string" && binding.expression.trim()) {
        args[key] = expressionToLegacy(binding.expression);
      } else if (Object.prototype.hasOwnProperty.call(binding, "literal")) {
        args[key] = binding.literal;
      }
    }

    const step: FlowEditorWorkflow["steps"][number] = {
      id: node.id,
      text: node.name || node.id,
      guard: node.guard || undefined,
      continue_on_error: node.execution?.on_error === "continue",
      publish_result: Boolean(node.publish_result),
      publish_mode:
        node.publish_mode === "immediate" || node.publish_mode === "topo"
          ? node.publish_mode
          : undefined,
      depends_on: incoming[node.id] ?? [],
    };

    if (node.type === "tool" && node.tool) {
      step.tool = Object.keys(args).length
        ? { name: node.tool, args }
        : { name: node.tool };
    }

    return step;
  });

  return {
    intent: workflow.id,
    description: workflow.description || workflow.name || workflow.id,
    keywords: Array.isArray(workflow.keywords) ? workflow.keywords : [],
    project_id:
      typeof workflow.project_id === "string" ? workflow.project_id : undefined,
    trigger: workflow.trigger,
    max_concurrency:
      typeof workflow.settings?.max_concurrency === "number"
        ? workflow.settings.max_concurrency
        : undefined,
    fail_fast: false,
    steps,
    ui: {
      layout: canvas?.nodes
        ? Object.fromEntries(
            Object.entries(canvas.nodes).map(([id, node]) => [
              id,
              {
                x: Number(node?.x ?? 0),
                y: Number(node?.y ?? 0),
                width: typeof node?.width === "number" ? node.width : undefined,
                height: typeof node?.height === "number" ? node.height : undefined,
                collapsed: typeof node?.collapsed === "boolean" ? node.collapsed : undefined,
                label: typeof node?.label === "string" ? node.label : undefined,
              },
            ]),
          )
        : undefined,
      parents: canvas?.parents ?? undefined,
      groups: Array.isArray(canvas?.groups)
        ? canvas.groups.map((group) => ({
            id: group.id,
            label: group.label,
            color: group.color,
            collapsed: group.collapsed,
          }))
        : undefined,
      notes: Array.isArray(canvas?.notes)
        ? canvas.notes.map((note) => ({
            id: note.id,
            label: note.label,
            color: note.color,
            note: note.note,
          }))
        : undefined,
    },
  };
}

export function editorWorkflowToFlowV2(
  workflow: FlowEditorWorkflow,
): FlowV2PutWorkflowRequest {
  const nodes: FlowV2Node[] = (workflow.steps ?? []).map((step) => {
    const args = (step.tool?.args ?? {}) as Record<string, unknown>;
    const inputs: Record<string, FlowV2InputBinding> = {};
    for (const [key, value] of Object.entries(args)) {
      if (typeof value === "string") {
        const expression = legacyToExpression(value);
        if (expression) {
          inputs[key] = { expression };
          continue;
        }
      }
      inputs[key] = { literal: value };
    }

    return {
      id: step.id,
      name: step.text || step.id,
      kind: step.tool?.name ? "action" : "data",
      type: step.tool?.name ? "tool" : "data",
      guard: step.guard,
      tool: step.tool?.name,
      publish_result: Boolean(step.publish_result),
      publish_mode: step.publish_mode,
      inputs,
      execution: {
        on_error: step.continue_on_error ? "continue" : "fail",
      },
    };
  });

  const edges: FlowV2Edge[] = [];
  for (const step of workflow.steps ?? []) {
    for (const dependency of step.depends_on ?? []) {
      edges.push({
        id: `e-${dependency}-${step.id}`,
        source: { node_id: dependency, port: "output" },
        target: { node_id: step.id, port: "input" },
      });
    }
  }

  return {
    workflow: {
      id: workflow.intent,
      name: workflow.description?.trim() || workflow.intent,
      description: workflow.description ?? "",
      keywords: workflow.keywords ?? [],
      project_id: workflow.project_id,
      trigger: workflow.trigger ?? { type: "manual" },
      nodes,
      edges,
      settings: {
        max_concurrency:
          typeof workflow.max_concurrency === "number"
            ? workflow.max_concurrency
            : undefined,
      },
    },
    canvas: {
      nodes: workflow.ui?.layout
        ? Object.fromEntries(
            Object.entries(workflow.ui.layout).map(([id, layout]) => [
              id,
              {
                x: layout.x,
                y: layout.y,
                width: typeof layout.width === "number" ? layout.width : undefined,
                height:
                  typeof layout.height === "number" ? layout.height : undefined,
                collapsed: typeof layout.collapsed === "boolean" ? layout.collapsed : undefined,
                label: layout.label || undefined,
              },
            ]),
          )
        : undefined,
      parents: workflow.ui?.parents,
      groups: workflow.ui?.groups?.map((group) => ({
        id: group.id,
        label: group.label,
        color: group.color,
        collapsed: group.collapsed,
      })),
      notes: workflow.ui?.notes?.map((note) => ({
        id: note.id,
        label: note.label,
        note: note.note,
        color: note.color,
      })),
    },
  };
}

function legacyToExpression(value: string): string | undefined {
  const trimmed = value.trim();
  if (looksLikeExpression(trimmed)) {
    return normalizeExpression(trimmed);
  }
  const match = /^\$\{A\.([^}]+)\}$/.exec(trimmed);
  if (!match) return undefined;
  const path = match[1];
  if (!path) return undefined;
  if (path === "query" || path === "utter" || path === "prompt") {
    return "={{$run.input.query}}";
  }
  const parts = path.split(".");
  if (parts.length >= 2) {
    const nodeId = parts[0];
    const rest = parts.slice(1).join(".");
    if (nodeId && rest) return `={{$node.${nodeId}.output.${rest}}}`;
  }
  return `={{$run.input.${path}}}`;
}

function expressionToLegacy(expression: string): string {
  return normalizeExpression(expression);
}

function normalizeExpr(expression: string): string {
  let out = (expression ?? "").trim();
  if (out.startsWith("=")) out = out.slice(1).trim();
  if (out.startsWith("{{") && out.endsWith("}}") && out.length >= 4) {
    out = out.slice(2, -2).trim();
  }
  return out;
}

function normalizeExpression(expression: string): string {
  const normalized = normalizeExpr(expression);
  return normalized ? `={{${normalized}}}` : expression;
}

function looksLikeExpression(value: string): boolean {
  return (
    value.startsWith("=") ||
    (value.startsWith("{{") && value.endsWith("}}")) ||
    value.startsWith("$run.") ||
    value.startsWith("$node.")
  );
}