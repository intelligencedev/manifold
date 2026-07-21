import {
  answerChatInputRequest as apiAnswerChatInputRequest,
  type ChatStreamEvent,
} from "@/api/chat";
import type {
  ChatInputRequest,
  ChatInputRequestChoice,
  ChatMessage,
} from "@/types/chat";
import type { ChatStoreState } from "@/stores/chatStoreState";
import {
  responsePartsForMessage,
  upsertResponseInputRequest,
} from "@/lib/chat/responseParts";

export function normalizeInputRequestChoices(
  choices: unknown,
): ChatInputRequestChoice[] {
  if (!Array.isArray(choices)) return [];
  return choices
    .map((choice, index) => {
      if (typeof choice === "string") {
        const label = choice.trim();
        if (!label) return null;
        return { id: `choice_${index + 1}`, label };
      }
      if (!choice || typeof choice !== "object") return null;
      const node = choice as Record<string, unknown>;
      const rawId = typeof node.id === "string" ? node.id.trim() : "";
      const rawLabel = typeof node.label === "string" ? node.label.trim() : "";
      const label = rawLabel || rawId;
      if (!label) return null;
      const description =
        typeof node.description === "string" && node.description.trim()
          ? node.description.trim()
          : undefined;
      return {
        id: rawId || `choice_${index + 1}`,
        label,
        description,
      };
    })
    .filter((choice): choice is ChatInputRequestChoice => Boolean(choice));
}

export function handleInputRequestEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  const requestId =
    typeof event.request_id === "string" && event.request_id.trim()
      ? event.request_id.trim()
      : "";
  const question =
    typeof event.question === "string" && event.question.trim()
      ? event.question.trim()
      : "";
  if (!requestId || !question) return;
  const request: ChatInputRequest = {
    id: requestId,
    runId:
      typeof event.run_id === "string" && event.run_id.trim()
        ? event.run_id.trim()
        : undefined,
    question,
    reason:
      typeof event.reason === "string" && event.reason.trim()
        ? event.reason.trim()
        : undefined,
    choices: normalizeInputRequestChoices(event.choices),
    allowFreeText: Boolean(event.allow_free_text),
    multiple: Boolean(event.multiple),
    agent:
      typeof event.agent === "string" && event.agent.trim()
        ? event.agent.trim()
        : undefined,
    model:
      typeof event.model === "string" && event.model.trim()
        ? event.model.trim()
        : undefined,
    callId:
      typeof event.call_id === "string" && event.call_id.trim()
        ? event.call_id.trim()
        : undefined,
    parentCallId:
      typeof event.parent_call_id === "string" && event.parent_call_id.trim()
        ? event.parent_call_id.trim()
        : undefined,
    depth:
      typeof event.depth === "number" && Number.isFinite(event.depth)
        ? event.depth
        : undefined,
    status: "pending",
    createdAt:
      typeof event.created_at === "string" && event.created_at.trim()
        ? event.created_at
        : new Date().toISOString(),
  };
  const targetMessageId = inputRequestTargetMessageId(
    state,
    sessionId,
    assistantId,
    request.runId,
  );
  const update = (m: ChatMessage) => {
    const requests = [...(m.inputRequests || [])];
    const existing = requests.findIndex((item) => item.id === request.id);
    if (existing === -1) requests.push(request);
    else requests.splice(existing, 1, { ...requests[existing], ...request });
    return {
      ...m,
      inputRequests: requests,
      responseParts: upsertResponseInputRequest(
        responsePartsForMessage(m),
        request.id,
      ),
    };
  };
  if (targetMessageId) {
    state.updateMessage(sessionId, targetMessageId, update);
    return;
  }
  state.appendMessage(
    sessionId,
    update({
      id: assistantId || `input-request-${request.id}`,
      role: "assistant",
      content: "",
      createdAt: request.createdAt,
      streaming: true,
      runId: request.runId,
    }),
    false,
  );
}

export function handleInputRequestCancelledEvent(
  state: ChatStoreState,
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  const requestId =
    typeof event.request_id === "string" && event.request_id.trim()
      ? event.request_id.trim()
      : "";
  if (!requestId) return;
  const targetMessageId =
    inputRequestMessageId(state, sessionId, requestId) || assistantId;
  updateInputRequest(
    state,
    sessionId,
    targetMessageId,
    requestId,
    (request) => ({
      ...request,
      status: "cancelled",
      error:
        typeof event.error === "string" && event.error.trim()
          ? event.error.trim()
          : "Request cancelled",
    }),
  );
}

function inputRequestTargetMessageId(
  state: ChatStoreState,
  sessionId: string,
  assistantId: string,
  runId?: string,
) {
  const messages = state.messagesBySession.value[sessionId] || [];
  if (messages.some((message) => message.id === assistantId)) {
    return assistantId;
  }
  const assistants = messages.filter((message) => message.role === "assistant");
  if (runId) {
    const matchingRun = [...assistants]
      .reverse()
      .find((message) => message.runId === runId);
    if (matchingRun) return matchingRun.id;
  }
  return (
    [...assistants].reverse().find((message) => message.streaming)?.id || ""
  );
}

function inputRequestMessageId(
  state: ChatStoreState,
  sessionId: string,
  requestId: string,
) {
  return [...(state.messagesBySession.value[sessionId] || [])]
    .reverse()
    .find((message) =>
      message.inputRequests?.some((request) => request.id === requestId),
    )?.id;
}

export function updateInputRequest(
  state: ChatStoreState,
  sessionId: string,
  messageId: string,
  requestId: string,
  updater: (request: ChatInputRequest) => ChatInputRequest,
) {
  state.updateMessage(sessionId, messageId, (m) => {
    const requests = m.inputRequests || [];
    const next = requests.map((request) =>
      request.id === requestId ? updater(request) : request,
    );
    return { ...m, inputRequests: next };
  });
}

export async function submitInputRequest(
  state: ChatStoreState,
  sessionId: string,
  messageId: string,
  requestId: string,
  answer: string,
  choiceIds: string[] = [],
) {
  const cleanChoiceIds = choiceIds
    .map((id) => id.trim())
    .filter((id) => id.length > 0);
  const cleanAnswer = answer.trim();
  try {
    const current = (state.messagesBySession.value[sessionId] || [])
      .find((message) => message.id === messageId)
      ?.inputRequests?.find((request) => request.id === requestId);
    await apiAnswerChatInputRequest(
      requestId,
      cleanAnswer,
      cleanChoiceIds,
      current?.runId,
    );
    updateInputRequest(state, sessionId, messageId, requestId, (request) => ({
      ...request,
      status: "answered",
      answer: cleanAnswer,
      choiceIds: cleanChoiceIds,
      answeredAt: new Date().toISOString(),
      error: undefined,
    }));
  } catch (error) {
    updateInputRequest(state, sessionId, messageId, requestId, (request) => ({
      ...request,
      status: "error",
      error:
        error instanceof Error ? error.message : "Failed to submit response",
    }));
    throw error;
  }
}
