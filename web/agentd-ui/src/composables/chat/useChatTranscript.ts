import { computed } from "vue";
import type { Ref } from "vue";
import type { AgentdSettings } from "@/api/client";
import type {
  ChatContextMetrics,
  ChatMessage,
  ChatRole,
  SummaryEvent,
} from "@/types/chat";
import {
  contextMetricsFromSummaryEvent,
  localContextMetricsForMessages,
} from "@/stores/chatHelpers";

type AgentDefaults = { agentName: string; model: string };

function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) return items[i];
  }
  return null;
}

export function useChatTranscript({
  activeMessages,
  activeSummaryEvent,
  summarySettingsData,
  selectedProjectId,
  activeSessionId,
}: {
  activeMessages: Ref<ChatMessage[]>;
  activeSummaryEvent: Ref<SummaryEvent | null | undefined>;
  summarySettingsData: Ref<AgentdSettings | undefined>;
  selectedProjectId: Ref<string>;
  activeSessionId: Ref<string>;
}) {
  // --- Context metrics ---
  const configuredSummaryBudget = computed(() =>
    summaryBudgetFromAgentdSettings(summarySettingsData.value),
  );

  const sessionContextMetrics = computed<ChatContextMetrics | undefined>(() => {
    const latestMetrics = findLast(activeMessages.value, (message) =>
      Boolean(message.contextMetrics),
    )?.contextMetrics;
    const streamingMetrics = findLast(activeMessages.value, (message) =>
      Boolean(message.streaming && message.contextMetrics),
    )?.contextMetrics;
    if (streamingMetrics) return streamingMetrics;
    const serverMetrics = findLast(
      activeMessages.value,
      (message) =>
        !!message.contextMetrics &&
        message.contextMetrics.phase !== "client_estimate",
    )?.contextMetrics;
    if (latestMetrics?.phase === "client_estimate") {
      return withKnownContextBudget(latestMetrics, serverMetrics);
    }
    if (serverMetrics) return serverMetrics;
    if (activeSummaryEvent.value) {
      const summaryMetrics = contextMetricsFromSummaryEvent(
        activeSummaryEvent.value,
        configuredSummaryBudget.value ?? undefined,
      );
      if (summaryMetrics) return summaryMetrics;
    }
    return localContextMetricsForMessages(
      activeMessages.value,
      configuredSummaryBudget.value ?? undefined,
    );
  });

  // --- Agent metadata ---
  function parseAgentModelLabel(label?: string): AgentDefaults {
    const raw = (label || "").trim();
    if (!raw) return { agentName: "", model: "" };
    const [maybeAgent, ...rest] = raw.split(":");
    if (rest.length) {
      return { agentName: maybeAgent, model: rest.join(":") };
    }
    return { agentName: "", model: raw };
  }

  function agentMetaForMessage(
    message: ChatMessage,
    sessionAgentDefaults: AgentDefaults,
  ) {
    if (message.role !== "assistant") return null;
    const agentName =
      (message.agentName || message.agent || "").trim() ||
      sessionAgentDefaults.agentName ||
      "Agent";
    const agentModel =
      (message.agentModel || message.model || "").trim() ||
      sessionAgentDefaults.model ||
      "";
    return { agentName, agentModel };
  }

  function agentNameFor(
    message: ChatMessage,
    sessionAgentDefaults: AgentDefaults,
  ) {
    const meta = agentMetaForMessage(message, sessionAgentDefaults);
    if (!meta) return labelForRole(message.role);
    return meta.agentName || labelForRole(message.role);
  }

  function labelForRole(role: ChatRole) {
    switch (role) {
      case "user":
        return "You";
      case "assistant":
        return "Agent";
      case "tool":
        return "Tool";
      case "system":
        return "System";
      default:
        return "Status";
    }
  }

  // --- Formatting ---
  const timeFormatter = new Intl.DateTimeFormat(undefined, {
    hour: "numeric",
    minute: "2-digit",
  });

  function formatTimestamp(value?: string) {
    if (!value) return "";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "";
    return timeFormatter.format(date);
  }

  function snippet(content: string, maxLength = 80) {
    if (!content) return "";
    const trimmed = content.replace(/\s+/g, " ").trim();
    const safeLength = Math.max(4, maxLength);
    return trimmed.length > safeLength
      ? `${trimmed.slice(0, safeLength - 3)}...`
      : trimmed;
  }

  function renderMarkdownOrHtml(
    content: string,
    renderMode: "markdown" | "html",
    renderMarkdownFn: (content: string) => string,
  ) {
    if (renderMode === "html") {
      return content || "";
    }
    return renderMarkdownFn(content);
  }

  return {
    configuredSummaryBudget,
    sessionContextMetrics,
    parseAgentModelLabel,
    agentMetaForMessage,
    agentNameFor,
    labelForRole,
    formatTimestamp,
    snippet,
    renderMarkdownOrHtml,
  };
}

function summaryBudgetFromAgentdSettings(
  settings?: AgentdSettings,
): Pick<
  ChatContextMetrics,
  "contextWindow" | "reserveTokens" | "summaryThreshold"
> | null {
  if (!settings) return null;
  const contextWindow = positiveMetricValue(
    settings.summaryContextWindowTokens,
  );
  const reserveTokens = positiveMetricValue(
    settings.summaryReserveBufferTokens,
  );
  const summaryThreshold =
    positiveMetricValue(settings.summaryTokenBudget) ??
    (contextWindow && reserveTokens
      ? summaryThresholdForBudget(contextWindow, reserveTokens)
      : null);
  if (!contextWindow || !summaryThreshold) return null;
  return {
    contextWindow,
    reserveTokens:
      reserveTokens ?? Math.max(contextWindow - summaryThreshold, 0),
    summaryThreshold,
  };
}

function positiveMetricValue(value: unknown): number | null {
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

function withKnownContextBudget(
  metrics: ChatContextMetrics,
  budgetSource?: ChatContextMetrics,
): ChatContextMetrics {
  if (!budgetSource) return metrics;
  return {
    ...metrics,
    contextWindow: budgetSource.contextWindow,
    summaryThreshold: budgetSource.summaryThreshold,
    reserveTokens: budgetSource.reserveTokens,
    willSummarize: metrics.inputTokens >= budgetSource.summaryThreshold,
  };
}
