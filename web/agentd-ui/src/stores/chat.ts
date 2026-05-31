import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import type {
  AgentThread,
  AgentTraceEntry,
  ChatAttachment,
  ChatInputRequest,
  ChatInputRequestChoice,
  ChatMessage,
  ChatRole,
  ChatSessionMeta,
} from "@/types/chat";
import {
  answerChatInputRequest as apiAnswerChatInputRequest,
  createChatSession as apiCreateChatSession,
  deleteChatSession as apiDeleteChatSession,
  deleteChatMessage as apiDeleteChatMessage,
  deleteChatMessagesAfter as apiDeleteChatMessagesAfter,
  fetchChatActivities,
  fetchChatMessages,
  generateChatSessionTitle,
  listChatSessions,
  renameChatSession as apiRenameChatSession,
  streamAgentRun,
  streamAgentVisionRun,
  updateChatSessionMemorySettings as apiUpdateChatSessionMemorySettings,
  updateChatSessionProject as apiUpdateChatSessionProject,
  type ChatStreamEvent,
} from "@/api/chat";
import {
  agentThreadKey,
  agentToolEntry,
  appendAgentEntry,
  computeLocalTitle,
  httpStatus,
  memoryContextFromEvent,
  mergeMemoryContext,
  newAgentThread,
  normalizeSessionMeta,
  snippet,
  withTeam,
} from "@/stores/chatHelpers";
import { stripLeadingSpecialistMention } from "@/utils/chatMentions";
import { createId } from "@/utils/uuid";

type FilesByAttachment = Map<string, File>;

export interface SummaryEvent {
  inputTokens: number;
  tokenBudget: number;
  messageCount: number;
  summarizedCount: number;
  timestamp: string;
}

