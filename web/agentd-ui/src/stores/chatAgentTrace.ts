import type { ChatStreamEvent } from "@/api/chat";
import type { AgentThread, AgentTraceEntry } from "@/types/chat";
import {
  agentToolEntry,
  appendAgentEntry,
  newAgentThread,
  toolDisplayTitle,
  withTeam,
} from "@/stores/chatHelpers";
import {
  responsePartsForMessage,
  upsertResponseTool,
} from "@/lib/chat/responseParts";
import type { ChatStoreState } from "@/stores/chatStoreState";
import { createId } from "@/utils/uuid";

export function handleAgentTraceEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  const now = new Date().toISOString();
  const callId =
    typeof event.call_id === "string" && event.call_id.trim()
      ? event.call_id.trim()
      : createId();
  const depth =
    typeof event.depth === "number" && event.depth >= 0 ? event.depth : 1;
  const parentCallId =
    typeof event.parent_call_id === "string" ? event.parent_call_id : undefined;
  const agentName = typeof event.agent === "string" ? event.agent : undefined;
  const team = typeof event.team === "string" ? event.team : undefined;
  const model = typeof event.model === "string" ? event.model : undefined;
  const contentText =
    typeof event.content === "string" ? event.content : undefined;
  const args = typeof event.args === "string" ? event.args : undefined;
  const data = typeof event.data === "string" ? event.data : undefined;
  const threadBase = {
    callId,
    parentCallId,
    agentName,
    team,
    model,
    depth,
    startedAt: now,
  };
  const saveThread = (
    factory: () => AgentThread,
    updater?: (thread: AgentThread) => AgentThread,
  ) => {
    state.upsertAgentThread(sessionId, callId, factory, updater, assistantId);
  };

  if (!parentCallId?.trim()) {
    handleDirectAgentTraceEvent(state, event, sessionId, assistantId, {
      agentName,
      model,
      contentText,
      data,
    });
    return;
  }

  switch (event.type) {
    case "agent_start": {
      saveThread(
        () => newAgentThread({ ...threadBase, prompt: contentText }),
        undefined,
      );
      break;
    }
    case "agent_delta": {
      saveThread(
        () =>
          newAgentThread({
            ...threadBase,
            prompt: contentText,
            content: contentText || "",
          }),
        (thread) => ({
          ...withTeam(thread, team),
          content: (thread.content || "") + (contentText || ""),
        }),
      );
      break;
    }
    case "agent_final": {
      saveThread(
        () =>
          newAgentThread({
            ...threadBase,
            prompt: contentText,
            status: "done",
            content: contentText || "",
            finishedAt: now,
          }),
        (thread) => ({
          ...withTeam(thread, team),
          status: "done",
          finishedAt: thread.finishedAt || now,
          content: contentText || thread.content,
        }),
      );
      break;
    }
    case "agent_tool_start": {
      const entry = agentToolEntry(event, "args", args, now);
      saveThread(
        () => newAgentThread({ ...threadBase, entries: [entry] }),
        (thread) => appendAgentEntry(thread, team, entry),
      );
      break;
    }
    case "agent_tool_result": {
      const entry = agentToolEntry(event, "data", data, now);
      saveThread(
        () => newAgentThread({ ...threadBase, entries: [entry] }),
        (thread) => appendAgentEntry(thread, team, entry),
      );
      break;
    }
    case "agent_error": {
      const errText =
        typeof event.error === "string" ? event.error : data || "Agent error";
      const entry: AgentTraceEntry = {
        id: createId(),
        type: "error",
        content: errText,
        createdAt: now,
      };
      saveThread(
        () =>
          newAgentThread({
            ...threadBase,
            prompt: contentText,
            status: "error",
            content: contentText || "",
            entries: [entry],
            finishedAt: now,
            error: errText,
          }),
        (thread) => ({
          ...appendAgentEntry(thread, team, entry),
          status: "error",
          finishedAt: thread.finishedAt || now,
          error: errText,
        }),
      );
      break;
    }
    case "agent_thought_summary": {
      const summary =
        typeof event.thought_summary === "string"
          ? event.thought_summary.trim()
          : "";
      if (!summary) break;
      saveThread(
        () => newAgentThread({ ...threadBase, thoughtSummaries: [summary] }),
        (thread) => {
          const baseThread = withTeam(thread, team);
          const existing = baseThread.thoughtSummaries || [];
          const last = existing[existing.length - 1];
          if (last) {
            if (summary === last) return baseThread;
            if (summary.length > last.length && summary.startsWith(last)) {
              const next = [...existing];
              next[next.length - 1] = summary;
              return { ...baseThread, thoughtSummaries: next };
            }
          }
          return {
            ...baseThread,
            thoughtSummaries: [...existing, summary],
          };
        },
      );
      break;
    }
    default:
      break;
  }
}

function handleDirectAgentTraceEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
  values: {
    agentName?: string;
    model?: string;
    contentText?: string;
    data?: string;
  },
) {
  switch (event.type) {
    case "agent_start": {
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        agentName: m.agentName || values.agentName,
        agentModel: m.agentModel || values.model,
        model: m.model || values.model,
      }));
      break;
    }
    case "agent_delta": {
      if (values.contentText) {
        state.updateMessage(sessionId, assistantId, (m) => ({
          ...m,
        }));
      }
      break;
    }
    case "agent_final": {
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
      }));
      break;
    }
    case "agent_tool_start":
    case "agent_tool_result": {
      const now = new Date().toISOString();
      const isResult = event.type === "agent_tool_result";
      const entryValue = isResult
        ? typeof event.data === "string"
          ? event.data
          : values.data
        : typeof event.args === "string"
          ? event.args
          : undefined;
      const entry = agentToolEntry(
        event,
        isResult ? "data" : "args",
        entryValue,
        now,
      );
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        activityToolTitle:
          typeof event.tool_title === "string" && event.tool_title.trim()
            ? event.tool_title
            : typeof event.title === "string" && event.title.trim()
              ? event.title
              : m.activityToolTitle,
        activityToolEntries: [...(m.activityToolEntries || []), entry],
        responseParts: upsertResponseTool(
          m.responseParts?.length
            ? m.responseParts
            : responsePartsForMessage(m),
          {
            id: directResponseToolID(event, callId),
            type: "tool",
            title: toolDisplayTitle(event),
            status: isResult ? "done" : "running",
            args: typeof event.args === "string" ? event.args : undefined,
            result: isResult ? entryValue : undefined,
          },
        ),
      }));
      break;
    }
    case "agent_error": {
      const errText =
        typeof event.error === "string"
          ? event.error
          : values.data || "Agent error";
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        streaming: false,
        error: errText,
      }));
      break;
    }
    case "agent_thought_summary": {
      const summary =
        typeof event.thought_summary === "string"
          ? event.thought_summary.trim()
          : "";
      if (!summary) break;
      state.updateMessage(sessionId, assistantId, (m) => ({
        ...m,
        activityThoughtSummary: summary,
      }));
      break;
    }
    default:
      break;
  }
}

function directResponseToolID(event: ChatStreamEvent, callId: string) {
  if (typeof event.tool_id === "string" && event.tool_id.trim()) {
    return `tool-${event.tool_id.trim()}`;
  }
  return `tool-${callId}-${toolDisplayTitle(event)}`;
}
