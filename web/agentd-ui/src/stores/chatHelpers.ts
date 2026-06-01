import type { ChatStreamEvent } from "@/api/chat";
import type {
  AgentThread,
  AgentTraceEntry,
  ChatContextMetricSegment,
  ChatContextMetricSegmentKind,
  ChatContextMetrics,
  ChatMemoryContext,
  ChatMemoryContextLane,
  ChatMessage,
  ChatSessionMeta,
  SummaryEvent,
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
    title: event.title || "Tool",
    [field]: value,
    createdAt,
  };
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
  return {
    ...meta,
    messageCount,
    projectId,
    memoryEnabled,
    evolvingMemoryEnabled: memoryEnabled,
    beliefMemoryEnabled: memoryEnabled,
  };
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

function numericEventValue(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value)
    ? value
    : undefined;
}

function booleanEventValue(value: unknown): boolean | undefined {
  return typeof value === "boolean" ? value : undefined;
}

function parseMemoryContextLanes(
  value: unknown,
): Record<string, ChatMemoryContextLane> | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const lanes: Record<string, ChatMemoryContextLane> = {};
  for (const [name, rawLane] of Object.entries(
    value as Record<string, unknown>,
  )) {
    if (!rawLane || typeof rawLane !== "object" || Array.isArray(rawLane)) {
      continue;
    }
    const lane = rawLane as Record<string, unknown>;
    lanes[name] = {
      enabled: booleanEventValue(lane.enabled),
      returned: booleanEventValue(lane.returned),
      timedOut: booleanEventValue(lane.timed_out ?? lane.timedOut),
      error:
        typeof lane.error === "string" && lane.error.trim()
          ? lane.error.trim()
          : undefined,
      durationMs: numericEventValue(lane.duration_ms ?? lane.durationMs),
      items: numericEventValue(lane.items),
      tokens: numericEventValue(lane.tokens),
    };
  }
  return Object.keys(lanes).length ? lanes : undefined;
}

export function memoryContextFromEvent(
  event: ChatStreamEvent,
  text: string,
): ChatMemoryContext {
  return {
    text,
    tokenEstimate: numericEventValue(event.token_estimate),
    truncated: event.truncated === true,
    durationMs: numericEventValue(event.duration_ms),
    lanes: parseMemoryContextLanes(event.lanes),
  };
}

export function mergeMemoryContext(
  existing: ChatMemoryContext | undefined,
  incoming: ChatMemoryContext,
): ChatMemoryContext {
  if (!existing?.text) return incoming;
  return {
    ...incoming,
    text: `${existing.text}\n\n${incoming.text}`,
    tokenEstimate:
      (existing.tokenEstimate ?? 0) + (incoming.tokenEstimate ?? 0) ||
      undefined,
    truncated: Boolean(existing.truncated || incoming.truncated),
    lanes: {
      ...(existing.lanes || {}),
      ...(incoming.lanes || {}),
    },
  };
}

const contextMetricKinds = new Set<ChatContextMetricSegmentKind>([
  "system",
  "history",
  "user",
  "memory",
  "tools",
  "summary",
  "assistant",
]);

const localMetricContextWindow = 32_000;
const localMetricReserveTokens = 25_000;

type ContextMetricBudget = {
  contextWindow?: number;
  reserveTokens?: number;
  summaryThreshold?: number;
};

export function localContextMetricsForMessages(
  messages: ChatMessage[],
  budget: ContextMetricBudget = {},
): ChatContextMetrics {
  const segments = localContextMetricSegments(messages);
  const inputTokens = segments.reduce(
    (sum, segment) => sum + segment.tokens,
    0,
  );
  const contextWindow =
    positiveNumber(budget.contextWindow) ?? localMetricContextWindow;
  const reserveTokens =
    positiveNumber(budget.reserveTokens) ?? localMetricReserveTokens;
  const summaryThreshold =
    positiveNumber(budget.summaryThreshold) ??
    summaryThresholdForBudget(contextWindow, reserveTokens);
  return {
    phase: "client_estimate",
    inputTokens,
    contextWindow,
    summaryThreshold,
    reserveTokens,
    messageCount: messages.length,
    willSummarize: inputTokens > summaryThreshold,
    segments,
  };
}

