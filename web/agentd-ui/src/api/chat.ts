import { apiClient } from "./client";
import type { ChatMessage, ChatSessionMeta } from "@/types/chat";

export type ChatStreamEventType =
  | "thought_summary"
  | "delta"
  | "final"
  | "tool_start"
  | "tool_result"
  | "tts_chunk"
  | "tts_audio"
  | "image"
  | "error"
  | "summary"
  | "agent_start"
  | "agent_delta"
  | "agent_final"
  | "agent_tool_start"
  | "agent_tool_result"
  | "agent_error";

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
  model?: string;
  call_id?: string;
  parent_call_id?: string;
  depth?: number;
  role?: string;
  content?: string;
  error?: string;
  // Summary event fields
  input_tokens?: number;
  token_budget?: number;
  message_count?: number;
  summarized_count?: number;
  [key: string]: unknown;
}

export interface StreamAgentRunOptions {
  prompt: string;
  sessionId?: string;
  fetchImpl?: typeof fetch;
  signal?: AbortSignal;
  onEvent: (event: ChatStreamEvent) => void;
  // Optional specialist override: when set (and not 'orchestrator'),
  // the backend will run that specialist for this request.
  specialist?: string;
  // Optional project context: when provided, backend will sandbox tools under
  // the user's project root and attach { project_id } in the JSON body.
  projectId?: string;
  // When true, request image output from providers that support it (e.g., Google Gemini).
  image?: boolean;
  imageSize?: string;
}

export async function listChatSessions(): Promise<ChatSessionMeta[]> {
  const { data } = await apiClient.get<ChatSessionMeta[]>("/chat/sessions");
  return data;
}

export async function createChatSession(
  name?: string,
): Promise<ChatSessionMeta> {
  const payload = name ? { name } : {};
  const { data } = await apiClient.post<ChatSessionMeta>(
    "/chat/sessions",
    payload,
  );
  return data;
}

export async function renameChatSession(
  id: string,
  name: string,
): Promise<ChatSessionMeta> {
  const { data } = await apiClient.patch<ChatSessionMeta>(
    `/chat/sessions/${encodeURIComponent(id)}`,
    { name },
  );
  return data;
}

export async function deleteChatSession(id: string): Promise<void> {
  await apiClient.delete(`/chat/sessions/${encodeURIComponent(id)}`);
}

export async function fetchChatMessages(
  sessionId: string,
  limit?: number,
): Promise<ChatMessage[]> {
  const { data } = await apiClient.get<ChatMessage[]>(
    `/chat/sessions/${encodeURIComponent(sessionId)}/messages`,
    {
      params: limit ? { limit } : undefined,
    },
  );
  return data;
}

export async function deleteChatMessage(
  sessionId: string,
  messageId: string,
): Promise<void> {
  await apiClient.delete(
    `/chat/sessions/${encodeURIComponent(sessionId)}/messages/${encodeURIComponent(messageId)}`,
  );
}

export async function deleteChatMessagesAfter(
  sessionId: string,
  messageId: string,
  inclusive = false,
): Promise<void> {
  await apiClient.delete(
    `/chat/sessions/${encodeURIComponent(sessionId)}/messages`,
    {
      params: {
        after: messageId,
        inclusive: inclusive ? "true" : "false",
      },
    },
  );
}

export async function generateChatSessionTitle(
  sessionId: string,
  prompt: string,
): Promise<ChatSessionMeta> {
  const { data } = await apiClient.post<ChatSessionMeta>(
    `/chat/sessions/${encodeURIComponent(sessionId)}/title`,
    { prompt },
  );
  return data;
}

const baseURL = (import.meta.env.VITE_AGENTD_BASE_URL || "").replace(/\/$/, "");
const runEndpoint = `${baseURL}/agent/run`;
const visionEndpoint = `${baseURL}/agent/vision`;

