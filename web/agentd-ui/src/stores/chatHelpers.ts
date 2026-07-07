import type { ChatStreamEvent } from "@/api/chat";
import type {
  AgentThread,
  AgentTraceEntry,
  ChatMessage,
  ChatSessionMeta,
} from "@/types/chat";
import { createId } from "@/utils/uuid";

export type AgentThreadBase = {
  callId: string;
  parentCallId?: string;
  agentName?: string;
  team?: string;
  model?: string;
  prompt?: string;
  depth: number;
  status?: AgentThread["status"];
  content?: string;
  entries?: AgentTraceEntry[];
  thoughtSummaries?: string[];
  startedAt: string;
  finishedAt?: string;
  error?: string;
};

export function agentThreadKey(callId: string, assistantMessageId?: string) {
  return assistantMessageId ? `${assistantMessageId}:${callId}` : callId;
}

export function newAgentThread(base: AgentThreadBase): AgentThread {
  return {
    callId: base.callId,
    parentCallId: base.parentCallId,
    agent: base.agentName,
    team: base.team,
    model: base.model,
    prompt: base.prompt,
    depth: base.depth,
    status: base.status ?? "running",
    content: base.content ?? "",
    entries: base.entries ?? [],
    thoughtSummaries: base.thoughtSummaries ?? [],
    startedAt: base.startedAt,
    finishedAt: base.finishedAt,
    error: base.error,
  };
}

export function withTeam(thread: AgentThread, team?: string): AgentThread {
  return thread.team || !team ? thread : { ...thread, team };
}

export function appendAgentEntry(
  thread: AgentThread,
  team: string | undefined,
  entry: AgentTraceEntry,
): AgentThread {
  const baseThread = withTeam(thread, team);
  return { ...baseThread, entries: [...baseThread.entries, entry] };
}

export function agentToolEntry(
  event: ChatStreamEvent,
  field: "args" | "data",
  value: string | undefined,
  createdAt: string,
): AgentTraceEntry {
  return {
    id: createId(),
    type: "tool",
    title: toolDisplayTitle(event),
    toolName:
      typeof event.tool_name === "string" && event.tool_name.trim()
        ? event.tool_name.trim()
        : undefined,
    toolTitle:
      typeof event.tool_title === "string" && event.tool_title.trim()
        ? event.tool_title.trim()
        : undefined,
    [field]: value,
    createdAt,
  };
}

export function toolDisplayTitle(event: ChatStreamEvent): string {
  if (typeof event.tool_title === "string" && event.tool_title.trim()) {
    return event.tool_title.trim();
  }
  if (typeof event.title === "string" && event.title.trim()) {
    return event.title.trim();
  }
  return "Tool";
}

export function toolInvocationName(event: ChatStreamEvent): string {
  if (typeof event.tool_name === "string" && event.tool_name.trim()) {
    return event.tool_name.trim();
  }
  if (typeof event.title === "string" && event.title.trim()) {
    return event.title.trim();
  }
  return "Tool call";
}

export function updateLatestUserBeforeMessage(
  messages: ChatMessage[],
  messageId: string,
  updater: (m: ChatMessage) => ChatMessage,
): ChatMessage[] | null {
  const assistantIndex = messages.findIndex((m) => m.id === messageId);
  if (assistantIndex <= 0) return null;
  for (let i = assistantIndex - 1; i >= 0; i -= 1) {
    if (messages[i].role !== "user") continue;
    const next = [...messages];
    next.splice(i, 1, updater(messages[i]));
    return next;
  }
  return null;
}

