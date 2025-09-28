import { apiClient } from './client'

export interface Prompt {
  id: string
  name: string
  description?: string
  tags?: string[]
  metadata?: Record<string, string>
  createdAt: string
}

export interface PromptVersion {
  id: string
  promptId: string
  semver?: string
  template: string
  variables?: Record<string, VariableSchema>
  guardrails?: Guardrails
  contentHash?: string
  createdBy?: string
  createdAt: string
}

export interface VariableSchema {
  name?: string
  type?: string
  description?: string
  required?: boolean
}

export interface Guardrails {
  maxTokens?: number
  validators?: string[]
}

export interface Dataset {
  id: string
  name: string
  description?: string
  tags?: string[]
  createdAt: string
  metadata?: Record<string, string>
  rows?: DatasetRow[]
}

export interface DatasetRow {
  id: string
  inputs: Record<string, any>
  expected?: any
  meta?: Record<string, any>
  split?: string
}

export interface ExperimentVariant {
  id: string
  promptVersionId: string
  model: string
  params?: Record<string, any>
}

export interface EvaluatorConfig {
  name: string
  params?: Record<string, any>
  weight?: number
}

export interface BudgetConfig {
  maxTokens?: number
  maxCost?: number
}

export interface ConcurrencyConfig {
  maxWorkers?: number
  maxRowsPerShard?: number
  maxVariantsPerRun?: number
}

export interface ExperimentSpec {
  id: string
  projectId?: string
  name: string
  datasetId: string
  snapshotId?: string
  sliceExpr?: string
  variants: ExperimentVariant[]
  evaluators?: EvaluatorConfig[]
  budgets?: BudgetConfig
  concurrency?: ConcurrencyConfig
  createdAt: string
  createdBy?: string
}

export type RunStatus = 'pending' | 'running' | 'failed' | 'completed'

export interface RunPlanShard {
  id: string
  rows: any[]
  variants: ExperimentVariant[]
}

export interface RunPlan {
  shards: RunPlanShard[]
}

export interface Run {
  id: string
  experimentId: string
  plan: RunPlan
  status: RunStatus
  createdAt: string
  startedAt?: string
  endedAt?: string
  error?: string
  metrics?: Record<string, number>
}

export interface RunResult {
  id: string
  runId: string
  rowId: string
  variantId: string
  promptVersionId?: string
  model?: string
  rendered?: string
  output?: string
  providerName?: string
  tokens?: number
  latency?: number
  artifacts?: Record<string, string>
  scores?: Record<string, number>
  expected?: any
}

export async function listPrompts(params?: { q?: string; tag?: string; page?: number; per_page?: number }): Promise<Prompt[]> {
  const { data } = await apiClient.get<{ prompts: Prompt[] }>('/v1/playground/prompts', { params })
  return data.prompts ?? []
}

export async function createPrompt(payload: Partial<Prompt>): Promise<Prompt> {
  const { data } = await apiClient.post<Prompt>('/v1/playground/prompts', payload)
  return data
}

export async function getPrompt(id: string): Promise<Prompt> {
  const { data } = await apiClient.get<Prompt>(`/v1/playground/prompts/${encodeURIComponent(id)}`)
  return data
}

export async function listPromptVersions(id: string): Promise<PromptVersion[]> {
  const { data } = await apiClient.get<{ versions: PromptVersion[] }>(`/v1/playground/prompts/${encodeURIComponent(id)}/versions`)
  return data.versions ?? []
}

export async function createPromptVersion(id: string, payload: Partial<PromptVersion>): Promise<PromptVersion> {
  const { data } = await apiClient.post<PromptVersion>(`/v1/playground/prompts/${encodeURIComponent(id)}/versions`, payload)
  return data
}

export async function listDatasets(): Promise<Dataset[]> {
  const { data } = await apiClient.get<{ datasets: Dataset[] }>('/v1/playground/datasets')
  return data.datasets ?? []
}

export async function getDataset(id: string): Promise<Dataset> {
  const { data } = await apiClient.get<Dataset>(`/v1/playground/datasets/${encodeURIComponent(id)}`)
  return data
}

export interface CreateDatasetPayload {
  dataset: {
    id?: string
    name: string
    description?: string
    tags?: string[]
    metadata?: Record<string, string>
  }
  rows: DatasetRow[]
}

export async function createDataset(payload: CreateDatasetPayload): Promise<Dataset> {
  const { data } = await apiClient.post<Dataset>('/v1/playground/datasets', payload)
  return data
}

export async function updateDataset(id: string, payload: CreateDatasetPayload): Promise<Dataset> {
  const { data } = await apiClient.put<Dataset>(`/v1/playground/datasets/${encodeURIComponent(id)}`, payload)
  return data
}

export async function listExperiments(): Promise<ExperimentSpec[]> {
  const { data } = await apiClient.get<{ experiments: ExperimentSpec[] }>('/v1/playground/experiments')
  return data.experiments ?? []
}

export async function getExperiment(id: string): Promise<ExperimentSpec> {
  const { data } = await apiClient.get<ExperimentSpec>(`/v1/playground/experiments/${encodeURIComponent(id)}`)
  return data
}

export async function createExperiment(spec: ExperimentSpec): Promise<ExperimentSpec> {
  const { data } = await apiClient.post<ExperimentSpec>('/v1/playground/experiments', spec)
  return data
}

export async function startExperimentRun(experimentId: string): Promise<Run> {
  const { data } = await apiClient.post<Run>(`/v1/playground/experiments/${encodeURIComponent(experimentId)}/runs`, {})
  return data
}

export async function listExperimentRuns(experimentId: string): Promise<Run[]> {
  const { data } = await apiClient.get<{ runs: Run[] }>(`/v1/playground/experiments/${encodeURIComponent(experimentId)}/runs`)
  return data.runs ?? []
}

export async function listRunResults(runId: string): Promise<RunResult[]> {
  const { data } = await apiClient.get<{ results: RunResult[] }>(`/v1/playground/runs/${encodeURIComponent(runId)}/results`)
  return data.results ?? []
}
