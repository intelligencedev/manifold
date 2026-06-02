import {
  cancelChatRun,
  generateChatSessionTitle,
  streamAgentRun,
  streamAgentVisionRun,
  type ChatStreamEvent,
} from "@/api/chat";
import type { ChatAttachment, ChatMessage } from "@/types/chat";
import {
  computeLocalTitle,
  isDefaultSessionName,
  localContextMetricsForMessages,
  updateLatestUserBeforeMessage,
} from "@/stores/chatHelpers";
import type { ChatSessionActions } from "@/stores/chatSessionActions";
import { handleStreamEvent } from "@/stores/chatStreamEvents";
import type { ChatStoreState } from "@/stores/chatStoreState";
import { stripLeadingChatMention } from "@/utils/chatMentions";
import { createId } from "@/utils/uuid";

type QueryInvalidator = {
  invalidateQueries(options: { queryKey: string[] }): unknown;
};

type SendPromptOptions = {
  echoUser?: boolean;
  specialist?: string;
  routingSpecialist?: string;
  routingTargetName?: string;
  teamName?: string;
  projectId?: string;
  image?: boolean;
  imageSize?: string;
  memoryEnabled?: boolean;
  evolvingMemoryEnabled?: boolean;
  beliefMemoryEnabled?: boolean;
  agentName?: string;
  agentModel?: string;
};

