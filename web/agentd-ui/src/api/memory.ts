import { apiClient } from "./client";
import type { ChatSessionMeta } from "@/types/chat";

export interface MemorySessionPlan {
  mode: string;
  contextWindowTokens: number;
  targetUtilizationPct: number;
  tailTokenBudget: number;
  minKeepLastMessages: number;
  maxSummaryChunkTokens: number;
  estimatedHistoryTokens: number;
  estimatedTailTokens: number;
  tailStartIndex: number;
  totalMessages: number;
}

export interface MemorySessionDebug {
  session: any;
  summary: string;
  summarizedCount: number;
  messages: Array<{
    role: string;
    content: string;
  }>;
  plan: MemorySessionPlan;
}

export interface EvolvingMemoryEntry {
  id: string;
  input: string;
  output: string;
  feedback: string;
  summary: string;
  raw_trace?: string;
  metadata?: Record<string, any>;
  memory_type?: string;
  strategy_card?: string;
  scope?: string;
  access_count?: number;
  created_at: string;
}

export interface ScoredEvolvingMemoryEntry {
  entry: EvolvingMemoryEntry;
  score: number;
}

export interface EvolvingMemorySearchDiagnostics {
  enableRAG: boolean;
  mode: string;
  vectorCandidates: number;
  keywordCandidates: number;
  usedServerVector: boolean;
  usedKeywordStore: boolean;
  embeddingInstructionUsed?: boolean;
  embeddingInstructionApplied?: boolean;
  embeddingInstructionUseCase?: string;
  embeddingInstructionFormat?: string;
  embeddingInstructionMode?: string;
  embeddingInstructionSource?: string;
  embeddingError?: string;
}

export interface EvolvingMemoryDebug {
  enabled: boolean;
  enableRAG?: boolean;
  totalEntries: number;
  topK: number;
  maxSize: number;
  windowSize: number;
  recentWindow: EvolvingMemoryEntry[];
  lastQuery?: string;
  search?: EvolvingMemorySearchDiagnostics;
  retrieved?: ScoredEvolvingMemoryEntry[];
}

export interface MemoryScoreExplanation {
  entry: EvolvingMemoryEntry;
  similarity: number;
  decay: number;
  qualityWeight: number;
  accessBoost: number;
  composite: number;
  mmrPenalty: number;
  finalScore: number;
}

export interface EvolvingMemoryExplainDebug {
  enabled: boolean;
  query?: string;
  userID?: number;
  sessionID?: string;
  explanations?: MemoryScoreExplanation[];
}

// List sessions via the debug API so Overview's Memory panel
// sees exactly what the memory engine sees.
export async function fetchMemorySessions(): Promise<ChatSessionMeta[]> {
  const { data } = await apiClient.get<ChatSessionMeta[]>(
    "/debug/memory/sessions",
  );
  return data;
}

export async function fetchMemorySessionDebug(
  sessionId: string,
): Promise<MemorySessionDebug> {
  const { data } = await apiClient.get<MemorySessionDebug>(
    `/debug/memory/sessions/${encodeURIComponent(sessionId)}`,
  );
  return data;
}

export async function fetchMemoryPlan(
  sessionId: string,
): Promise<MemorySessionPlan> {
  const { data } = await apiClient.get<MemorySessionPlan>(
    "/debug/memory/plan",
    {
      params: { session_id: sessionId },
    },
  );
  return data;
}

export async function fetchEvolvingMemory(
  query?: string,
  sessionId?: string,
): Promise<EvolvingMemoryDebug> {
  const params: Record<string, string> = {};
  if (query) {
    params.query = query;
  }
  if (sessionId) {
    params.session_id = sessionId;
  }
  const { data } = await apiClient.get<EvolvingMemoryDebug>(
    "/debug/memory/evolving",
    {
      params: Object.keys(params).length ? params : undefined,
    },
  );
  return data;
}

export async function fetchEvolvingMemoryExplain(
  query: string,
  sessionId?: string,
): Promise<EvolvingMemoryExplainDebug> {
  const params: Record<string, string> = { query };
  if (sessionId) {
    params.session_id = sessionId;
  }
  const { data } = await apiClient.get<EvolvingMemoryExplainDebug>(
    "/debug/memory/explain",
    { params },
  );
  return data;
}
