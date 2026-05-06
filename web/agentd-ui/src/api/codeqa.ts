import { apiClient } from "@/api/client";

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || "").replace(/\/$/, "");
const codeqaEventsBase = `${baseURL}/api/codeqa/runs`;

export type CodeQARunStatus = "queued" | "running" | "completed" | "failed";
export type CodeQAAction = "accept" | "reject" | "revert_candidate" | "human_review";

export interface CodeQAChangedFile {
  path: string;
  status: string;
  related_tests?: string[];
}

export interface CodeQAGateResult {
  name: string;
  ref?: string;
  ok: boolean;
  hard_fail: boolean;
  skipped?: boolean;
  metrics?: Record<string, number>;
  stdout?: string;
  stderr?: string;
  duration_ms: number;
}

export interface CodeQAJudgeVerdict {
  judge_id: string;
  verdict: string;
  confidence: number;
  scores: Record<string, number>;
  blocking_concerns?: string[];
  swap_applied: boolean;
  evidence?: string[];
}

export interface CodeQAAggregate {
  quality_delta: number;
  confidence: number;
  hard_failures?: string[];
  action: CodeQAAction;
  rationale: string;
}

export interface CodeQADiffBundle {
  base_ref: string;
  head_ref: string;
  files: CodeQAChangedFile[];
  unified_diff: string;
  repo_context?: string;
  truncated: boolean;
}

export interface CodeQARun {
  run_id: string;
  mode: string;
  status: CodeQARunStatus;
  project_id?: string;
  repository: string;
  error?: string;
  diff: CodeQADiffBundle;
  gates: CodeQAGateResult[];
  judges: CodeQAJudgeVerdict[];
  aggregate: CodeQAAggregate;
  started_at: string;
  completed_at?: string;
}

export interface CodeQARunEvent {
  run_id: string;
  sequence: number;
  type:
    | "queued"
    | "run_started"
    | "diff_packaged"
    | "gates_completed"
    | "judges_completed"
    | "run_completed"
    | "run_failed";
  payload?: Record<string, unknown>;
  occurred_at: string;
}

export interface CodeQARunEventsResponse {
  run_id: string;
  status: CodeQARunStatus;
  events: CodeQARunEvent[];
}

export interface StartCodeQARunRequest {
  project_id?: string;
  repository_path?: string;
  base_ref?: string;
  head_ref?: string;
  include_repo_context?: boolean;
  max_diff_bytes?: number;
  max_changed_files?: number;
  accept_threshold?: number;
  min_confidence?: number;
}

export async function listCodeQARuns(limit = 50): Promise<CodeQARun[]> {
  const { data } = await apiClient.get<{ runs: CodeQARun[] }>("/codeqa/runs", {
    params: { limit },
  });
  return data.runs ?? [];
}

export async function fetchCodeQARun(runId: string): Promise<CodeQARun> {
  const { data } = await apiClient.get<CodeQARun>(
    `/codeqa/runs/${encodeURIComponent(runId)}`,
  );
  return data;
}

export async function fetchCodeQARunEvents(
  runId: string,
): Promise<CodeQARunEventsResponse> {
  const { data } = await apiClient.get<CodeQARunEventsResponse>(
    `/codeqa/runs/${encodeURIComponent(runId)}/events`,
  );
  return data;
}

export async function startCodeQARun(
  payload: StartCodeQARunRequest,
): Promise<{ run_id: string; status: CodeQARunStatus }> {
  const { data } = await apiClient.post<{ run_id: string; status: CodeQARunStatus }>(
    "/codeqa/runs",
    payload,
  );
  return data;
}

export function streamCodeQARunEvents(
  runId: string,
  onEvent: (event: CodeQARunEvent) => void,
  onError?: (error: Event) => void,
): () => void {
  const source = new EventSource(
    `${codeqaEventsBase}/${encodeURIComponent(runId)}/events`,
    { withCredentials: true },
  );
  source.onmessage = (message) => {
    try {
      onEvent(JSON.parse(message.data) as CodeQARunEvent);
    } catch {
      // Ignore malformed events.
    }
  };
  source.onerror = (error) => {
    onError?.(error);
  };
  return () => source.close();
}