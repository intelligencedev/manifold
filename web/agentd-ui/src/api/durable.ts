import { apiClient } from "@/api/client";

export type DurableTaskStatus =
  | "queued"
  | "running"
  | "waiting"
  | "completed"
  | "failed"
  | "cancelled";

export interface DurableRetryPolicy {
  max_attempts?: number;
  backoff?: string;
  base_delay_seconds?: number;
  max_delay_seconds?: number;
}

export interface DurableQueueStats {
  queue: string;
  queued: number;
  running: number;
  waiting: number;
  completed: number;
  failed: number;
  cancelled: number;
}

export interface DurableTask {
  id: string;
  queue: string;
  name: string;
  user_id: number;
  status: DurableTaskStatus;
  params?: Record<string, unknown>;
  headers?: Record<string, unknown>;
  idempotency_key?: string;
  parent_task_id?: string;
  parent_run_id?: string;
  retry_policy?: DurableRetryPolicy;
  attempt: number;
  available_at: string;
  result?: unknown;
  failure?: unknown;
  error?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

export interface DurableEvent {
  id: number;
  task_id?: string;
  queue?: string;
  name: string;
  sequence: number;
  payload?: Record<string, unknown>;
  occurred_at: string;
}

export interface DurableTaskListParams {
  queue?: string;
  status?: DurableTaskStatus | "";
  name?: string;
  limit?: number;
}

export interface DurableTaskEventsResponse {
  task_id: string;
  status: DurableTaskStatus;
  events: DurableEvent[];
}

export async function fetchDurableQueues(): Promise<DurableQueueStats[]> {
  const response = await apiClient.get<{ queues?: DurableQueueStats[] }>(
    "/durable/queues",
  );
  return response.data.queues ?? [];
}

export async function listDurableTasks(
  params: DurableTaskListParams = {},
): Promise<DurableTask[]> {
  const response = await apiClient.get<{ tasks?: DurableTask[] }>(
    "/durable/tasks",
    {
      params: compactParams(params),
    },
  );
  return response.data.tasks ?? [];
}

export async function fetchDurableTask(taskId: string): Promise<DurableTask> {
  const response = await apiClient.get<{ task: DurableTask }>(
    `/durable/tasks/${encodeURIComponent(taskId)}`,
  );
  return response.data.task;
}

export async function fetchDurableTaskEvents(
  taskId: string,
): Promise<DurableTaskEventsResponse> {
  const response = await apiClient.get<DurableTaskEventsResponse>(
    `/durable/tasks/${encodeURIComponent(taskId)}/events`,
  );
  return response.data;
}

export async function cancelDurableTask(taskId: string): Promise<void> {
  await apiClient.post(`/durable/tasks/${encodeURIComponent(taskId)}/cancel`);
}

export async function retryDurableTask(
  taskId: string,
  resetCheckpoints = false,
): Promise<DurableTask> {
  const response = await apiClient.post<{ task: DurableTask }>(
    `/durable/tasks/${encodeURIComponent(taskId)}/retry`,
    {
      reset_checkpoints: resetCheckpoints,
    },
  );
  return response.data.task;
}

function compactParams(params: DurableTaskListParams) {
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => {
      if (value === undefined || value === null) return false;
      if (typeof value === "string" && value.trim() === "") return false;
      return true;
    }),
  );
}
