/**
 * Shared graph types used by both the layout engine and PixiJS renderer.
 */

export type NodeKind = "orchestrator" | "specialist" | "team" | "run";
export type NodeStatus = "online" | "paused" | "running" | "completed" | "failed" | "idle";
export type EdgeKind = "membership" | "delegation" | "active";

export interface GraphNode {
  id: string;
  label: string;
  kind: NodeKind;
  status: NodeStatus;
  /** Sub-label shown below main label (model name, prompt snippet, etc.) */
  sublabel?: string;
}

export interface GraphEdge {
  source: string;
  target: string;
  kind: EdgeKind;
  label?: string;
  /** Whether to animate this edge (dashes move) */
  animated?: boolean;
}
