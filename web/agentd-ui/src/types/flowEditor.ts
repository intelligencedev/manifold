import type { FlowV2Trigger } from "@/types/flowV2";

export interface FlowEditorTool {
  name: string;
  description?: string;
  parameters?: Record<string, any>;
}

export interface FlowEditorToolRef {
  name: string;
  args?: Record<string, any>;
}

export interface FlowEditorNodeLayout {
  x: number;
  y: number;
  width?: number;
  height?: number;
  collapsed?: boolean;
  label?: string;
}

export interface FlowEditorGroupUIEntry {
  id: string;
  label: string;
  collapsed?: boolean;
  color?: string;
}

export interface FlowEditorNoteUIEntry {
  id: string;
  label?: string;
  color?: string;
  note?: string;
}

export interface FlowEditorWorkflowUI {
  layout?: Record<string, FlowEditorNodeLayout>;
  parents?: Record<string, string>;
  groups?: FlowEditorGroupUIEntry[];
  notes?: FlowEditorNoteUIEntry[];
  edgeStyle?: string;
}

export interface FlowEditorStep {
  id: string;
  text: string;
  guard?: string;
  publish_result?: boolean;
  publish_mode?: "immediate" | "topo";
  continue_on_error?: boolean;
  tool?: FlowEditorToolRef;
  depends_on?: string[];
}

export interface FlowEditorWorkflow {
  intent: string;
  description?: string;
  keywords?: string[];
  project_id?: string;
  trigger?: FlowV2Trigger;
  max_concurrency?: number;
  fail_fast?: boolean;
  steps: FlowEditorStep[];
  ui?: FlowEditorWorkflowUI;
}

export type FlowEditorTraceStatus =
  | "completed"
  | "skipped"
  | "noop"
  | "error";

export interface FlowEditorStepTrace {
  stepId: string;
  text?: string;
  renderedArgs?: Record<string, any>;
  delta?: Record<string, any>;
  payload?: unknown;
  status?: FlowEditorTraceStatus;
  error?: string;
}