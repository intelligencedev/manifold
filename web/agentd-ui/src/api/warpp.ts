import type { WarppStepTrace, WarppTool, WarppWorkflow } from "@/types/warpp";

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || "").replace(/\/$/, "");
const flowV2ApiBase = `${baseURL}/api/flows/v2`;

async function handleResponse<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(text || `request failed (${resp.status})`);
  }
  return (await resp.json()) as T;
}

export async function fetchWarppTools(): Promise<WarppTool[]> {
  const resp = await fetch(`${flowV2ApiBase}/tools`);
  return handleResponse<WarppTool[]>(resp);
}

export async function fetchWarppWorkflows(): Promise<WarppWorkflow[]> {
  const resp = await fetch(`${flowV2ApiBase}/workflows`);
  const raw = await handleResponse<FlowV2ListResponse>(resp);
  const ids = Array.isArray(raw.workflows)
    ? raw.workflows
        .map((w) => String(w?.id ?? "").trim())
        .filter((id) => !!id)
    : [];
  const workflows = await Promise.all(ids.map((id) => fetchWarppWorkflow(id)));
  return workflows;
}

export async function fetchWarppWorkflow(
  intent: string,
): Promise<WarppWorkflow> {
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(intent)}`,
  );
  const raw = await handleResponse<FlowV2GetWorkflowResponse>(resp);
  return flowV2ToWarpp(raw.workflow, raw.canvas);
}

export async function saveWarppWorkflow(
  workflow: WarppWorkflow,
): Promise<WarppWorkflow> {
  const payload = warppToFlowV2(workflow);
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(workflow.intent)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  );
  const saved = await handleResponse<FlowV2GetWorkflowResponse>(resp);
  return flowV2ToWarpp(saved.workflow, saved.canvas);
}

export async function deleteWarppWorkflow(intent: string): Promise<void> {
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(intent)}`,
    {
      method: "DELETE",
    },
  );
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(text || `request failed (${resp.status})`);
  }
}

export interface WarppRunResponse {
  result: string;
  trace: WarppStepTrace[];
}

export async function runWarppWorkflow(
  intent: string,
  prompt?: string,
  signal?: AbortSignal,
  projectId?: string,
): Promise<WarppRunResponse> {
  const resp = await fetch(`${flowV2ApiBase}/run`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(
      projectId && projectId.trim()
        ? {
            workflow_id: intent,
            input: buildRunInput(prompt),
            project_id: projectId.trim(),
          }
        : { workflow_id: intent, input: buildRunInput(prompt) },
    ),
    signal,
  });
  const start = await handleResponse<FlowV2RunResponse>(resp);
  const final = await waitForRunCompletion(start.run_id, signal);
  const trace = eventsToTrace(final.events ?? []);
  const result = extractRunResult(final.events ?? []);
  return { result, trace };
}

function buildRunInput(prompt?: string): Record<string, unknown> {
  const text = typeof prompt === "string" ? prompt.trim() : "";
  if (!text) return {};
  return {
    query: text,
    utter: text,
    prompt: text,
  };
}

async function waitForRunCompletion(
  runId: string,
  signal?: AbortSignal,
): Promise<FlowV2RunEventsResponse> {
  const startedAt = Date.now();
  while (true) {
    const resp = await fetch(
      `${flowV2ApiBase}/runs/${encodeURIComponent(runId)}/events`,
      { signal },
    );
    const payload = await handleResponse<FlowV2RunEventsResponse>(resp);
    if (payload.status && payload.status !== "running") return payload;
    if (Date.now() - startedAt > 120_000) {
      throw new Error(`run timed out while waiting for completion (${runId})`);
    }
    await sleepWithSignal(250, signal);
  }
}

function sleepWithSignal(ms: number, signal?: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new DOMException("The operation was aborted.", "AbortError"));
      return;
    }
    const timer = window.setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    const onAbort = () => {
      window.clearTimeout(timer);
      reject(new DOMException("The operation was aborted.", "AbortError"));
    };
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