export function contextMetricsFromEvent(
  event: ChatStreamEvent,
): ChatContextMetrics | null {
  const inputTokens = numericEventValue(
    event.input_tokens ?? event.inputTokens,
  );
  const contextWindow = numericEventValue(
    event.context_window ?? event.contextWindow,
  );
  const summaryThreshold = numericEventValue(
    event.summary_threshold ?? event.summaryThreshold ?? event.token_budget,
  );
  const reserveTokens = numericEventValue(
    event.reserve_tokens ?? event.reserveTokens,
  );
  if (!inputTokens || !contextWindow || !summaryThreshold) return null;
  return {
    phase: typeof event.phase === "string" ? event.phase : "",
    inputTokens,
    contextWindow,
    summaryThreshold,
    reserveTokens:
      reserveTokens ?? Math.max(contextWindow - summaryThreshold, 0),
    messageCount:
      numericEventValue(event.message_count ?? event.messageCount) ?? 0,
    summarizedCount: numericEventValue(
      event.summarized_count ?? event.summarizedCount,
    ),
    willSummarize:
      event.will_summarize === true || event.willSummarize === true,
    segments: parseContextMetricSegments(event.segments),
  };
}

export function contextMetricsFromSummaryEvent(
  event: SummaryEvent,
  budget: ContextMetricBudget = {},
): ChatContextMetrics | null {
  if (event.inputTokens <= 0 || event.tokenBudget <= 0) return null;
  const contextWindow =
    positiveNumber(event.contextWindow) ??
    positiveNumber(budget.contextWindow) ??
    event.tokenBudget;
  const summaryThreshold = event.tokenBudget;
  const reserveTokens =
    positiveNumber(event.reserveTokens) ??
    positiveNumber(budget.reserveTokens) ??
    Math.max(contextWindow - summaryThreshold, 0);
  return {
    phase: "summary_triggered",
    inputTokens: event.inputTokens,
    contextWindow,
    summaryThreshold,
    reserveTokens,
    messageCount: event.messageCount,
    summarizedCount: event.summarizedCount,
    willSummarize: event.inputTokens >= summaryThreshold,
    segments: [{ kind: "history", tokens: event.inputTokens }],
  };
}

function positiveNumber(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? value
    : null;
}

function summaryThresholdForBudget(
  contextWindow: number,
  reserveTokens: number,
): number {
  const budget = contextWindow - reserveTokens;
  return budget > 0 ? budget : Math.floor(contextWindow / 2);
}

function localContextMetricSegments(
  messages: ChatMessage[],
): ChatContextMetricSegment[] {
  const totals = new Map<ChatContextMetricSegmentKind, number>();
  for (const message of messages) {
    const tokens = estimateResponseTokens(message.content) + 8;
    if (tokens <= 8) continue;
    const kind = localContextMetricKind(message.role);
    totals.set(kind, (totals.get(kind) ?? 0) + tokens);
  }
  return Array.from(totals, ([kind, tokens]) => ({ kind, tokens }));
}

function localContextMetricKind(
  role: ChatMessage["role"],
): ChatContextMetricSegmentKind {
  if (role === "system") return "system";
  if (role === "assistant") return "assistant";
  if (role === "tool") return "tools";
  if (role === "status") return "history";
  return "user";
}

export function estimateResponseTokens(text: string): number {
  const trimmed = text.trim();
  if (!trimmed) return 0;
  return Math.max(1, Math.ceil(trimmed.length / 4));
}

export function withEstimatedAssistantTokens(
  metrics: ChatContextMetrics | undefined,
  content: string,
): ChatContextMetrics | undefined {
  if (!metrics) return metrics;
  const estimatedTokens = estimateResponseTokens(content);
  if (estimatedTokens <= 0) return metrics;
  if (
    metrics.phase === "assistant_added" &&
    metrics.segments.some((segment) => segment.kind === "assistant")
  ) {
    return metrics;
  }
  const segments = metrics.segments.filter(
    (segment) => segment.kind !== "assistant",
  );
  segments.push({ kind: "assistant", tokens: estimatedTokens });
  const nonAssistantTotal = segments.reduce(
    (sum, segment) => sum + segment.tokens,
    0,
  );
  return {
    ...metrics,
    inputTokens: Math.max(metrics.inputTokens, nonAssistantTotal),
    segments,
  };
}

function parseContextMetricSegments(
  value: unknown,
): ChatContextMetricSegment[] {
  if (!Array.isArray(value)) return [];
  const segments: ChatContextMetricSegment[] = [];
  for (const raw of value) {
    if (!raw || typeof raw !== "object") continue;
    const segment = raw as Record<string, unknown>;
    const kind = typeof segment.kind === "string" ? segment.kind : "";
    const tokens = numericEventValue(segment.tokens);
    if (!contextMetricKinds.has(kind as ChatContextMetricSegmentKind)) {
      continue;
    }
    if (!tokens || tokens <= 0) continue;
    segments.push({ kind: kind as ChatContextMetricSegmentKind, tokens });
  }
  return segments;
}
