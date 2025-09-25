import axios from 'axios'

const baseURL = import.meta.env.VITE_AGENT_API_BASE_URL || '/api'

export const apiClient = axios.create({
  baseURL,
  timeout: 30_000,
  withCredentials: true,
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

// Specialists CRUD
export interface Specialist {
  id?: number
  name: string
  baseURL: string
  apiKey?: string
  model: string
  enableTools: boolean
  paused: boolean
  allowTools?: string[]
  reasoningEffort?: 'low' | 'medium' | 'high' | ''
  system?: string
  extraHeaders?: Record<string, string>
  extraParams?: Record<string, any>
}

export async function listSpecialists(): Promise<Specialist[]> {
  const { data } = await apiClient.get<Specialist[]>('/specialists')
  return data
}

export async function getSpecialist(name: string): Promise<Specialist> {
  const { data } = await apiClient.get<Specialist>(`/specialists/${encodeURIComponent(name)}`)
  return data
}

export async function upsertSpecialist(sp: Specialist): Promise<Specialist> {
  // POST for create, PUT for update by name
  if (sp.name && sp.id == null) {
    const { data } = await apiClient.post<Specialist>('/specialists', sp)
    return data
  }
  const { data } = await apiClient.put<Specialist>(`/specialists/${encodeURIComponent(sp.name)}`, sp)
  return data
}

export async function deleteSpecialist(name: string): Promise<void> {
  await apiClient.delete(`/specialists/${encodeURIComponent(name)}`)
}

// Users & Roles
export interface User {
  id: number
  email: string
  name: string
  picture?: string
  provider?: string
  subject?: string
  roles: string[]
}

export async function listUsers(): Promise<User[]> {
  const { data } = await apiClient.get<User[]>('/users')
  return data
}

export async function createUser(u: Partial<User>): Promise<User> {
  const { data } = await apiClient.post<User>('/users', u)
  return data
}

export async function updateUser(id: number, u: Partial<User>): Promise<User> {
  const { data } = await apiClient.put<User>(`/users/${id}`, u)
  return data
}

export async function deleteUser(id: number): Promise<void> {
  await apiClient.delete(`/users/${id}`)
}
