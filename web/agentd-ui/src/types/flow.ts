import type { FlowEditorStep } from "@/types/flowEditor";

export interface StepNodeData {
  step: FlowEditorStep;
  order: number;
  kind?: "step" | "utility";
  // UI-only: whether the node card is collapsed to its header
  collapsed?: boolean;
  // UI-only: optional display label override (defaults to tool name on canvas)
  label?: string;
  groupId?: string;
}

export interface GroupNodeData {
  kind: "group";
  label: string;
  collapsed?: boolean;
  color?: string;
}

export interface StickyNoteNodeData {
  // treat as utility for sizing/stacking
  kind: "utility";
  label?: string;
  color?: string;
  note?: string;
  collapsed?: boolean;
  groupId?: string;
}
