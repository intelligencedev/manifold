import {
  answerChatInputRequest as apiAnswerChatInputRequest,
  type ChatStreamEvent,
} from "@/api/chat";
import type { ChatInputRequest, ChatInputRequestChoice } from "@/types/chat";
import type { ChatStoreState } from "@/stores/chatStoreState";

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
  state.updateMessage(sessionId, assistantId, (m) => {
    const requests = [...(m.inputRequests || [])];
    const existing = requests.findIndex((item) => item.id === request.id);
    if (existing === -1) requests.push(request);
    else requests.splice(existing, 1, { ...requests[existing], ...request });
    return { ...m, inputRequests: requests };
  });
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
  updateInputRequest(state, sessionId, assistantId, requestId, (request) => ({
    ...request,
    status: "cancelled",
    error:
      typeof event.error === "string" && event.error.trim()
        ? event.error.trim()
        : "Request cancelled",
  }));
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
    await apiAnswerChatInputRequest(requestId, cleanAnswer, cleanChoiceIds);
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
