import { defineStore } from "pinia";
import { computed, ref } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import type {
  AgentThread,
  ChatAttachment,
  ChatMessage,
  ChatRole,
  ChatSessionMeta,
} from "@/types/chat";
import {
  createChatSession as apiCreateChatSession,
  deleteChatSession as apiDeleteChatSession,
  fetchChatMessages,
  generateChatSessionTitle,
  listChatSessions,
  renameChatSession as apiRenameChatSession,
  streamAgentRun,
  streamAgentVisionRun,
  type ChatStreamEvent,
} from "@/api/chat";

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
    return Boolean(streamingStateBySession.value[sessionId]);
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
  ): AgentThread {
    const idx = threadIndexFor(sessionId);
    const existing = idx.get(callId);
    const thread = existing
      ? updater
        ? updater(existing)
        : existing
      : factory();
    idx.set(callId, thread);
    const list = agentThreadsBySession.value[sessionId] || [];
    const found = list.findIndex((t) => t.callId === callId);
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

  function snippet(content: string) {
    if (!content) return "";
    const trimmed = content.replace(/\s+/g, " ").trim();
    return trimmed.length > 80 ? `${trimmed.slice(0, 77)}…` : trimmed;
  }

  function normalizeSessionMeta(meta: ChatSessionMeta): ChatSessionMeta {
    const rawCount = (meta as any).messageCount ?? (meta as any).message_count;
    const messageCount =
      typeof rawCount === "number" && Number.isFinite(rawCount) && rawCount >= 0
        ? rawCount
        : 0;
    return { ...meta, messageCount };
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

  function httpStatus(error: unknown): number | null {
    // Best-effort Axios compatibility check
    // @ts-ignore
    const isAxios =
      !!error && typeof error === "object" && "isAxiosError" in (error as any);
    // @ts-ignore
    return isAxios ? ((error as any).response?.status ?? null) : null;
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
    try {
      const data = (await fetchChatMessages(sessionId)) ?? [];
      fetchedMessageSessions.add(sessionId);
      setMessages(sessionId, data);
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

  async function sendPrompt(
    text: string,
    attachments: ChatAttachment[] = [],
    filesByAttachment?: FilesByAttachment,
    options: {
      echoUser?: boolean;
      specialist?: string;
      projectId?: string;
      image?: boolean;
      imageSize?: string;
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

    resetAgentThreads(sessionId);
    if (!wasStreaming) resetThoughtSummaries(sessionId);

    if (content) {
      void maybeAutoTitle(sessionId, content);
    }

    if (options.echoUser !== false) {
      const attachmentsCopy = attachments.map((a) => ({ ...a }));
      appendMessage(sessionId, {
        id: crypto.randomUUID(),
        role: "user",
        content,
        createdAt: now,
        attachments: attachmentsCopy,
      });
    }

    const assistantId = crypto.randomUUID();
    const streamId = crypto.randomUUID();
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
      let promptToSend = content;
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
          files: imageFiles,
          signal: controller.signal,
          onEvent: (e) => handleStreamEvent(e, sessionId, assistantId, streamId),
          specialist: options.specialist,
          projectId: options.projectId,
        });
      } else {
        await streamAgentRun({
          prompt: promptToSend,
          sessionId,
          signal: controller.signal,
          onEvent: (e) => handleStreamEvent(e, sessionId, assistantId, streamId),
          specialist: options.specialist,
          projectId: options.projectId,
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
    }
  }

  // --- Local title generation mirrors backend behavior ---
  const CHAT_TITLE_MAX_RUNES = 48;
  function collapseWhitespace(s: string): string {
    if (!s || !s.trim()) return "";
    return s.trim().replace(/\s+/g, " ");
  }
  function truncateRunes(s: string, max: number): string {
    if (max <= 0) return "";
    const codepoints = Array.from(s);
    if (codepoints.length <= max) return s.trim();
    return codepoints.slice(0, max).join("").trim();
  }
  function firstSentence(s: string): string {
    const input = s.trim();
    if (!input) return "";
    for (let i = 0; i < input.length; i++) {
      const ch = input[i];
      if (ch === "." || ch === "?" || ch === "!" || ch === "\n") {
        return input.slice(0, i + 1).trim();
      }
    }
    return input;
  }
  function computeLocalTitle(prompt: string): string {
    const sentence = firstSentence(prompt) || prompt;
    const collapsed = collapseWhitespace(sentence);
    if (!collapsed) return "Conversation";
    return truncateRunes(collapsed, CHAT_TITLE_MAX_RUNES);
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
        const now = new Date().toISOString();
        const key =
          typeof event.tool_id === "string" && event.tool_id.trim()
            ? event.tool_id
            : null;
        const messageId = crypto.randomUUID();
        if (key) toolIndexFor(sessionId, streamId).set(key, messageId);
        appendMessage(
          sessionId,
          {
            id: messageId,
            role: "tool" as ChatRole,
            title: event.title || "Tool call",
            content: "",
            toolArgs: typeof event.args === "string" ? event.args : undefined,
            createdAt: now,
            streaming: true,
          },
          false,
        );
        break;
      }
      case "tool_result": {
        const now = new Date().toISOString();
        const result = typeof event.data === "string" ? event.data : "";
        const key =
          typeof event.tool_id === "string" && event.tool_id.trim()
            ? event.tool_id
            : null;
        const toolIndex = toolIndexFor(sessionId, streamId);
        if (key && toolIndex.has(key)) {
          const messageId = toolIndex.get(key) as string;
          updateMessage(sessionId, messageId, (m) => ({
            ...m,
            title: m.title || event.title || "Tool result",
            content: m.content ? `${m.content}${result}` : result,
            streaming: false,
          }));
          toolIndex.delete(key);
        } else {
          // Fallback: attach to last streaming tool message
          const msgs = messagesBySession.value[sessionId] || [];
          const pendingIdx = findLastIndex(
            msgs,
            (msg) => msg.role === "tool" && !!msg.streaming,
          );
          if (pendingIdx !== -1) {
            const messageId = msgs[pendingIdx].id;
            updateMessage(sessionId, messageId, (m) => ({
              ...m,
              title: m.title || event.title || "Tool result",
              content: m.content ? `${m.content}${result}` : result,
              streaming: false,
            }));
          } else {
            appendMessage(
              sessionId,
              {
                id: crypto.randomUUID(),
                role: "tool",
                title: event.title || "Tool result",
                content: result,
                createdAt: now,
              },
              false,
            );
          }
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
            id: crypto.randomUUID(),
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
              id: crypto.randomUUID(),
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
      case "agent_error": {
        handleAgentTraceEvent(event, sessionId);
        break;
      }
      default:
        break;
    }
  }

  function handleAgentTraceEvent(event: ChatStreamEvent, sessionId: string) {
    const now = new Date().toISOString();
    const callId =
      typeof event.call_id === "string" && event.call_id.trim()
        ? event.call_id.trim()
        : crypto.randomUUID();
    const depth =
      typeof event.depth === "number" && event.depth >= 0 ? event.depth : 1;
    const parentCallId =
      typeof event.parent_call_id === "string"
        ? event.parent_call_id
        : undefined;
    const agentName = typeof event.agent === "string" ? event.agent : undefined;
    const model = typeof event.model === "string" ? event.model : undefined;
    const contentText =
      typeof event.content === "string" ? event.content : undefined;
    const args = typeof event.args === "string" ? event.args : undefined;
    const data = typeof event.data === "string" ? event.data : undefined;

    switch (event.type) {
      case "agent_start": {
        upsertAgentThread(sessionId, callId, () => ({
          callId,
          parentCallId,
          agent: agentName,
          model,
          prompt: contentText,
          depth,
          status: "running",
          content: "",
          entries: [],
          startedAt: now,
        }));
        break;
      }
      case "agent_delta": {
        upsertAgentThread(
          sessionId,
          callId,
          () => ({
            callId,
            parentCallId,
            agent: agentName,
            model,
            prompt: contentText,
            depth,
            status: "running",
            content: contentText || "",
            entries: [],
            startedAt: now,
          }),
          (thread) => ({
            ...thread,
            content: (thread.content || "") + (contentText || ""),
          }),
        );
        break;
      }
      case "agent_final": {
        upsertAgentThread(
          sessionId,
          callId,
          () => ({
            callId,
            parentCallId,
            agent: agentName,
            model,
            prompt: contentText,
            depth,
            status: "done",
            content: contentText || "",
            entries: [],
            startedAt: now,
            finishedAt: now,
          }),
          (thread) => ({
            ...thread,
            status: "done",
            finishedAt: thread.finishedAt || now,
            content: contentText || thread.content,
          }),
        );
        break;
      }
      case "agent_tool_start": {
        upsertAgentThread(
          sessionId,
          callId,
          () => ({
            callId,
            parentCallId,
            agent: agentName,
            model,
            prompt: "",
            depth,
            status: "running",
            content: "",
            entries: [
              {
                id: crypto.randomUUID(),
                type: "tool",
                title: event.title || "Tool",
                args,
                createdAt: now,
              },
            ],
            startedAt: now,
          }),
          (thread) => ({
            ...thread,
            entries: [
              ...thread.entries,
              {
                id: crypto.randomUUID(),
                type: "tool",
                title: event.title || "Tool",
                args,
                createdAt: now,
              },
            ],
          }),
        );
        break;
      }
      case "agent_tool_result": {
        upsertAgentThread(
          sessionId,
          callId,
          () => ({
            callId,
            parentCallId,
            agent: agentName,
            model,
            prompt: "",
            depth,
            status: "running",
            content: "",
            entries: [
              {
                id: crypto.randomUUID(),
                type: "tool",
                title: event.title || "Tool",
                data,
                createdAt: now,
              },
            ],
            startedAt: now,
          }),
          (thread) => ({
            ...thread,
            entries: [
              ...thread.entries,
              {
                id: crypto.randomUUID(),
                type: "tool",
                title: event.title || "Tool",
                data,
                createdAt: now,
              },
            ],
          }),
        );
        break;
      }
      case "agent_error": {
        const errText =
          typeof event.error === "string" ? event.error : data || "Agent error";
        upsertAgentThread(
          sessionId,
          callId,
          () => ({
            callId,
            parentCallId,
            agent: agentName,
            model,
            prompt: contentText,
            depth,
            status: "error",
            content: contentText || "",
            entries: [
              {
                id: crypto.randomUUID(),
                type: "error",
                content: errText,
                createdAt: now,
              },
            ],
            startedAt: now,
            finishedAt: now,
            error: errText,
          }),
          (thread) => ({
            ...thread,
            status: "error",
            finishedAt: thread.finishedAt || now,
            error: errText,
            entries: [
              ...thread.entries,
              {
                id: crypto.randomUUID(),
                type: "error",
                content: errText,
                createdAt: now,
              },
            ],
          }),
        );
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
            id: crypto.randomUUID(),
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
      projectId?: string;
      agentName?: string;
      agentModel?: string;
    } = {},
  ) {
    const sessionId = ensureSession();
    if (isSessionStreaming(sessionId)) return;
    const messages = messagesBySession.value[sessionId] || [];
    const lastUser = [...messages].reverse().find((m) => m.role === "user");
    const lastAssistantIdx = [...messages]
      .reverse()
      .findIndex((m) => m.role === "assistant");
    if (!lastUser || lastAssistantIdx === -1) return;
    // Remove last assistant message
    const targetIndex = messages.findLastIndex(
      (m: ChatMessage) => m.role === "assistant",
    );
    const next = [...messages];
    if (targetIndex !== -1) next.splice(targetIndex, 1);
    setMessages(sessionId, next);
    await sendPrompt(lastUser.content, [], undefined, {
      echoUser: false,
      specialist: options.specialist,
      projectId: options.projectId,
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
    renameSession,
    sendPrompt,
    stopStreaming,
    regenerateAssistant,
    clearSummaryEvent,
    clearThoughtSummaries,
  };
});