export function createChatStreamActions(
  state: ChatStoreState,
  queryClient: QueryInvalidator,
  sessionActions: ChatSessionActions,
) {
  async function sendPrompt(
    text: string,
    attachments: ChatAttachment[] = [],
    filesByAttachment?: Map<string, File>,
    options: SendPromptOptions = {},
  ) {
    const content = (text || "").trim();
    const sessionId = state.ensureSession();
    if (!content && !attachments.length) return;
    const wasStreaming = state.isSessionStreaming(sessionId);
    if (wasStreaming) {
      interruptStreaming(sessionId, {
        reason: "Interrupted by user",
        archiveThoughtSummaries: true,
        clearThoughtSummaries: true,
      });
    }
    const now = new Date().toISOString();
    const agentName = (options.agentName || "").trim();
    const agentModel = (options.agentModel || "").trim();

    if (!wasStreaming) state.resetThoughtSummaries(sessionId);
    if (content) void maybeAutoTitle(sessionId, content);

    const userMessageId = createId();
    if (options.echoUser !== false) {
      const attachmentsCopy = attachments.map((a) => ({ ...a }));
      state.appendMessage(sessionId, {
        id: userMessageId,
        role: "user",
        content,
        createdAt: now,
        attachments: attachmentsCopy,
      });
    }

    const assistantId = createId();
    const streamId = createId();
    const localMetrics = localContextMetricsForMessages(
      state.messagesBySession.value[sessionId] || [],
    );
    state.appendMessage(sessionId, {
      id: assistantId,
      role: "assistant",
      content: "",
      createdAt: now,
      streaming: true,
      agentName: agentName || undefined,
      agentModel: agentModel || undefined,
      model: agentModel || undefined,
      contextMetrics: localMetrics,
    });
    const withUserMetrics = updateLatestUserBeforeMessage(
      state.messagesBySession.value[sessionId] || [],
      assistantId,
      (m) => ({ ...m, contextMetrics: localMetrics }),
    );
    if (withUserMetrics) state.setMessages(sessionId, withUserMetrics);

    const controller = new AbortController();
    controller.signal.addEventListener(
      "abort",
      () => {
        console.warn("chat stream aborted", {
          sessionId,
          assistantId,
          reason: controller.signal.reason,
        });
      },
      { once: true },
    );
    state.setStreamingState(sessionId, {
      assistantId,
      abortController: controller,
      streamId,
    });
    state.toolIndexFor(sessionId, streamId);

    try {
      let promptToSend = stripLeadingChatMention(
        content,
        options.routingTargetName ||
          options.routingSpecialist ||
          options.specialist ||
          options.teamName,
      );
      const textAtts = attachments.filter((a) => a.kind === "text");
      const imgAtts = attachments.filter((a) => a.kind === "image");
      for (const att of textAtts) {
        const f = filesByAttachment?.get(att.id);
        if (!f) continue;
        const textContent = await f.text();
        const header = `\n\n--- Attached Document: ${att.name} (${att.mime || "text"}) ---\n`;
        const footer = `\n--- End Document ---\n`;
        promptToSend += header + textContent + footer;
      }
      const imageFiles: File[] = [];
      for (const att of imgAtts) {
        const f = filesByAttachment?.get(att.id);
        if (f) imageFiles.push(f);
      }

      const onEvent = (event: ChatStreamEvent) =>
        handleStreamEvent(
          state,
          queryClient,
          event,
          sessionId,
          assistantId,
          streamId,
        );
      if (imageFiles.length) {
        await streamAgentVisionRun({
          prompt: promptToSend,
          sessionId,
          userMessageId,
          assistantMessageId: assistantId,
          files: imageFiles,
          signal: controller.signal,
          onEvent,
          specialist: options.specialist,
          teamName: options.teamName,
          projectId: options.projectId,
          memoryEnabled: options.memoryEnabled,
          evolvingMemoryEnabled: options.evolvingMemoryEnabled,
          beliefMemoryEnabled: options.beliefMemoryEnabled,
        });
      } else {
        await streamAgentRun({
          prompt: promptToSend,
          sessionId,
          userMessageId,
          assistantMessageId: assistantId,
          signal: controller.signal,
          onEvent,
          specialist: options.specialist,
          teamName: options.teamName,
          projectId: options.projectId,
          memoryEnabled: options.memoryEnabled,
          evolvingMemoryEnabled: options.evolvingMemoryEnabled,
          beliefMemoryEnabled: options.beliefMemoryEnabled,
          image: options.image,
          imageSize: options.imageSize,
        });
      }
    } catch (error: any) {
      if (error instanceof DOMException && error.name === "AbortError") {
        console.warn("chat stream aborted (fetch)", {
          sessionId,
          assistantId,
          reason: controller.signal.reason,
        });
      } else {
        console.warn("chat stream error", error);
      }
      const assistantUpdater = (m: ChatMessage) => {
        if (!m.streaming) return m;
        return {
          ...m,
          streaming: false,
          error:
            error instanceof DOMException && error.name === "AbortError"
              ? "Generation stopped"
              : error instanceof Error
                ? error.message
                : "Unexpected error",
        };
      };
      state.updateMessage(sessionId, assistantId, assistantUpdater);
    } finally {
      if (state.isStreamCurrent(sessionId, streamId)) {
        state.clearStreamingState(sessionId);
        state.clearToolIndex(sessionId, streamId);
      }
    }
  }

  async function maybeAutoTitle(sessionId: string, prompt: string) {
    const currentSession = state.sessions.value.find((s) => s.id === sessionId);
    if (!currentSession || !isDefaultSessionName(currentSession.name)) return;
    if (state.hasUserPrompt(sessionId)) return;
    const trimmed = prompt.trim();
    if (!trimmed) return;
    try {
      const localTitle = computeLocalTitle(trimmed);
      if (localTitle) {
        state.upsertSessionMeta({
          id: sessionId,
          name: localTitle,
          createdAt: currentSession.createdAt,
          updatedAt: new Date().toISOString(),
        });
      }
      const updated = await generateChatSessionTitle(sessionId, trimmed);
      state.upsertSessionMeta(updated);
    } catch (error) {
      console.warn("auto-title failed", error);
      return;
    }
  }

  function interruptStreaming(
    sessionId: string,
    options: {
      reason?: string;
      archiveThoughtSummaries?: boolean;
      clearThoughtSummaries?: boolean;
    } = {},
  ) {
    const streamState = state.streamingStateFor(sessionId);
    if (!streamState) return false;
    const reason = options.reason || "Interrupted";
    const now = new Date().toISOString();

    if (options.archiveThoughtSummaries) {
      const summaries = state.thoughtSummariesBySession.value[sessionId] || [];
      if (summaries.length) {
        state.appendMessage(
          sessionId,
          {
            id: createId(),
            role: "tool",
            title: "Thought summaries (interrupted)",
            content: summaries.join("\n"),
            createdAt: now,
          },
          false,
        );
      }
    }

    state.updateMessage(sessionId, streamState.assistantId, (m) => ({
      ...m,
      streaming: false,
      error: reason,
      inputRequests: (m.inputRequests || []).map((request) =>
        request.status === "pending" || request.status === "error"
          ? { ...request, status: "cancelled", error: reason }
          : request,
      ),
    }));

    const existing = state.messagesBySession.value[sessionId] || [];
    if (existing.some((m) => m.role === "tool" && m.streaming)) {
      const next = existing.map((m) =>
        m.role === "tool" && m.streaming
          ? { ...m, streaming: false, error: reason }
          : m,
      );
      state.setMessages(sessionId, next);
    }

    if (options.clearThoughtSummaries) {
      state.clearThoughtSummaries(sessionId);
    }

    streamState.abortController.abort("interrupt");
    return true;
  }

  function stopStreaming(sessionId?: string) {
    const targetSessionId = sessionId || state.activeSessionId.value;
    if (!targetSessionId) return;
    const streamState = state.streamingStateFor(targetSessionId);
    if (streamState?.runId) {
      void cancelChatRun(streamState.runId).catch((error) => {
        console.warn("chat run cancel failed", error);
      });
    }
    if (!interruptStreaming(targetSessionId, { reason: "Generation stopped" }))
      return;
    console.warn("chat stopStreaming called", { sessionId: targetSessionId });
  }

  async function regenerateAssistant(
    options: SendPromptOptions & { messageId?: string } = {},
  ) {
    const sessionId = state.ensureSession();
    if (state.isSessionStreaming(sessionId)) return;
    const messages = state.messagesBySession.value[sessionId] || [];
    const targetIndex = options.messageId
      ? messages.findIndex((m) => m.id === options.messageId)
      : messages.findLastIndex((m) => m.role === "assistant");
    if (targetIndex === -1) return;
    const target = messages[targetIndex];
    if (!target || target.role !== "assistant" || !target.id) return;
    let lastUser: ChatMessage | undefined;
    for (let i = targetIndex - 1; i >= 0; i -= 1) {
      if (messages[i].role === "user") {
        lastUser = messages[i];
        break;
      }
    }
    if (!lastUser) return;
    await sessionActions.deleteMessagesAfter(sessionId, target.id, true);
    await sendPrompt(lastUser.content, [], undefined, {
      echoUser: false,
      specialist: options.specialist,
      routingSpecialist: options.routingSpecialist,
      routingTargetName: options.routingTargetName,
      teamName: options.teamName,
      projectId: options.projectId,
      memoryEnabled: options.memoryEnabled,
      evolvingMemoryEnabled: options.evolvingMemoryEnabled,
      beliefMemoryEnabled: options.beliefMemoryEnabled,
      agentName: options.agentName,
      agentModel: options.agentModel,
    });
  }

  return { sendPrompt, stopStreaming, regenerateAssistant };
}
