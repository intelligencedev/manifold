<template>
  <div class="flex h-full min-h-0 flex-1 overflow-hidden">
    <section class="grid flex-1 min-h-0 overflow-hidden gap-6 lg:grid-cols-[280px_1fr] xl:grid-cols-[300px_1fr_260px]">
    <!-- Sessions sidebar -->
    <aside class="hidden min-h-0 lg:flex flex-col gap-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-4">
      <header class="flex items-center justify-between">
        <h2 class="text-sm font-semibold text-slate-200">Conversations</h2>
        <button
          type="button"
          class="rounded-lg border border-slate-700 px-2 py-1 text-xs font-semibold text-slate-200 transition hover:border-emerald-500 hover:text-emerald-300"
          @click="createSession"
        >
          New
        </button>
      </header>
      <div class="flex-1 space-y-1 overflow-y-auto pr-1 text-sm">
        <div
          v-for="session in sessions"
          :key="session.id"
          class="group rounded-lg border border-transparent px-3 py-2 transition"
          :class="session.id === activeSessionId ? 'border-emerald-500/70 bg-slate-800/60' : 'hover:border-slate-700 hover:bg-slate-800/30'"
          @click="selectSession(session.id)"
        >
          <div class="flex items-center justify-between gap-2">
            <template v-if="renamingSessionId === session.id">
              <input
                ref="renameInput"
                v-model="renamingName"
                type="text"
                class="w-full rounded bg-slate-900 px-2 py-1 text-xs text-slate-100 outline-none ring-2 ring-emerald-500/80"
                @keyup.enter.prevent="commitRename(session.id)"
                @keyup.esc.prevent="cancelRename"
                @blur="commitRename(session.id)"
              />
            </template>
            <template v-else>
              <p class="truncate font-medium text-slate-200">{{ session.name }}</p>
              <button
                type="button"
                class="rounded px-2 py-1 text-[10px] text-slate-500 opacity-0 transition group-hover:opacity-100 hover:text-emerald-300"
                @click.stop="startRename(session)"
              >
                Rename
              </button>
            </template>
          </div>
          <p class="mt-1 truncate text-xs text-slate-500">
            {{ session.lastMessagePreview || 'No messages yet' }}
          </p>
          <div class="mt-2 flex items-center justify-between text-[10px] text-slate-600">
            <span>{{ formatTimestamp(session.updatedAt) }}</span>
            <button
              type="button"
              class="rounded px-1 text-[10px] text-rose-400 opacity-0 transition group-hover:opacity-100 hover:text-rose-300"
              @click.stop="deleteSession(session.id)"
            >
              Delete
            </button>
          </div>
        </div>
      </div>
    </aside>

    <!-- Chat pane -->
  <section class="relative flex min-h-0 flex-col overflow-hidden rounded-2xl border border-slate-800 bg-slate-900/60">
      <header class="flex flex-wrap items-center justify-between gap-3 border-b border-slate-800 px-4 py-3">
        <div>
          <h1 class="text-base font-semibold text-slate-200">
            {{ activeSession?.name || 'Conversation' }}
          </h1>
          <p class="text-xs text-slate-500">{{ activeSession?.model || 'Model: agent default' }}</p>
        </div>
        <div class="flex items-center gap-2 text-xs text-slate-400">
          <span v-if="isStreaming" class="flex items-center gap-1 text-emerald-300">
            <span class="h-2 w-2 animate-pulse rounded-full bg-emerald-300"></span>
            Streaming response…
          </span>
          <button
            type="button"
            class="rounded border border-slate-700 px-3 py-1 font-medium text-slate-200 transition hover:border-emerald-500 hover:text-emerald-300"
            @click="goToDashboard"
          >
            Dashboard
          </button>
        </div>
      </header>

      <div
        ref="messagesPane"
        class="flex-1 min-h-0 space-y-5 overflow-y-auto overflow-x-hidden overscroll-contain px-4 py-6"
        @scroll="handleMessagesScroll"
        @click="handleMarkdownClick"
      >
        <div
          v-if="!chatMessages.length"
          class="flex h-full flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-slate-800 bg-slate-900/60 p-8 text-center text-sm text-slate-500"
        >
          <p class="text-base font-medium text-slate-200">Start a new conversation</p>
          <p>Ask the agent anything about your operations, tooling, or recent runs.</p>
        </div>

        <article
          v-for="message in chatMessages"
          :key="message.id"
          class="relative rounded-xl border border-slate-800 bg-slate-900/80 p-4"
        >
          <header class="flex flex-wrap items-center gap-2">
            <span class="rounded-full bg-slate-800 px-2 py-1 text-xs font-semibold text-slate-300">
              {{ labelForRole(message.role) }}
            </span>
            <span class="text-xs text-slate-500">{{ formatTimestamp(message.createdAt) }}</span>
            <span v-if="message.streaming" class="flex items-center gap-1 text-xs text-emerald-300">
              <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-300"></span>
              Streaming
            </span>
            <span v-if="message.error" class="rounded bg-rose-500/20 px-2 py-0.5 text-[11px] text-rose-300">
              {{ message.error }}
            </span>
          </header>

          <div class="mt-3 space-y-3 break-words text-sm leading-relaxed text-slate-100">
            <p v-if="message.title" class="font-semibold text-slate-200">{{ message.title }}</p>
            <pre v-if="message.toolArgs" class="whitespace-pre-wrap rounded-md bg-slate-950/60 p-3 text-xs text-slate-300">{{ message.toolArgs }}</pre>
            <div
              v-if="message.content"
              class="chat-markdown"
              v-html="renderMarkdown(message.content)"
            ></div>
            <audio
              v-if="message.audioUrl"
              :src="message.audioUrl"
              controls
              class="w-full"
            ></audio>
          </div>

          <footer class="mt-3 flex flex-wrap items-center gap-2 text-xs text-slate-500">
            <button
              v-if="message.role === 'assistant' && message.content"
              type="button"
              class="rounded border border-slate-700 px-2 py-1 transition hover:border-emerald-500 hover:text-emerald-300"
              @click="copyMessage(message)"
            >
              <span v-if="copiedMessageId === message.id">Copied</span>
              <span v-else>Copy</span>
            </button>
            <button
              v-if="canRegenerate && message.id === lastAssistantId"
              type="button"
              class="rounded border border-slate-700 px-2 py-1 transition hover:border-emerald-500 hover:text-emerald-300"
              @click="regenerateAssistant"
            >
              Regenerate
            </button>
          </footer>
        </article>
      </div>

      <button
        type="button"
        class="absolute bottom-28 right-6 z-10 flex items-center gap-2 rounded-full bg-slate-800/90 px-3 py-2 text-xs font-semibold text-slate-200 shadow-lg ring-1 ring-slate-700/50 transition-all duration-200"
        :class="showScrollToBottom ? 'pointer-events-auto opacity-100 translate-y-0' : 'pointer-events-none opacity-0 translate-y-2'"
        @click="handleScrollToLatest"
      >
        <span class="h-2 w-2 rounded-full bg-emerald-400"></span>
        <span>Scroll to latest</span>
      </button>

      <footer class="border-t border-slate-800 p-4">
        <form class="space-y-3" @submit.prevent="sendCurrentPrompt">
          <div class="rounded-xl border border-slate-800 bg-slate-950/70 p-3">
            <textarea
              ref="composer"
              v-model="draft"
              rows="1"
              class="w-full resize-none bg-transparent text-sm text-slate-100 outline-none"
              placeholder="Message the agent..."
              @keydown="handleComposerKeydown"
              @input="autoSizeComposer"
            ></textarea>
          </div>
          <div class="flex flex-wrap items-center justify-between gap-3 text-xs text-slate-500">
            <p>Shift+Enter for newline</p>
            <div class="flex items-center gap-2">
              <button
                v-if="isStreaming"
                type="button"
                class="rounded-lg border border-rose-500/70 px-3 py-2 font-semibold text-rose-300 transition hover:border-rose-400 hover:text-rose-200"
                @click="stopStreaming"
              >
                Stop
              </button>
              <button
                type="submit"
                class="rounded-lg bg-emerald-600 px-4 py-2 font-semibold text-white transition hover:bg-emerald-500 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="!draft.trim() || isStreaming"
              >
                Send
              </button>
            </div>
          </div>
        </form>
      </footer>
    </section>

    <!-- Context sidebar -->
    <aside class="hidden min-h-0 xl:flex flex-col gap-4 rounded-2xl border border-slate-800 bg-slate-900/60 p-4 text-sm text-slate-300">
      <div>
        <h2 class="text-sm font-semibold text-slate-200">Session details</h2>
        <p class="mt-2 text-xs text-slate-500">Session ID: {{ activeSessionId }}</p>
        <p class="text-xs text-slate-500">Messages: {{ activeMessages.length }}</p>
      </div>
        <div class="flex-1 h-full space-y-2 overflow-y-auto pr-1" @click="handleMarkdownClick">
          <div
            v-if="!toolMessages.length"
            class="rounded border border-dashed border-slate-800 bg-slate-900/40 p-3 text-xs text-slate-500"
          >
            No tool activity yet
          </div>
          <article
            v-for="tool in toolMessages"
            :key="tool.id"
            class="rounded-lg border border-slate-800 bg-slate-900/70 p-3 text-xs"
          >
            <header class="mb-2 flex flex-wrap items-center gap-2">
              <span class="rounded bg-slate-800 px-2 py-0.5 text-[11px] text-slate-300">
                {{ tool.title || 'Tool' }}
              </span>
              <span class="text-[11px] text-slate-500">{{ formatTimestamp(tool.createdAt) }}</span>
              <span v-if="tool.streaming" class="flex items-center gap-1 text-[11px] text-emerald-300">
                <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-emerald-300"></span>
                Running
              </span>
              <span v-if="tool.error" class="rounded bg-rose-500/20 px-2 py-0.5 text-[11px] text-rose-300">
                {{ tool.error }}
              </span>
            </header>
            <pre v-if="tool.toolArgs" class="whitespace-pre-wrap rounded bg-slate-950/60 p-2 text-[11px] text-slate-400">{{ tool.toolArgs }}</pre>
            <div v-if="tool.content" class="chat-markdown mt-2" v-html="renderMarkdown(tool.content)"></div>
            <audio v-if="tool.audioUrl" :src="tool.audioUrl" controls class="mt-2 w-full"></audio>
          </article>
        </div>
    </aside>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { streamAgentRun, type ChatStreamEvent } from '@/api/chat'