function eventsToTrace(events: FlowV2RunEvent[]): WarppStepTrace[] {
  const byStep = new Map<string, WarppStepTrace>();
  for (const event of events) {
    const stepId = String(event.node_id ?? "").trim();
    if (!stepId) continue;
    const trace = byStep.get(stepId) ?? { stepId };
    switch (event.type) {
      case "node_completed":
        trace.status = "completed";
        if (isPlainObject(event.output)) {
          trace.delta = { ...event.output };
          const payload = event.output.payload;
          if (payload !== undefined) trace.payload = payload;
          if (isPlainObject(event.output.inputs))
            trace.renderedArgs = { ...event.output.inputs };
        }
        break;
      case "node_failed":
        trace.status = "error";
        trace.error =
          typeof event.error === "string"
            ? event.error
            : typeof event.message === "string"
              ? event.message
              : "node failed";
        break;
      case "node_skipped":
        trace.status = "skipped";
        break;
      default:
        // Keep existing status from prior events.
        break;
    }
    if (!trace.text && typeof event.message === "string") {
      trace.text = event.message;
    }
    byStep.set(stepId, trace);
  }
  return Array.from(byStep.values());
}

function extractRunResult(events: FlowV2RunEvent[]): string {
  let lastCompletedOutput: any = null;
  let runFailure: string | undefined;
  for (const event of events) {
    if (event.type === "node_completed" && isPlainObject(event.output)) {
      lastCompletedOutput = event.output;
    }
    if (event.type === "run_failed") {
      runFailure =
        typeof event.error === "string" && event.error.trim()
          ? event.error
          : typeof event.message === "string"
            ? event.message
            : "run failed";
    }
  }
  if (runFailure) return runFailure;
  if (isPlainObject(lastCompletedOutput)) {
    const preferredKeys = [
      "result",
      "text",
      "output",
      "llm_output",
      "report_md",
      "payload",
    ];
    for (const key of preferredKeys) {
      const v = (lastCompletedOutput as any)[key];
      if (typeof v === "string" && v.trim()) return v;
    }
    try {
      return JSON.stringify(lastCompletedOutput, null, 2);
    } catch {
      return "run completed";
    }
  }
  return "run completed";
}

function flowV2ToWarpp(
  workflow: FlowV2Workflow,
  canvas?: FlowV2WorkflowCanvas,
): WarppWorkflow {
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
    for (const [k, binding] of Object.entries(node.inputs ?? {})) {
      if (!binding || typeof binding !== "object") continue;
      if (typeof binding.expression === "string" && binding.expression.trim()) {
        args[k] = expressionToLegacy(binding.expression);
      } else if (Object.prototype.hasOwnProperty.call(binding, "literal")) {
        args[k] = binding.literal;
      }
    }
    const step: any = {
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
      step.tool = Object.keys(args).length ? { name: node.tool, args } : { name: node.tool };
    }
    return step;
  });

  const ui = {
    layout: canvas?.nodes
      ? Object.fromEntries(
          Object.entries(canvas.nodes).map(([id, n]) => [
            id,
            {
              x: Number(n?.x ?? 0),
              y: Number(n?.y ?? 0),
              width:
                typeof n?.width === "number" ? n.width : undefined,
              height:
                typeof n?.height === "number" ? n.height : undefined,
            },
          ]),
        )
      : undefined,
    parents: canvas?.parents ?? undefined,
    groups: Array.isArray(canvas?.groups)
      ? canvas?.groups.map((g) => ({
          id: g.id,
          label: g.label,
          color: g.color,
          collapsed: g.collapsed,
        }))
      : undefined,
    notes: Array.isArray(canvas?.notes)
      ? canvas?.notes.map((n) => ({
          id: n.id,
          label: n.label,
          color: n.color,
          note: n.note,
        }))
      : undefined,
  };
  const out: WarppWorkflow = {
    intent: workflow.id,
    description: workflow.description || workflow.name || workflow.id,
    keywords: Array.isArray(workflow.keywords) ? workflow.keywords : [],
    project_id:
      typeof workflow.project_id === "string" ? workflow.project_id : undefined,
    max_concurrency:
      typeof workflow.settings?.max_concurrency === "number"
        ? workflow.settings.max_concurrency
        : undefined,
    fail_fast: false,
    steps,
    ui,
  };
  return out;
}

