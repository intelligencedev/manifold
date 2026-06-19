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
  updateChatSessionActiveTarget as apiUpdateChatSessionActiveTarget,
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

const chatMessagePageSize = 100;
const chatMessageFetchLimit = chatMessagePageSize + 1;

type LoadMessagesOptions = { force?: boolean };

function messagePage(messages: ChatMessage[]) {
  const hasOlder = messages.length > chatMessagePageSize;
  return {
    hasOlder,
    messages: hasOlder ? messages.slice(1) : messages,
  };
}

async function fetchLatestMessagePage(sessionId: string) {
  return messagePage(
    await fetchChatMessages(sessionId, { limit: chatMessageFetchLimit }),
  );
}

function setMessagePageState(
  state: ChatStoreState,
  sessionId: string,
  page: ReturnType<typeof messagePage>,
  loadingOlder = false,
) {
  state.setMessagePaging(sessionId, {
    hasOlder: page.hasOlder,
    loadingOlder,
    error: "",
  });
}

async function replaceWithLatestMessagePage(
  state: ChatStoreState,
  sessionId: string,
) {
  const refreshed = await fetchLatestMessagePage(sessionId);
  state.setMessages(sessionId, refreshed.messages);
  setMessagePageState(state, sessionId, refreshed);
}

function createMessageDeletionActions(state: ChatStoreState) {
  async function deleteMessage(sessionId: string, messageId: string) {
    if (!sessionId || !messageId) return;
    await apiDeleteChatMessage(sessionId, messageId);
    await replaceWithLatestMessagePage(state, sessionId);
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
    await replaceWithLatestMessagePage(state, sessionId);
    state.clearThoughtSummaries(sessionId);
    state.clearSummaryEvent(sessionId);
  }

  return { deleteMessage, deleteMessagesAfter };
}

function createSessionLoadActions(state: ChatStoreState) {
  const loadingMessageSessions = new Set<string>();

  async function init() {
    if (state.sessions.value.length) return;
    await refreshSessionsFromServer(true);
  }

  async function refreshSessionsFromServer(initial = false) {
    await refreshSessionsFromServerImpl(
      state,
      initial,
      loadMessagesFromServer,
    );
  }

  async function loadMessagesFromServer(
    sessionId: string,
    options: LoadMessagesOptions = {},
  ) {
    await loadMessagesForSession(
      state,
      loadingMessageSessions,
      refreshSessionsFromServer,
      sessionId,
      options,
    );
  }

  async function loadOlderMessages(sessionId: string) {
    await loadOlderMessagesForSession(state, sessionId);
  }

  return {
    init,
    refreshSessionsFromServer,
    loadMessagesFromServer,
    loadOlderMessages,
  };
}

async function refreshSessionsFromServerImpl(
  state: ChatStoreState,
  initial: boolean,
  loadMessagesFromServer: (
    sessionId: string,
    options?: LoadMessagesOptions,
  ) => Promise<void>,
) {
  state.sessionsLoading.value = true;
  if (!initial) state.sessionsError.value = null;
  try {
    const ordered = sortChatSessions(await normalizedRemoteSessions(initial));
    state.sessionsError.value = null;
    state.sessions.value = ordered;
    reconcileSessionMessages(state, ordered);
    await loadActiveSessionMessages(state, ordered, loadMessagesFromServer);
  } catch (error) {
    applySessionLoadError(state, error);
  } finally {
    state.sessionsLoading.value = false;
  }
}

async function loadActiveSessionMessages(
  state: ChatStoreState,
  ordered: ReturnType<typeof normalizeSessionMeta>[],
  loadMessagesFromServer: (
    sessionId: string,
    options?: LoadMessagesOptions,
  ) => Promise<void>,
) {
  if (!ordered.length) {
    state.activeSessionId.value = "";
    return;
  }
  if (!ordered.some((s) => s.id === state.activeSessionId.value)) {
    state.activeSessionId.value = ordered[0].id;
  }
  if (state.activeSessionId.value) {
    await loadMessagesFromServer(state.activeSessionId.value, { force: true });
  }
}

async function loadMessagesForSession(
  state: ChatStoreState,
  loadingMessageSessions: Set<string>,
  refreshSessionsFromServer: () => Promise<void>,
  sessionId: string,
  options: LoadMessagesOptions,
) {
  if (shouldSkipMessageLoad(state, loadingMessageSessions, sessionId, options)) {
    return;
  }
  loadingMessageSessions.add(sessionId);
  const beforeFetch = state.messagesBySession.value[sessionId] || [];
  try {
    await loadMessagesForSessionImpl(state, sessionId, beforeFetch);
  } catch (error) {
    await handleMessageLoadError(state, refreshSessionsFromServer, error);
  } finally {
    loadingMessageSessions.delete(sessionId);
  }
}