import type { ChatMessage, ChatSessionMeta, ChatRole } from '@/types/chat'
import { renderMarkdown } from '@/utils/markdown'
import 'highlight.js/styles/github-dark-dimmed.css'

const router = useRouter()
const isBrowser = typeof window !== 'undefined'
const SCROLL_LOCK_THRESHOLD = 80

const sessionsStorageKey = 'agentd.chat.sessions.v1'
const messagesStorageKey = 'agentd.chat.messages.v1'

function parseSessions(): ChatSessionMeta[] {
  if (!isBrowser) return []
  const raw = window.localStorage.getItem(sessionsStorageKey)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      return parsed.filter((item) => typeof item?.id === 'string' && typeof item?.name === 'string')
    }
  } catch (error) {
    console.warn('Failed to parse stored sessions', error)
  }
  return []
}

function parseMessages(): Record<string, ChatMessage[]> {
  if (!isBrowser) return {}
  const raw = window.localStorage.getItem(messagesStorageKey)
  if (!raw) return {}
  try {
    const parsed = JSON.parse(raw)
    if (parsed && typeof parsed === 'object') {
      return parsed as Record<string, ChatMessage[]>
    }
  } catch (error) {
    console.warn('Failed to parse stored messages', error)
  }
  return {}
}

function makeSession(name?: string): ChatSessionMeta {
  const now = new Date().toISOString()
  return {
    id: crypto.randomUUID(),
    name: name || 'New Chat',
    createdAt: now,
    updatedAt: now
  }
}

