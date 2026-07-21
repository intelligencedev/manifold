import type { ChatStreamEvent } from "@/api/chat";
import type { SummaryEvent } from "@/types/chat";
import {
  contextMetricsFromEvent,
  contextMetricsFromSummaryEvent,
  memoryContextFromEvent,
  mergeMemoryContext,
  snippet,
  toolDisplayTitle,
  toolInvocationName,
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
import {
  emitAssistantDelta,
  emitAssistantFinal,
  emitAssistantRollback,
  emitAssistantStop,
} from "@/lib/tts/supertonic/speechBus";
import {
  appendResponseText,
  reconcileResponseText,
  responsePartsForMessage,
  rollbackResponseText,
  upsertResponseTool,
} from "@/lib/chat/responseParts";

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
          responseParts: appendResponseText(
            m.responseParts?.length
              ? m.responseParts
              : responsePartsForMessage(m),
            event.data,
          ),
          contextMetrics: withEstimatedAssistantTokens(
            m.contextMetrics,
            m.content + event.data,
          ),
        }));
        emitAssistantDelta(sessionId, assistantId, event.data);
      }
      break;
    }
    case "delta_rollback": {
      const count =
        typeof event.count === "number" && event.count > 0
          ? Math.floor(event.count)
          : 0;
      if (count > 0) {
        state.updateMessage(sessionId, assistantId, (m) => {
          const nextContent = m.content.slice(
            0,
            Math.max(0, m.content.length - count),
          );
          return {
            ...m,
            content: nextContent,
            responseParts: rollbackResponseText(
              m.responseParts?.length
                ? m.responseParts
                : responsePartsForMessage(m),
              count,
            ),
            contextMetrics: withEstimatedAssistantTokens(
              m.contextMetrics,
              nextContent,
            ),
          };
        });
        emitAssistantRollback(sessionId, assistantId, count);
      }
      break;
    }
    case "final": {
      const text = typeof event.data === "string" ? event.data : "";
      const durationMs = eventDurationMs(event);
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        content: text || m.content,
        responseParts: reconcileResponseText(
          m.responseParts,
          text || m.content,
        ),
        durationMs: durationMs ?? m.durationMs,
        contextMetrics: withEstimatedAssistantTokens(
          m.contextMetrics,
          text || m.content,
        ),
        streaming: false,
      }));
      if (text) state.touchSession(sessionId, snippet(text));
      emitAssistantFinal(sessionId, assistantId, text);
      try {
        queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
      } catch {}
      break;
    }
    case "tool_start": {
      upsertToolMessage(state, event, sessionId, streamId, "start");
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        activityToolTitle: toolDisplayTitle(event),
        responseParts: upsertResponseTool(
          m.responseParts?.length
            ? m.responseParts
            : responsePartsForMessage(m),
          {
            id: responseToolID(event),
            type: "tool",
            title: toolDisplayTitle(event),
            status: "running",
            args: typeof event.args === "string" ? event.args : undefined,
          },
        ),
      }));
      break;
    }
    case "tool_result": {
      upsertToolMessage(state, event, sessionId, streamId, "result");
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        activityToolTitle: toolDisplayTitle(event),
        responseParts: upsertResponseTool(
          m.responseParts?.length
            ? m.responseParts
            : responsePartsForMessage(m),
          {
            id: responseToolID(event),
            type: "tool",
            title: toolDisplayTitle(event),
            status: "done",
            args: typeof event.args === "string" ? event.args : undefined,
            result: typeof event.data === "string" ? event.data : undefined,
          },
        ),
      }));
      break;
    }
    case "image": {
      handleImageEvent(state, event, sessionId, assistantId);
      break;
    }
    case "video": {
      handleVideoEvent(state, event, sessionId, assistantId);
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
      emitAssistantStop(sessionId);
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

function responseToolID(event: ChatStreamEvent) {
  if (typeof event.tool_id === "string" && event.tool_id.trim()) {
    return `tool-${event.tool_id.trim()}`;
  }
  const sequence = eventSequence(event);
  return `tool-${toolInvocationName(event)}-${sequence || "current"}`;
}

function upsertToolMessage(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  streamId: string,
  phase: "start" | "result",
) {
  const title = toolInvocationName(event);
  const activityTitle = toolDisplayTitle(event);
  const toolId =
    typeof event.tool_id === "string" && event.tool_id.trim()
      ? event.tool_id.trim()
      : undefined;
  const key = toolId || `${title}:${eventSequence(event) || Date.now()}`;
  const toolIndex = state.toolIndexFor(sessionId, streamId);
  const existingId = toolIndex.get(key);
  const existing = existingId
    ? (state.messagesBySession.value[sessionId] || []).find(
        (message) => message.id === existingId,
      )
    : undefined;
  const now = new Date().toISOString();
  const args = typeof event.args === "string" ? event.args : undefined;
  const data = typeof event.data === "string" ? event.data : undefined;
  if (!existingId || !existing) {
    const id = createId();
    toolIndex.set(key, id);
    state.appendMessage(
      sessionId,
      {
        id,
        role: "tool",
        title,
        content:
          phase === "result"
            ? data || "Tool completed."
            : "Tool invocation started.",
        toolArgs: args,
        createdAt: now,
        streaming: phase === "start",
        activityToolTitle: activityTitle,
      },
      false,
    );
    return;
  }

  state.updateMessage(sessionId, existingId, (message) => ({
    ...message,
    title,
    activityToolTitle: activityTitle,
    content:
      phase === "result"
        ? data || message.content || "Tool completed."
        : message.content || "Tool invocation started.",
    toolArgs: args ?? message.toolArgs,
    streaming: phase === "start",
  }));
}

function eventDurationMs(event: ChatStreamEvent) {
  const raw = event.durationMs ?? event.duration_ms;
  const duration = typeof raw === "number" ? raw : Number(raw);
  return Number.isFinite(duration) && duration >= 0 ? duration : undefined;
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
  appendGeneratedMediaEvent(state, event, sessionId, assistantId, {
    kind: "image",
    fallbackName: "generated image",
    noteLabel: "Image",
  });
}

function handleVideoEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  appendGeneratedMediaEvent(state, event, sessionId, assistantId, {
    kind: "video",
    fallbackName: "generated video",
    noteLabel: "Video",
    includeVideoFields: true,
  });
}

function appendGeneratedMediaEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
  options: {
    kind: "image" | "video";
    fallbackName: string;
    noteLabel: string;
    includeVideoFields?: boolean;
  },
) {
  const name =
    typeof event.name === "string" && event.name.trim()
      ? event.name.trim()
      : options.fallbackName;
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
      kind: options.kind,
      name: name || savedPath || options.kind,
      mime,
      previewUrl: previewUrl || undefined,
      path: savedPath,
    });
    let content = m.content;
    if (savedPath && !content.includes(savedPath)) {
      const note = `${options.noteLabel} saved: ${savedPath}`;
      content = content ? `${content}\n\n${note}` : note;
    }
    if (!options.includeVideoFields) {
      return { ...m, attachments, content };
    }
    return {
      ...m,
      attachments,
      content,
      videoUrl: previewUrl || m.videoUrl,
      videoFilePath: savedPath || m.videoFilePath,
    };
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
