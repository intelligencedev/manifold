import { computed, ref } from "vue";
import type {
  AgentThread,
  ChatMessage,
  ChatSessionMeta,
  SummaryEvent,
} from "@/types/chat";
import {
  agentThreadKey,
  normalizeSessionMeta,
  sortChatSessions,
  snippet,
} from "@/stores/chatHelpers";
import { createStreamStateActions } from "@/stores/chatStreamState";

export type StreamState = {
  assistantId: string;
  abortController: AbortController;
  streamId: string;
  runId?: string;
};

function createChatStoreRefs() {
  const sessions = ref<ChatSessionMeta[]>([]);
  const messagesBySession = ref<Record<string, ChatMessage[]>>({});
  const sessionsLoading = ref(false);
  const sessionsError = ref<string | null>(null);
  const activeSessionId = ref<string>("");
  const streamingStateBySession = ref<Record<string, StreamState>>({});
  return {
    sessions,
    messagesBySession,
    sessionsLoading,
    sessionsError,
    fetchedMessageSessions: new Set<string>(),
    activeSessionId,
    streamingStateBySession,
    toolMessageIndex: new Map<
      string,
      { streamId: string; index: Map<string, string> }
    >(),
    thoughtSummariesBySession: ref<Record<string, string[]>>({}),
    agentThreadsBySession: ref<Record<string, AgentThread[]>>({}),
    agentThreadIndex: new Map<string, Map<string, AgentThread>>(),
    summaryEventBySession: ref<Record<string, SummaryEvent | null>>({}),
  };
}

export type ChatStoreRefs = ReturnType<typeof createChatStoreRefs>;

function createChatComputed(refs: ChatStoreRefs) {
  const activeMessages = computed(
    () => refs.messagesBySession.value[refs.activeSessionId.value] || [],
  );
  return {
    isStreaming: computed(() => {
      const sessionId = refs.activeSessionId.value;
      return Boolean(
        sessionId && refs.streamingStateBySession.value[sessionId],
      );
    }),
    activeSession: computed(
      () =>
        refs.sessions.value.find((s) => s.id === refs.activeSessionId.value) ||
        null,
    ),
    activeMessages,
    chatMessages: computed(() =>
      activeMessages.value.filter((m) => m.role !== "tool"),
    ),
    toolMessages: computed(() =>
      activeMessages.value.filter((m) => m.role === "tool"),
    ),
    agentThreads: computed(
      () => refs.agentThreadsBySession.value[refs.activeSessionId.value] || [],
    ),
    activeSummaryEvent: computed(
      () =>
        refs.summaryEventBySession.value[refs.activeSessionId.value] || null,
    ),
    activeThoughtSummaries: computed(
      () =>
        refs.thoughtSummariesBySession.value[refs.activeSessionId.value] || [],
    ),
  };
}

function createThoughtSummaryActions(refs: ChatStoreRefs) {
  function clearSummaryEvent(sessionId?: string) {
    const id = sessionId || refs.activeSessionId.value;
    if (id) {
      refs.summaryEventBySession.value = {
        ...refs.summaryEventBySession.value,
        [id]: null,
      };
    }
  }

  function clearThoughtSummaries(sessionId?: string) {
    const id = sessionId || refs.activeSessionId.value;
    if (!id) return;
    refs.thoughtSummariesBySession.value = {
      ...refs.thoughtSummariesBySession.value,
      [id]: [],
    };
  }

  function resetThoughtSummaries(sessionId: string) {
    refs.thoughtSummariesBySession.value = {
      ...refs.thoughtSummariesBySession.value,
      [sessionId]: [],
    };
  }

  return {
    clearSummaryEvent,
    clearThoughtSummaries,
    resetThoughtSummaries,
    appendThoughtSummary: (sessionId: string, summary: string) =>
      appendThoughtSummary(refs, sessionId, summary),
  };
}

