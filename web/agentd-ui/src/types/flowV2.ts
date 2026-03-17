export interface FlowV2Tool {
  name: string;
  description?: string;
  parameters?: Record<string, any>;
}

export interface FlowV2WorkflowSummary {
  id: string;
  name?: string;
  description?: string;
}

export type FlowV2TriggerType = "manual" | "schedule" | "webhook" | "event";
export type FlowV2NodeKind = "action" | "logic" | "data";
export type FlowV2BackoffStrategy = "" | "fixed" | "exponential";
export type FlowV2ErrorStrategy = "fail" | "continue";

export interface FlowV2ScheduleTrigger {
  cron: string;
}

export interface FlowV2WebhookTrigger {
  method: string;
  path: string;
}

export interface FlowV2EventTrigger {
  name: string;
}

export interface FlowV2Trigger {
  type: FlowV2TriggerType;
  schedule?: FlowV2ScheduleTrigger;
  webhook?: FlowV2WebhookTrigger;
  event?: FlowV2EventTrigger;
}

export interface FlowV2InputBinding {
  literal?: unknown;
  expression?: string;
}

export interface FlowV2RetryPolicy {
  max?: number;
  backoff?: FlowV2BackoffStrategy;
}

export interface FlowV2NodeExecution {
  timeout?: string;
  retries?: FlowV2RetryPolicy;
  on_error?: FlowV2ErrorStrategy;
}

export interface FlowV2Node {
  id: string;
  name: string;
  kind: FlowV2NodeKind;
  type: string;
  guard?: string;
  tool?: string;
  publish_result?: boolean;
  publish_mode?: string;
  inputs?: Record<string, FlowV2InputBinding>;
  execution?: FlowV2NodeExecution;
}

export interface FlowV2PortRef {
  node_id: string;
  port: string;
}

export interface FlowV2FieldMapping {
  from: string;
  to: string;
}

export interface FlowV2Edge {
  id?: string;
  source: FlowV2PortRef;
  target: FlowV2PortRef;
  mapping?: FlowV2FieldMapping[];
}

export interface FlowV2WorkflowSettings {
  max_concurrency?: number;
  default_execution?: FlowV2NodeExecution;
}

export interface FlowV2Workflow {
  id: string;
  name: string;
  description?: string;
  keywords?: string[];
  project_id?: string;
  trigger: FlowV2Trigger;
  nodes: FlowV2Node[];
  edges?: FlowV2Edge[];
  settings?: FlowV2WorkflowSettings;
}

export interface FlowV2CanvasNode {
  x: number;
  y: number;
  width?: number;
  height?: number;
  collapsed?: boolean;
}

export interface FlowV2CanvasGroup {
  id: string;
  label: string;
  color?: string;
  collapsed?: boolean;
}

export interface FlowV2CanvasNote {
  id: string;
  label?: string;
  note?: string;
  color?: string;
}

export interface FlowV2WorkflowCanvas {
  nodes?: Record<string, FlowV2CanvasNode>;
  parents?: Record<string, string>;
  groups?: FlowV2CanvasGroup[];
  notes?: FlowV2CanvasNote[];
}

export interface FlowV2ListResponse {
  workflows: FlowV2WorkflowSummary[];
}

export interface FlowV2GetWorkflowResponse {
  workflow: FlowV2Workflow;
  canvas?: FlowV2WorkflowCanvas;
}

export interface FlowV2PutWorkflowRequest {
  workflow: FlowV2Workflow;
  canvas?: FlowV2WorkflowCanvas;
}

export interface FlowV2RunResponse {
  run_id: string;
  status: string;
}

export interface FlowV2RunEvent {
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

export interface FlowV2RunEventsResponse {
  run_id: string;
  status: string;
  events: FlowV2RunEvent[];
}

export type FlowStepTraceStatus = "completed" | "skipped" | "noop" | "error";

export interface FlowStepTrace {
  stepId: string;
  text?: string;
  renderedArgs?: Record<string, any>;
  delta?: Record<string, any>;
  payload?: unknown;
  status?: FlowStepTraceStatus;
  error?: string;
}

export interface FlowRunResult {
  result: string;
  trace: FlowStepTrace[];
}