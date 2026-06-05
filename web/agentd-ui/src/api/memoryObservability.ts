import { apiClient } from "./clientCore";
import type { MemoryLatencyMetrics, MemoryMetricTotals } from "./clientMetrics";

export interface MemoryObservabilityConfig {
  memoryEnabled: boolean;
  evolvingEnabled: boolean;
  beliefEnabled: boolean;
  magmaEnabled: boolean;
  graphBackend?: string;
  vectorBackend?: string;
}

export interface MemoryObservabilityMagmaStats {
  enabled: boolean;
  maintenanceEnabled: boolean;
  queueDepth: number;
  processedTotal: number;
  failedTotal: number;
  droppedTotal: number;
  lastError?: string;
}

export interface MemoryObservabilityGraphStats {
  nodes: number;
  edges: number;
  events: number;
  entities: number;
  reviewEdges: number;
  byType?: Record<string, number>;
}

export interface MemoryObservabilityLane {
  id: string;
  label: string;
  enabled: boolean;
  status: string;
  detail?: string;
}

export interface MemoryObservabilityOverview {
  timestamp: number;
  windowSeconds?: number;
  source: string;
  config: MemoryObservabilityConfig;
  totals: MemoryMetricTotals;
  latency: MemoryLatencyMetrics;
  graph: MemoryObservabilityGraphStats;
  magma: MemoryObservabilityMagmaStats;
  lanes: MemoryObservabilityLane[];
  warnings?: string[];
}

export interface MemoryObservabilityNode {
  id: string;
  type: string;
  label: string;
  tenant?: string;
  session?: string;
  text?: string;
  createdAt?: string;
  metadata?: Record<string, unknown>;
}

export interface MemoryObservabilityEdge {
  id: string;
  source: string;
  target: string;
  graphType: string;
  rel: string;
  weight?: number;
  confidence?: number;
  reviewState?: string;
  reason?: string;
  props?: Record<string, unknown>;
}

export interface MemoryObservabilityGraph {
  timestamp: number;
  graph: MemoryObservabilityGraphStats;
  nodes: MemoryObservabilityNode[];
  edges: MemoryObservabilityEdge[];
  warnings?: string[];
}

export interface MemoryObservabilityTimelineItem {
  id: string;
  time?: string;
  lane: string;
  kind: string;
  title: string;
  detail?: string;
  severity?: string;
  sessionId?: string;
}

export interface MemoryObservabilityTimeline {
  timestamp: number;
  items: MemoryObservabilityTimelineItem[];
}

export interface MemoryObservabilityReviewEdges {
  timestamp: number;
  edges: MemoryObservabilityEdge[];
}

export interface MemoryObservabilityExplain {
  query: string;
  intent: string;
  graphViews: string[];
  anchorCount: number;
  maxHops: number;
  maxNodes: number;
  context: string;
  events: MemoryObservabilityNode[];
  diagnostics?: Record<string, unknown>;
}

export interface MemoryObservabilityActionResponse {
  ok: boolean;
  message: string;
  result?: unknown;
}

export interface MemoryObservabilityParams {
  window?: string;
  tenant?: string;
  sessionId?: string;
  graphType?: string;
  q?: string;
  limit?: number;
}

export interface MagmaEdgeSelector {
  source: string;
  graphType: string;
  rel: string;
  target: string;
}

export async function fetchMemoryObservabilityOverview(
  params?: MemoryObservabilityParams,
): Promise<MemoryObservabilityOverview> {
  const response = await apiClient.get<MemoryObservabilityOverview>(
    "/observability/memory/overview",
    { params },
  );
  return response.data;
}

export async function fetchMemoryObservabilityGraph(
  params?: MemoryObservabilityParams,
): Promise<MemoryObservabilityGraph> {
  const response = await apiClient.get<MemoryObservabilityGraph>(
    "/observability/memory/graph",
    { params },
  );
  return response.data;
}

export async function fetchMemoryObservabilityTimeline(
  params?: MemoryObservabilityParams,
): Promise<MemoryObservabilityTimeline> {
  const response = await apiClient.get<MemoryObservabilityTimeline>(
    "/observability/memory/timeline",
    { params },
  );
  return response.data;
}

export async function fetchMemoryObservabilityReviewEdges(
  params?: MemoryObservabilityParams,
): Promise<MemoryObservabilityReviewEdges> {
  const response = await apiClient.get<MemoryObservabilityReviewEdges>(
    "/observability/memory/review-edges",
    { params },
  );
  return response.data;
}

export async function fetchMemoryObservabilityExplain(params: {
  q: string;
  tenant?: string;
  maxNodes?: number;
  maxHops?: number;
}): Promise<MemoryObservabilityExplain> {
  const response = await apiClient.get<MemoryObservabilityExplain>(
    "/observability/memory/retrieval/explain",
    { params },
  );
  return response.data;
}

export async function pruneMagmaMemory(payload: {
  dryRun?: boolean;
  eventTTLHours?: number;
  maxEdgesPerSourceRel?: number;
  minSemanticWeight?: number;
  lowConfidenceThreshold?: number;
  requireReviewApproval?: boolean;
}): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/prune",
    payload,
  );
  return response.data;
}

export async function approveMagmaEdge(payload: {
  selector: MagmaEdgeSelector;
  reviewer?: string;
}): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/approve-edge",
    payload,
  );
  return response.data;
}

export async function retractMagmaEdge(payload: {
  selector: MagmaEdgeSelector;
  reason?: string;
}): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/retract-edge",
    payload,
  );
  return response.data;
}

export async function deleteMagmaNode(payload: {
  nodeId: string;
}): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/delete-node",
    payload,
  );
  return response.data;
}

export async function drainMagmaConsolidation(
  limit = 25,
): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/drain-consolidation",
    { limit },
  );
  return response.data;
}

export async function rebuildEvolvingMemoryEmbeddings(payload: {
  sessionId: string;
  userId?: number;
}): Promise<MemoryObservabilityActionResponse> {
  const response = await apiClient.post<MemoryObservabilityActionResponse>(
    "/observability/memory/actions/rebuild-embeddings",
    payload,
  );
  return response.data;
}
