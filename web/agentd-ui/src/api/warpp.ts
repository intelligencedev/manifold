import type { WarppStepTrace, WarppTool, WarppWorkflow } from '@/types/warpp'

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || '').replace(/\/$/, '')
const apiBase = `${baseURL}/api/warpp`

async function handleResponse<T>(resp: Response): Promise<T> {
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(text || `request failed (${resp.status})`)
  }
  return (await resp.json()) as T
}

export async function fetchWarppTools(): Promise<WarppTool[]> {
  const resp = await fetch(`${apiBase}/tools`)
  return handleResponse<WarppTool[]>(resp)
}

export async function fetchWarppWorkflows(): Promise<WarppWorkflow[]> {
  const resp = await fetch(`${apiBase}/workflows`)
  return handleResponse<WarppWorkflow[]>(resp)
}

export async function fetchWarppWorkflow(intent: string): Promise<WarppWorkflow> {
  const resp = await fetch(`${apiBase}/workflows/${encodeURIComponent(intent)}`)
  return handleResponse<WarppWorkflow>(resp)
}

export async function saveWarppWorkflow(workflow: WarppWorkflow): Promise<WarppWorkflow> {
  const resp = await fetch(`${apiBase}/workflows/${encodeURIComponent(workflow.intent)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(workflow),
  })
  return handleResponse<WarppWorkflow>(resp)
}

export async function deleteWarppWorkflow(intent: string): Promise<void> {
  const resp = await fetch(`${apiBase}/workflows/${encodeURIComponent(intent)}`, {
    method: 'DELETE',
  })
  if (!resp.ok) {
    const text = await resp.text()
    throw new Error(text || `request failed (${resp.status})`)
  }
}

export interface WarppRunResponse {
  result: string
  trace: WarppStepTrace[]
}

export async function runWarppWorkflow(
  intent: string,
  prompt?: string,
  signal?: AbortSignal,
): Promise<WarppRunResponse> {
  const resp = await fetch(`${apiBase}/run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ intent, prompt }),
    signal,
  })
  const raw = await handleResponse<{ result: string; trace?: any[] }>(resp)
  const trace = Array.isArray(raw.trace) ? raw.trace.map(deserializeTrace) : []
  return { result: raw.result ?? '', trace }
}

function deserializeTrace(entry: any): WarppStepTrace {
  if (!entry || typeof entry !== 'object') {
    return { stepId: '', renderedArgs: {} }
  }
  const stepId = String(entry.step_id ?? entry.stepId ?? '')
  const status = typeof entry.status === 'string' ? (entry.status as WarppStepTrace['status']) : undefined
  const mapped: WarppStepTrace = {
    stepId,
    text: typeof entry.text === 'string' ? entry.text : undefined,
    renderedArgs: isPlainObject(entry.rendered_args) ? { ...entry.rendered_args } : undefined,
    delta: isPlainObject(entry.delta) ? { ...entry.delta } : undefined,
    payload: entry.payload,
    status,
    error: typeof entry.error === 'string' ? entry.error : undefined,
  }
  return mapped
}

function isPlainObject(value: unknown): value is Record<string, any> {
  return !!value && typeof value === 'object' && !Array.isArray(value)
}