export async function streamAgentRun(
  options: StreamAgentRunOptions,
): Promise<void> {
  const {
    prompt,
    sessionId,
    fetchImpl,
    signal,
    onEvent,
    specialist,
    projectId,
  } = options;
  const fetchFn = fetchImpl ?? fetch;
  const payload: Record<string, any> = { prompt, session_id: sessionId };
  if (projectId && projectId.trim()) payload.project_id = projectId.trim();
  if (options.image) payload.image = true;
  if (options.imageSize && options.imageSize.trim())
    payload.image_size = options.imageSize.trim();
  const decoder = new TextDecoder();

  let response: Response;

  // Build endpoint with optional specialist override as a query param.
  let url = runEndpoint;
  if (
    specialist &&
    specialist.trim() &&
    specialist.trim().toLowerCase() !== "orchestrator"
  ) {
    const qp = `specialist=${encodeURIComponent(specialist.trim())}`;
    url = `${url}?${qp}`;
  }

  try {
    response = await fetchFn(url, {
      method: "POST",
      headers: {
        Accept: "text/event-stream",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(payload),
      // Keep auth/session cookies attached for long-running streams.
      credentials: "include",
      // Avoid intermediary caching/buffering affecting SSE delivery.
      cache: "no-store",
      // Hint the browser to keep the request alive on navigation/unload when possible.
      // Note: behavior varies by browser; the server also sends heartbeats.
      keepalive: true,
      signal,
    });
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) {
      onEvent({
        type: "error",
        data: error instanceof Error ? error.message : String(error),
      });
    }
    throw error;
  }

  if (!response.ok) {
    const message = `agent run failed (${response.status})`;
    onEvent({ type: "error", data: message });
    throw new Error(message);
  }

  const contentType = response.headers.get("content-type") || "";

  if (!contentType.includes("text/event-stream")) {
    const body = await response.json().catch(() => ({}));
    const result = typeof body?.result === "string" ? body.result : "";
    onEvent({ type: "final", data: result });
    return;
  }

  if (!response.body) {
    onEvent({ type: "error", data: "stream body missing" });
    throw new Error("stream body missing");
  }

  const reader = response.body.getReader();
  let buffer = "";

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        break;
      }
      const chunk = decoder.decode(value, { stream: true });
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SSE === "true") {
        console.log("[SSE chunk]", JSON.stringify(chunk));
      }
      buffer += chunk;
      buffer = processBuffer(buffer, onEvent);
    }
    // flush remaining buffered data
    if (buffer.trim().length > 0) {
      if (import.meta.env.DEV && import.meta.env.VITE_DEBUG_SSE === "true") {
        console.log("[SSE flush]", JSON.stringify(buffer));
      }
      processBuffer(buffer, onEvent, true);
    }
  } finally {
    reader.releaseLock();
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

// Stream a vision run using multipart/form-data with one or more images.
// The backend endpoint accepts fields:
//  - prompt: string
//  - session_id: string (optional)
//  - images: one or more file parts
export async function streamAgentVisionRun(
  options: Omit<StreamAgentRunOptions, "prompt"> & {
    prompt: string;
    files: File[];
  },
): Promise<void> {
  const {
    prompt,
    sessionId,
    files,
    fetchImpl,
    signal,
    onEvent,
    specialist,
    projectId,
  } = options;
  const fetchFn = fetchImpl ?? fetch;
  const form = new FormData();
  form.set("prompt", prompt);
  if (sessionId) form.set("session_id", sessionId);
  if (projectId && projectId.trim()) form.set("project_id", projectId.trim());
  for (const f of files) {
    form.append("images", f, f.name);
  }

  let response: Response;
  const decoder = new TextDecoder();
  // Build endpoint with optional specialist override as a query param.
  let url = visionEndpoint;
  if (
    specialist &&
    specialist.trim() &&
    specialist.trim().toLowerCase() !== "orchestrator"
  ) {
    const qp = `specialist=${encodeURIComponent(specialist.trim())}`;
    url = `${url}?${qp}`;
  }
  try {
    response = await fetchFn(url, {
      method: "POST",
      headers: { Accept: "text/event-stream" },
      body: form,
      credentials: "include",
      cache: "no-store",
      keepalive: true,
      signal,
    });
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) {
      onEvent({
        type: "error",
        data: error instanceof Error ? error.message : String(error),
      });
    }
    throw error;
  }

  if (!response.ok) {
    const message = `agent vision run failed (${response.status})`;
    onEvent({ type: "error", data: message });
    throw new Error(message);
  }

  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("text/event-stream")) {
    const body = await response.json().catch(() => ({}));
    const result =
      typeof (body as any)?.result === "string" ? (body as any).result : "";
    onEvent({ type: "final", data: result });
    return;
  }
  if (!response.body) {
    onEvent({ type: "error", data: "stream body missing" });
    throw new Error("stream body missing");
  }

  const reader = response.body.getReader();
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
