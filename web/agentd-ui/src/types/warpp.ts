// Types mirroring the WARPP backend JSON (internal/warpp).

export interface WarppBinding {
  from?: string;
  value?: unknown;
}

export type WarppInput =
  | WarppBinding
  | WarppBinding[]
  | Record<string, WarppBinding>;

export interface WarppRetries {
  max?: number;
  backoff?: string;
}

export interface WarppPolicy {
  timeout?: string;
  retries?: WarppRetries;
  on_error?: string;
}

export interface WarppBody {
  nodes: WarppNode[];
  outputs: Record<string, WarppBinding>;
}

export interface WarppNode {
  id: string;
  type: string;
  inputs?: Record<string, WarppInput>;
  policy?: WarppPolicy;
  body?: WarppBody;
}

export interface WarppPortSpec {
  name: string;
  type: string;
  required?: boolean;
  default?: unknown;
  variadic?: "" | "list" | "named";
  description?: string;
}

export interface WarppSettings {
  max_concurrency?: number;
  default_policy?: WarppPolicy;
}

export interface WarppDocument {
  id: string;
  name: string;
  description?: string;
  project_id?: string;
  inputs?: WarppPortSpec[];
  nodes: WarppNode[];
  outputs?: Record<string, WarppBinding>;
  settings?: WarppSettings;
  publish?: { tool?: boolean };
}

export interface WarppManifest {
  type: string;
  title: string;
  category: string;
  description?: string;
  inputs: WarppPortSpec[];
  outputs: WarppPortSpec[];
}

export interface WarppCanvasNode {
  x: number;
  y: number;
  width?: number;
  height?: number;
  label?: string;
}

export interface WarppCanvas {
  nodes?: Record<string, WarppCanvasNode>;
  groups?: unknown[];
  notes?: unknown[];
  edge_style?: string;
}

export interface WarppWorkflowSummary {
  id: string;
  name: string;
  description?: string;
  publish_tool?: boolean;
}

export interface WarppCatalog {
  manifests: WarppManifest[];
  coercions: [string, string][];
  workflows: WarppWorkflowSummary[];
}

export interface WarppDiagnostic {
  severity: "error" | "warning";
  code: string;
  message: string;
  path?: string;
}

export interface WarppRunEvent {
  run_id: string;
  sequence: number;
  type: string;
  node_path?: string;
  status?: string;
  message?: string;
  outputs?: Record<string, unknown>;
  error?: string;
  occurred_at: string;
}

export interface WarppGetResponse {
  document: WarppDocument;
  canvas: WarppCanvas;
}
