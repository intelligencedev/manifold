import type { ChatStreamEvent } from "@/api/chat";
import type {
  ChatContextMetricSegment,
  ChatContextMetricSegmentKind,
  ChatContextMetrics,
  ChatMemoryContext,
  ChatMemoryContextLane,
  ChatMessage,
  SummaryEvent,
} from "@/types/chat";

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
