export type ChatStreamEventType =
  | 'delta'
  | 'final'
  | 'tool_start'
  | 'tool_result'
  | 'tts_chunk'
  | 'tts_audio'
  | 'error'

export interface ChatStreamEvent {
  type: ChatStreamEventType
  data?: string
  title?: string
  tool_id?: string
  args?: string
  bytes?: number
  b64?: string
  url?: string
  file_path?: string
  [key: string]: unknown
}

export interface StreamAgentRunOptions {
  prompt: string
  sessionId?: string
  fetchImpl?: typeof fetch
  signal?: AbortSignal
  onEvent: (event: ChatStreamEvent) => void
}

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || '').replace(/\/$/, '')
const runEndpoint = `${baseURL}/agent/run`

export async function streamAgentRun(options: StreamAgentRunOptions): Promise<void> {
  const { prompt, sessionId, fetchImpl, signal, onEvent } = options
  const fetchFn = fetchImpl ?? fetch
  const payload = { prompt, session_id: sessionId }
  const decoder = new TextDecoder()

  let response: Response

  try {
    response = await fetchFn(runEndpoint, {
      method: 'POST',
      headers: {
        Accept: 'text/event-stream',
        'Content-Type': 'application/json'
      },
      body: JSON.stringify(payload),
      signal
    })
  } catch (error) {
    if (!(error instanceof DOMException && error.name === 'AbortError')) {
      onEvent({ type: 'error', data: error instanceof Error ? error.message : String(error) })
    }
    throw error
  }

  if (!response.ok) {
    const message = `agent run failed (${response.status})`
    onEvent({ type: 'error', data: message })
    throw new Error(message)
  }

  const contentType = response.headers.get('content-type') || ''

  if (!contentType.includes('text/event-stream')) {
    const body = await response.json().catch(() => ({}))
    const result = typeof body?.result === 'string' ? body.result : ''
    onEvent({ type: 'final', data: result })
    return
  }

  if (!response.body) {
    onEvent({ type: 'error', data: 'stream body missing' })
    throw new Error('stream body missing')
  }

  const reader = response.body.getReader()
  let buffer = ''

  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) {
        break
      }
      buffer += decoder.decode(value, { stream: true })
      buffer = processBuffer(buffer, onEvent)
    }
    // flush remaining buffered data
    if (buffer.trim().length > 0) {
      processBuffer(buffer, onEvent, true)
    }
  } finally {
    reader.releaseLock()
  }
}

function processBuffer(buffer: string, onEvent: (event: ChatStreamEvent) => void, flush = false): string {
  const parts = buffer.split('\n\n')
  const leftover = flush ? '' : parts.pop() || ''

  for (const part of parts) {
    const payload = extractEventPayload(part)
    if (payload) {
      onEvent(payload)
    }
  }

  return leftover
}

export function extractEventPayload(raw: string): ChatStreamEvent | null {
  const lines = raw
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)

  let dataLine = ''
  for (const line of lines) {
    if (line.startsWith('data:')) {
      dataLine += line.slice(5).trim()
    }
  }

  if (!dataLine) {
    return null
  }

  try {
    const parsed = JSON.parse(dataLine) as ChatStreamEvent
    if (typeof parsed.type !== 'string') {
      return null
    }
    return parsed
  } catch (error) {
    console.error('Failed to parse SSE payload', error)
    return null
  }
}
