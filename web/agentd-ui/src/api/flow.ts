import type {
  FlowRunResult,
  FlowStepTrace,
  FlowV2GetWorkflowResponse,
  FlowV2ListResponse,
  FlowV2PutWorkflowRequest,
  FlowV2RunEvent,
  FlowV2RunEventsResponse,
  FlowV2RunResponse,
  FlowV2Tool,
} from "@/types/flowV2";

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || "").replace(/\/$/, "");
const flowV2ApiBase = `${baseURL}/api/flows/v2`;

async function handleResponse<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(text || `request failed (${resp.status})`);
  }
  return (await resp.json()) as T;
}

export async function fetchFlowTools(): Promise<FlowV2Tool[]> {
  const resp = await fetch(`${flowV2ApiBase}/tools`);
  return handleResponse<FlowV2Tool[]>(resp);
}

export async function fetchFlowWorkflowList(): Promise<FlowV2ListResponse> {
  const resp = await fetch(`${flowV2ApiBase}/workflows`);
  return handleResponse<FlowV2ListResponse>(resp);
}

export async function fetchFlowWorkflow(
  workflowId: string,
): Promise<FlowV2GetWorkflowResponse> {
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(workflowId)}`,
  );
  return handleResponse<FlowV2GetWorkflowResponse>(resp);
}

export async function saveFlowWorkflow(
  workflowId: string,
  payload: FlowV2PutWorkflowRequest,
): Promise<FlowV2GetWorkflowResponse> {
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(workflowId)}`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  );
  return handleResponse<FlowV2GetWorkflowResponse>(resp);
}

export async function deleteFlowWorkflow(workflowId: string): Promise<void> {
  const resp = await fetch(
    `${flowV2ApiBase}/workflows/${encodeURIComponent(workflowId)}`,
    { method: "DELETE" },
  );
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(text || `request failed (${resp.status})`);
  }
}

export async function runFlowWorkflow(
  workflowId: string,
  prompt?: string,
  signal?: AbortSignal,
  projectId?: string,
): Promise<FlowRunResult> {
  const resp = await fetch(`${flowV2ApiBase}/run`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(
      projectId && projectId.trim()
        ? {
            workflow_id: workflowId,
            input: buildRunInput(prompt),
            project_id: projectId.trim(),
          }
        : { workflow_id: workflowId, input: buildRunInput(prompt) },
    ),
    signal,
  });
  const start = await handleResponse<FlowV2RunResponse>(resp);
  const final = await waitForRunCompletion(start.run_id, signal);
  return {
    result: extractRunResult(final.events ?? []),
    trace: eventsToTrace(final.events ?? []),
  };
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

function eventsToTrace(events: FlowV2RunEvent[]): FlowStepTrace[] {
  const byStep = new Map<string, FlowStepTrace>();
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
          if (isPlainObject(event.output.inputs)) {
            trace.renderedArgs = { ...event.output.inputs };
          }
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
    const preferredKeys = ["result", "text", "output", "llm_output", "report_md", "payload"];
    for (const key of preferredKeys) {
      const value = lastCompletedOutput[key];
      if (typeof value === "string" && value.trim()) return value;
    }
    try {
      return JSON.stringify(lastCompletedOutput, null, 2);
    } catch {
      return "run completed";
    }
  }
  return "run completed";
}

function isPlainObject(value: unknown): value is Record<string, any> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}