function shouldSkipMessageLoad(
  state: ChatStoreState,
  loadingMessageSessions: Set<string>,
  sessionId: string,
  options: LoadMessagesOptions,
) {
  if (!sessionId) return true;
  if (!options.force && state.fetchedMessageSessions.has(sessionId)) return true;
  return loadingMessageSessions.has(sessionId);
}

async function loadMessagesForSessionImpl(
  state: ChatStoreState,
  sessionId: string,
  beforeFetch: ChatMessage[],
) {
  const [rawMessages, activities] = await Promise.all([
    fetchChatMessages(sessionId, { limit: chatMessageFetchLimit }),
    fetchChatActivities(sessionId),
  ]);
  const page = messagePage(rawMessages);
  state.fetchedMessageSessions.add(sessionId);
  if (shouldPreserveLocalMessages(state, sessionId, beforeFetch)) {
    setMessagePageState(state, sessionId, page);
    return;
  }
  state.setMessages(sessionId, page.messages);
  if (!page.hasOlder) {
    state.syncSessionMessageCount(sessionId, page.messages.length);
  }
  setMessagePageState(state, sessionId, page);
  state.setAgentThreads(sessionId, activities || []);
  void recoverActiveChatRun(state, sessionId);
}

function shouldPreserveLocalMessages(
  state: ChatStoreState,
  sessionId: string,
  beforeFetch: ChatMessage[],
) {
  const current = state.messagesBySession.value[sessionId] || [];
  return current !== beforeFetch && current.some((m) => !!m.streaming);
}

async function handleMessageLoadError(
  state: ChatStoreState,
  refreshSessionsFromServer: () => Promise<void>,
  error: unknown,
) {
  const status = httpStatus(error);
  if (status === 403) {
    state.sessionsError.value = "Access denied for this conversation.";
  } else if (status === 404) {
    await refreshSessionsFromServer();
  }
  console.error("Failed to load chat messages", error);
}

async function loadOlderMessagesForSession(
  state: ChatStoreState,
  sessionId: string,
) {
  const cursor = olderMessagesCursor(state, sessionId);
  if (!cursor) return;
  state.setMessagePaging(sessionId, { loadingOlder: true, error: "" });
  try {
    const page = messagePage(
      await fetchChatMessages(sessionId, {
        limit: chatMessageFetchLimit,
        before: cursor,
      }),
    );
    state.prependMessages(sessionId, page.messages);
    setMessagePageState(state, sessionId, page);
  } catch (error) {
    state.setMessagePaging(sessionId, {
      loadingOlder: false,
      error: "Failed to load older messages.",
    });
    console.error("Failed to load older chat messages", error);
  }
}

function olderMessagesCursor(state: ChatStoreState, sessionId: string) {
  if (!sessionId) return "";
  const paging = state.messagePagingBySession.value[sessionId];
  if (!paging?.hasOlder || paging.loadingOlder) return "";
  const cursor = state.messagesBySession.value[sessionId]?.[0]?.id || "";
  if (!cursor) {
    state.setMessagePaging(sessionId, { hasOlder: false });
  }
  return cursor;
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
  const nextPaging: ChatStoreState["messagePagingBySession"]["value"] = {};
  for (const s of remote) {
    const existing = state.messagesBySession.value[s.id] || [];
    nextMessages[s.id] = existing;
    nextPaging[s.id] = state.messagePagingBySession.value[s.id] || {
      hasOlder: false,
      loadingOlder: false,
    };
    const count =
      typeof s.messageCount === "number" ? s.messageCount : existing.length;
    state.syncSessionMessageCount(s.id, count);
  }
  state.messagesBySession.value = nextMessages;
  state.messagePagingBySession.value = nextPaging;
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
    options?: LoadMessagesOptions,
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
    state.setMessagePaging(normalized.id, {
      hasOlder: false,
      loadingOlder: false,
      error: "",
    });
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
    const { [sessionId]: _removedPaging, ...restPaging } =
      state.messagePagingBySession.value;
    state.messagePagingBySession.value = restPaging;
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
    options?: LoadMessagesOptions,
  ) => Promise<void>,
) {
  const fresh = await apiCreateChatSession("New Chat");
  const normalizedFresh = normalizeSessionMeta(fresh);
  state.sessions.value = [normalizedFresh];
  state.setMessages(normalizedFresh.id, []);
  state.setMessagePaging(normalizedFresh.id, {
    hasOlder: false,
    loadingOlder: false,
    error: "",
  });
  state.fetchedMessageSessions.delete(normalizedFresh.id);
  state.activeSessionId.value = normalizedFresh.id;
  await loadMessagesFromServer(normalizedFresh.id, { force: true });
}

