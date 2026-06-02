import type {
  ChatContextMetricSegment,
  ChatInputRequestChoice,
  ChatMemoryContextLane,
} from "@/types/chat";

export type ChatStreamEventType =
  | "thought_summary"
  | "memory_context"
  | "context_metrics"
  | "delta"
  | "final"
  | "tool_start"
  | "tool_result"
  | "tts_chunk"
  | "tts_audio"
  | "image"
  | "error"
  | "summary"
  | "run_started"
  | "input_request"
  | "input_request_cancelled"
  | "agent_start"
  | "agent_delta"
  | "agent_final"
  | "agent_tool_start"
  | "agent_tool_result"
  | "agent_error"
  | "agent_thought_summary";

export interface ChatStreamEvent {
  type: ChatStreamEventType;
  data?: string;
  title?: string;
  tool_id?: string;
  args?: string;
  bytes?: number;
  b64?: string;
  url?: string;
  file_path?: string;
  data_url?: string;
  rel_path?: string;
  mime?: string;
  name?: string;
  agent?: string;
  team?: string;
  model?: string;
  call_id?: string;
  parent_call_id?: string;
  depth?: number;
  role?: string;
  content?: string;
  error?: string;
  thought_summary?: string;
  request_id?: string;
  question?: string;
  reason?: string;
  choices?: ChatInputRequestChoice[];
  allow_free_text?: boolean;
  multiple?: boolean;
  session_id?: string;
  run_id?: string;
  created_at?: string;
  input_tokens?: number;
  token_budget?: number;
  message_count?: number;
  summarized_count?: number;
  token_estimate?: number;
  truncated?: boolean;
  duration_ms?: number;
  lanes?: Record<string, ChatMemoryContextLane>;
  phase?: string;
  context_window?: number;
  summary_threshold?: number;
  reserve_tokens?: number;
  will_summarize?: boolean;
  segments?: ChatContextMetricSegment[];
  [key: string]: unknown;
}

export interface StreamAgentRunOptions {
  prompt: string;
  sessionId?: string;
  userMessageId?: string;
  assistantMessageId?: string;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
  onEvent: (event: ChatStreamEvent) => void;
  specialist?: string;
  teamName?: string;
  projectId?: string;
  memoryEnabled?: boolean;
  evolvingMemoryEnabled?: boolean;
  beliefMemoryEnabled?: boolean;
  image?: boolean;
  imageSize?: string;
}

export interface ChatRunStartResponse {
  run_id: string;
  session_id: string;
  user_message_id: string;
  assistant_message_id: string;
  status: string;
}

export interface ChatRunSummary {
  run_id: string;
  session_id: string;
  user_message_id?: string;
  assistant_message_id?: string;
  status: string;
  error?: string;
  created_at?: string;
  updated_at?: string;
  last_sequence?: number;
  last_retry_sequence?: number;
}

export interface ChatRunResumeResponse {
  run_id: string;
  status: string;
  last_sequence?: number;
  last_retry_sequence?: number;
}

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || "").replace(/\/$/, "");
const runEndpoint = `${baseURL}/agent/run`;
const chatRunsEndpoint = `${baseURL}/api/chat/runs`;
const chatSessionsEndpoint = `${baseURL}/api/chat/sessions`;
const visionEndpoint = `${baseURL}/agent/vision`;

function chatTargetURL(
  endpoint: string,
  specialist?: string,
  teamName?: string,
): string {
  const params = new URLSearchParams();
  const trimmedSpecialist = specialist?.trim();
  if (trimmedSpecialist && trimmedSpecialist.toLowerCase() !== "orchestrator") {
    params.set("specialist", trimmedSpecialist);
  }
  const trimmedTeam = teamName?.trim();
  if (trimmedTeam) {
    params.set("team", trimmedTeam);
  }
  const query = params.toString();
  return query ? `${endpoint}?${query}` : endpoint;
}

function emitFetchError(
  error: unknown,
  onEvent: (event: ChatStreamEvent) => void,
) {
  if (error instanceof DOMException && error.name === "AbortError") {
    return;
  }
  onEvent({
    type: "error",
    data: error instanceof Error ? error.message : String(error),
  });
}

