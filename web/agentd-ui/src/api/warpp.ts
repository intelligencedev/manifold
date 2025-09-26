import type { WarppTool, WarppWorkflow } from '@/types/warpp'

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

export async function runWarppWorkflow(
  intent: string,
  prompt?: string,
  signal?: AbortSignal,
): Promise<{ result: string }> {
  const resp = await fetch(`${apiBase}/run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ intent, prompt }),
    signal,
  })
  return handleResponse<{ result: string }>(resp)
}
