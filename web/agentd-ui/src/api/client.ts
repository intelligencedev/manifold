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

export interface TokenMetricsRow {
  model: string
  prompt: number
  completion: number
  total: number
}

export interface TokenMetricsResponse {
  timestamp: number
  windowSeconds?: number
  source?: string
  models: TokenMetricsRow[]
}

export async function fetchTokenMetrics(): Promise<TokenMetricsResponse> {
  const response = await apiClient.get<TokenMetricsResponse>('/metrics/tokens')
  return response.data
}

// Projects API --------------------------------------------------------------

export interface ProjectSummary {
  id: string
  name: string
  createdAt: string
  updatedAt: string
  sizeBytes: number
  files: number
}

export interface FileEntry {
  name: string
  path: string
  isDir: boolean
  sizeBytes: number
  modTime: string
}

export async function listProjects(): Promise<ProjectSummary[]> {
  const { data } = await apiClient.get<{ projects: ProjectSummary[] }>('/projects')
  return data.projects || []
}

export async function createProject(name: string): Promise<ProjectSummary> {
  const { data } = await apiClient.post<ProjectSummary>('/projects', { name })
  return data
}

export async function deleteProject(id: string): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(id)}`)
}

export async function listProjectTree(id: string, path = '.'):
  Promise<FileEntry[]> {
  const { data } = await apiClient.get<{ entries: FileEntry[] }>(
    `/projects/${encodeURIComponent(id)}/tree`,
    { params: path ? { path } : undefined },
  )
  return data.entries || []
}

export async function createDir(id: string, path: string): Promise<void> {
  await apiClient.post(`/projects/${encodeURIComponent(id)}/dirs`, null, {
    params: { path },
  })
}

export async function deletePath(id: string, path: string): Promise<void> {
  await apiClient.delete(`/projects/${encodeURIComponent(id)}/files`, {
    params: { path },
  })
}

export async function uploadFile(
  id: string,
  dirPath: string,
  file: File,
  name?: string,
): Promise<void> {
  const form = new FormData()
  form.append('file', file, file.name)
  if (name) form.append('name', name)
  await apiClient.post(`/projects/${encodeURIComponent(id)}/files`, form, {
    params: { path: dirPath, name },
  })
}

// Build a direct URL to fetch a file's content for preview/download.
export function projectFileUrl(id: string, path: string): string {
  const b = baseURL.replace(/\/$/, '')
  const qp = new URLSearchParams({ path }).toString()
  return `${b}/projects/${encodeURIComponent(id)}/files?${qp}`
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

// Agentd configuration ------------------------------------------------------

export interface AgentdSettings {
  openaiSummaryModel: string
  openaiSummaryUrl: string
  summaryEnabled: boolean
  summaryThreshold: number
  summaryKeepLast: number

  embedBaseUrl: string
  embedModel: string
  embedApiKey: string
  embedApiHeader: string
  embedPath: string

  agentRunTimeoutSeconds: number
  streamRunTimeoutSeconds: number
  workflowTimeoutSeconds: number

  blockBinaries: string
  maxCommandSeconds: number
  outputTruncateBytes: number

  otelServiceName: string
  serviceVersion: string
  environment: string
  otelExporterOtlpEndpoint: string

  logPath: string
  logLevel: string
  logPayloads: boolean

  searxngUrl: string
  webSearxngUrl: string

  databaseUrl: string
  dbUrl: string
  postgresDsn: string

  searchBackend: string
  searchDsn: string
  searchIndex: string

  vectorBackend: string
  vectorDsn: string
  vectorIndex: string
  vectorDimensions: number
  vectorMetric: string

  graphBackend: string
  graphDsn: string
}

export async function fetchAgentdSettings(): Promise<AgentdSettings> {
  const { data } = await apiClient.get<AgentdSettings>('/config/agentd')
  return data
}

export async function updateAgentdSettings(payload: AgentdSettings): Promise<AgentdSettings> {
  const { data } = await apiClient.put<AgentdSettings>('/config/agentd', payload)
  return data
}