export async function streamAgentRun(
  options: StreamAgentRunOptions,
): Promise<void> {
  const run = await startChatRun(options);
  options.onEvent({
    type: "run_started" as ChatStreamEventType,
    run_id: run.run_id,
    session_id: run.session_id,
  });
  await streamChatRunEvents({
    runId: run.run_id,
    fetchImpl: options.fetchImpl,
    signal: options.signal,
    onEvent: options.onEvent,
  });
}

export async function startChatRun(
  options: StreamAgentRunOptions,
): Promise<ChatRunStartResponse> {
  const { response } = await postAgentRun(options, chatRunsEndpoint, {
    Accept: "application/json",
    "Content-Type": "application/json",
  });
  if (!response.ok) {
    throw new Error(`chat run start failed (${response.status})`);
  }
  return (await response.json()) as ChatRunStartResponse;
}

async function postAgentRun(
  options: StreamAgentRunOptions,
  endpoint = runEndpoint,
  headers: Record<string, string> = {
    Accept: "text/event-stream",
    "Content-Type": "application/json",
  },
) {
  const {
    prompt,
    sessionId,
    userMessageId,
    assistantMessageId,
    fetchImpl,
    signal,
    onEvent,
    specialist,
    teamName,
    projectId,
    memoryEnabled,
    evolvingMemoryEnabled,
    beliefMemoryEnabled,
  } = options;
  const payload: Record<string, any> = { prompt, session_id: sessionId };
  if (userMessageId && userMessageId.trim())
    payload.user_message_id = userMessageId.trim();
  if (assistantMessageId && assistantMessageId.trim())
    payload.assistant_message_id = assistantMessageId.trim();
  if (projectId && projectId.trim()) payload.project_id = projectId.trim();
  if (typeof memoryEnabled === "boolean")
    payload.memory_enabled = memoryEnabled;
  else if (typeof evolvingMemoryEnabled === "boolean")
    payload.evolving_memory_enabled = evolvingMemoryEnabled;
  if (
    typeof memoryEnabled !== "boolean" &&
    typeof beliefMemoryEnabled === "boolean"
  )
    payload.belief_memory_enabled = beliefMemoryEnabled;
  if (options.image) payload.image = true;
  if (options.imageSize && options.imageSize.trim())
    payload.image_size = options.imageSize.trim();

  return postStreamRequest({
    fetchImpl,
    signal,
    onEvent,
    url: chatTargetURL(endpoint, specialist, teamName),
    init: {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
    },
  });
}

export async function streamChatRunEvents(options: {
  runId: string;
  after?: number;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
  onEvent: (event: ChatStreamEvent) => void;
}): Promise<void> {
  const { runId, after = 0, fetchImpl, signal, onEvent } = options;
  const params = new URLSearchParams();
  if (after > 0) params.set("after", String(after));
  const query = params.toString();
  const url = `${chatRunsEndpoint}/${encodeURIComponent(runId)}/events${query ? `?${query}` : ""}`;
  const response = await (fetchImpl ?? fetch)(url, {
    method: "GET",
    headers: { Accept: "text/event-stream" },
    credentials: "include",
    cache: "no-store",
    signal,
  });
  await streamAgentResponse(response, new TextDecoder(), onEvent, "chat run events");
}

export async function cancelChatRun(runId: string): Promise<void> {
  await fetch(`${chatRunsEndpoint}/${encodeURIComponent(runId)}/cancel`, {
    method: "POST",
    credentials: "include",
    cache: "no-store",
  });
}

export async function resumeChatRun(
  runId: string,
): Promise<ChatRunResumeResponse> {
  const response = await fetch(
    `${chatRunsEndpoint}/${encodeURIComponent(runId)}/resume`,
    {
      method: "POST",
      credentials: "include",
      cache: "no-store",
    },
  );
  if (!response.ok) {
    const text = await response.text().catch(() => "");
    throw new Error(text.trim() || `chat run resume failed (${response.status})`);
  }
  return (await response.json()) as ChatRunResumeResponse;
}

export async function listActiveChatRuns(
  sessionId: string,
): Promise<ChatRunSummary[]> {
  const response = await fetch(
    `${chatSessionsEndpoint}/${encodeURIComponent(sessionId)}/runs?active=true`,
    {
      method: "GET",
      credentials: "include",
      cache: "no-store",
    },
  );
  if (!response.ok) return [];
  const body = (await response.json().catch(() => ({}))) as {
    runs?: ChatRunSummary[];
  };
  return body.runs || [];
}

