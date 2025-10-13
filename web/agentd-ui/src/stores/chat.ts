import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useQueryClient } from '@tanstack/vue-query'
import type { ChatAttachment, ChatMessage, ChatRole, ChatSessionMeta } from '@/types/chat'
import {
  createChatSession as apiCreateChatSession,
  deleteChatSession as apiDeleteChatSession,
  fetchChatMessages,
  listChatSessions,
  renameChatSession as apiRenameChatSession,
  streamAgentRun,
  streamAgentVisionRun,
  type ChatStreamEvent,
} from '@/api/chat'

type FilesByAttachment = Map<string, File>

export const useChatStore = defineStore('chat', () => {
  const queryClient = useQueryClient()

  const sessions = ref<ChatSessionMeta[]>([])
  const messagesBySession = ref<Record<string, ChatMessage[]>>({})
  const sessionsLoading = ref(false)
  const sessionsError = ref<string | null>(null)
  const fetchedMessageSessions = new Set<string>()

  const activeSessionId = ref<string>('')
  const isStreaming = ref(false)
  const abortController = ref<AbortController | null>(null)
  const streamingAssistantId = ref<string | null>(null)
  const toolMessageIndex = new Map<string, string>()

  const activeSession = computed(
    () => sessions.value.find((s) => s.id === activeSessionId.value) || null,
  )
  const activeMessages = computed(
    () => messagesBySession.value[activeSessionId.value] || [],
  )
  const chatMessages = computed(() =>
    activeMessages.value.filter((m) => m.role !== 'tool'),
  )
  const toolMessages = computed(() =>
    activeMessages.value.filter((m) => m.role === 'tool'),
  )

  function setMessages(sessionId: string, messages: ChatMessage[]) {
    messagesBySession.value = { ...messagesBySession.value, [sessionId]: messages }
  }

  function appendMessage(sessionId: string, message: ChatMessage, updatePreview = true) {
    const existing = messagesBySession.value[sessionId] || []
    setMessages(sessionId, [...existing, message])
    if (updatePreview && (message.role === 'assistant' || message.role === 'user')) {
      touchSession(sessionId, snippet(message.content))
    }
  }

  function updateMessage(
    sessionId: string,
    messageId: string,
    updater: (m: ChatMessage) => ChatMessage,
  ) {
    const existing = messagesBySession.value[sessionId] || []
    let updated = false
    const next = existing.map((m) => {
      if (m.id === messageId) {
        updated = true
        return updater(m)
      }
      return m
    })
    if (updated) setMessages(sessionId, next)
  }

  function touchSession(sessionId: string, preview?: string) {
    const idx = sessions.value.findIndex((s) => s.id === sessionId)
    if (idx === -1) return
    const session = sessions.value[idx]
    const updated: ChatSessionMeta = {
      ...session,
      updatedAt: new Date().toISOString(),
      lastMessagePreview: preview ?? session.lastMessagePreview,
    }
    const clone = [...sessions.value]
    clone.splice(idx, 1, updated)
    sessions.value = clone
  }

  function ensureSession(): string {
    if (!activeSessionId.value) throw new Error('No active conversation')
    if (!(activeSessionId.value in messagesBySession.value)) {
      messagesBySession.value = { ...messagesBySession.value, [activeSessionId.value]: [] }
    }
    return activeSessionId.value
  }

  function snippet(content: string) {
    if (!content) return ''
    const trimmed = content.replace(/\s+/g, ' ').trim()
    return trimmed.length > 80 ? `${trimmed.slice(0, 77)}…` : trimmed
  }

  function httpStatus(error: unknown): number | null {
    // Best-effort Axios compatibility check
    // @ts-ignore
    const isAxios = !!error && typeof error === 'object' && 'isAxiosError' in (error as any)
    // @ts-ignore
    return isAxios ? (error as any).response?.status ?? null : null
  }

  async function init() {
    if (sessions.value.length) return
    await refreshSessionsFromServer(true)
  }

  async function refreshSessionsFromServer(initial = false) {
    sessionsLoading.value = true
    if (!initial) sessionsError.value = null
    try {
      let remote = await listChatSessions()
      if (!remote) remote = []
      if (initial && remote.length === 0) {
        const created = await apiCreateChatSession('New Chat')
        if (created) remote = [created]
      }
      sessionsError.value = null
      sessions.value = remote
      const nextMessages: Record<string, ChatMessage[]> = {}
      for (const s of remote) nextMessages[s.id] = messagesBySession.value[s.id] || []
      messagesBySession.value = nextMessages
      fetchedMessageSessions.clear()
      if (!remote.length) {
        activeSessionId.value = ''
        return
      }
      if (!remote.some((s) => s.id === activeSessionId.value)) {
        activeSessionId.value = remote[0].id
      }
      if (activeSessionId.value) await loadMessagesFromServer(activeSessionId.value, { force: true })
    } catch (error) {
      const status = httpStatus(error)
      if (status === 401) sessionsError.value = 'Authentication required.'
      else if (status === 403) sessionsError.value = 'Access denied. You do not have permission to view conversations.'
      else sessionsError.value = 'Failed to load conversations.'
      console.error('Failed to load chat sessions', error)
    } finally {
      sessionsLoading.value = false
    }
  }

  async function loadMessagesFromServer(sessionId: string, options: { force?: boolean } = {}) {
    if (!sessionId) return
    if (!options.force && fetchedMessageSessions.has(sessionId)) return
    try {
      const data = (await fetchChatMessages(sessionId)) ?? []
      fetchedMessageSessions.add(sessionId)
      messagesBySession.value = { ...messagesBySession.value, [sessionId]: data }
    } catch (error) {
      const status = httpStatus(error)
      if (status === 403) sessionsError.value = 'Access denied for this conversation.'
      else if (status === 404) await refreshSessionsFromServer()
      console.error('Failed to load chat messages', error)
    }
  }

  function selectSession(sessionId: string) {
    activeSessionId.value = sessionId
    void loadMessagesFromServer(sessionId)
  }

  async function createSession(name = 'New Chat') {
    const session = await apiCreateChatSession(name)
    if (!session) return
    sessionsError.value = null
    sessions.value = [session, ...sessions.value]
    messagesBySession.value = { ...messagesBySession.value, [session.id]: [] }
    fetchedMessageSessions.delete(session.id)
    activeSessionId.value = session.id
    await loadMessagesFromServer(session.id, { force: true })
  }

  async function deleteSession(sessionId: string) {
    await apiDeleteChatSession(sessionId)
    sessionsError.value = null
    const nextSessions = sessions.value.filter((s) => s.id !== sessionId)
    const { [sessionId]: _removed, ...rest } = messagesBySession.value
    messagesBySession.value = rest
    fetchedMessageSessions.delete(sessionId)
    if (!nextSessions.length) {
      const fresh = await apiCreateChatSession('New Chat')
      sessions.value = [fresh]
      messagesBySession.value = { [fresh.id]: [] }
      fetchedMessageSessions.delete(fresh.id)
      activeSessionId.value = fresh.id
      await loadMessagesFromServer(fresh.id, { force: true })
      return
    }
    sessions.value = nextSessions
    if (activeSessionId.value === sessionId) {
      activeSessionId.value = nextSessions[0]?.id || ''
      if (activeSessionId.value) await loadMessagesFromServer(activeSessionId.value, { force: true })
    }
  }

  async function renameSession(sessionId: string, name: string) {
    const updated = await apiRenameChatSession(sessionId, name)
    sessionsError.value = null
    const idx = sessions.value.findIndex((s) => s.id === sessionId)
    if (idx !== -1) {
      const clone = [...sessions.value]
      clone.splice(idx, 1, { ...clone[idx], name: updated.name })
      sessions.value = clone
    }
  }

  async function sendPrompt(
    text: string,
    attachments: ChatAttachment[] = [],
    filesByAttachment?: FilesByAttachment,
    options: { echoUser?: boolean; specialist?: string; projectId?: string } = {},
  ) {
    const content = (text || '').trim()
    if ((!content && !attachments.length) || isStreaming.value) return
    const sessionId = ensureSession()
    const now = new Date().toISOString()

    if (options.echoUser !== false) {
      const attachmentsCopy = attachments.map((a) => ({ ...a }))
      appendMessage(sessionId, {
        id: crypto.randomUUID(),
        role: 'user',
        content,
        createdAt: now,
        attachments: attachmentsCopy,
      })
    }

    const assistantId = crypto.randomUUID()
    appendMessage(sessionId, {
      id: assistantId,
      role: 'assistant',
      content: '',
      createdAt: now,
      streaming: true,
    })

    streamingAssistantId.value = assistantId
    isStreaming.value = true
    toolMessageIndex.clear()
    abortController.value = new AbortController()

    try {
      // Expand text attachments into the prompt
      let promptToSend = content
      const textAtts = attachments.filter((a) => a.kind === 'text')
      const imgAtts = attachments.filter((a) => a.kind === 'image')
      for (const att of textAtts) {
        const f = filesByAttachment?.get(att.id)
        if (!f) continue
        const textContent = await f.text()
        const header = `\n\n--- Attached Document: ${att.name} (${att.mime || 'text'}) ---\n`
        const footer = `\n--- End Document ---\n`
        promptToSend += header + textContent + footer
      }
      const imageFiles: File[] = []
      for (const att of imgAtts) {
        const f = filesByAttachment?.get(att.id)
        if (f) imageFiles.push(f)
      }

      if (imageFiles.length) {
        await streamAgentVisionRun({
          prompt: promptToSend,
          sessionId,
          files: imageFiles,
          signal: abortController.value!.signal,
          onEvent: (e) => handleStreamEvent(e, sessionId, assistantId),
          specialist: options.specialist,
          projectId: options.projectId,
        })
      } else {
        await streamAgentRun({
          prompt: promptToSend,
          sessionId,
          signal: abortController.value!.signal,
          onEvent: (e) => handleStreamEvent(e, sessionId, assistantId),
          specialist: options.specialist,
          projectId: options.projectId,
        })
      }
    } catch (error: any) {
      const assistantUpdater = (m: ChatMessage) => ({
        ...m,
        streaming: false,
        error:
          error instanceof DOMException && error.name === 'AbortError'
            ? 'Generation stopped'
            : error instanceof Error
              ? error.message
              : 'Unexpected error',
      })
      updateMessage(sessionId, assistantId, assistantUpdater)
    } finally {
      isStreaming.value = false
      streamingAssistantId.value = null
      abortController.value = null
    }
  }

  function handleStreamEvent(event: ChatStreamEvent, sessionId: string, assistantId: string) {
    switch (event.type) {
      case 'delta': {
        if (typeof event.data === 'string' && event.data) {
          updateMessage(sessionId, assistantId, (m) => ({ ...m, content: m.content + event.data }))
        }
        break
      }
      case 'final': {
        const text = typeof event.data === 'string' ? event.data : ''
        updateMessage(sessionId, assistantId, (m) => ({ ...m, content: text || m.content, streaming: false }))
        if (text) touchSession(sessionId, snippet(text))
        try {
          queryClient.invalidateQueries({ queryKey: ['agent-runs'] })
        } catch {}
        break
      }
      case 'tool_start': {
        const now = new Date().toISOString()
        const key = typeof event.tool_id === 'string' ? event.tool_id : crypto.randomUUID()
        const messageId = crypto.randomUUID()
        toolMessageIndex.set(key, messageId)
        appendMessage(
          sessionId,
          {
            id: messageId,
            role: 'tool' as ChatRole,
            title: event.title || 'Tool call',
            content: '',
            toolArgs: typeof event.args === 'string' ? event.args : undefined,
            createdAt: now,
            streaming: true,
          },
          false,
        )
        break
      }
      case 'tool_result': {
        const now = new Date().toISOString()
        const result = typeof event.data === 'string' ? event.data : ''
        const key = typeof event.tool_id === 'string' ? event.tool_id : null
        if (key && toolMessageIndex.has(key)) {
          const messageId = toolMessageIndex.get(key) as string
          updateMessage(sessionId, messageId, (m) => ({ ...m, content: result, streaming: false }))
          toolMessageIndex.delete(key)
        } else {
          // Fallback: attach to last streaming tool message
          const msgs = messagesBySession.value[sessionId] || []
          const pendingIdx = findLastIndex(msgs, (msg) => msg.role === 'tool' && !!msg.streaming)
          if (pendingIdx !== -1) {
            const messageId = msgs[pendingIdx].id
            updateMessage(sessionId, messageId, (m) => ({
              ...m,
              title: m.title || event.title || 'Tool result',
              content: result,
              streaming: false,
            }))
          } else {
            appendMessage(
              sessionId,
              { id: crypto.randomUUID(), role: 'tool', title: event.title || 'Tool result', content: result, createdAt: now },
              false,
            )
          }
        }
        break
      }
      case 'tts_chunk':
        break
      case 'tts_audio': {
        const now = new Date().toISOString()
        if (typeof event.url === 'string') {
          appendMessage(
            sessionId,
            {
              id: crypto.randomUUID(),
              role: 'tool',
              title: event.title || 'Audio response',
              content: 'The agent produced an audio reply.',
              createdAt: now,
              audioUrl: event.url,
              audioFilePath: typeof event.file_path === 'string' ? event.file_path : undefined,
            },
            false,
          )
        }
        break
      }
      case 'error': {
        const message = typeof event.data === 'string' ? event.data : 'Agent error'
        updateMessage(sessionId, assistantId, (existing) => ({ ...existing, streaming: false, error: message }))
        break
      }
      default:
        break
    }
  }

  function findLastIndex<T>(items: T[], predicate: (t: T) => boolean): number {
    for (let i = items.length - 1; i >= 0; i -= 1) if (predicate(items[i])) return i
    return -1
  }

  function stopStreaming() {
    abortController.value?.abort()
  }

  async function regenerateAssistant(options: { specialist?: string } = {}) {
    if (isStreaming.value) return
    const sessionId = ensureSession()
    const messages = messagesBySession.value[sessionId] || []
    const lastUser = [...messages].reverse().find((m) => m.role === 'user')
    const lastAssistantIdx = [...messages].reverse().findIndex((m) => m.role === 'assistant')
    if (!lastUser || lastAssistantIdx === -1) return
    // Remove last assistant message
    const targetIndex = messages.findLastIndex ? (messages as any).findLastIndex((m: ChatMessage) => m.role === 'assistant') : messages.length - 1 - lastAssistantIdx
    const next = [...messages]
    if (targetIndex !== -1) next.splice(targetIndex, 1)
    setMessages(sessionId, next)
    await sendPrompt(lastUser.content, [], undefined, { echoUser: false, specialist: options.specialist })
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
  }
})

