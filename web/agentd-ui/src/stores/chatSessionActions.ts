import type { ChatMessage } from "@/types/chat";
import type { ChatStreamEvent } from "@/api/chat";
import {
  createChatSession as apiCreateChatSession,
  deleteChatSession as apiDeleteChatSession,
  deleteChatMessage as apiDeleteChatMessage,
  deleteChatMessagesAfter as apiDeleteChatMessagesAfter,
  fetchChatActivities,
  fetchChatMessages,
  listActiveChatRuns,
  listChatSessions,
  renameChatSession as apiRenameChatSession,
  streamChatRunEvents,
  updateChatSessionCommandPolicyAllowAll as apiUpdateChatSessionCommandPolicyAllowAll,
  updateChatSessionMemorySettings as apiUpdateChatSessionMemorySettings,
  updateChatSessionPinned as apiUpdateChatSessionPinned,
  updateChatSessionProject as apiUpdateChatSessionProject,
} from "@/api/chat";
import {
  httpStatus,
  normalizeSessionMeta,
  sortChatSessions,
} from "@/stores/chatHelpers";
import { handleStreamEvent } from "@/stores/chatStreamEvents";
import type { ChatStoreState } from "@/stores/chatStoreState";
import { createId } from "@/utils/uuid";

function createMessageDeletionActions(state: ChatStoreState) {
  async function deleteMessage(sessionId: string, messageId: string) {
    if (!sessionId || !messageId) return;
    await apiDeleteChatMessage(sessionId, messageId);
    const refreshed = await fetchChatMessages(sessionId);
    state.setMessages(sessionId, refreshed);
    state.clearThoughtSummaries(sessionId);
    state.clearSummaryEvent(sessionId);
  }

  async function deleteMessagesAfter(
    sessionId: string,
    messageId: string,
    inclusive = false,
  ) {
    if (!sessionId || !messageId) return;
    await apiDeleteChatMessagesAfter(sessionId, messageId, inclusive);
    const refreshed = await fetchChatMessages(sessionId);
    state.setMessages(sessionId, refreshed);
    state.clearThoughtSummaries(sessionId);
    state.clearSummaryEvent(sessionId);
  }

  return { deleteMessage, deleteMessagesAfter };
}

function createSessionLoadActions(state: ChatStoreState) {
  async function init() {
    if (state.sessions.value.length) return;
    await refreshSessionsFromServer(true);
  }

  async function refreshSessionsFromServer(initial = false) {
    state.sessionsLoading.value = true;
    if (!initial) state.sessionsError.value = null;
    try {
      const remote = await normalizedRemoteSessions(initial);
      const ordered = sortChatSessions(remote);
      state.sessionsError.value = null;
      state.sessions.value = ordered;
      reconcileSessionMessages(state, ordered);
      if (!ordered.length) {
        state.activeSessionId.value = "";
        return;
      }
      if (!ordered.some((s) => s.id === state.activeSessionId.value)) {
        state.activeSessionId.value = ordered[0].id;
      }
      if (state.activeSessionId.value) {
        await loadMessagesFromServer(state.activeSessionId.value, {
          force: true,
        });
      }
    } catch (error) {
      applySessionLoadError(state, error);
    } finally {
      state.sessionsLoading.value = false;
    }
  }

  async function loadMessagesFromServer(
    sessionId: string,
    options: { force?: boolean } = {},
  ) {
    if (!sessionId) return;
    if (!options.force && state.fetchedMessageSessions.has(sessionId)) return;
    const beforeFetch = state.messagesBySession.value[sessionId] || [];
    try {
      const [data, activities] = await Promise.all([
        fetchChatMessages(sessionId),
        fetchChatActivities(sessionId),
      ]);
      state.fetchedMessageSessions.add(sessionId);
      const current = state.messagesBySession.value[sessionId] || [];
      const changedDuringFetch = current !== beforeFetch;
      const preserveLocal =
        changedDuringFetch &&
        (current.some((m) => !!m.streaming) || current.length > data.length);
      if (preserveLocal) {
        state.syncSessionMessageCount(sessionId, current.length);
        return;
      }
      state.setMessages(sessionId, data);
      state.setAgentThreads(sessionId, activities || []);
      void recoverActiveChatRun(state, sessionId);
    } catch (error) {
      const status = httpStatus(error);
      if (status === 403) {
        state.sessionsError.value = "Access denied for this conversation.";
      } else if (status === 404) await refreshSessionsFromServer();
      console.error("Failed to load chat messages", error);
    }
  }

  return { init, refreshSessionsFromServer, loadMessagesFromServer };
}

