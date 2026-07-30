import { baseURL } from "./clientCore";
import type {
  WarppCanvas,
  WarppCatalog,
  WarppDiagnostic,
  WarppDocument,
  WarppGetResponse,
  WarppRunEvent,
  WarppWorkflowSummary,
} from "@/types/warpp";

const warppBase = `${baseURL.replace(/\/$/, "")}/warpp`;

export class WarppValidationError extends Error {
  diagnostics: WarppDiagnostic[];
  constructor(diagnostics: WarppDiagnostic[]) {
    super("workflow validation failed");
    this.name = "WarppValidationError";
    this.diagnostics = diagnostics;
  }
}

async function asJSON<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    const text = await resp.text();
    throw new Error(text || `request failed (${resp.status})`);
  }
  return (await resp.json()) as T;
}

export function fetchCatalog(): Promise<WarppCatalog> {
  return fetch(`${warppBase}/catalog`).then((r) => asJSON<WarppCatalog>(r));
}

export function listWorkflows(): Promise<{ workflows: WarppWorkflowSummary[] }> {
  return fetch(`${warppBase}/workflows`).then((r) =>
    asJSON<{ workflows: WarppWorkflowSummary[] }>(r),
  );
}

export function getWorkflow(id: string): Promise<WarppGetResponse> {
  return fetch(`${warppBase}/workflows/${encodeURIComponent(id)}`).then((r) =>
    asJSON<WarppGetResponse>(r),
  );
}

export async function saveWorkflow(
  id: string,
  payload: { document: WarppDocument; canvas: WarppCanvas },
): Promise<WarppGetResponse> {
  const resp = await fetch(`${warppBase}/workflows/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (resp.status === 400) {
    const body = (await resp.json().catch(() => null)) as {
      diagnostics?: WarppDiagnostic[];
    } | null;
    if (body && body.diagnostics) {
      throw new WarppValidationError(body.diagnostics);
    }
  }
  return asJSON<WarppGetResponse>(resp);
}

export async function deleteWorkflow(id: string): Promise<void> {
  const resp = await fetch(`${warppBase}/workflows/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
  if (!resp.ok) {
    throw new Error(`delete failed (${resp.status})`);
  }
}

export function validateWorkflow(
  document: WarppDocument,
): Promise<{ valid: boolean; diagnostics?: WarppDiagnostic[] }> {
  return fetch(`${warppBase}/validate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ document }),
  }).then((r) =>
    asJSON<{ valid: boolean; diagnostics?: WarppDiagnostic[] }>(r),
  );
}

export function startRun(
  workflowId: string,
  input: Record<string, unknown>,
): Promise<{ run_id: string; status: string }> {
  return fetch(`${warppBase}/runs`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ workflow_id: workflowId, input }),
  }).then((r) => asJSON<{ run_id: string; status: string }>(r));
}

const TERMINAL = new Set(["run_completed", "run_failed", "run_cancelled"]);

// streamRunEvents subscribes to a run's SSE stream and returns a cancel fn.
export function streamRunEvents(
  runId: string,
  onEvent: (ev: WarppRunEvent) => void,
  onDone: () => void,
): () => void {
  const url = `${warppBase}/runs/${encodeURIComponent(runId)}/events`;
  const source = new EventSource(url);
  const close = () => source.close();
  source.onmessage = (msg) => {
    try {
      const ev = JSON.parse(msg.data) as WarppRunEvent;
      onEvent(ev);
      if (TERMINAL.has(ev.type)) {
        close();
        onDone();
      }
    } catch {
      // ignore malformed frames
    }
  };
  source.onerror = () => {
    close();
    onDone();
  };
  return close;
}
