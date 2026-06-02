import type { ChatStreamEvent } from "@/api/chat";
import type { SummaryEvent } from "@/types/chat";
import {
  contextMetricsFromEvent,
  contextMetricsFromSummaryEvent,
  memoryContextFromEvent,
  mergeMemoryContext,
  snippet,
  updateLatestUserBeforeMessage,
  withEstimatedAssistantTokens,
} from "@/stores/chatHelpers";
import { handleAgentTraceEvent } from "@/stores/chatAgentTrace";
import {
  handleInputRequestCancelledEvent,
  handleInputRequestEvent,
} from "@/stores/chatInputRequests";
import type { ChatStoreState } from "@/stores/chatStoreState";
import { createId } from "@/utils/uuid";

type QueryInvalidator = {
  invalidateQueries(options: { queryKey: string[] }): unknown;
};

export function handleStreamEvent(
  state: ChatStoreState,
  queryClient: QueryInvalidator,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
  streamId: string,
) {
  if (!state.isStreamCurrent(sessionId, streamId)) return;
  const sequence = eventSequence(event);
  if (sequence > 0) {
    state.updateMessage(sessionId, assistantId, (message) => ({
      ...message,
      lastRunSequence: Math.max(message.lastRunSequence || 0, sequence),
    }));
  }
  switch (event.type) {
    case "run_started": {
      if (typeof event.run_id === "string" && event.run_id.trim()) {
        const runId = event.run_id.trim();
        const current = state.streamingStateFor(sessionId);
        if (current) {
          state.setStreamingState(sessionId, {
            ...current,
            runId,
          });
        }
        state.updateMessage(sessionId, assistantId, (message) => ({
          ...message,
          runId,
        }));
      }
      break;
    }
    case "thought_summary": {
      if (typeof event.data === "string" && event.data.trim()) {
        state.appendThoughtSummary(sessionId, event.data);
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          activityThoughtSummary: event.data,
        }));
      }
      break;
    }
    case "memory_context": {
      const text = typeof event.data === "string" ? event.data.trim() : "";
      if (text) {
        const incoming = memoryContextFromEvent(event, text);
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          memoryContext: mergeMemoryContext(m.memoryContext, incoming),
        }));
      }
      break;
    }
    case "context_metrics": {
      const metrics = contextMetricsFromEvent(event);
      if (metrics) {
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          contextMetrics: withEstimatedAssistantTokens(metrics, m.content),
        }));
        const nextMessages = updateLatestUserBeforeMessage(
          state.messagesBySession.value[sessionId] || [],
          assistantId,
          (m) => ({ ...m, contextMetrics: metrics }),
        );
        if (nextMessages) state.setMessages(sessionId, nextMessages);
      }
      break;
    }
    case "delta": {
      if (typeof event.data === "string" && event.data) {
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          content: m.content + event.data,
          contextMetrics: withEstimatedAssistantTokens(
            m.contextMetrics,
            m.content + event.data,
          ),
        }));
      }
      break;
    }
    case "final": {
      const text = typeof event.data === "string" ? event.data : "";
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        content: text || m.content,
        contextMetrics: withEstimatedAssistantTokens(
          m.contextMetrics,
          text || m.content,
        ),
        streaming: false,
      }));
      if (text) state.touchSession(sessionId, snippet(text));
      try {
        queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
      } catch {}
      break;
    }
    case "tool_start": {
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        activityToolTitle: event.title || "Tool call",
      }));
      break;
    }
    case "tool_result": {
      if (typeof event.title === "string" && event.title.trim()) {
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          activityToolTitle: event.title,
        }));
      }
      break;
    }
    case "image": {
      handleImageEvent(state, event, sessionId, assistantId);
      break;
    }
    case "tts_chunk":
      break;
    case "tts_audio": {
      const now = new Date().toISOString();
      if (typeof event.url === "string") {
        state.appendMessage(
          sessionId,
          {
            id: createId(),
            role: "tool",
            title: event.title || "Audio response",
            content: "The agent produced an audio reply.",
            createdAt: now,
            audioUrl: event.url,
            audioFilePath:
              typeof event.file_path === "string" ? event.file_path : undefined,
          },
          false,
        );
      }
      break;
    }
    case "summary": {
      handleSummaryEvent(state, event, sessionId, assistantId);
      break;
    }
    case "input_request": {
      handleInputRequestEvent(state, event, sessionId, assistantId);
      break;
    }
    case "input_request_cancelled": {
      handleInputRequestCancelledEvent(state, event, sessionId, assistantId);
      break;
    }
    case "error": {
      const message =
        typeof event.data === "string" ? event.data : "Agent error";
      state.updateMessage(sessionId, assistantId, (existing) => ({
        ...existing,
        streaming: false,
        error: message,
      }));
      break;
    }
    case "agent_start":
    case "agent_delta":
    case "agent_final":
    case "agent_tool_start":
    case "agent_tool_result":
    case "agent_error":
    case "agent_thought_summary": {
      handleAgentTraceEvent(state, event, sessionId, assistantId);
      break;
    }
    default:
      break;
  }
}