function appendThoughtSummary(
  refs: ChatStoreRefs,
  sessionId: string,
  summary: string,
) {
  const text = (summary || "").trim();
  if (!text) return;
  const existing = refs.thoughtSummariesBySession.value[sessionId] || [];
  const last = existing[existing.length - 1];
  if (last && text === last) return;
  if (last && text.length > last.length && text.startsWith(last)) {
    const next = [...existing];
    next[next.length - 1] = text;
    refs.thoughtSummariesBySession.value = {
      ...refs.thoughtSummariesBySession.value,
      [sessionId]: next,
    };
    return;
  }
  refs.thoughtSummariesBySession.value = {
    ...refs.thoughtSummariesBySession.value,
    [sessionId]: [...existing, text],
  };
}

function createAgentThreadActions(refs: ChatStoreRefs) {
  function setAgentThreads(sessionId: string, threads: AgentThread[]) {
    const nextThreads = sortAgentThreads(threads);
    refs.agentThreadsBySession.value = {
      ...refs.agentThreadsBySession.value,
      [sessionId]: nextThreads,
    };
    const idx = new Map<string, AgentThread>();
    nextThreads.forEach((thread) =>
      idx.set(agentThreadKey(thread.callId, thread.assistantMessageId), thread),
    );
    refs.agentThreadIndex.set(sessionId, idx);
  }
  return {
    threadIndexFor: (sessionId: string) => threadIndexFor(refs, sessionId),
    resetAgentThreads: (sessionId: string) => {
      refs.agentThreadsBySession.value = {
        ...refs.agentThreadsBySession.value,
        [sessionId]: [],
      };
      refs.agentThreadIndex.delete(sessionId);
    },
    setAgentThreads,
    upsertAgentThread: (
      sessionId: string,
      callId: string,
      factory: () => AgentThread,
      updater?: (t: AgentThread) => AgentThread,
      assistantMessageId?: string,
    ) =>
      upsertAgentThread(
        refs,
        sessionId,
        callId,
        factory,
        updater,
        assistantMessageId,
      ),
  };
}

function threadIndexFor(refs: ChatStoreRefs, sessionId: string) {
  let idx = refs.agentThreadIndex.get(sessionId);
  if (!idx) {
    idx = new Map<string, AgentThread>();
    refs.agentThreadIndex.set(sessionId, idx);
  }
  return idx;
}

function sortAgentThreads(threads: AgentThread[]) {
  return [...threads].sort((a, b) => {
    const aTime = Date.parse(a.finishedAt || a.startedAt || "") || 0;
    const bTime = Date.parse(b.finishedAt || b.startedAt || "") || 0;
    return bTime - aTime;
  });
}

function upsertAgentThread(
  refs: ChatStoreRefs,
  sessionId: string,
  callId: string,
  factory: () => AgentThread,
  updater?: (t: AgentThread) => AgentThread,
  assistantMessageId?: string,
): AgentThread {
  const idx = threadIndexFor(refs, sessionId);
  const key = agentThreadKey(callId, assistantMessageId);
  const existing = idx.get(key);
  const rawThread = existing
    ? updater
      ? updater(existing)
      : existing
    : factory();
  const thread = assistantMessageId
    ? { ...rawThread, assistantMessageId }
    : rawThread;
  idx.set(key, thread);
  const list = refs.agentThreadsBySession.value[sessionId] || [];
  const found = list.findIndex(
    (t) => agentThreadKey(t.callId, t.assistantMessageId) === key,
  );
  const nextList = [...list];
  if (found === -1) nextList.push(thread);
  else nextList.splice(found, 1, thread);
  refs.agentThreadsBySession.value = {
    ...refs.agentThreadsBySession.value,
    [sessionId]: nextList,
  };
  return thread;
}

function createMessageActions(refs: ChatStoreRefs) {
  function syncSessionMessageCount(sessionId: string, count: number) {
    const idx = refs.sessions.value.findIndex((s) => s.id === sessionId);
    if (idx === -1) return;
    const current = refs.sessions.value[idx].messageCount ?? 0;
    if (current === count) return;
    const clone = [...refs.sessions.value];
    clone.splice(idx, 1, { ...clone[idx], messageCount: count });
    refs.sessions.value = clone;
  }

  function setMessages(sessionId: string, messages: ChatMessage[]) {
    refs.messagesBySession.value = {
      ...refs.messagesBySession.value,
      [sessionId]: messages,
    };
    syncSessionMessageCount(sessionId, messages.length);
  }

  return {
    syncSessionMessageCount,
    setMessages,
    appendMessage: (
      sessionId: string,
      message: ChatMessage,
      updatePreview = true,
    ) => appendMessage(refs, setMessages, sessionId, message, updatePreview),
    updateMessage: (
      sessionId: string,
      messageId: string,
      updater: (m: ChatMessage) => ChatMessage,
    ) => updateMessage(refs, setMessages, sessionId, messageId, updater),
  };
}

