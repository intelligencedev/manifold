import axios from 'axios'

const baseURL = import.meta.env.VITE_AGENT_API_BASE_URL || '/api'

export const apiClient = axios.create({
  baseURL,
  timeout: 30_000
})

export interface AgentStatus {
  id: string
  name: string
  state: 'online' | 'offline' | 'degraded'
  model: string
  updatedAt: string
}

export async function fetchAgentStatus(): Promise<AgentStatus[]> {
  const response = await apiClient.get<AgentStatus[]>('/status')
  return response.data
}

export interface AgentRun {
  id: string
  prompt: string
  createdAt: string
  status: 'running' | 'failed' | 'completed'
  tokens?: number
}

export async function fetchAgentRuns(): Promise<AgentRun[]> {
  const response = await apiClient.get<AgentRun[]>('/runs')
  return response.data
}