function eventSequence(event: ChatStreamEvent) {
  const raw = event.sequence;
  const value = typeof raw === "number" ? raw : Number(raw);
  return Number.isFinite(value) && value > 0 ? value : 0;
}

function handleImageEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  const name =
    typeof event.name === "string" && event.name.trim()
      ? event.name.trim()
      : "generated image";
  const mime = typeof event.mime === "string" ? event.mime : undefined;
  const relPath =
    typeof event.rel_path === "string" ? event.rel_path : undefined;
  const filePath =
    typeof event.file_path === "string" ? event.file_path : undefined;
  const url = typeof event.url === "string" ? event.url : undefined;
  const dataUrl =
    typeof event.data_url === "string" ? event.data_url : undefined;
  const previewUrl = dataUrl || url || relPath || filePath;
  const savedPath = relPath || filePath || url;
  state.updateMessage(sessionId, assistantId, (m) => {
    const attachments = [...(m.attachments || [])];
    attachments.push({
      id: createId(),
      kind: "image",
      name: name || savedPath || "image",
      mime,
      previewUrl: previewUrl || undefined,
      path: savedPath,
    });
    let content = m.content;
    if (savedPath && !content.includes(savedPath)) {
      const note = `Image saved: ${savedPath}`;
      content = content ? `${content}\n\n${note}` : note;
    }
    return { ...m, attachments, content };
  });
}

function handleSummaryEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  const summaryEvt: SummaryEvent = {
    inputTokens:
      typeof event.input_tokens === "number" ? event.input_tokens : 0,
    tokenBudget:
      typeof event.token_budget === "number" ? event.token_budget : 0,
    contextWindow:
      typeof event.context_window === "number"
        ? event.context_window
        : undefined,
    reserveTokens:
      typeof event.reserve_tokens === "number"
        ? event.reserve_tokens
        : undefined,
    messageCount:
      typeof event.message_count === "number" ? event.message_count : 0,
    summarizedCount:
      typeof event.summarized_count === "number" ? event.summarized_count : 0,
    timestamp: new Date().toISOString(),
  };
  state.summaryEventBySession.value = {
    ...state.summaryEventBySession.value,
    [sessionId]: summaryEvt,
  };
  const summaryMetrics = contextMetricsFromSummaryEvent(summaryEvt);
  if (summaryMetrics) {
    state.updateMessage(sessionId, assistantId, (m) => ({
      ...m,
      contextMetrics: summaryMetrics,
    }));
    const nextMessages = updateLatestUserBeforeMessage(
      state.messagesBySession.value[sessionId] || [],
      assistantId,
      (m) => ({ ...m, contextMetrics: summaryMetrics }),
    );
    if (nextMessages) state.setMessages(sessionId, nextMessages);
  }
}