export const useChatStore = defineStore("chat", () => {
  const queryClient = useQueryClient();

  const sessions = ref<ChatSessionMeta[]>([]);
  const messagesBySession = ref<Record<string, ChatMessage[]>>({});
  const sessionsLoading = ref(false);
  const sessionsError = ref<string | null>(null);
  const fetchedMessageSessions = new Set<string>();

  const activeSessionId = ref<string>("");
  type StreamState = {
    assistantId: string;
    abortController: AbortController;
    streamId: string;
  };
  const streamingStateBySession = ref<Record<string, StreamState>>({});
  const isStreaming = computed(() => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return false;
    return Boolean(streamingStateBySession.value[sessionId]);
  });
  const toolMessageIndex = new Map<
    string,
    { streamId: string; index: Map<string, string> }
  >();
  const thoughtSummariesBySession = ref<Record<string, string[]>>({});
  const agentThreadsBySession = ref<Record<string, AgentThread[]>>({});
  const agentThreadIndex = new Map<string, Map<string, AgentThread>>();
  // Track summary events per session - cleared after display
  const summaryEventBySession = ref<Record<string, SummaryEvent | null>>({});

  const activeSession = computed(
    () => sessions.value.find((s) => s.id === activeSessionId.value) || null,
  );
  const activeMessages = computed(
    () => messagesBySession.value[activeSessionId.value] || [],
  );
  const chatMessages = computed(() =>
    activeMessages.value.filter((m) => m.role !== "tool"),
  );
  const toolMessages = computed(() =>
    activeMessages.value.filter((m) => m.role === "tool"),
  );
  const agentThreads = computed(
    () => agentThreadsBySession.value[activeSessionId.value] || [],
  );
  const activeSummaryEvent = computed(
    () => summaryEventBySession.value[activeSessionId.value] || null,
  );
  const activeThoughtSummaries = computed(
    () => thoughtSummariesBySession.value[activeSessionId.value] || [],
  );

  function clearSummaryEvent(sessionId?: string) {
    const id = sessionId || activeSessionId.value;
    if (id) {
      summaryEventBySession.value = {
        ...summaryEventBySession.value,
        [id]: null,
      };
    }
  }

  function clearThoughtSummaries(sessionId?: string) {
    const id = sessionId || activeSessionId.value;
    if (!id) return;
    thoughtSummariesBySession.value = {
      ...thoughtSummariesBySession.value,
      [id]: [],
    };
  }

  function appendThoughtSummary(sessionId: string, summary: string) {
    const text = (summary || "").trim();
    if (!text) return;
    const existing = thoughtSummariesBySession.value[sessionId] || [];
    const last = existing[existing.length - 1];
    if (last) {
      if (text === last) return;
      if (text.length > last.length && text.startsWith(last)) {
        const next = [...existing];
        next[next.length - 1] = text;
        thoughtSummariesBySession.value = {
          ...thoughtSummariesBySession.value,
          [sessionId]: next,
        };
        return;
      }
    }
    thoughtSummariesBySession.value = {
      ...thoughtSummariesBySession.value,
      [sessionId]: [...existing, text],
    };
  }

  function isSessionStreaming(sessionId: string) {
    return streamingStateFor(sessionId) !== undefined;
  }

  function streamingStateFor(sessionId: string) {
    return streamingStateBySession.value[sessionId];
  }

  function setStreamingState(sessionId: string, state: StreamState) {
    streamingStateBySession.value = {
      ...streamingStateBySession.value,
      [sessionId]: state,
    };
  }

  function clearStreamingState(sessionId: string) {
    if (!(sessionId in streamingStateBySession.value)) return;
    const { [sessionId]: _removed, ...rest } = streamingStateBySession.value;
    streamingStateBySession.value = rest;
  }

  function toolIndexFor(sessionId: string, streamId: string) {
    let entry = toolMessageIndex.get(sessionId);
    if (!entry || entry.streamId !== streamId) {
      entry = { streamId, index: new Map<string, string>() };
      toolMessageIndex.set(sessionId, entry);
    }
    return entry.index;
  }

  function clearToolIndex(sessionId: string, streamId: string) {
    const entry = toolMessageIndex.get(sessionId);
    if (entry?.streamId === streamId) toolMessageIndex.delete(sessionId);
  }

  function isStreamCurrent(sessionId: string, streamId: string) {
    const state = streamingStateFor(sessionId);
    return Boolean(state && state.streamId === streamId);
  }

  function threadIndexFor(sessionId: string) {
    let idx = agentThreadIndex.get(sessionId);
    if (!idx) {
      idx = new Map<string, AgentThread>();
      agentThreadIndex.set(sessionId, idx);
    }
    return idx;
  }

  function resetAgentThreads(sessionId: string) {
    const next = { ...agentThreadsBySession.value, [sessionId]: [] };
    agentThreadsBySession.value = next;
    agentThreadIndex.delete(sessionId);
  }

  function setAgentThreads(sessionId: string, threads: AgentThread[]) {
    const nextThreads = [...threads].sort((a, b) => {
      const aTime = Date.parse(a.finishedAt || a.startedAt || "") || 0;
      const bTime = Date.parse(b.finishedAt || b.startedAt || "") || 0;
      return bTime - aTime;
    });
    agentThreadsBySession.value = {
      ...agentThreadsBySession.value,
      [sessionId]: nextThreads,
    };
    const idx = new Map<string, AgentThread>();
    nextThreads.forEach((thread) =>
      idx.set(agentThreadKey(thread.callId, thread.assistantMessageId), thread),
    );
    agentThreadIndex.set(sessionId, idx);
  }

  function resetThoughtSummaries(sessionId: string) {
    thoughtSummariesBySession.value = {
      ...thoughtSummariesBySession.value,
      [sessionId]: [],
    };
  }

  function upsertAgentThread(
    sessionId: string,
    callId: string,
    factory: () => AgentThread,
    updater?: (t: AgentThread) => AgentThread,
    assistantMessageId?: string,
  ): AgentThread {
    const idx = threadIndexFor(sessionId);
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
    const list = agentThreadsBySession.value[sessionId] || [];
    const found = list.findIndex(
      (t) => agentThreadKey(t.callId, t.assistantMessageId) === key,
    );
    const nextList = [...list];
    if (found === -1) nextList.push(thread);
    else nextList.splice(found, 1, thread);
    agentThreadsBySession.value = {
      ...agentThreadsBySession.value,
      [sessionId]: nextList,
    };
    return thread;
  }

  function syncSessionMessageCount(sessionId: string, count: number) {
    const idx = sessions.value.findIndex((s) => s.id === sessionId);
    if (idx === -1) return;
    const current = sessions.value[idx].messageCount ?? 0;
    if (current === count) return;
    const clone = [...sessions.value];
    clone.splice(idx, 1, { ...clone[idx], messageCount: count });
    sessions.value = clone;
  }

  function setMessages(sessionId: string, messages: ChatMessage[]) {
    messagesBySession.value = {
      ...messagesBySession.value,
      [sessionId]: messages,
    };
    syncSessionMessageCount(sessionId, messages.length);
  }

  function appendMessage(
    sessionId: string,
    message: ChatMessage,
    updatePreview = true,
  ) {
    const existing = messagesBySession.value[sessionId] || [];
    setMessages(sessionId, [...existing, message]);
    if (
      updatePreview &&
      (message.role === "assistant" || message.role === "user")
    ) {
      touchSession(sessionId, snippet(message.content));
    }
  }

  async function deleteMessage(sessionId: string, messageId: string) {
    if (!sessionId || !messageId) return;
    await apiDeleteChatMessage(sessionId, messageId);
    const refreshed = await fetchChatMessages(sessionId);
    setMessages(sessionId, refreshed);
    clearThoughtSummaries(sessionId);
    clearSummaryEvent(sessionId);
  }

  async function deleteMessagesAfter(
    sessionId: string,
    messageId: string,
    inclusive = false,
  ) {
    if (!sessionId || !messageId) return;
    await apiDeleteChatMessagesAfter(sessionId, messageId, inclusive);
    const refreshed = await fetchChatMessages(sessionId);
    setMessages(sessionId, refreshed);
    clearThoughtSummaries(sessionId);
    clearSummaryEvent(sessionId);
  }

  function updateMessage(
    sessionId: string,
    messageId: string,
    updater: (m: ChatMessage) => ChatMessage,
  ) {
    const existing = messagesBySession.value[sessionId] || [];
    let updated = false;
    const next = existing.map((m) => {
      if (m.id === messageId) {
        updated = true;
        return updater(m);
      }
      return m;
    });
    if (updated) setMessages(sessionId, next);
  }

  function touchSession(sessionId: string, preview?: string) {
    const idx = sessions.value.findIndex((s) => s.id === sessionId);
    if (idx === -1) return;
    const session = sessions.value[idx];
    const updated: ChatSessionMeta = {
      ...session,
      updatedAt: new Date().toISOString(),
      lastMessagePreview: preview ?? session.lastMessagePreview,
    };
    const clone = [...sessions.value];
    clone.splice(idx, 1, updated);
    sessions.value = clone;
  }

  function ensureSession(): string {
    if (!activeSessionId.value) throw new Error("No active conversation");
    if (!(activeSessionId.value in messagesBySession.value)) {
      setMessages(activeSessionId.value, []);
    }
    return activeSessionId.value;
  }

  const defaultSessionNames = new Set(["", "new chat", "conversation"]);

  function isDefaultSessionName(name?: string | null) {
    if (!name) return true;
    return defaultSessionNames.has(name.trim().toLowerCase());
  }

  function hasUserPrompt(sessionId: string) {
    const existing = messagesBySession.value[sessionId] || [];
    return existing.some((m) => m.role === "user");
  }

  function upsertSessionMeta(meta: ChatSessionMeta) {
    const idx = sessions.value.findIndex((s) => s.id === meta.id);
    if (idx === -1) return;
    const existing = sessions.value[idx];
    const merged = normalizeSessionMeta({ ...existing, ...meta });
    const clone = [...sessions.value];
    clone.splice(idx, 1, merged);
    sessions.value = clone;
  }

  async function init() {
    if (sessions.value.length) return;
    await refreshSessionsFromServer(true);
  }

  async function refreshSessionsFromServer(initial = false) {
    sessionsLoading.value = true;
    if (!initial) sessionsError.value = null;
    try {
      let remote = await listChatSessions();
      if (!remote) remote = [];
      remote = remote.map(normalizeSessionMeta);
      if (initial && remote.length === 0) {
        const created = await apiCreateChatSession("New Chat");
        if (created) remote = [normalizeSessionMeta(created)];
      }
      sessionsError.value = null;
      sessions.value = remote;
      const nextMessages: Record<string, ChatMessage[]> = {};
      for (const s of remote) {
        const existing = messagesBySession.value[s.id] || [];
        nextMessages[s.id] = existing;
        const fallbackCount =
          typeof s.messageCount === "number" ? s.messageCount : 0;
        const count = existing.length ? existing.length : fallbackCount;
        syncSessionMessageCount(s.id, count);
      }
      messagesBySession.value = nextMessages;
      fetchedMessageSessions.clear();
      if (!remote.length) {
        activeSessionId.value = "";
        return;
      }
      if (!remote.some((s) => s.id === activeSessionId.value)) {
        activeSessionId.value = remote[0].id;
      }
      if (activeSessionId.value)
        await loadMessagesFromServer(activeSessionId.value, { force: true });
    } catch (error) {
      const status = httpStatus(error);
      if (status === 401) sessionsError.value = "Authentication required.";
      else if (status === 403)
        sessionsError.value =
          "Access denied. You do not have permission to view conversations.";
      else sessionsError.value = "Failed to load conversations.";
      console.error("Failed to load chat sessions", error);
    } finally {
      sessionsLoading.value = false;
    }
  }

  async function loadMessagesFromServer(
    sessionId: string,
    options: { force?: boolean } = {},
  ) {
    if (!sessionId) return;
    if (!options.force && fetchedMessageSessions.has(sessionId)) return;
    const beforeFetch = messagesBySession.value[sessionId] || [];
    try {
      const [data, activities] = await Promise.all([
        fetchChatMessages(sessionId),
        fetchChatActivities(sessionId),
      ]);
      fetchedMessageSessions.add(sessionId);
      const current = messagesBySession.value[sessionId] || [];
      const changedDuringFetch = current !== beforeFetch;
      const preserveLocal =
        changedDuringFetch &&
        (current.some((m) => !!m.streaming) || current.length > data.length);
      if (preserveLocal) {
        syncSessionMessageCount(sessionId, current.length);
        return;
      }
      setMessages(sessionId, data);
      setAgentThreads(sessionId, activities || []);
    } catch (error) {
      const status = httpStatus(error);
      if (status === 403)
        sessionsError.value = "Access denied for this conversation.";
      else if (status === 404) await refreshSessionsFromServer();
      console.error("Failed to load chat messages", error);
    }
  }

  function selectSession(sessionId: string) {
    activeSessionId.value = sessionId;
    void loadMessagesFromServer(sessionId);
  }

  async function createSession(name = "New Chat") {
    const session = await apiCreateChatSession(name);
    if (!session) return;
    const normalized = normalizeSessionMeta(session);
    sessionsError.value = null;
    sessions.value = [normalized, ...sessions.value];
    setMessages(normalized.id, []);
    fetchedMessageSessions.delete(normalized.id);
    activeSessionId.value = normalized.id;
    await loadMessagesFromServer(normalized.id, { force: true });
  }

  async function deleteSession(sessionId: string) {
    await apiDeleteChatSession(sessionId);
    sessionsError.value = null;
    const nextSessions = sessions.value.filter((s) => s.id !== sessionId);
    const { [sessionId]: _removed, ...rest } = messagesBySession.value;
    messagesBySession.value = rest;
    const { [sessionId]: _removedThreads, ...restThreads } =
      agentThreadsBySession.value;
    agentThreadsBySession.value = restThreads;
    agentThreadIndex.delete(sessionId);
    fetchedMessageSessions.delete(sessionId);
    if (!nextSessions.length) {
      const fresh = await apiCreateChatSession("New Chat");
      const normalizedFresh = normalizeSessionMeta(fresh);
      sessions.value = [normalizedFresh];
      setMessages(normalizedFresh.id, []);
      fetchedMessageSessions.delete(normalizedFresh.id);
      activeSessionId.value = normalizedFresh.id;
      await loadMessagesFromServer(normalizedFresh.id, { force: true });
      return;
    }
    sessions.value = nextSessions;
    if (activeSessionId.value === sessionId) {
      activeSessionId.value = nextSessions[0]?.id || "";
      if (activeSessionId.value)
        await loadMessagesFromServer(activeSessionId.value, { force: true });
    }
  }

  async function renameSession(sessionId: string, name: string) {
    const updated = await apiRenameChatSession(sessionId, name);
    sessionsError.value = null;
    upsertSessionMeta(updated);
  }

  async function updateSessionProject(sessionId: string, projectId: string) {
    const cleanProjectID = (projectId || "").trim();
    const existing = sessions.value.find((s) => s.id === sessionId);
    if (existing && (existing.projectId || "") === cleanProjectID) {
      return existing;
    }
    const updated = await apiUpdateChatSessionProject(
      sessionId,
      cleanProjectID,
    );
    sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    upsertSessionMeta(normalized);
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
    const existing = sessions.value.find((s) => s.id === sessionId);
    const nextMemory =
      typeof settings.memoryEnabled === "boolean"
        ? settings.memoryEnabled
        : typeof settings.evolvingMemoryEnabled === "boolean" ||
            typeof settings.beliefMemoryEnabled === "boolean"
          ? (settings.evolvingMemoryEnabled ??
              existing?.evolvingMemoryEnabled ??
              false) &&
            (settings.beliefMemoryEnabled ??
              existing?.beliefMemoryEnabled ??
              false)
          : (existing?.memoryEnabled ?? false);
    if (existing && (existing.memoryEnabled ?? false) === nextMemory) {
      return existing;
    }
    const updated = await apiUpdateChatSessionMemorySettings(sessionId, {
      memoryEnabled: nextMemory,
    });
    sessionsError.value = null;
    const normalized = normalizeSessionMeta(updated);
    upsertSessionMeta(normalized);
    return normalized;
  }

  async function sendPrompt(
    text: string,
    attachments: ChatAttachment[] = [],
    filesByAttachment?: FilesByAttachment,
    options: {
      echoUser?: boolean;
      specialist?: string;
      routingSpecialist?: string;
      teamName?: string;
      projectId?: string;
      image?: boolean;
      imageSize?: string;
      memoryEnabled?: boolean;
      evolvingMemoryEnabled?: boolean;
      beliefMemoryEnabled?: boolean;
      agentName?: string;
      agentModel?: string;
    } = {},
  ) {
    const content = (text || "").trim();
    const sessionId = ensureSession();
    if (!content && !attachments.length) return;
    const wasStreaming = isSessionStreaming(sessionId);
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

    if (!wasStreaming) resetThoughtSummaries(sessionId);

    if (content) {
      void maybeAutoTitle(sessionId, content);
    }

    if (options.echoUser !== false) {
      const attachmentsCopy = attachments.map((a) => ({ ...a }));
      appendMessage(sessionId, {
        id: createId(),
        role: "user",
        content,
        createdAt: now,
        attachments: attachmentsCopy,
      });
    }

    const assistantId = createId();
    const streamId = createId();
    appendMessage(sessionId, {
      id: assistantId,
      role: "assistant",
      content: "",
      createdAt: now,
      streaming: true,
      agentName: agentName || undefined,
      agentModel: agentModel || undefined,
      model: agentModel || undefined,
    });

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
    setStreamingState(sessionId, {
      assistantId,
      abortController: controller,
      streamId,
    });
    toolIndexFor(sessionId, streamId);

    try {
      // Expand text attachments into the prompt
      let promptToSend = stripLeadingSpecialistMention(
        content,
        options.routingSpecialist || options.specialist,
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

      if (imageFiles.length) {
        await streamAgentVisionRun({
          prompt: promptToSend,
          sessionId,
          assistantMessageId: assistantId,
          files: imageFiles,
          signal: controller.signal,
          onEvent: (e) =>
            handleStreamEvent(e, sessionId, assistantId, streamId),
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
          assistantMessageId: assistantId,
          signal: controller.signal,
          onEvent: (e) =>
            handleStreamEvent(e, sessionId, assistantId, streamId),
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
      updateMessage(sessionId, assistantId, assistantUpdater);
    } finally {
      if (isStreamCurrent(sessionId, streamId)) {
        clearStreamingState(sessionId);
        clearToolIndex(sessionId, streamId);
      }
    }
  }

  async function maybeAutoTitle(sessionId: string, prompt: string) {
    const currentSession = sessions.value.find((s) => s.id === sessionId);
    if (!currentSession || !isDefaultSessionName(currentSession.name)) return;
    if (hasUserPrompt(sessionId)) return;
    const trimmed = prompt.trim();
    if (!trimmed) return;
    try {
      // Optimistically set title immediately on the client for instant UI updates
      const localTitle = computeLocalTitle(trimmed);
      if (localTitle) {
        upsertSessionMeta({
          id: sessionId,
          name: localTitle,
          createdAt: currentSession.createdAt,
          updatedAt: new Date().toISOString(),
        });
      }
      const updated = await generateChatSessionTitle(sessionId, trimmed);
      upsertSessionMeta(updated);
    } catch (error) {
      console.warn("auto-title failed", error);
      return;
    }
  }

  function handleStreamEvent(
    event: ChatStreamEvent,
    sessionId: string,
    assistantId: string,
    streamId: string,
  ) {
    if (!isStreamCurrent(sessionId, streamId)) return;
    switch (event.type) {
      case "thought_summary": {
        if (typeof event.data === "string" && event.data.trim()) {
          appendThoughtSummary(sessionId, event.data);
          updateMessage(sessionId, assistantId, (m) => ({
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
          updateMessage(sessionId, assistantId, (m) => ({
            ...m,
            memoryContext: mergeMemoryContext(m.memoryContext, incoming),
          }));
        }
        break;
      }
      case "delta": {
        if (typeof event.data === "string" && event.data) {
          updateMessage(sessionId, assistantId, (m) => ({
            ...m,
            content: m.content + event.data,
          }));
        }
        break;
      }
      case "final": {
        const text = typeof event.data === "string" ? event.data : "";
        updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          content: text || m.content,
          streaming: false,
        }));
        if (text) touchSession(sessionId, snippet(text));
        try {
          queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
        } catch {}
        break;
      }
      case "tool_start": {
        updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          activityToolTitle: event.title || "Tool call",
        }));
        break;
      }
      case "tool_result": {
        if (typeof event.title === "string" && event.title.trim()) {
          updateMessage(sessionId, assistantId, (m) => ({
            ...m,
            activityToolTitle: event.title,
          }));
        }
        break;
      }
      case "image": {
        const name =
          typeof event.name === "string" && event.name.trim()
            ? event.name.trim()
            : "generated image";
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
        updateMessage(sessionId, assistantId, (m) => {
          const attachments = [...(m.attachments || [])];
          attachments.push({
            id: createId(),
            kind: "image",
            name: name || savedPath || "image",
            mime,
            previewUrl: previewUrl || undefined,
            path: savedPath,
          });
          let content = m.content;
          if (savedPath && !content.includes(savedPath)) {
            const note = `Image saved: ${savedPath}`;
            content = content ? `${content}\n\n${note}` : note;
          }
          return { ...m, attachments, content };
        });
        break;
      }
      case "tts_chunk":
        break;
      case "tts_audio": {
        const now = new Date().toISOString();
        if (typeof event.url === "string") {
          appendMessage(
            sessionId,
            {
              id: createId(),
              role: "tool",
              title: event.title || "Audio response",
              content: "The agent produced an audio reply.",
              createdAt: now,
              audioUrl: event.url,
              audioFilePath:
                typeof event.file_path === "string"
                  ? event.file_path
                  : undefined,
            },
            false,
          );
        }
        break;
      }
      case "summary": {
        const summaryEvt: SummaryEvent = {
          inputTokens:
            typeof event.input_tokens === "number" ? event.input_tokens : 0,
          tokenBudget:
            typeof event.token_budget === "number" ? event.token_budget : 0,
          messageCount:
            typeof event.message_count === "number" ? event.message_count : 0,
          summarizedCount:
            typeof event.summarized_count === "number"
              ? event.summarized_count
              : 0,
          timestamp: new Date().toISOString(),
        };
        summaryEventBySession.value = {
          ...summaryEventBySession.value,
          [sessionId]: summaryEvt,
        };
        break;
      }
      case "input_request": {
        handleInputRequestEvent(event, sessionId, assistantId);
        break;
      }
      case "input_request_cancelled": {
        handleInputRequestCancelledEvent(event, sessionId, assistantId);
        break;
      }
      case "error": {
        const message =
          typeof event.data === "string" ? event.data : "Agent error";
        updateMessage(sessionId, assistantId, (existing) => ({
          ...existing,
          streaming: false,
          error: message,
        }));
        break;
      }
      case "agent_start":
      case "agent_delta":
      case "agent_final":
      case "agent_tool_start":
      case "agent_tool_result":
      case "agent_error":
      case "agent_thought_summary": {
        handleAgentTraceEvent(event, sessionId, assistantId);
        break;
      }
      default:
        break;
    }
  }

  function normalizeInputRequestChoices(
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
        const rawLabel =
          typeof node.label === "string" ? node.label.trim() : "";
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

  function handleInputRequestEvent(
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
    updateMessage(sessionId, assistantId, (m) => {
      const requests = [...(m.inputRequests || [])];
      const existing = requests.findIndex((item) => item.id === request.id);
      if (existing === -1) requests.push(request);
      else requests.splice(existing, 1, { ...requests[existing], ...request });
      return { ...m, inputRequests: requests };
    });
  }

  function handleInputRequestCancelledEvent(
    event: ChatStreamEvent,
    sessionId: string,
    assistantId: string,
  ) {
    const requestId =
      typeof event.request_id === "string" && event.request_id.trim()
        ? event.request_id.trim()
        : "";
    if (!requestId) return;
    updateInputRequest(sessionId, assistantId, requestId, (request) => ({
      ...request,
      status: "cancelled",
      error:
        typeof event.error === "string" && event.error.trim()
          ? event.error.trim()
          : "Request cancelled",
    }));
  }

  function updateInputRequest(
    sessionId: string,
    messageId: string,
    requestId: string,
    updater: (request: ChatInputRequest) => ChatInputRequest,
  ) {
    updateMessage(sessionId, messageId, (m) => {
      const requests = m.inputRequests || [];
      const next = requests.map((request) =>
        request.id === requestId ? updater(request) : request,
      );
      return { ...m, inputRequests: next };
    });
  }

  async function submitInputRequest(
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
      updateInputRequest(sessionId, messageId, requestId, (request) => ({
        ...request,
        status: "answered",
        answer: cleanAnswer,
        choiceIds: cleanChoiceIds,
        answeredAt: new Date().toISOString(),
        error: undefined,
      }));
    } catch (error) {
      updateInputRequest(sessionId, messageId, requestId, (request) => ({
        ...request,
        status: "error",
        error:
          error instanceof Error ? error.message : "Failed to submit response",
      }));
      throw error;
    }
  }

  function handleAgentTraceEvent(
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
      typeof event.parent_call_id === "string"
        ? event.parent_call_id
        : undefined;
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
      upsertAgentThread(sessionId, callId, factory, updater, assistantId);
    };

    if (!parentCallId?.trim()) {
      handleDirectAgentTraceEvent(event, sessionId, assistantId, {
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
        updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          agentName: m.agentName || values.agentName,
          agentModel: m.agentModel || values.model,
          model: m.model || values.model,
        }));
        break;
      }
      case "agent_delta": {
        if (values.contentText) {
          updateMessage(sessionId, assistantId, (m) => ({
            ...m,
          }));
        }
        break;
      }
      case "agent_final": {
        updateMessage(sessionId, assistantId, (m) => ({
          ...m,
        }));
        break;
      }
      case "agent_tool_start":
      case "agent_tool_result": {
        if (typeof event.title === "string" && event.title.trim()) {
          updateMessage(sessionId, assistantId, (m) => ({
            ...m,
            activityToolTitle: event.title,
          }));
        }
        break;
      }
      case "agent_error": {
        const errText =
          typeof event.error === "string"
            ? event.error
            : values.data || "Agent error";
        updateMessage(sessionId, assistantId, (m) => ({
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
        updateMessage(sessionId, assistantId, (m) => ({
          ...m,
          activityThoughtSummary: summary,
        }));
        break;
      }
      default:
        break;
    }
  }

  function findLastIndex<T>(items: T[], predicate: (t: T) => boolean): number {
    for (let i = items.length - 1; i >= 0; i -= 1)
      if (predicate(items[i])) return i;
    return -1;
  }

  function interruptStreaming(
    sessionId: string,
    options: {
      reason?: string;
      archiveThoughtSummaries?: boolean;
      clearThoughtSummaries?: boolean;
    } = {},
  ) {
    const state = streamingStateFor(sessionId);
    if (!state) return false;
    const reason = options.reason || "Interrupted";
    const now = new Date().toISOString();

    if (options.archiveThoughtSummaries) {
      const summaries = thoughtSummariesBySession.value[sessionId] || [];
      if (summaries.length) {
        appendMessage(
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

    updateMessage(sessionId, state.assistantId, (m) => ({
      ...m,
      streaming: false,
      error: reason,
      inputRequests: (m.inputRequests || []).map((request) =>
        request.status === "pending" || request.status === "error"
          ? { ...request, status: "cancelled", error: reason }
          : request,
      ),
    }));

    const existing = messagesBySession.value[sessionId] || [];
    if (existing.some((m) => m.role === "tool" && m.streaming)) {
      const next = existing.map((m) =>
        m.role === "tool" && m.streaming
          ? { ...m, streaming: false, error: reason }
          : m,
      );
      setMessages(sessionId, next);
    }

    if (options.clearThoughtSummaries) {
      clearThoughtSummaries(sessionId);
    }

    state.abortController.abort("interrupt");
    return true;
  }

  function stopStreaming(sessionId?: string) {
    const targetSessionId = sessionId || activeSessionId.value;
    if (!targetSessionId) return;
    if (!interruptStreaming(targetSessionId, { reason: "Generation stopped" }))
      return;
    console.warn("chat stopStreaming called", { sessionId: targetSessionId });
  }

  async function regenerateAssistant(
    options: {
      specialist?: string;
      routingSpecialist?: string;
      teamName?: string;
      projectId?: string;
      memoryEnabled?: boolean;
      evolvingMemoryEnabled?: boolean;
      beliefMemoryEnabled?: boolean;
      agentName?: string;
      agentModel?: string;
      messageId?: string;
    } = {},
  ) {
    const sessionId = ensureSession();
    if (isSessionStreaming(sessionId)) return;
    const messages = messagesBySession.value[sessionId] || [];
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
    await deleteMessagesAfter(sessionId, target.id, true);
    await sendPrompt(lastUser.content, [], undefined, {
      echoUser: false,
      specialist: options.specialist,
      routingSpecialist: options.routingSpecialist,
      teamName: options.teamName,
      projectId: options.projectId,
      memoryEnabled: options.memoryEnabled,
      evolvingMemoryEnabled: options.evolvingMemoryEnabled,
      beliefMemoryEnabled: options.beliefMemoryEnabled,
      agentName: options.agentName,
      agentModel: options.agentModel,
    });
  }

  return {
    // state
    sessions,
    messagesBySession,
    sessionsLoading,
    sessionsError,
    activeSessionId,
    isStreaming,
    activeSession,
    activeMessages,
    chatMessages,
    toolMessages,
    agentThreads,
    activeSummaryEvent,
    activeThoughtSummaries,
    isSessionStreaming,
    // actions
    init,
    refreshSessionsFromServer,
    loadMessagesFromServer,
    selectSession,
    createSession,
    deleteSession,
    deleteMessage,
    renameSession,
    updateSessionProject,
    updateSessionMemorySettings,
    submitInputRequest,
    sendPrompt,
    stopStreaming,
    regenerateAssistant,
    clearSummaryEvent,
    clearThoughtSummaries,
  };
});