async function recoverActiveChatRun(state: ChatStoreState, sessionId: string) {
  if (state.isSessionStreaming(sessionId)) return;
  const runs = await listActiveChatRuns(sessionId).catch(() => []);
  const run = runs.find((candidate) =>
    ["queued", "running", "waiting", "failed"].includes(candidate.status),
  );
  if (!run?.run_id) return;
  const assistantId = run.assistant_message_id || createId();
  const messages = state.messagesBySession.value[sessionId] || [];
  const running = run.status !== "failed";
  const lastSequence = numericSequence(run.last_sequence);
  const lastRetrySequence = numericSequence(run.last_retry_sequence);
  if (!messages.some((m) => m.id === assistantId)) {
    state.appendMessage(sessionId, {
      id: assistantId,
      role: "assistant",
      content: "",
      createdAt: new Date().toISOString(),
      runId: run.run_id,
      streaming: running,
      error: running ? undefined : run.error || "Run failed",
      lastRunSequence: lastSequence || undefined,
    });
  } else {
    state.updateMessage(sessionId, assistantId, (message) => ({
      ...message,
      runId: run.run_id,
      streaming: running,
      error: running ? undefined : run.error || message.error || "Run failed",
      lastRunSequence:
        Math.max(message.lastRunSequence || 0, lastSequence) || undefined,
    }));
  }
  if (!running) return;
  const streamId = createId();
  const controller = new AbortController();
  state.setStreamingState(sessionId, {
    assistantId,
    abortController: controller,
    streamId,
    runId: run.run_id,
  });
  state.toolIndexFor(sessionId, streamId);
  const queryClient = { invalidateQueries: () => undefined };
  try {
    await streamChatRunEvents({
      runId: run.run_id,
      signal: controller.signal,
      onEvent: (event) => {
        if (isStaleRetryError(event, lastRetrySequence)) return;
        handleStreamEvent(
          state,
          queryClient,
          event,
          sessionId,
          assistantId,
          streamId,
        );
      },
    });
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) {
      state.updateMessage(sessionId, assistantId, (message) => ({
        ...message,
        streaming: false,
        error:
          error instanceof Error ? error.message : "Stream recovery failed",
      }));
    }
  } finally {
    if (state.isStreamCurrent(sessionId, streamId)) {
      state.clearStreamingState(sessionId);
      state.clearToolIndex(sessionId, streamId);
    }
  }
}

function numericSequence(value: unknown) {
  const sequence = typeof value === "number" ? value : Number(value);
  return Number.isFinite(sequence) && sequence > 0 ? sequence : 0;
}

function isStaleRetryError(event: ChatStreamEvent, retrySequence: number) {
  if (retrySequence <= 0 || event.type !== "error") return false;
  const sequence = numericSequence(event.sequence);
  return sequence > 0 && sequence <= retrySequence;
}

async function normalizedRemoteSessions(initial: boolean) {
  let remote = await listChatSessions();
  if (!remote) remote = [];
  remote = remote.map(normalizeSessionMeta);
  if (initial && remote.length === 0) {
    const created = await apiCreateChatSession("New Chat");
    if (created) remote = [normalizeSessionMeta(created)];
  }
  return remote;
}

function reconcileSessionMessages(
  state: ChatStoreState,
  remote: ReturnType<typeof normalizeSessionMeta>[],
) {
  const nextMessages: Record<string, ChatMessage[]> = {};
  for (const s of remote) {
    const existing = state.messagesBySession.value[s.id] || [];
    nextMessages[s.id] = existing;
    const fallbackCount =
      typeof s.messageCount === "number" ? s.messageCount : 0;
    const count = existing.length ? existing.length : fallbackCount;
    state.syncSessionMessageCount(s.id, count);
  }
  state.messagesBySession.value = nextMessages;
  state.fetchedMessageSessions.clear();
}

function applySessionLoadError(state: ChatStoreState, error: unknown) {
  const status = httpStatus(error);
  if (status === 401) state.sessionsError.value = "Authentication required.";
  else if (status === 403) {
    state.sessionsError.value =
      "Access denied. You do not have permission to view conversations.";
  } else state.sessionsError.value = "Failed to load conversations.";
  console.error("Failed to load chat sessions", error);
}

function createSessionCrudActions(
  state: ChatStoreState,
  loadMessagesFromServer: (
    sessionId: string,
    options?: { force?: boolean },
  ) => Promise<void>,
) {
  function selectSession(sessionId: string) {
    state.activeSessionId.value = sessionId;
    void loadMessagesFromServer(sessionId);
  }

  async function createSession(name = "New Chat") {
    const session = await apiCreateChatSession(name);
    if (!session) return;
    const normalized = normalizeSessionMeta(session);
    state.sessionsError.value = null;
    state.sessions.value = sortChatSessions([
      normalized,
      ...state.sessions.value,
    ]);
    state.setMessages(normalized.id, []);
    state.fetchedMessageSessions.delete(normalized.id);
    state.activeSessionId.value = normalized.id;
    await loadMessagesFromServer(normalized.id, { force: true });
  }

  async function deleteSession(sessionId: string) {
    await apiDeleteChatSession(sessionId);
    state.sessionsError.value = null;
    const nextSessions = state.sessions.value.filter((s) => s.id !== sessionId);
    const { [sessionId]: _removed, ...rest } = state.messagesBySession.value;
    state.messagesBySession.value = rest;
    const { [sessionId]: _removedThreads, ...restThreads } =
      state.agentThreadsBySession.value;
    state.agentThreadsBySession.value = restThreads;
    state.agentThreadIndex.delete(sessionId);
    state.fetchedMessageSessions.delete(sessionId);
    if (!nextSessions.length) {
      await createReplacementSession(state, loadMessagesFromServer);
      return;
    }
    state.sessions.value = nextSessions;
    if (state.activeSessionId.value === sessionId) {
      state.activeSessionId.value = nextSessions[0]?.id || "";
      if (state.activeSessionId.value) {
        await loadMessagesFromServer(state.activeSessionId.value, {
          force: true,
        });
      }
    }
  }

  async function renameSession(sessionId: string, name: string) {
    const updated = await apiRenameChatSession(sessionId, name);
    state.sessionsError.value = null;
    state.upsertSessionMeta(updated);
  }

  return { selectSession, createSession, deleteSession, renameSession };
}

