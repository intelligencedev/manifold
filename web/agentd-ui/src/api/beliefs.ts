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
  mode: string;
  statement: string;
  scope?: string;
  status?: string;
}

export async function searchBeliefs(params: {
  q?: string;
  status?: string;
  limit?: number;
}): Promise<BeliefSearchResult[]> {
  const { data } = await apiClient.get<BeliefSearchResult[]>(
    "/debug/beliefs/search",
    { params },
  );
  return data;
}

export async function fetchBeliefDetail(id: string): Promise<BeliefDetail> {
  const { data } = await apiClient.get<BeliefDetail>(
    `/debug/beliefs/${encodeURIComponent(id)}`,
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
    "/debug/beliefs/influence",
    { params },
  );
  return data;
}

export async function fetchBeliefPolicies(params: {
  project_id?: string;
  role?: string;
}): Promise<PolicyRecord[]> {
  const { data } = await apiClient.get<PolicyRecord[]>(
    "/debug/beliefs/policies",
    { params },
  );
  return data;
}

export async function retractBelief(
  id: string,
  reason: string,
): Promise<BeliefRecord> {
  const { data } = await apiClient.post<BeliefRecord>(
    `/debug/beliefs/${encodeURIComponent(id)}/retract`,
    { reason },
  );
  return data;
}
