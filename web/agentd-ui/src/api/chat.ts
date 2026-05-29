import { apiClient } from "./client";
import type {
  AgentThread,
  AgentTraceEntry,
  ChatMessage,
  ChatSessionMeta,
  SpecialistActivityEntry,
  SpecialistActivityRecord,
} from "@/types/chat";

export {
  extractEventPayload,
  streamAgentRun,
  streamAgentVisionRun,
} from "./chatStream";
export type {
  ChatStreamEvent,
  ChatStreamEventType,
  StreamAgentRunOptions,
} from "./chatStream";

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

export async function updateChatSessionProject(
  id: string,
  projectId: string,
): Promise<ChatSessionMeta> {
  const { data } = await apiClient.patch<ChatSessionMeta>(
    `/chat/sessions/${encodeURIComponent(id)}`,
    { projectId },
  );
  return data;
}

export async function updateChatSessionMemorySettings(
  id: string,
  settings: {
    evolvingMemoryEnabled?: boolean;
    beliefMemoryEnabled?: boolean;
  },
): Promise<ChatSessionMeta> {
  const { data } = await apiClient.patch<ChatSessionMeta>(
    `/chat/sessions/${encodeURIComponent(id)}`,
    settings,
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

export async function fetchChatActivities(
  sessionId: string,
): Promise<AgentThread[]> {
  const { data } = await apiClient.get<SpecialistActivityRecord[]>(
    `/chat/sessions/${encodeURIComponent(sessionId)}/activities`,
  );
  return (data || [])
    .filter((record) => Boolean(record?.parentCallId?.trim()))
    .map(mapActivityRecordToThread);
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

export async function answerChatInputRequest(
  requestId: string,
  answer: string,
  choiceIds: string[] = [],
): Promise<void> {
  await apiClient.post(
    `/chat/input-requests/${encodeURIComponent(requestId)}/answer`,
    {
      answer,
      choice_ids: choiceIds,
    },
  );
}

function mapActivityRecordToThread(
  record: SpecialistActivityRecord,
): AgentThread {
  return {
    callId: record.callId,
    assistantMessageId: record.assistantMessageId,
    parentCallId: record.parentCallId,
    agent: record.agent,
    team: record.team,
    model: record.model,
    prompt: record.prompt,
    depth: record.depth,
    status: record.status === "idle" ? "running" : record.status,
    content: record.content || "",
    entries: (record.entries || []).map(mapActivityEntry),
    thoughtSummaries: record.thoughtSummaries || [],
    startedAt: record.startedAt,
    finishedAt: record.finishedAt,
    error: record.error,
  };
}

function mapActivityEntry(entry: SpecialistActivityEntry): AgentTraceEntry {
  const kind =
    entry.type === "error"
      ? "error"
      : entry.type === "tool"
        ? "tool"
        : entry.type === "input_request"
          ? "input_request"
          : "message";
  return {
    id: entry.id,
    type: kind,
    title: entry.title,
    content: entry.content,
    args: entry.args,
    data: entry.data,
    createdAt: entry.createdAt,
  };
}