const sessions = ref<ChatSessionMeta[]>(parseSessions())
const messagesBySession = ref<Record<string, ChatMessage[]>>(parseMessages())

if (!sessions.value.length) {
  const first = makeSession('New Chat')
  sessions.value = [first]
  messagesBySession.value = { [first.id]: [] }
}

const activeSessionId = ref<string>(sessions.value[0]?.id || '')
const draft = ref('')
const isStreaming = ref(false)
const abortController = ref<AbortController | null>(null)
const streamingAssistantId = ref<string | null>(null)
const toolMessageIndex = new Map<string, string>()
const renamingSessionId = ref<string | null>(null)
const renamingName = ref('')
const renameInput = ref<HTMLInputElement | null>(null)
const messagesPane = ref<HTMLDivElement | null>(null)
const composer = ref<HTMLTextAreaElement | null>(null)
const copiedMessageId = ref<string | null>(null)
const autoScrollEnabled = ref(true)
function handleMarkdownClick(e: MouseEvent) {
  const target = e.target as HTMLElement
  const btn = target.closest('[data-copy]') as HTMLElement | null
  if (!btn) return
  const wrapper = btn.closest('.md-codeblock') as HTMLElement | null
  if (!wrapper) return
  const codeEl = wrapper.querySelector('pre > code') as HTMLElement | null
  if (!codeEl) return
  const text = codeEl.innerText || codeEl.textContent || ''
  if (!text) return
  navigator.clipboard?.writeText(text).then(() => {
    btn.classList.add('copied')
    btn.textContent = 'Copied'
    setTimeout(() => {
      btn.classList.remove('copied')
      btn.textContent = 'Copy'
    }, 1200)
  }).catch(() => {})
}

