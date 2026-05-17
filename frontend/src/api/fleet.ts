import { apiClient } from "./client";
import { connectSSE } from "@/lib/sse";

export interface FleetEvent {
  kind: string;
  run_id?: string;
  session_id?: string;
  project_id?: string;
  objective_id?: string;
  workflow_id?: string;
  specialist?: string;
  agent?: string;
  call_id?: string;
  parent_call_id?: string;
  tool_id?: string;
  depth?: number;
  status?: string;
  title?: string;
  message?: string;
  at?: string;
  data?: Record<string, unknown>;
}

export interface FleetState {
  runs: Array<{ id: string; prompt: string; createdAt: string; status: string; tokens?: number }>;
  specialists: Array<Record<string, unknown>>;
  teams: Array<Record<string, unknown>>;
  open_input_requests: Array<Record<string, unknown>>;
  active_delegation_edges: Array<Record<string, unknown>>;
  recent_events?: FleetEvent[];
}

export async function fetchFleetState(): Promise<FleetState> {
  const { data } = await apiClient.get<FleetState>("/fleet/state");
  return data;
}

export function subscribeFleetEvents(onMessage: (event: FleetEvent) => void): EventSource {
  return connectSSE<FleetEvent>("/api/fleet/events", onMessage);
}

export async function fetchRunTimeline(runId: string): Promise<{ run_id: string; status?: string; events: unknown[] }> {
  const { data } = await apiClient.get<{ run_id: string; status?: string; events: unknown[] }>(`/runs/${encodeURIComponent(runId)}/timeline`);
  return data;
}