export function normalizeSessionMeta(meta: ChatSessionMeta): ChatSessionMeta {
  type ChatSessionMetaWire = ChatSessionMeta & {
    message_count?: unknown;
    project_id?: unknown;
    memory_enabled?: unknown;
    evolving_memory_enabled?: unknown;
    belief_memory_enabled?: unknown;
    command_policy_allow_all?: unknown;
    active_specialist?: unknown;
    active_team?: unknown;
    pinned?: unknown;
  };
  const wire = meta as ChatSessionMetaWire;
  const rawCount = wire.messageCount ?? wire.message_count;
  const messageCount =
    typeof rawCount === "number" && Number.isFinite(rawCount) && rawCount >= 0
      ? rawCount
      : 0;
  const rawProjectID = wire.projectId ?? wire.project_id;
  const projectId = typeof rawProjectID === "string" ? rawProjectID : "";
  const rawEvolvingMemoryEnabled =
    wire.evolvingMemoryEnabled ?? wire.evolving_memory_enabled;
  const rawBeliefMemoryEnabled =
    wire.beliefMemoryEnabled ?? wire.belief_memory_enabled;
  const rawMemoryEnabled = wire.memoryEnabled ?? wire.memory_enabled;
  const legacyMemoryEnabled =
    typeof rawEvolvingMemoryEnabled === "boolean" &&
    typeof rawBeliefMemoryEnabled === "boolean"
      ? rawEvolvingMemoryEnabled && rawBeliefMemoryEnabled
      : false;
  const memoryEnabled =
    typeof rawMemoryEnabled === "boolean"
      ? rawMemoryEnabled
      : legacyMemoryEnabled;
  const rawCommandPolicyAllowAll =
    wire.commandPolicyAllowAll ?? wire.command_policy_allow_all;
  const rawActiveSpecialist =
    wire.activeSpecialist ?? wire.active_specialist;
  const activeSpecialist =
    typeof rawActiveSpecialist === "string"
      ? rawActiveSpecialist.trim()
      : "";
  const rawActiveTeam = wire.activeTeam ?? wire.active_team;
  const activeTeam = typeof rawActiveTeam === "string" ? rawActiveTeam.trim() : "";
  const rawPinned = wire.pinned;
  return {
    ...meta,
    messageCount,
    projectId,
    memoryEnabled,
    evolvingMemoryEnabled: memoryEnabled,
    beliefMemoryEnabled: memoryEnabled,
    commandPolicyAllowAll:
      typeof rawCommandPolicyAllowAll === "boolean"
        ? rawCommandPolicyAllowAll
        : false,
    activeSpecialist,
    activeTeam,
    pinned: typeof rawPinned === "boolean" ? rawPinned : false,
  };
}

export function sortChatSessions(
  sessions: ChatSessionMeta[],
): ChatSessionMeta[] {
  return [...sessions].sort((a, b) => {
    const aPinned = Boolean(a.pinned);
    const bPinned = Boolean(b.pinned);
    if (aPinned !== bPinned) return aPinned ? -1 : 1;
    const updatedDiff = timeValue(b.updatedAt) - timeValue(a.updatedAt);
    if (updatedDiff !== 0) return updatedDiff;
    const createdDiff = timeValue(b.createdAt) - timeValue(a.createdAt);
    if (createdDiff !== 0) return createdDiff;
    return a.id.localeCompare(b.id);
  });
}

function timeValue(value?: string) {
  if (!value) return 0;
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : 0;
}

const defaultSessionNames = new Set(["", "new chat", "conversation"]);

export function isDefaultSessionName(name?: string | null) {
  return !name || defaultSessionNames.has(name.trim().toLowerCase());
}

export function httpStatus(error: unknown): number | null {
  if (!error || typeof error !== "object" || !("isAxiosError" in error)) {
    return null;
  }
  const maybeResponse = (error as { response?: { status?: unknown } }).response;
  return typeof maybeResponse?.status === "number"
    ? maybeResponse.status
    : null;
}

export function snippet(content: string) {
  if (!content) return "";
  const trimmed = content.replace(/\s+/g, " ").trim();
  return trimmed.length > 80 ? `${trimmed.slice(0, 77)}...` : trimmed;
}

const CHAT_TITLE_MAX_RUNES = 48;

function collapseWhitespace(s: string): string {
  if (!s || !s.trim()) return "";
  return s.trim().replace(/\s+/g, " ");
}

function truncateRunes(s: string, max: number): string {
  if (max <= 0) return "";
  const codepoints = Array.from(s);
  if (codepoints.length <= max) return s.trim();
  return codepoints.slice(0, max).join("").trim();
}

function firstSentence(s: string): string {
  const input = s.trim();
  if (!input) return "";
  for (let i = 0; i < input.length; i++) {
    const ch = input[i];
    if (ch === "." || ch === "?" || ch === "!" || ch === "\n") {
      return input.slice(0, i + 1).trim();
    }
  }
  return input;
}

export function computeLocalTitle(prompt: string): string {
  const sentence = firstSentence(prompt) || prompt;
  const collapsed = collapseWhitespace(sentence);
  if (!collapsed) return "Conversation";
  return truncateRunes(collapsed, CHAT_TITLE_MAX_RUNES);
}

export {
  contextMetricsFromEvent,
  contextMetricsFromSummaryEvent,
  estimateResponseTokens,
  localContextMetricsForMessages,
  memoryContextFromEvent,
  mergeMemoryContext,
  withEstimatedAssistantTokens,
} from "@/stores/chatContextMetrics";
