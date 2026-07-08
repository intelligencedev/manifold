import { nextTick, ref, toValue } from "vue";
import type { MaybeRefOrGetter } from "vue";
import type { ChatInputRequest, ChatMessage } from "@/types/chat";

type SessionIdSource = MaybeRefOrGetter<string>;

type ChatInputRequestActions = {
  submitInputRequest: (
    sessionId: string,
    messageId: string,
    requestId: string,
    answer: string,
    choiceIds: string[],
  ) => Promise<unknown>;
  isSessionStreaming: (sessionId: string) => boolean;
  resumeDurableRun: (
    sessionId: string,
    messageId: string,
    runId: string,
  ) => Promise<unknown>;
};

export function useChatInputRequests({
  activeSessionId,
  chat,
}: {
  activeSessionId: SessionIdSource;
  chat: ChatInputRequestActions;
}) {
  const inputRequestDrafts = ref<Record<string, string>>({});
  const inputRequestSelections = ref<Record<string, string[]>>({});
  const inputRequestSubmitting = ref<Record<string, boolean>>({});
  const inputRequestErrors = ref<Record<string, string>>({});

  function inputRequestKey(message: ChatMessage, request: ChatInputRequest) {
    return `${message.id}:${request.id}`;
  }

  function inputRequestFieldName(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    return `input-request-${inputRequestKey(message, request)}`;
  }

  function isInputRequestRespondable(request: ChatInputRequest) {
    return request.status === "pending" || request.status === "error";
  }

  function inputRequestStatusLabel(request: ChatInputRequest) {
    switch (request.status) {
      case "answered":
        return "Response submitted";
      case "cancelled":
        return "Request cancelled";
      case "error":
        return "Response required";
      default:
        return "Response required";
    }
  }

  function inputRequestCardClasses(request: ChatInputRequest) {
    return {
      "input-request-card--answered": request.status === "answered",
      "input-request-card--cancelled": request.status === "cancelled",
      "input-request-card--error": request.status === "error",
    };
  }

  function inputRequestSelection(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    const key = inputRequestKey(message, request);
    return inputRequestSelections.value[key] || request.choiceIds || [];
  }

  function inputRequestChoiceSelected(
    message: ChatMessage,
    request: ChatInputRequest,
    choiceId: string,
  ) {
    return inputRequestSelection(message, request).includes(choiceId);
  }

  function toggleInputRequestChoice(
    message: ChatMessage,
    request: ChatInputRequest,
    choiceId: string,
  ) {
    if (!isInputRequestRespondable(request)) return;
    const key = inputRequestKey(message, request);
    const current = inputRequestSelection(message, request);
    let next: string[];
    if (request.multiple) {
      next = current.includes(choiceId)
        ? current.filter((id) => id !== choiceId)
        : [...current, choiceId];
    } else {
      next = [choiceId];
    }
    inputRequestSelections.value = {
      ...inputRequestSelections.value,
      [key]: next,
    };
    inputRequestErrors.value = { ...inputRequestErrors.value, [key]: "" };
  }

  function inputRequestDraft(message: ChatMessage, request: ChatInputRequest) {
    return inputRequestDrafts.value[inputRequestKey(message, request)] || "";
  }

  function setInputRequestDraft(
    message: ChatMessage,
    request: ChatInputRequest,
    value: string,
  ) {
    inputRequestDrafts.value = {
      ...inputRequestDrafts.value,
      [inputRequestKey(message, request)]: value,
    };
  }

  function isInputRequestSubmitting(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    return Boolean(
      inputRequestSubmitting.value[inputRequestKey(message, request)],
    );
  }

  function inputRequestLocalError(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    return inputRequestErrors.value[inputRequestKey(message, request)] || "";
  }

  function canSubmitInputRequest(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    if (!isInputRequestRespondable(request)) return false;
    if (isInputRequestSubmitting(message, request)) return false;
    const selected = inputRequestSelection(message, request);
    const text = inputRequestDraft(message, request).trim();
    if (request.choices.length && selected.length > 0) return true;
    if (request.allowFreeText && text) return true;
    return false;
  }

  function inputRequestAnswerSummary(request: ChatInputRequest) {
    const labels = (request.choiceIds || [])
      .map(
        (id) => request.choices.find((choice) => choice.id === id)?.label || id,
      )
      .filter(Boolean);
    const parts = [...labels];
    if (request.answer) parts.push(request.answer);
    return parts.join(", ") || "Response submitted";
  }

  async function submitInputRequest(
    message: ChatMessage,
    request: ChatInputRequest,
  ) {
    const key = inputRequestKey(message, request);
    const sessionId = toValue(activeSessionId);
    if (!sessionId || !canSubmitInputRequest(message, request)) {
      return;
    }
    inputRequestSubmitting.value = {
      ...inputRequestSubmitting.value,
      [key]: true,
    };
    inputRequestErrors.value = { ...inputRequestErrors.value, [key]: "" };
    try {
      const runId = request.runId?.trim() || "";
      await chat.submitInputRequest(
        sessionId,
        message.id,
        request.id,
        inputRequestDraft(message, request),
        inputRequestSelection(message, request),
      );
      const drafts = { ...inputRequestDrafts.value };
      const selections = { ...inputRequestSelections.value };
      delete drafts[key];
      delete selections[key];
      inputRequestDrafts.value = drafts;
      inputRequestSelections.value = selections;
      await nextTick();
      if (runId && !chat.isSessionStreaming(sessionId)) {
        await chat.resumeDurableRun(sessionId, message.id, runId);
      }
    } catch (error) {
      inputRequestErrors.value = {
        ...inputRequestErrors.value,
        [key]:
          error instanceof Error ? error.message : "Failed to submit response",
      };
    } finally {
      inputRequestSubmitting.value = {
        ...inputRequestSubmitting.value,
        [key]: false,
      };
    }
  }

  return {
    inputRequestKey,
    inputRequestFieldName,
    isInputRequestRespondable,
    inputRequestStatusLabel,
    inputRequestCardClasses,
    inputRequestChoiceSelected,
    toggleInputRequestChoice,
    inputRequestDraft,
    setInputRequestDraft,
    isInputRequestSubmitting,
    inputRequestLocalError,
    canSubmitInputRequest,
    inputRequestAnswerSummary,
    submitInputRequest,
  };
}