function appendMessage(
  refs: ChatStoreRefs,
  setMessages: (sessionId: string, messages: ChatMessage[]) => void,
  sessionId: string,
  message: ChatMessage,
  updatePreview = true,
) {
  const existing = refs.messagesBySession.value[sessionId] || [];
  setMessages(sessionId, [...existing, message]);
  if (
    updatePreview &&
    (message.role === "assistant" || message.role === "user")
  ) {
    touchSession(refs, sessionId, snippet(message.content));
  }
}

function updateMessage(
  refs: ChatStoreRefs,
  setMessages: (sessionId: string, messages: ChatMessage[]) => void,
  sessionId: string,
  messageId: string,
  updater: (m: ChatMessage) => ChatMessage,
) {
  const existing = refs.messagesBySession.value[sessionId] || [];
  let updated = false;
  const next = existing.map((m) => {
    if (m.id !== messageId) return m;
    updated = true;
    return updater(m);
  });
  if (updated) setMessages(sessionId, next);
}

function createSessionMetaActions(
  refs: ChatStoreRefs,
  setMessages: (sessionId: string, messages: ChatMessage[]) => void,
) {
  return {
    touchSession: (sessionId: string, preview?: string) =>
      touchSession(refs, sessionId, preview),
    ensureSession: () => ensureSession(refs, setMessages),
    hasUserPrompt: (sessionId: string) => {
      const existing = refs.messagesBySession.value[sessionId] || [];
      return existing.some((m) => m.role === "user");
    },
    upsertSessionMeta: (meta: ChatSessionMeta) => upsertSessionMeta(refs, meta),
  };
}

function touchSession(
  refs: ChatStoreRefs,
  sessionId: string,
  preview?: string,
) {
  const idx = refs.sessions.value.findIndex((s) => s.id === sessionId);
  if (idx === -1) return;
  const session = refs.sessions.value[idx];
  const updated: ChatSessionMeta = {
    ...session,
    updatedAt: new Date().toISOString(),
    lastMessagePreview: preview ?? session.lastMessagePreview,
  };
  const clone = [...refs.sessions.value];
  clone.splice(idx, 1, updated);
  refs.sessions.value = sortChatSessions(clone);
}

function ensureSession(
  refs: ChatStoreRefs,
  setMessages: (sessionId: string, messages: ChatMessage[]) => void,
): string {
  if (!refs.activeSessionId.value) throw new Error("No active conversation");
  if (!(refs.activeSessionId.value in refs.messagesBySession.value)) {
    setMessages(refs.activeSessionId.value, []);
  }
  return refs.activeSessionId.value;
}

function upsertSessionMeta(refs: ChatStoreRefs, meta: ChatSessionMeta) {
  const idx = refs.sessions.value.findIndex((s) => s.id === meta.id);
  if (idx === -1) return;
  const existing = refs.sessions.value[idx];
  const merged = normalizeSessionMeta({ ...existing, ...meta });
  const clone = [...refs.sessions.value];
  clone.splice(idx, 1, merged);
  refs.sessions.value = sortChatSessions(clone);
}

export function createChatStoreState() {
  const refs = createChatStoreRefs();
  const computedRefs = createChatComputed(refs);
  const thoughtActions = createThoughtSummaryActions(refs);
  const streamActions = createStreamStateActions(refs);
  const threadActions = createAgentThreadActions(refs);
  const messageActions = createMessageActions(refs);
  const metaActions = createSessionMetaActions(
    refs,
    messageActions.setMessages,
  );
  return {
    ...refs,
    ...computedRefs,
    ...thoughtActions,
    ...streamActions,
    ...threadActions,
    ...messageActions,
    ...metaActions,
  };
}

export type ChatStoreState = ReturnType<typeof createChatStoreState>;