function createSessionSettingsActions(state: ChatStoreState) {
  return {
    updateSessionProject: (sessionId: string, projectId: string) =>
      updateSessionProject(state, sessionId, projectId),
    updateSessionMemorySettings: (
      sessionId: string,
      settings: SessionMemorySettings,
    ) => updateSessionMemorySettings(state, sessionId, settings),
    updateSessionCommandPolicyAllowAll: (sessionId: string, allow: boolean) =>
      updateSessionCommandPolicyAllowAll(state, sessionId, allow),
    updateSessionActiveTarget: (
      sessionId: string,
      activeSpecialist: string,
      activeTeam: string,
    ) =>
      updateSessionActiveTarget(state, sessionId, activeSpecialist, activeTeam),
    updateSessionPinned: (sessionId: string, pinned: boolean) =>
      updateSessionPinned(state, sessionId, pinned),
  };
}

async function updateSessionProject(
  state: ChatStoreState,
  sessionId: string,
  projectId: string,
) {
  const cleanProjectID = (projectId || "").trim();
  const existing = findSession(state, sessionId);
  if (existing && (existing.projectId || "") === cleanProjectID) {
    return existing;
  }
  const updated = await apiUpdateChatSessionProject(sessionId, cleanProjectID);
  return upsertUpdatedSession(state, updated);
}

type SessionMemorySettings = {
  memoryEnabled?: boolean;
  evolvingMemoryEnabled?: boolean;
  beliefMemoryEnabled?: boolean;
};

async function updateSessionMemorySettings(
  state: ChatStoreState,
  sessionId: string,
  settings: SessionMemorySettings,
) {
  const existing = findSession(state, sessionId);
  const nextMemory = nextMemoryEnabled(existing, settings);
  if (existing && (existing.memoryEnabled ?? false) === nextMemory) {
    return existing;
  }
  const updated = await apiUpdateChatSessionMemorySettings(sessionId, {
    memoryEnabled: nextMemory,
  });
  return upsertUpdatedSession(state, updated);
}

async function updateSessionCommandPolicyAllowAll(
  state: ChatStoreState,
  sessionId: string,
  allow: boolean,
) {
  const existing = findSession(state, sessionId);
  if (existing && (existing.commandPolicyAllowAll ?? false) === allow) {
    return existing;
  }
  const updated = await apiUpdateChatSessionCommandPolicyAllowAll(
    sessionId,
    allow,
  );
  return upsertUpdatedSession(state, updated);
}

async function updateSessionPinned(
  state: ChatStoreState,
  sessionId: string,
  pinned: boolean,
) {
  const existing = findSession(state, sessionId);
  if (existing && (existing.pinned ?? false) === pinned) {
    return existing;
  }
  const updated = await apiUpdateChatSessionPinned(sessionId, pinned);
  return upsertUpdatedSession(state, updated);
}

async function updateSessionActiveTarget(
  state: ChatStoreState,
  sessionId: string,
  activeSpecialist: string,
  activeTeam: string,
) {
  const nextSpecialist = (activeSpecialist || "").trim();
  const nextTeam = (activeTeam || "").trim();
  const existing = findSession(state, sessionId);
  const existingSpecialist =
    (existing?.activeSpecialist || "orchestrator").trim() || "orchestrator";
  if (
    existing &&
    existingSpecialist === (nextSpecialist || "orchestrator") &&
    (existing.activeTeam || "") === nextTeam
  ) {
    return existing;
  }
  const updated = await apiUpdateChatSessionActiveTarget(sessionId, {
    activeSpecialist: nextSpecialist,
    activeTeam: nextTeam,
  });
  return upsertUpdatedSession(state, updated);
}

function findSession(state: ChatStoreState, sessionId: string) {
  return state.sessions.value.find((s) => s.id === sessionId);
}

function upsertUpdatedSession(
  state: ChatStoreState,
  updated: ReturnType<typeof normalizeSessionMeta>,
) {
  state.sessionsError.value = null;
  const normalized = normalizeSessionMeta(updated);
  state.upsertSessionMeta(normalized);
  return normalized;
}

function nextMemoryEnabled(
  existing: ReturnType<typeof normalizeSessionMeta> | undefined,
  settings: SessionMemorySettings,
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
