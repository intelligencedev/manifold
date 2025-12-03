import { apiClient } from './client'
import type { ChatSessionMeta } from '@/types/chat'

export interface MemorySessionPlan {
  mode: string
  contextWindowTokens: number
  targetUtilizationPct: number
  tailTokenBudget: number
  minKeepLastMessages: number
  maxSummaryChunkTokens: number
  estimatedHistoryTokens: number
  estimatedTailTokens: number
  tailStartIndex: number
  totalMessages: number
}

export interface MemorySessionDebug {
  session: any
  summary: string
  summarizedCount: number
  messages: Array<{
    role: string
    content: string
  }>
  plan: MemorySessionPlan
}

export interface EvolvingMemoryEntry {
  id: string
  input: string
  output: string
  feedback: string
  summary: string
  raw_trace?: string
  metadata?: Record<string, any>
  created_at: string
}

export interface ScoredEvolvingMemoryEntry {
  entry: EvolvingMemoryEntry
  score: number
}

export interface EvolvingMemoryDebug {
  enabled: boolean
  totalEntries: number
  topK: number
  maxSize: number
  windowSize: number
  recentWindow: EvolvingMemoryEntry[]
  lastQuery?: string
  retrieved?: ScoredEvolvingMemoryEntry[]
}

// List sessions via the debug API so Overview's Memory panel
// sees exactly what the memory engine sees.
export async function fetchMemorySessions(): Promise<ChatSessionMeta[]> {
  const { data } = await apiClient.get<ChatSessionMeta[]>('/debug/memory/sessions')
  return data
}

export async function fetchMemorySessionDebug(sessionId: string): Promise<MemorySessionDebug> {
  const { data } = await apiClient.get<MemorySessionDebug>(`/debug/memory/sessions/${encodeURIComponent(sessionId)}`)
  return data
}

export async function fetchMemoryPlan(sessionId: string): Promise<MemorySessionPlan> {
  const { data } = await apiClient.get<MemorySessionPlan>('/debug/memory/plan', {
    params: { session_id: sessionId },
  })
  return data
}

export async function fetchEvolvingMemory(query?: string): Promise<EvolvingMemoryDebug> {
  const { data } = await apiClient.get<EvolvingMemoryDebug>('/debug/memory/evolving', {
    params: query ? { query } : undefined,
  })
  return data
}

