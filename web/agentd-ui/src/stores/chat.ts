import { defineStore } from "pinia";
import { useQueryClient } from "@tanstack/vue-query";
import { createChatSessionActions } from "@/stores/chatSessionActions";
import { createChatStoreState } from "@/stores/chatStoreState";
import { createChatStreamActions } from "@/stores/chatStreamActions";
import { submitInputRequest as submitInputRequestWithState } from "@/stores/chatInputRequests";

export const useChatStore = defineStore("chat", () => {
  const queryClient = useQueryClient();
  const state = createChatStoreState();
  const sessionActions = createChatSessionActions(state);
  const streamActions = createChatStreamActions(
    state,
    queryClient,
    sessionActions,
  );

  return {
    sessions: state.sessions,
    messagesBySession: state.messagesBySession,
    messagePagingBySession: state.messagePagingBySession,
    sessionsLoading: state.sessionsLoading,
    sessionsError: state.sessionsError,
    activeSessionId: state.activeSessionId,
    isStreaming: state.isStreaming,
    activeSession: state.activeSession,
    activeMessages: state.activeMessages,
    chatMessages: state.chatMessages,
    toolMessages: state.toolMessages,
    activeMessagePaging: state.activeMessagePaging,
    agentThreads: state.agentThreads,
    activeSummaryEvent: state.activeSummaryEvent,
    activeThoughtSummaries: state.activeThoughtSummaries,
    isSessionStreaming: state.isSessionStreaming,
    init: sessionActions.init,
    refreshSessionsFromServer: sessionActions.refreshSessionsFromServer,
    loadMessagesFromServer: sessionActions.loadMessagesFromServer,
    loadOlderMessages: sessionActions.loadOlderMessages,
    selectSession: sessionActions.selectSession,
    createSession: sessionActions.createSession,
    deleteSession: sessionActions.deleteSession,
    deleteMessage: sessionActions.deleteMessage,
    renameSession: sessionActions.renameSession,
    updateSessionProject: sessionActions.updateSessionProject,
    updateSessionMemorySettings: sessionActions.updateSessionMemorySettings,
    updateSessionCommandPolicyAllowAll:
      sessionActions.updateSessionCommandPolicyAllowAll,
    updateSessionActiveTarget: sessionActions.updateSessionActiveTarget,
    updateSessionPinned: sessionActions.updateSessionPinned,
    updateMessage: state.updateMessage,
    submitInputRequest: (
      sessionId: string,
      messageId: string,
      requestId: string,
      answer: string,
      choiceIds: string[] = [],
    ) =>
      submitInputRequestWithState(
        state,
        sessionId,
        messageId,
        requestId,
        answer,
        choiceIds,
      ),
    sendPrompt: streamActions.sendPrompt,
    stopStreaming: streamActions.stopStreaming,
    regenerateAssistant: streamActions.regenerateAssistant,
    resumeDurableRun: streamActions.resumeDurableRun,
    clearSummaryEvent: state.clearSummaryEvent,
    clearThoughtSummaries: state.clearThoughtSummaries,
  };
});