const activeSession = computed(() => sessions.value.find((session) => session.id === activeSessionId.value) || null)
const activeMessages = computed(() => messagesBySession.value[activeSessionId.value] || [])
const chatMessages = computed(() => activeMessages.value.filter((m) => m.role !== 'tool'))
const toolMessages = computed(() => activeMessages.value.filter((m) => m.role === 'tool'))
const showScrollToBottom = computed(() => !autoScrollEnabled.value && chatMessages.value.length > 0)
const lastUser = computed(() => findLast(activeMessages.value, (msg) => msg.role === 'user'))
const lastAssistant = computed(() => findLast(activeMessages.value, (msg) => msg.role === 'assistant'))
const lastAssistantId = computed(() => lastAssistant.value?.id || '')
const canRegenerate = computed(() => Boolean(!isStreaming.value && lastUser.value && lastAssistant.value))

watch(
  sessions,
  (value) => {
    if (!isBrowser) return
    window.localStorage.setItem(sessionsStorageKey, JSON.stringify(value))
  },
  { deep: true }
)

watch(
  messagesBySession,
  (value) => {
    if (!isBrowser) return
    window.localStorage.setItem(messagesStorageKey, JSON.stringify(value))
  },
  { deep: true }
)

watch(
  () => activeMessages.value.map((msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`),
  () => scrollMessagesToBottom(),
  { flush: 'post' }
)

watch(renamingSessionId, (value) => {
  if (!value) return
  nextTick(() => {
    renameInput.value?.focus()
    renameInput.value?.select()
  })
})

onMounted(() => {
  nextTick(() => {
    autoSizeComposer()
    scrollMessagesToBottom({ force: true, behavior: 'auto' })
  })
})

watch(draft, () => autoSizeComposer())

function setRenameInput(el: HTMLInputElement | null) {
  renameInput.value = el
}

function ensureSession(): string {
  if (!activeSessionId.value) {
    const session = makeSession('New Chat')
    sessions.value = [session, ...sessions.value]
    messagesBySession.value = { ...messagesBySession.value, [session.id]: [] }
    activeSessionId.value = session.id
    autoScrollEnabled.value = true
  } else if (!(activeSessionId.value in messagesBySession.value)) {
    messagesBySession.value = { ...messagesBySession.value, [activeSessionId.value]: [] }
  }
  return activeSessionId.value
}

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

function updateMessage(sessionId: string, messageId: string, updater: (message: ChatMessage) => ChatMessage) {
  const existing = messagesBySession.value[sessionId] || []
  let updated = false
  const next = existing.map((message) => {
    if (message.id === messageId) {
      updated = true
      return updater(message)
    }
    return message
  })
  if (updated) {
    setMessages(sessionId, next)
  }
}

function touchSession(sessionId: string, preview?: string) {
  const index = sessions.value.findIndex((session) => session.id === sessionId)
  if (index === -1) return
  const session = sessions.value[index]
  const updated: ChatSessionMeta = {
    ...session,
    updatedAt: new Date().toISOString(),
    lastMessagePreview: preview ?? session.lastMessagePreview
  }
  const clone = [...sessions.value]
  clone.splice(index, 1, updated)
  sessions.value = clone
}

function selectSession(sessionId: string) {
  activeSessionId.value = sessionId
  autoScrollEnabled.value = true
  nextTick(() => scrollMessagesToBottom({ force: true, behavior: 'auto' }))
}

function createSession() {
  const session = makeSession('New Chat')
  sessions.value = [session, ...sessions.value]
  messagesBySession.value = { ...messagesBySession.value, [session.id]: [] }
  activeSessionId.value = session.id
  renamingSessionId.value = session.id
  renamingName.value = session.name
  autoScrollEnabled.value = true
  nextTick(() => scrollMessagesToBottom({ force: true, behavior: 'auto' }))
}

function deleteSession(sessionId: string) {
  const nextSessions = sessions.value.filter((session) => session.id !== sessionId)
  const { [sessionId]: _removed, ...rest } = messagesBySession.value
  if (!nextSessions.length) {
    const fresh = makeSession('New Chat')
    sessions.value = [fresh]
    messagesBySession.value = { [fresh.id]: [] }
    activeSessionId.value = fresh.id
    autoScrollEnabled.value = true
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: 'auto' }))
    return
  }
  sessions.value = nextSessions
  messagesBySession.value = rest
  if (activeSessionId.value === sessionId) {
    activeSessionId.value = nextSessions[0]?.id || ''
    autoScrollEnabled.value = true
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: 'auto' }))
  }
}

function startRename(session: ChatSessionMeta) {
  renamingSessionId.value = session.id
  renamingName.value = session.name
}

function commitRename(sessionId: string) {
  if (renamingSessionId.value !== sessionId) return
  const name = renamingName.value.trim()
  if (!name) {
    cancelRename()
    return
  }
  const index = sessions.value.findIndex((session) => session.id === sessionId)
  if (index === -1) return
  const clone = [...sessions.value]
  clone.splice(index, 1, { ...clone[index], name })
  sessions.value = clone
  cancelRename()
}

function cancelRename() {
  renamingSessionId.value = null
  renamingName.value = ''
}

async function sendCurrentPrompt() {
  await sendPrompt(draft.value)
}

async function sendPrompt(text: string, options: { echoUser?: boolean } = {}) {
  const content = text.trim()
  if (!content || isStreaming.value) return

  const sessionId = ensureSession()
  const now = new Date().toISOString()
  autoScrollEnabled.value = true

  if (options.echoUser !== false) {
    appendMessage(sessionId, {
      id: crypto.randomUUID(),
      role: 'user',
      content,
      createdAt: now
    })
  }

  const assistantId = crypto.randomUUID()
  appendMessage(sessionId, {
    id: assistantId,
    role: 'assistant',
    content: '',
    createdAt: now,
    streaming: true
  })

  streamingAssistantId.value = assistantId
  isStreaming.value = true
  draft.value = options.echoUser === false ? draft.value : ''
  toolMessageIndex.clear()
  abortController.value = new AbortController()

  try {
    await streamAgentRun({
      prompt: content,
      sessionId,
      signal: abortController.value.signal,
      onEvent: (event) => handleStreamEvent(event, sessionId, assistantId)
    })
  } catch (error) {
    const assistantUpdater = (message: ChatMessage) => ({
      ...message,
      streaming: false,
      error: error instanceof DOMException && error.name === 'AbortError'
        ? 'Generation stopped'
        : error instanceof Error
          ? error.message
          : 'Unexpected error'
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
        updateMessage(sessionId, assistantId, (message) => ({
          ...message,
          content: message.content + event.data
        }))
      }
      break
    }
    case 'final': {
      const text = typeof event.data === 'string' ? event.data : ''
      updateMessage(sessionId, assistantId, (message) => ({
        ...message,
        content: text || message.content,
        streaming: false
      }))
      if (text) {
        touchSession(sessionId, snippet(text))
      }
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
          role: 'tool',
          title: event.title || 'Tool call',
          content: '',
          toolArgs: typeof event.args === 'string' ? event.args : undefined,
          createdAt: now,
          streaming: true
        },
        false
      )
      break
    }
    case 'tool_result': {
      const now = new Date().toISOString()
      const result = typeof event.data === 'string' ? event.data : ''
      const key = typeof event.tool_id === 'string' ? event.tool_id : null
      if (key && toolMessageIndex.has(key)) {
        const messageId = toolMessageIndex.get(key) as string
        updateMessage(sessionId, messageId, (message) => ({
          ...message,
          content: result,
          streaming: false
        }))
        toolMessageIndex.delete(key)
      } else {
        const pending = findLastIndex(messagesBySession.value[sessionId] || [], (msg) => msg.role === 'tool' && !!msg.streaming)
        if (pending !== -1) {
          const messageId = (messagesBySession.value[sessionId] || [])[pending].id
          updateMessage(sessionId, messageId, (message) => ({
            ...message,
            title: message.title || event.title || 'Tool result',
            content: result,
            streaming: false
          }))
        } else {
          appendMessage(
            sessionId,
            {
              id: crypto.randomUUID(),
              role: 'tool',
              title: event.title || 'Tool result',
              content: result,
              createdAt: now
            },
            false
          )
        }
      }
      break
    }
    case 'tts_chunk':
      // Ignore incremental binary metadata for now.
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
            audioFilePath: typeof event.file_path === 'string' ? event.file_path : undefined
          },
          false
        )
      }
      break
    }
    case 'error': {
      const message = typeof event.data === 'string' ? event.data : 'Agent error'
      updateMessage(sessionId, assistantId, (existing) => ({
        ...existing,
        streaming: false,
        error: message
      }))
      break
    }
    default:
      break
  }
}

function stopStreaming() {
  abortController.value?.abort()
}

async function regenerateAssistant() {
  if (!canRegenerate.value || !lastUser.value) return
  const sessionId = ensureSession()
  const messages = messagesBySession.value[sessionId] || []
  const targetIndex = findLastIndex(messages, (message) => message.role === 'assistant')
  if (targetIndex !== -1) {
    const next = [...messages]
    next.splice(targetIndex, 1)
    setMessages(sessionId, next)
  }
  await sendPrompt(lastUser.value.content, { echoUser: false })
}

function copyMessage(message: ChatMessage) {
  if (!navigator.clipboard || !message.content) return
  navigator.clipboard
    .writeText(message.content)
    .then(() => {
      copiedMessageId.value = message.id
      setTimeout(() => {
        if (copiedMessageId.value === message.id) {
          copiedMessageId.value = null
        }
      }, 1500)
    })
    .catch(() => {
      copiedMessageId.value = null
    })
}

function labelForRole(role: ChatRole) {
  switch (role) {
    case 'user':
      return 'You'
    case 'assistant':
      return 'Agent'
    case 'tool':
      return 'Tool'
    case 'system':
      return 'System'
    default:
      return 'Status'
  }
}

const timeFormatter = new Intl.DateTimeFormat(undefined, {
  hour: 'numeric',
  minute: '2-digit'
})

function formatTimestamp(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return timeFormatter.format(date)
}

function snippet(content: string) {
  if (!content) return ''
  const trimmed = content.replace(/\s+/g, ' ').trim()
  return trimmed.length > 80 ? `${trimmed.slice(0, 77)}…` : trimmed
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.key === 'Enter' && !event.shiftKey && !event.isComposing) {
    event.preventDefault()
    sendCurrentPrompt()
  }
}

function autoSizeComposer() {
  const el = composer.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = `${Math.min(el.scrollHeight, 160)}px`
}

type ScrollToBottomOptions = {
  force?: boolean
  behavior?: ScrollBehavior
}

function scrollMessagesToBottom(options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    const container = messagesPane.value
    if (!container) return

    if (!options.force && !autoScrollEnabled.value) {
      return
    }

    const behavior = options.behavior ?? (options.force ? 'smooth' : 'auto')
    const target = Math.max(container.scrollHeight - container.clientHeight, 0)
    container.scrollTo({ top: target, behavior })

    if (options.force) {
      autoScrollEnabled.value = true
    }
  })
}

function isNearBottom(container: HTMLElement) {
  const distance = container.scrollHeight - (container.scrollTop + container.clientHeight)
  return distance <= SCROLL_LOCK_THRESHOLD
}

function handleMessagesScroll(event: Event) {
  const container = event.target as HTMLElement | null
  if (!container) return
  autoScrollEnabled.value = isNearBottom(container)
}

function handleScrollToLatest() {
  scrollMessagesToBottom({ force: true, behavior: 'smooth' })
}

function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return items[i]
    }
  }
  return null
}

function findLastIndex<T>(items: T[], predicate: (item: T) => boolean): number {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return i
    }
  }
  return -1
}

function goToDashboard() {
  router.push({ name: 'overview' })
}
</script>

<style scoped>
.chat-markdown {
  white-space: normal;
  overflow-wrap: anywhere; /* allow breaking long tokens */
  word-break: break-word;  /* legacy support */
}

.chat-markdown p {
  margin: 0 0 0.75rem;
}

.chat-markdown p:last-child {
  margin-bottom: 0;
}

.chat-markdown ul,
.chat-markdown ol {
  margin: 0 0 0.75rem 1.25rem;
  padding: 0;
}

.chat-markdown li {
  margin-bottom: 0.25rem;
}

.chat-markdown pre {
  margin: 0 0 0.75rem;
}

.chat-markdown code {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
  font-size: 0.875rem;
}

.chat-markdown :deep(pre.hljs) {
  border-radius: 0.5rem;
  overflow-x: auto;
  padding: 0.75rem;
  background-color: rgba(15, 23, 42, 0.85);
  max-width: 100%;
}

.chat-markdown :deep(code.hljs) {
  display: block;
  white-space: pre;
  max-width: 100%;
}
/* Code block wrapper and toolbar */
.chat-markdown :deep(.md-codeblock) {
  position: relative;
}

.chat-markdown :deep(.md-codeblock-toolbar) {
  position: sticky;
  top: 0;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.35rem 0.5rem;
  background: linear-gradient(rgba(15, 23, 42, 0.95), rgba(15, 23, 42, 0.6));
  border-top-left-radius: 0.5rem;
  border-top-right-radius: 0.5rem;
  z-index: 1;
}

.chat-markdown :deep(.md-codeblock .hljs) {
  margin-top: 0; /* snug under toolbar */
}

.chat-markdown :deep(.md-lang) {
  font-size: 0.75rem;
  color: rgb(148 163 184); /* slate-400 */
}

.chat-markdown :deep(.md-copy-btn) {
  font-size: 0.75rem;
  line-height: 1;
  color: rgb(226 232 240); /* slate-200 */
  background: rgb(30 41 59 / 0.8); /* slate-800/80 */
  border: 1px solid rgb(51 65 85); /* slate-700 */
  padding: 0.25rem 0.5rem;
  border-radius: 0.375rem;
  cursor: pointer;
}

.chat-markdown :deep(.md-copy-btn:hover) {
  color: rgb(16 185 129); /* emerald-500 */
  border-color: rgb(16 185 129);
}

.chat-markdown :deep(.md-copy-btn.copied) {
  color: rgb(74 222 128); /* emerald-400 */
  border-color: rgb(74 222 128);
}

/* Ensure images and tables don't overflow horizontally */
.chat-markdown :deep(img),
.chat-markdown :deep(table) {
  max-width: 100%;
  width: 100%;
}

/* Markdown tables: wrap cell content when needed */
.chat-markdown :deep(table) {
  display: block;
  overflow-x: auto; /* allow scroll within table if necessary */
}
</style>