function warppToFlowV2(workflow: WarppWorkflow): FlowV2PutWorkflowRequest {
  const nodes: FlowV2Node[] = (workflow.steps ?? []).map((step) => {
    const args = (step.tool?.args ?? {}) as Record<string, unknown>;
    const inputs: Record<string, FlowV2InputBinding> = {};
    for (const [k, v] of Object.entries(args)) {
      if (typeof v === "string") {
        const expr = legacyToExpression(v);
        if (expr) {
          inputs[k] = { expression: expr };
          continue;
        }
      }
      inputs[k] = { literal: v };
    }
    const node: FlowV2Node = {
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
    return node;
  });
  const edges: FlowV2Edge[] = [];
  for (const step of workflow.steps ?? []) {
    for (const dep of step.depends_on ?? []) {
      edges.push({
        id: `e-${dep}-${step.id}`,
        source: { node_id: dep, port: "output" },
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
      trigger: { type: "manual" },
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
            Object.entries(workflow.ui.layout).map(([id, l]) => [
              id,
              {
                x: l.x,
                y: l.y,
                width: typeof l.width === "number" ? l.width : undefined,
                height: typeof l.height === "number" ? l.height : undefined,
              },
            ]),
          )
        : undefined,
      parents: workflow.ui?.parents,
      groups: workflow.ui?.groups?.map((g) => ({
        id: g.id,
        label: g.label,
        color: g.color,
        collapsed: g.collapsed,
      })),
      notes: workflow.ui?.notes?.map((n) => ({
        id: n.id,
        label: n.label,
        note: n.note,
        color: n.color,
      })),
    },
  };
}

function legacyToExpression(value: string): string | undefined {
  const trimmed = value.trim();
  const m = /^\$\{A\.([^}]+)\}$/.exec(trimmed);
  if (!m) return undefined;
  const path = m[1];
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

function expressionToLegacy(expr: string): string {
  const normalized = normalizeExpr(expr);
  if (normalized.startsWith("$run.input.")) {
    const path = normalized.replace("$run.input.", "");
    return `\${A.${path}}`;
  }
  if (normalized === "$run.input") return "${A.query}";
  if (normalized.startsWith("$node.")) {
    const rest = normalized.replace("$node.", "");
    const firstDot = rest.indexOf(".");
    if (firstDot > 0) {
      const nodeId = rest.slice(0, firstDot);
      const tail = rest.slice(firstDot + 1);
      if (tail === "output") return `\${A.${nodeId}}`;
      if (tail.startsWith("output.")) {
        const path = tail.replace("output.", "");
        return `\${A.${nodeId}.${path}}`;
      }
    }
  }
  return expr;
}

function normalizeExpr(expr: string): string {
  let out = (expr ?? "").trim();
  if (out.startsWith("=")) out = out.slice(1).trim();
  if (out.startsWith("{{") && out.endsWith("}}") && out.length >= 4) {
    out = out.slice(2, -2).trim();
  }
  return out;
}

function isPlainObject(value: unknown): value is Record<string, any> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

interface FlowV2WorkflowSummary {
  id: string;
  name?: string;
  description?: string;
}

interface FlowV2ListResponse {
  workflows: FlowV2WorkflowSummary[];
}

interface FlowV2GetWorkflowResponse {
  workflow: FlowV2Workflow;
  canvas?: FlowV2WorkflowCanvas;
}

interface FlowV2PutWorkflowRequest {
  workflow: FlowV2Workflow;
  canvas?: FlowV2WorkflowCanvas;
}

interface FlowV2RunResponse {
  run_id: string;
  status: string;
}

interface FlowV2RunEventsResponse {
  run_id: string;
  status: string;
  events: FlowV2RunEvent[];
}

interface FlowV2RunEvent {
  run_id?: string;
  sequence?: number;
  type?: string;
  node_id?: string;
  status?: string;
  message?: string;
  output?: Record<string, any>;
  error?: string;
  occurred_at?: string;
}

interface FlowV2Workflow {
  id: string;
  name: string;
  description?: string;
  keywords?: string[];
  project_id?: string;
  trigger: {
    type: string;
  };
  nodes: FlowV2Node[];
  edges?: FlowV2Edge[];
  settings?: {
    max_concurrency?: number;
  };
}

interface FlowV2Node {
  id: string;
  name: string;
  kind: string;
  type: string;
  guard?: string;
  tool?: string;
  publish_result?: boolean;
  publish_mode?: "immediate" | "topo";
  inputs?: Record<string, FlowV2InputBinding>;
  execution?: {
    on_error?: "fail" | "continue";
  };
}

interface FlowV2InputBinding {
  literal?: unknown;
  expression?: string;
}

interface FlowV2Edge {
  id?: string;
  source: {
    node_id: string;
    port: string;
  };
  target: {
    node_id: string;
    port: string;
  };
}

interface FlowV2WorkflowCanvas {
  nodes?: Record<
    string,
    { x: number; y: number; width?: number; height?: number }
  >;
  parents?: Record<string, string>;
  groups?: Array<{ id: string; label: string; color?: string; collapsed?: boolean }>;
  notes?: Array<{ id: string; label?: string; note?: string; color?: string }>;
}
