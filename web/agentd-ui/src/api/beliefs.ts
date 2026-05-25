import { apiClient } from "./client";

export interface BeliefScope {
  id: string;
  tenantId: number;
  kind: string;
  parentId?: string;
  path: string;
  label?: string;
}

export interface BeliefRecord {
  id: string;
  tenantId: number;
  scopeId: string;
  statement: string;
  statementHash?: string;
  kind: "fact" | "preference" | "procedure" | "constraint" | "capability";
  enforcement: "none" | "prompt" | "soft_policy" | "hard_constraint";
  sourceQuality: number;
  reviewState:
    | "auto_active"
    | "needs_review"
    | "operator_approved"
    | "operator_rejected";
  confidence: number;
  evidenceFor: number;
  evidenceAgainst: number;
  status: "active" | "superseded" | "retracted";
  createdAt: string;
  updatedAt: string;
}

export interface BeliefSearchResult {
  belief: BeliefRecord;
  scope?: BeliefScope;
  score: number;
  reason?: string;
}

export interface BeliefEvidence {
  id: string;
  tenantId: number;
  beliefId: string;
  sourceKind: string;
  sourceId: string;
  polarity: string;
  weight: number;
  note?: string;
  createdAt: string;
}

export interface BeliefPromotion {
  id: string;
  tenantId: number;
  beliefId: string;
  fromScope: string;
  toScope: string;
  reason?: string;
  confidenceBefore: number;
  confidenceAfter: number;
  createdAt: string;
}

export interface BeliefDetail {
  belief: BeliefRecord;
  evidence?: BeliefEvidence[];
  promotions?: BeliefPromotion[];
}

export interface BeliefCandidate {
  id: string;
  tenantId: number;
  episodeId?: string;
  scopeId?: string;
  statement: string;
  statementHash?: string;
  kind: BeliefRecord["kind"];
  enforcement: BeliefRecord["enforcement"];
  polarity: "for" | "against";
  confidence: number;
  sourceQuality: number;
  reviewState: BeliefRecord["reviewState"];
  evidenceNote?: string;
  validationStatus: "accepted" | "queued" | "rejected";
  rejectionReason?: string;
  acceptedBeliefId?: string;
  model?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BeliefPromptPreview {
  text: string;
  selected?: BeliefSearchResult[];
  overflow?: BeliefSearchResult[];
  tokenEstimate?: number;
}

export interface BeliefInfluenceEntry {
  source: string;
  result: BeliefSearchResult;
}

export interface BeliefInfluenceTrace {
  results: BeliefInfluenceEntry[];
  prompt: BeliefPromptPreview;
}

export interface PolicyRecord {
  id: string;
  name?: string;
  kind: string;
  severity?: string;
  mode?: string;
  statement: string;
  scope?: string;
  status?: string;
  approvalState?: string;
}

export async function searchBeliefs(params: {
  q?: string;
  status?: string;
  limit?: number;
}): Promise<BeliefSearchResult[]> {
  const { data } = await apiClient.get<BeliefSearchResult[]>(
    "/beliefs/search",
    { params },
  );
  return data;
}

export async function fetchBeliefDetail(id: string): Promise<BeliefDetail> {
  const { data } = await apiClient.get<BeliefDetail>(
    `/beliefs/${encodeURIComponent(id)}`,
  );
  return data;
}

export async function fetchBeliefInfluence(params: {
  q?: string;
  project_id?: string;
  objective_id?: string;
  session_id?: string;
  role?: string;
  limit?: number;
}): Promise<BeliefInfluenceTrace> {
  const { data } = await apiClient.get<BeliefInfluenceTrace>(
    "/beliefs/influence",
    { params },
  );
  return data;
}

export async function fetchBeliefPolicies(params: {
  project_id?: string;
  role?: string;
}): Promise<PolicyRecord[]> {
  const { data } = await apiClient.get<PolicyRecord[]>(
    "/beliefs/policies",
    { params },
  );
  return data;
}

export async function retractBelief(
  id: string,
  reason: string,
): Promise<BeliefRecord> {
  const { data } = await apiClient.post<BeliefRecord>(
    `/beliefs/${encodeURIComponent(id)}/retract`,
    { reason },
  );
  return data;
}

export async function fetchBeliefCandidates(params: {
  review_state?: string;
  validation_status?: string;
  limit?: number;
}): Promise<BeliefCandidate[]> {
  const { data } = await apiClient.get<BeliefCandidate[]>("/beliefs/candidates", {
    params,
  });
  return data;
}

export async function acceptBeliefCandidate(id: string): Promise<{
  candidate: BeliefCandidate;
  belief: BeliefRecord;
}> {
  const { data } = await apiClient.post<{
    candidate: BeliefCandidate;
    belief: BeliefRecord;
  }>(`/beliefs/candidates/${encodeURIComponent(id)}/accept`);
  return data;
}

export async function rejectBeliefCandidate(
  id: string,
): Promise<BeliefCandidate> {
  const { data } = await apiClient.post<BeliefCandidate>(
    `/beliefs/candidates/${encodeURIComponent(id)}/reject`,
  );
  return data;
}