async function createReplacementSession(
  state: ChatStoreState,
  loadMessagesFromServer: (
    sessionId: string,
    options?: { force?: boolean },
  ) => Promise<void>,
) {
  const fresh = await apiCreateChatSession("New Chat");
  const normalizedFresh = normalizeSessionMeta(fresh);
  state.sessions.value = [normalizedFresh];
  state.setMessages(normalizedFresh.id, []);
  state.fetchedMessageSessions.delete(normalizedFresh.id);
  state.activeSessionId.value = normalizedFresh.id;
  await loadMessagesFromServer(normalizedFresh.id, { force: true });
}

function createSessionSettingsActions(state: ChatStoreState) {
  async function updateSessionProject(sessionId: string, projectId: string) {
    const cleanProjectID = (projectId || "").trim();
    const existing = state.sessions.value.find((s) => s.id === sessionId);
    if (existing && (existing.projectId || "") === cleanProjectID) {
      return existing;
    }
    const updated = await apiUpdateChatSessionProject(
      sessionId,
      cleanProjectID,
    );
    state.sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    state.upsertSessionMeta(normalized);
    return normalized;
  }

  async function updateSessionMemorySettings(
    sessionId: string,
    settings: {
      memoryEnabled?: boolean;
      evolvingMemoryEnabled?: boolean;
      beliefMemoryEnabled?: boolean;
    },
  ) {
    const existing = state.sessions.value.find((s) => s.id === sessionId);
    const nextMemory = nextMemoryEnabled(existing, settings);
    if (existing && (existing.memoryEnabled ?? false) === nextMemory) {
      return existing;
    }
    const updated = await apiUpdateChatSessionMemorySettings(sessionId, {
      memoryEnabled: nextMemory,
    });
    state.sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    state.upsertSessionMeta(normalized);
    return normalized;
  }

  async function updateSessionCommandPolicyAllowAll(
    sessionId: string,
    allow: boolean,
  ) {
    const existing = state.sessions.value.find((s) => s.id === sessionId);
    if (existing && (existing.commandPolicyAllowAll ?? false) === allow) {
      return existing;
    }
    const updated = await apiUpdateChatSessionCommandPolicyAllowAll(
      sessionId,
      allow,
    );
    state.sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    state.upsertSessionMeta(normalized);
    return normalized;
  }

  async function updateSessionPinned(sessionId: string, pinned: boolean) {
    const existing = state.sessions.value.find((s) => s.id === sessionId);
    if (existing && (existing.pinned ?? false) === pinned) {
      return existing;
    }
    const updated = await apiUpdateChatSessionPinned(sessionId, pinned);
    state.sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    state.upsertSessionMeta(normalized);
    return normalized;
  }

  return {
    updateSessionProject,
    updateSessionMemorySettings,
    updateSessionCommandPolicyAllowAll,
    updateSessionPinned,
  };
}

function nextMemoryEnabled(
  existing: ReturnType<typeof normalizeSessionMeta> | undefined,
  settings: {
    memoryEnabled?: boolean;
    evolvingMemoryEnabled?: boolean;
    beliefMemoryEnabled?: boolean;
  },
) {
  if (typeof settings.memoryEnabled === "boolean")
    return settings.memoryEnabled;
  if (
    typeof settings.evolvingMemoryEnabled === "boolean" ||
    typeof settings.beliefMemoryEnabled === "boolean"
  ) {
    return (
      (settings.evolvingMemoryEnabled ??
        existing?.evolvingMemoryEnabled ??
        false) &&
      (settings.beliefMemoryEnabled ?? existing?.beliefMemoryEnabled ?? false)
    );
  }
  return existing?.memoryEnabled ?? false;
}

export function createChatSessionActions(state: ChatStoreState) {
  const deletionActions = createMessageDeletionActions(state);
  const loadActions = createSessionLoadActions(state);
  const crudActions = createSessionCrudActions(
    state,
    loadActions.loadMessagesFromServer,
  );
  const settingsActions = createSessionSettingsActions(state);
  return {
    ...deletionActions,
    ...loadActions,
    ...crudActions,
    ...settingsActions,
  };
}

export type ChatSessionActions = ReturnType<typeof createChatSessionActions>;