export function extractEventPayload(raw: string): ChatStreamEvent | null {
  const lines = raw
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);

  let dataLine = "";
  for (const line of lines) {
    if (line.startsWith("data:")) {
      dataLine += line.slice(5).trim();
    }
  }

  if (!dataLine) {
    return null;
  }

  try {
    const parsed = JSON.parse(dataLine) as ChatStreamEvent;
    if (typeof parsed.type !== "string") {
      return null;
    }
    return parsed;
  } catch (error) {
    console.error("Failed to parse SSE payload", error);
    return null;
  }
}

function processBuffer(
  buffer: string,
  onEvent: (event: ChatStreamEvent) => void,
  flush = false,
): string {
  const parts = buffer.split(/\n\n|\r\n\r\n/);
  const leftover = flush ? "" : (parts.pop() ?? "");

  for (const part of parts) {
    const payload = extractEventPayload(part);
    if (payload) {
      onEvent(payload);
    }
  }

  return leftover;
}

export async function streamAgentVisionRun(
  options: Omit<StreamAgentRunOptions, "prompt"> & {
    prompt: string;
    files: File[];
  },
): Promise<void> {
  const { response, decoder } = await postAgentVisionRun(options);
  await streamAgentResponse(
    response,
    decoder,
    options.onEvent,
    "agent vision run",
  );
}

async function postAgentVisionRun(
  options: Omit<StreamAgentRunOptions, "prompt"> & {
    prompt: string;
    files: File[];
  },
) {
  const {
    prompt,
    sessionId,
    assistantMessageId,
    files,
    fetchImpl,
    signal,
    onEvent,
    specialist,
    teamName,
    projectId,
  } = options;
  const form = new FormData();
  form.set("prompt", prompt);
  if (sessionId) form.set("session_id", sessionId);
  if (assistantMessageId && assistantMessageId.trim())
    form.set("assistant_message_id", assistantMessageId.trim());
  if (projectId && projectId.trim()) form.set("project_id", projectId.trim());
  for (const file of files) {
    form.append("images", file, file.name);
  }

  return postStreamRequest({
    fetchImpl,
    signal,
    onEvent,
    url: chatTargetURL(visionEndpoint, specialist, teamName),
    init: {
      method: "POST",
      headers: { Accept: "text/event-stream" },
      body: form,
    },
  });
}

async function postStreamRequest(options: {
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
  onEvent: (event: ChatStreamEvent) => void;
  url: string;
  init: RequestInit;
}) {
  const { fetchImpl, signal, onEvent, url, init } = options;
  try {
    const response = await (fetchImpl ?? fetch)(url, {
      ...init,
      credentials: "include",
      cache: "no-store",
      keepalive: true,
      signal,
    });
    return { response, decoder: new TextDecoder() };
  } catch (error) {
    emitFetchError(error, onEvent);
    throw error;
  }
}

async function streamAgentResponse(
  response: Response,
  decoder: TextDecoder,
  onEvent: (event: ChatStreamEvent) => void,
  label: string,
) {
  if (!response.ok) {
    const message = `${label} failed (${response.status})`;
    onEvent({ type: "error", data: message });
    throw new Error(message);
  }
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("text/event-stream")) {
    await emitJSONFinal(response, onEvent);
    return;
  }
  if (!response.body) {
    onEvent({ type: "error", data: "stream body missing" });
    throw new Error("stream body missing");
  }
  await readEventStream(response.body.getReader(), decoder, onEvent);
}

async function emitJSONFinal(
  response: Response,
  onEvent: (event: ChatStreamEvent) => void,
) {
  const body: unknown = await response.json().catch(() => ({}));
  const result =
    body &&
    typeof body === "object" &&
    "result" in body &&
    typeof body.result === "string"
      ? body.result
      : "";
  onEvent({ type: "final", data: result });
}

async function readEventStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  decoder: TextDecoder,
  onEvent: (event: ChatStreamEvent) => void,
) {
  let buffer = "";
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      buffer = processBuffer(buffer, onEvent);
    }
    if (buffer.trim().length > 0) processBuffer(buffer, onEvent, true);
  } finally {
    reader.releaseLock();
  }
}
