<template>
  <div class="flex h-full min-h-0 flex-1 overflow-hidden">
    <section
      class="grid flex-1 min-h-0 overflow-hidden gap-6 lg:grid-cols-[280px_1fr] xl:grid-cols-[300px_1fr_260px]"
    >
      <!-- Sessions sidebar -->
      <aside
        class="hidden min-h-0 lg:flex flex-col gap-4 rounded-5 border border-border bg-surface p-4 surface-noise"
      >
        <header class="flex items-center justify-between">
          <h2 class="text-sm font-semibold text-foreground">Conversations</h2>
          <button
            type="button"
            class="rounded-4 border border-border px-2 py-1 text-xs font-semibold text-foreground transition hover:border-accent hover:text-accent"
            @click="createSession()"
          >
            New
          </button>
        </header>
        <p
          v-if="sessionsError"
          class="rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
        >
          {{ sessionsError }}
        </p>
        <div class="flex-1 space-y-1 overflow-y-auto pr-1 text-sm">
          <p
            v-if="sessionsLoading"
            class="px-3 py-2 text-xs text-subtle-foreground"
          >
            Loading conversations…
          </p>
          <p
            v-else-if="!sessions.length"
            class="px-3 py-2 text-xs text-subtle-foreground"
          >
            No conversations yet.
          </p>
          <div
            v-for="session in sessions"
            :key="session.id"
            class="group rounded-lg border border-transparent px-3 py-2 transition"
            :class="
              session.id === activeSessionId
                ? 'border-accent/70 bg-surface-muted/60'
                : 'hover:border-border hover:bg-surface-muted/40'
            "
            @click="selectSession(session.id)"
          >
            <div class="flex items-center justify-between gap-2">
              <template v-if="renamingSessionId === session.id">
                <input
                  ref="renameInput"
                  v-model="renamingName"
                  type="text"
                  class="w-full rounded bg-surface px-2 py-1 text-xs text-foreground outline-none focus:ring-0 focus:border-accent focus-visible:shadow-outline"
                  @keyup.enter.prevent="commitRename(session.id)"
                  @keyup.esc.prevent="cancelRename"
                  @blur="commitRename(session.id)"
                />
              </template>
              <template v-else>
                <p class="truncate font-medium text-foreground">
                  {{ session.name }}
                </p>
                <button
                  type="button"
                  class="rounded px-2 py-1 text-[10px] text-faint-foreground opacity-0 transition group-hover:opacity-100 hover:text-accent"
                  @click.stop="startRename(session)"
                >
                  Rename
                </button>
              </template>
            </div>
            <p class="mt-1 truncate text-xs text-subtle-foreground">
              {{ session.lastMessagePreview || "No messages yet" }}
            </p>
            <div
              class="mt-2 flex items-center justify-between text-[10px] text-faint-foreground"
            >
              <span>{{ formatTimestamp(session.updatedAt) }}</span>
              <button
                type="button"
                class="rounded px-1 text-[10px] text-danger opacity-0 transition group-hover:opacity-100 hover:text-danger/80"
                @click.stop="deleteSession(session.id)"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      </aside>

      <!-- Chat pane -->
      <section
        class="relative flex min-h-0 flex-col overflow-hidden rounded-5 border border-border bg-surface"
      >
        <header
          class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3"
        >
          <div>
            <h1 class="text-base font-semibold text-foreground">
              {{ activeSession?.name || "Conversation" }}
            </h1>
            <p class="text-xs text-subtle-foreground">
              {{ activeSession?.model || "Model: agent default" }}
            </p>
          </div>
          <div class="flex items-center gap-2 text-xs text-subtle-foreground">
            <span
              v-if="isStreaming"
              class="flex items-center gap-1 text-accent"
            >
              <span class="h-2 w-2 animate-pulse rounded-full bg-accent"></span>
              Streaming response…
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="rounded-4 border border-border px-3 py-1 font-medium text-foreground transition hover:border-accent hover:text-accent"
                @click="goToDashboard"
              >
                Dashboard
              </button>
              <label
                class="inline-flex items-center gap-2 text-[11px] text-subtle-foreground"
              >
                <select
                  v-model="renderMode"
                  class="rounded-4 border border-border bg-surface px-2 py-1 text-xs text-foreground outline-none"
                  title="Render mode for assistant responses"
                  aria-label="Render mode"
                >
                  <option value="markdown">Markdown</option>
                  <option value="html">HTML</option>
                </select>
              </label>
            </div>
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
            class="flex h-full flex-col items-center justify-center gap-2 rounded-5 border border-dashed border-border bg-surface p-8 text-center text-sm text-subtle-foreground"
          >
            <p class="text-base font-medium text-foreground">
              Start a new conversation
            </p>
            <p>
              Ask the agent anything about your operations, tooling, or recent
              runs.
            </p>
          </div>

          <article
            v-for="message in chatMessages"
            :key="message.id"
            class="relative max-w-[72ch] rounded-5 border border-border bg-surface shadow-1 p-5"
            :class="[
              message.role === 'assistant' ? 'bg-accent/5' : '',
              message.role === 'user' ? 'ml-auto' : '',
            ]"
          >
            <header class="flex flex-wrap items-center gap-2">
              <span
                class="rounded-full px-2 py-1 text-xs font-semibold"
                :class="
                  message.role === 'assistant'
                    ? 'bg-accent/10 text-accent'
                    : message.role === 'user'
                      ? 'bg-surface-muted text-muted-foreground'
                      : 'bg-surface-muted text-muted-foreground'
                "
              >
                {{ labelForRole(message.role) }}
              </span>
              <span class="text-xs text-faint-foreground">{{
                formatTimestamp(message.createdAt)
              }}</span>
              <span
                v-if="message.streaming"
                class="flex items-center gap-1 text-xs text-accent"
              >
                <span
                  class="h-1.5 w-1.5 animate-pulse rounded-full bg-accent"
                ></span>
                Streaming
              </span>
              <span
                v-if="message.error"
                class="rounded bg-danger px-2 py-0.5 text-[11px] text-danger-foreground font-semibold"
              >
                {{ message.error }}
              </span>
            </header>

            <div
              class="mt-3 space-y-3 break-words text-sm leading-relaxed text-foreground"
            >
              <p v-if="message.title" class="font-semibold text-foreground">
                {{ message.title }}
              </p>
              <pre
                v-if="message.toolArgs"
                class="whitespace-pre-wrap rounded-4 border border-border bg-surface-muted/60 p-3 text-xs text-subtle-foreground"
                >{{ message.toolArgs }}</pre
              >
              <div
                v-if="message.content"
                class="chat-markdown"
                v-html="renderMarkdownOrHtml(message.content)"
              ></div>
              <div v-if="message.attachments?.length" class="space-y-2">
                <div
                  v-if="message.attachments.some((a) => a.kind === 'image')"
                  class="flex gap-2 overflow-x-auto pb-1"
                >
                  <img
                    v-for="img in message.attachments.filter(
                      (a) => a.kind === 'image',
                    )"
                    :key="img.id"
                    :src="img.previewUrl"
                    :alt="img.name"
                    class="h-16 w-16 rounded object-cover border border-border"
                  />
                </div>
                <div
                  v-if="message.attachments.some((a) => a.kind === 'text')"
                  class="flex flex-wrap gap-2"
                >
                  <span
                    v-for="t in message.attachments.filter(
                      (a) => a.kind === 'text',
                    )"
                    :key="t.id"
                    class="inline-flex items-center gap-1 rounded-full border border-border bg-surface px-2 py-1 text-[11px]"
                  >
                    <span class="max-w-[180px] truncate">{{ t.name }}</span>
                  </span>
                </div>
              </div>
              <audio
                v-if="message.audioUrl"
                :src="message.audioUrl"
                controls
                class="w-full"
              ></audio>
            </div>

            <footer
              class="mt-3 flex flex-wrap items-center gap-2 text-xs text-subtle-foreground"
            >
              <button
                v-if="message.role === 'assistant' && message.content"
                type="button"
                class="rounded-4 border border-border px-2 py-1 transition hover:border-accent hover:text-accent"
                @click="copyMessage(message)"
              >
                <span v-if="copiedMessageId === message.id">Copied</span>
                <span v-else>Copy</span>
              </button>
              <button
                v-if="message.role === 'user' && message.content"
                type="button"
                class="rounded-4 border border-border px-2 py-1 transition hover:border-accent hover:text-accent"
                @click="copyMessage(message)"
              >
                <span v-if="copiedMessageId === message.id">Copied</span>
                <span v-else>Copy</span>
              </button>
              <button
                v-if="canRegenerate && message.id === lastAssistantId"
                type="button"
                class="rounded-4 border border-border px-2 py-1 transition hover:border-accent hover:text-accent"
                @click="regenerateAssistant"
              >
                Regenerate
              </button>
            </footer>
          </article>
        </div>

        <button
          type="button"
          class="absolute bottom-28 right-6 z-10 flex items-center gap-2 rounded-full bg-surface px-3 py-2 text-xs font-semibold text-foreground shadow-2 ring-1 ring-border/50 transition-all duration-200"
          :class="
            showScrollToBottom
              ? 'pointer-events-auto opacity-100 translate-y-0'
              : 'pointer-events-none opacity-0 translate-y-2'
          "
          @click="handleScrollToLatest"
        >
          <span class="h-2 w-2 rounded-full bg-accent"></span>
          <span>Scroll to latest</span>
        </button>

        <footer class="border-t border-border p-4">
          <form
            class="space-y-3"
            @submit.prevent="sendCurrentPrompt"
            @dragover.prevent
            @drop.prevent="handleDrop"
          >
            <div
              class="relative rounded-4 border border-border bg-surface-muted/70 p-3 etched-dark"
            >
              <textarea
                ref="composer"
                v-model="draft"
                rows="1"
                class="w-full resize-none bg-transparent text-sm text-foreground outline-none placeholder:text-faint-foreground pr-28"
                placeholder="Message the agent..."
                @keydown="handleComposerKeydown"
                @input="autoSizeComposer"
              ></textarea>

              <!-- Inline actions inside the input box (right aligned) -->
              <div class="absolute inset-y-2 right-2 flex items-end gap-1">
                <!-- Hidden file input to trigger Attach -->
                <input
                  ref="fileInput"
                  type="file"
                  multiple
                  class="hidden"
                  accept="image/png,image/jpeg,text/plain,text/markdown,text/*"
                  @change="handleFileInputChange"
                />

                <!-- Attach -->
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-3 text-foreground/80 hover:text-accent focus-visible:shadow-outline"
                  title="Attach files"
                  aria-label="Attach files"
                  @click="fileInput?.click()"
                >
                  <SolarPaperclip2Bold class="h-5 w-5" />
                </button>

                <!-- Record / Stop Recording -->
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline"
                  :class="[
                    isRecording
                      ? 'text-danger hover:text-danger/90'
                      : 'text-foreground/80 hover:text-accent',
                    isStreaming || !canUseMic
                      ? 'opacity-50 cursor-not-allowed'
                      : '',
                  ]"
                  :disabled="isStreaming || !canUseMic"
                  :title="
                    isRecording ? 'Stop recording' : 'Record voice prompt'
                  "
                  :aria-label="
                    isRecording ? 'Stop recording' : 'Record voice prompt'
                  "
                  @click="isRecording ? stopRecording() : startRecording()"
                >
                  <SolarMicrophone3Bold class="h-5 w-5" />
                </button>

                <!-- Send / Stop Streaming -->
                <button
                  type="button"
                  :class="[
                    'inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline',
                    isStreaming
                      ? 'border border-danger/60 text-foreground/80 hover:text-danger'
                      : 'bg-accent text-accent-foreground hover:bg-accent/90',
                  ]"
                  :title="isStreaming ? 'Stop generating' : 'Send message'"
                  :aria-label="isStreaming ? 'Stop generating' : 'Send message'"
                  @click="isStreaming ? stopStreaming() : sendCurrentPrompt()"
                  :disabled="
                    !isStreaming && !draft.trim() && !pendingAttachments.length
                  "
                >
                  <SolarStopBold v-if="isStreaming" class="h-4 w-4" />
                  <SolarArrowToTopLeftBold v-else class="h-4 w-4" />
                </button>
              </div>
            </div>
            <div v-if="pendingAttachments.length" class="space-y-2">
              <div
                v-if="imageAttachments.length"
                class="flex gap-2 overflow-x-auto pb-1"
              >
                <div
                  v-for="img in imageAttachments"
                  :key="img.id"
                  class="relative shrink-0"
                >
                  <img
                    :src="img.previewUrl"
                    :alt="img.name"
                    class="h-16 w-16 rounded object-cover border border-border"
                  />
                  <button
                    type="button"
                    class="absolute -right-1 -top-1 rounded-full bg-surface px-1 text-[10px] shadow ring-1 ring-border hover:text-danger"
                    @click="removeAttachment(img.id)"
                  >
                    ×
                  </button>
                </div>
              </div>
              <div v-if="textAttachments.length" class="flex flex-wrap gap-2">
                <span
                  v-for="t in textAttachments"
                  :key="t.id"
                  class="inline-flex items-center gap-1 rounded-full border border-border bg-surface px-2 py-1 text-[11px]"
                >
                  <span class="max-w-[180px] truncate">{{ t.name }}</span>
                  <button
                    type="button"
                    class="text-faint-foreground hover:text-danger"
                    @click="removeAttachment(t.id)"
                  >
                    ×
                  </button>
                </span>
              </div>
            </div>
          </form>
        </footer>
      </section>

      <!-- Context sidebar -->
      <aside
        class="hidden min-h-0 xl:flex relative flex-col gap-4 rounded-5 border border-border bg-surface p-4 text-sm text-subtle-foreground surface-noise"
      >
        <div>
          <h2 class="text-sm font-semibold text-foreground">Session details</h2>
          <p class="mt-2 text-xs text-subtle-foreground">
            Session ID: {{ activeSessionId }}
          </p>
          <p class="text-xs text-subtle-foreground">
            Messages: {{ activeMessages.length }}
          </p>
        </div>
        <div
          ref="toolsPane"
          class="flex-1 h-full space-y-2 overflow-y-auto pr-1"
          @scroll="handleToolsScroll"
          @click="handleMarkdownClick"
        >
          <div
            v-if="!toolMessages.length"
            class="rounded-4 border border-dashed border-border bg-surface p-3 text-xs text-subtle-foreground"
          >
            No tool activity yet
          </div>
          <article
            v-for="tool in toolMessages"
            :key="tool.id"
            class="overflow-hidden rounded-4 border border-border bg-surface p-3 text-xs shadow-1"
          >
            <header class="flex items-center justify-between gap-2">
              <div class="min-w-0 flex-1">
                <span
                  class="block max-w-full truncate rounded bg-surface-muted px-2 py-0.5 text-[11px] text-muted-foreground"
                >
                  {{ tool.title || "Tool" }}
                </span>
              </div>
              <div
                class="flex shrink-0 items-center gap-2 text-[11px] text-faint-foreground"
              >
                <span>{{ formatTimestamp(tool.createdAt) }}</span>
                <span
                  v-if="tool.streaming"
                  class="flex items-center gap-1 text-accent"
                >
                  <span
                    class="h-1.5 w-1.5 animate-pulse rounded-full bg-accent"
                  ></span>
                  Running
                </span>
                <span
                  v-if="tool.error"
                  class="rounded bg-danger px-2 py-0.5 text-danger-foreground font-semibold"
                >
                  {{ tool.error }}
                </span>
              </div>
            </header>
            <details
              class="group mt-2 overflow-hidden rounded-4 border border-border bg-surface"
            >
              <summary
                class="flex cursor-pointer items-center justify-between gap-2 px-2 py-1 text-[11px] font-semibold text-subtle-foreground hover:text-foreground focus-visible:outline-none focus-visible:shadow-outline"
              >
                <span>View details</span>
                <span
                  class="text-xs text-faint-foreground transition-transform group-open:rotate-45"
                  >+</span
                >
              </summary>
              <div class="space-y-2 px-2 pb-2 pt-1 text-subtle-foreground">
                <pre
                  v-if="tool.toolArgs"
                  class="max-w-full overflow-x-hidden whitespace-pre-wrap rounded-4 border border-border bg-surface-muted/60 p-2 text-[11px] text-subtle-foreground"
                  >{{ tool.toolArgs }}</pre
                >
                <div
                  v-if="tool.content"
                  class="chat-markdown mt-1 break-words"
                  v-html="renderMarkdownOrHtml(tool.content)"
                ></div>
                <audio
                  v-if="tool.audioUrl"
                  :src="tool.audioUrl"
                  controls
                  class="mt-1 w-full"
                ></audio>
              </div>
            </details>
          </article>
        </div>

        <button
          type="button"
          class="absolute bottom-6 right-6 z-10 flex items-center gap-2 rounded-full bg-surface px-3 py-2 text-xs font-semibold text-foreground shadow-2 ring-1 ring-border/50 transition-all duration-200"
          :class="
            showToolScrollToBottom
              ? 'pointer-events-auto opacity-100 translate-y-0'
              : 'pointer-events-none opacity-0 translate-y-2'
          "
          @click="handleScrollToolsToLatest"
        >
          <span class="h-2 w-2 rounded-full bg-accent"></span>
          <span>Scroll to latest</span>
        </button>
      </aside>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { useQueryClient } from "@tanstack/vue-query";
import { useRouter } from "vue-router";
import axios from "axios";
import {
  streamAgentRun,
  streamAgentVisionRun,
  listChatSessions,
  createChatSession as apiCreateChatSession,
  renameChatSession as apiRenameChatSession,
  deleteChatSession as apiDeleteChatSession,
  fetchChatMessages,
  type ChatStreamEvent,
} from "@/api/chat";
import type {
  ChatAttachment,
  ChatMessage,
  ChatSessionMeta,
  ChatRole,
} from "@/types/chat";
import { renderMarkdown } from "@/utils/markdown";
import "highlight.js/styles/github-dark-dimmed.css";
import SolarPaperclip2Bold from "@/components/icons/SolarPaperclip2Bold.vue";
import SolarMicrophone3Bold from "@/components/icons/SolarMicrophone3Bold.vue";
import SolarArrowToTopLeftBold from "@/components/icons/SolarArrowToTopLeftBold.vue";
import SolarStopBold from "@/components/icons/SolarStopBold.vue";

const router = useRouter();
const queryClient = useQueryClient();
const isBrowser = typeof window !== "undefined";
const SCROLL_LOCK_THRESHOLD = 80;

const sessions = ref<ChatSessionMeta[]>([]);
const messagesBySession = ref<Record<string, ChatMessage[]>>({});
const sessionsLoading = ref(false);
const sessionsError = ref<string | null>(null);
const fetchedMessageSessions = new Set<string>();

const activeSessionId = ref<string>("");
const draft = ref("");
const isStreaming = ref(false);
const abortController = ref<AbortController | null>(null);
const streamingAssistantId = ref<string | null>(null);
const toolMessageIndex = new Map<string, string>();
const renamingSessionId = ref<string | null>(null);
const renamingName = ref("");
const renameInput = ref<HTMLInputElement | null>(null);
const messagesPane = ref<HTMLDivElement | null>(null);
const composer = ref<HTMLTextAreaElement | null>(null);
const copiedMessageId = ref<string | null>(null);
const autoScrollEnabled = ref(true);
// Tools pane scrolling state
const toolsPane = ref<HTMLDivElement | null>(null);
const toolAutoScrollEnabled = ref(true);
// Attachments state for composer
const fileInput = ref<HTMLInputElement | null>(null);
const pendingAttachments = ref<ChatAttachment[]>([]);
const imageAttachments = computed(() =>
  pendingAttachments.value.filter((a) => a.kind === "image"),
);
const textAttachments = computed(() =>
  pendingAttachments.value.filter((a) => a.kind === "text"),
);
const filesByAttachment: Map<string, File> = new Map();
// Render mode for streamed responses: 'markdown' (default) or 'html'
const renderMode = ref<"markdown" | "html">("markdown");

function httpStatus(error: unknown): number | null {
  if (axios.isAxiosError(error)) {
    return error.response?.status ?? null;
  }
  return null;
}

async function refreshSessionsFromServer(initial = false) {
  sessionsLoading.value = true;
  if (!initial) {
    sessionsError.value = null;
  }
  try {
    let remote = await listChatSessions();
    if (!remote) {
      remote = [];
    }
    if (initial && remote.length === 0) {
      const created = await apiCreateChatSession("New Chat");
      if (created) {
        remote = [created];
      }
    }
    sessionsError.value = null;
    sessions.value = remote;
    const nextMessages: Record<string, ChatMessage[]> = {};
    for (const session of remote) {
      nextMessages[session.id] = messagesBySession.value[session.id] || [];
    }
    messagesBySession.value = nextMessages;
    fetchedMessageSessions.clear();
    if (!remote.length) {
      activeSessionId.value = "";
      return;
    }
    if (!remote.some((session) => session.id === activeSessionId.value)) {
      activeSessionId.value = remote[0].id;
    }
    if (activeSessionId.value) {
      await loadMessagesFromServer(activeSessionId.value, { force: true });
    }
  } catch (error) {
    const status = httpStatus(error);
    if (status === 401) {
      sessionsError.value = "Authentication required.";
    } else if (status === 403) {
      sessionsError.value =
        "Access denied. You do not have permission to view conversations.";
    } else {
      sessionsError.value = "Failed to load conversations.";
    }
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
  if (!options.force && fetchedMessageSessions.has(sessionId)) {
    return;
  }
  try {
    const data = (await fetchChatMessages(sessionId)) ?? [];
    fetchedMessageSessions.add(sessionId);
    messagesBySession.value = { ...messagesBySession.value, [sessionId]: data };
  } catch (error) {
    const status = httpStatus(error);
    if (status === 403) {
      sessionsError.value = "Access denied for this conversation.";
    } else if (status === 404) {
      // Session may have been removed; refresh list
      await refreshSessionsFromServer();
    }
    console.error("Failed to load chat messages", error);
  }
}

function validateFile(f: File): "image" | "text" | null {
  const type = (f.type || "").toLowerCase();
  if (type === "image/png" || type === "image/jpeg") return "image";
  if (type.startsWith("text/")) return "text";
  // Fallback to extension check if type missing
  const name = f.name.toLowerCase();
  if (name.endsWith(".png") || name.endsWith(".jpg") || name.endsWith(".jpeg"))
    return "image";
  if (name.endsWith(".txt") || name.endsWith(".md") || name.endsWith(".log"))
    return "text";
  return null;
}

async function addFiles(files: FileList | File[]) {
  const arr = Array.from(files);
  for (const f of arr) {
    const kind = validateFile(f);
    if (!kind) continue;
    if (kind === "image") {
      const id = crypto.randomUUID();
      filesByAttachment.set(id, f);
      const url = await new Promise<string>((resolve) => {
        const reader = new FileReader();
        reader.onload = () => resolve(String(reader.result));
        reader.readAsDataURL(f);
      });
      pendingAttachments.value.push({
        id,
        kind: "image",
        name: f.name,
        size: f.size,
        mime: f.type || undefined,
        previewUrl: url,
      });
    } else {
      // For text, store the File and read on send
      const id = crypto.randomUUID();
      filesByAttachment.set(id, f);
      pendingAttachments.value.push({
        id,
        kind: "text",
        name: f.name,
        size: f.size,
        mime: f.type || undefined,
      });
    }
  }
}

function handleFileInputChange(e: Event) {
  const input = e.target as HTMLInputElement;
  if (!input.files) return;
  void addFiles(input.files);
  // reset so selecting the same file again still triggers change
  input.value = "";
}

function handleDrop(e: DragEvent) {
  const items = e.dataTransfer?.files;
  if (!items) return;
  void addFiles(items);
}

function removeAttachment(id: string) {
  pendingAttachments.value = pendingAttachments.value.filter(
    (a) => a.id !== id,
  );
  filesByAttachment.delete(id);
}
function handleMarkdownClick(e: MouseEvent) {
  const target = e.target as HTMLElement;
  const btn = target.closest("[data-copy]") as HTMLElement | null;
  if (!btn) return;
  const wrapper = btn.closest(".md-codeblock") as HTMLElement | null;
  if (!wrapper) return;
  const codeEl = wrapper.querySelector("pre > code") as HTMLElement | null;
  if (!codeEl) return;
  const text = codeEl.innerText || codeEl.textContent || "";
  if (!text) return;
  navigator.clipboard
    ?.writeText(text)
    .then(() => {
      btn.classList.add("copied");
      btn.textContent = "Copied";
      setTimeout(() => {
        btn.classList.remove("copied");
        btn.textContent = "Copy";
      }, 1200);
    })
    .catch(() => {});
}

function renderMarkdownOrHtml(content: string) {
  if (renderMode.value === "html") {
    // When HTML mode is selected, render content as raw HTML
    return content || "";
  }
  // Default: render as markdown
  return renderMarkdown(content);
}

const activeSession = computed(
  () =>
    sessions.value.find((session) => session.id === activeSessionId.value) ||
    null,
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
const showScrollToBottom = computed(
  () => !autoScrollEnabled.value && chatMessages.value.length > 0,
);
const showToolScrollToBottom = computed(
  () => !toolAutoScrollEnabled.value && toolMessages.value.length > 0,
);
const lastUser = computed(() =>
  findLast(activeMessages.value, (msg) => msg.role === "user"),
);
const lastAssistant = computed(() =>
  findLast(activeMessages.value, (msg) => msg.role === "assistant"),
);
const lastAssistantId = computed(() => lastAssistant.value?.id || "");
const canRegenerate = computed(() =>
  Boolean(!isStreaming.value && lastUser.value && lastAssistant.value),
);

watch(
  () =>
    activeMessages.value.map(
      (msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`,
    ),
  () => scrollMessagesToBottom(),
  { flush: "post" },
);

// Tools pane: auto-scroll on content changes
watch(
  () =>
    toolMessages.value.map(
      (msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`,
    ),
  () => scrollToolsToBottom(),
  { flush: "post" },
);

watch(activeSessionId, (sessionId) => {
  if (sessionId) {
    void loadMessagesFromServer(sessionId);
  }
});

watch(renamingSessionId, (value) => {
  if (!value) return;
  nextTick(() => {
    renameInput.value?.focus();
    renameInput.value?.select();
  });
});

onMounted(() => {
  void refreshSessionsFromServer(true);
  nextTick(() => {
    autoSizeComposer();
    scrollMessagesToBottom({ force: true, behavior: "auto" });
    scrollToolsToBottom({ force: true, behavior: "auto" });
  });
});

watch(draft, () => autoSizeComposer());

function setRenameInput(el: HTMLInputElement | null) {
  renameInput.value = el;
}

function ensureSession(): string {
  if (!activeSessionId.value) {
    throw new Error("No active conversation");
  }
  if (!(activeSessionId.value in messagesBySession.value)) {
    messagesBySession.value = {
      ...messagesBySession.value,
      [activeSessionId.value]: [],
    };
  }
  return activeSessionId.value;
}

function setMessages(sessionId: string, messages: ChatMessage[]) {
  messagesBySession.value = {
    ...messagesBySession.value,
    [sessionId]: messages,
  };
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
  updater: (message: ChatMessage) => ChatMessage,
) {
  const existing = messagesBySession.value[sessionId] || [];
  let updated = false;
  const next = existing.map((message) => {
    if (message.id === messageId) {
      updated = true;
      return updater(message);
    }
    return message;
  });
  if (updated) {
    setMessages(sessionId, next);
  }
}

function touchSession(sessionId: string, preview?: string) {
  const index = sessions.value.findIndex((session) => session.id === sessionId);
  if (index === -1) return;
  const session = sessions.value[index];
  const updated: ChatSessionMeta = {
    ...session,
    updatedAt: new Date().toISOString(),
    lastMessagePreview: preview ?? session.lastMessagePreview,
  };
  const clone = [...sessions.value];
  clone.splice(index, 1, updated);
  sessions.value = clone;
}

function selectSession(sessionId: string) {
  activeSessionId.value = sessionId;
  void loadMessagesFromServer(sessionId);
  autoScrollEnabled.value = true;
  toolAutoScrollEnabled.value = true;
  nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
  nextTick(() => scrollToolsToBottom({ force: true, behavior: "auto" }));
}

async function createSession(name = "New Chat") {
  try {
    const session = await apiCreateChatSession(name);
    sessionsError.value = null;
    if (!session) {
      return;
    }
    sessions.value = [session, ...sessions.value];
    messagesBySession.value = { ...messagesBySession.value, [session.id]: [] };
    fetchedMessageSessions.delete(session.id);
    activeSessionId.value = session.id;
    renamingSessionId.value = session.id;
    renamingName.value = session.name;
    autoScrollEnabled.value = true;
    toolAutoScrollEnabled.value = true;
    await loadMessagesFromServer(session.id, { force: true });
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
    nextTick(() => scrollToolsToBottom({ force: true, behavior: "auto" }));
  } catch (error) {
    const status = httpStatus(error);
    if (status === 403) {
      sessionsError.value =
        "You do not have permission to create conversations.";
    }
    console.error("Failed to create session", error);
  }
}

async function deleteSession(sessionId: string) {
  try {
    await apiDeleteChatSession(sessionId);
    sessionsError.value = null;
  } catch (error) {
    const status = httpStatus(error);
    if (status === 403) {
      sessionsError.value =
        "You do not have permission to delete this conversation.";
    }
    console.error("Failed to delete session", error);
    return;
  }

  const nextSessions = sessions.value.filter(
    (session) => session.id !== sessionId,
  );
  const { [sessionId]: _removed, ...rest } = messagesBySession.value;
  messagesBySession.value = rest;
  fetchedMessageSessions.delete(sessionId);

  if (!nextSessions.length) {
    try {
      const fresh = await apiCreateChatSession("New Chat");
      sessions.value = [fresh];
      messagesBySession.value = { [fresh.id]: [] };
      fetchedMessageSessions.delete(fresh.id);
      activeSessionId.value = fresh.id;
      autoScrollEnabled.value = true;
      toolAutoScrollEnabled.value = true;
      await loadMessagesFromServer(fresh.id, { force: true });
      nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
      nextTick(() => scrollToolsToBottom({ force: true, behavior: "auto" }));
      return;
    } catch (error) {
      sessions.value = [];
      activeSessionId.value = "";
      sessionsError.value = "No conversations available.";
      console.error("Failed to create replacement session", error);
      return;
    }
  }

  sessions.value = nextSessions;
  if (activeSessionId.value === sessionId) {
    activeSessionId.value = nextSessions[0]?.id || "";
    if (activeSessionId.value) {
      void loadMessagesFromServer(activeSessionId.value, { force: true });
    }
    autoScrollEnabled.value = true;
    toolAutoScrollEnabled.value = true;
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
    nextTick(() => scrollToolsToBottom({ force: true, behavior: "auto" }));
  }
}

function startRename(session: ChatSessionMeta) {
  renamingSessionId.value = session.id;
  renamingName.value = session.name;
}

async function commitRename(sessionId: string) {
  if (renamingSessionId.value !== sessionId) return;
  const name = renamingName.value.trim();
  if (!name) {
    cancelRename();
    return;
  }
  const index = sessions.value.findIndex((session) => session.id === sessionId);
  if (index === -1) return;
  try {
    const updated = await apiRenameChatSession(sessionId, name);
    sessionsError.value = null;
    const clone = [...sessions.value];
    clone.splice(index, 1, { ...clone[index], name: updated.name });
    sessions.value = clone;
  } catch (error) {
    const status = httpStatus(error);
    if (status === 403) {
      sessionsError.value =
        "You do not have permission to rename this conversation.";
    }
    console.error("Failed to rename session", error);
  }
  cancelRename();
}

function cancelRename() {
  renamingSessionId.value = null;
  renamingName.value = "";
}

async function sendCurrentPrompt() {
  await sendPrompt(draft.value);
}

async function sendPrompt(text: string, options: { echoUser?: boolean } = {}) {
  const content = text.trim();
  if ((!content && !pendingAttachments.value.length) || isStreaming.value)
    return;

  const sessionId = ensureSession();
  const now = new Date().toISOString();
  autoScrollEnabled.value = true;

  if (options.echoUser !== false) {
    const attachmentsCopy = pendingAttachments.value.map((a) => ({ ...a }));
    appendMessage(sessionId, {
      id: crypto.randomUUID(),
      role: "user",
      content,
      createdAt: now,
      attachments: attachmentsCopy,
    });
  }

  const assistantId = crypto.randomUUID();
  appendMessage(sessionId, {
    id: assistantId,
    role: "assistant",
    content: "",
    createdAt: now,
    streaming: true,
  });

  streamingAssistantId.value = assistantId;
  isStreaming.value = true;
  draft.value = options.echoUser === false ? draft.value : "";
  toolMessageIndex.clear();
  abortController.value = new AbortController();

  try {
    // Append text attachments into the prompt with delimiters
    let promptToSend = content;
    for (const att of textAttachments.value) {
      const f = filesByAttachment.get(att.id);
      if (!f) continue;
      const textContent = await f.text();
      const header = `\n\n--- Attached Document: ${att.name} (${att.mime || "text"}) ---\n`;
      const footer = `\n--- End Document ---\n`;
      promptToSend += header + textContent + footer;
    }

    // Gather image files if any
    const imageFiles: File[] = [];
    for (const att of imageAttachments.value) {
      const f = filesByAttachment.get(att.id);
      if (f) imageFiles.push(f);
    }

    if (imageFiles.length) {
      await streamAgentVisionRun({
        prompt: promptToSend,
        sessionId,
        files: imageFiles,
        signal: abortController.value!.signal,
        onEvent: (event) => handleStreamEvent(event, sessionId, assistantId),
      });
    } else {
      await streamAgentRun({
        prompt: promptToSend,
        sessionId,
        signal: abortController.value!.signal,
        onEvent: (event) => handleStreamEvent(event, sessionId, assistantId),
      });
    }
  } catch (error) {
    const assistantUpdater = (message: ChatMessage) => ({
      ...message,
      streaming: false,
      error:
        error instanceof DOMException && error.name === "AbortError"
          ? "Generation stopped"
          : error instanceof Error
            ? error.message
            : "Unexpected error",
    });
    updateMessage(sessionId, assistantId, assistantUpdater);
  } finally {
    isStreaming.value = false;
    streamingAssistantId.value = null;
    abortController.value = null;
    pendingAttachments.value = [];
    filesByAttachment.clear();
  }
}

function handleStreamEvent(
  event: ChatStreamEvent,
  sessionId: string,
  assistantId: string,
) {
  switch (event.type) {
    case "delta": {
      if (typeof event.data === "string" && event.data) {
        updateMessage(sessionId, assistantId, (message) => ({
          ...message,
          content: message.content + event.data,
        }));
      }
      break;
    }
    case "final": {
      const text = typeof event.data === "string" ? event.data : "";
      updateMessage(sessionId, assistantId, (message) => ({
        ...message,
        content: text || message.content,
        streaming: false,
      }));
      if (text) {
        touchSession(sessionId, snippet(text));
      }
      // Refresh runs list so Runs view reflects completed runs
      try {
        queryClient.invalidateQueries({ queryKey: ["agent-runs"] });
      } catch (e) {
        // ignore if queryClient unavailable in some contexts
      }
      break;
    }
    case "tool_start": {
      const now = new Date().toISOString();
      const key =
        typeof event.tool_id === "string" ? event.tool_id : crypto.randomUUID();
      const messageId = crypto.randomUUID();
      toolMessageIndex.set(key, messageId);
      appendMessage(
        sessionId,
        {
          id: messageId,
          role: "tool",
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
      const key = typeof event.tool_id === "string" ? event.tool_id : null;
      if (key && toolMessageIndex.has(key)) {
        const messageId = toolMessageIndex.get(key) as string;
        updateMessage(sessionId, messageId, (message) => ({
          ...message,
          content: result,
          streaming: false,
        }));
        toolMessageIndex.delete(key);
      } else {
        const pending = findLastIndex(
          messagesBySession.value[sessionId] || [],
          (msg) => msg.role === "tool" && !!msg.streaming,
        );
        if (pending !== -1) {
          const messageId = (messagesBySession.value[sessionId] || [])[pending]
            .id;
          updateMessage(sessionId, messageId, (message) => ({
            ...message,
            title: message.title || event.title || "Tool result",
            content: result,
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
    case "tts_chunk":
      // Ignore incremental binary metadata for now.
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
              typeof event.file_path === "string" ? event.file_path : undefined,
          },
          false,
        );
      }
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
    default:
      break;
  }
}

function stopStreaming() {
  abortController.value?.abort();
}

async function regenerateAssistant() {
  if (!canRegenerate.value || !lastUser.value) return;
  const sessionId = ensureSession();
  const messages = messagesBySession.value[sessionId] || [];
  const targetIndex = findLastIndex(
    messages,
    (message) => message.role === "assistant",
  );
  if (targetIndex !== -1) {
    const next = [...messages];
    next.splice(targetIndex, 1);
    setMessages(sessionId, next);
  }
  await sendPrompt(lastUser.value.content, { echoUser: false });
}

function copyMessage(message: ChatMessage) {
  if (!navigator.clipboard || !message.content) return;
  navigator.clipboard
    .writeText(message.content)
    .then(() => {
      copiedMessageId.value = message.id;
      setTimeout(() => {
        if (copiedMessageId.value === message.id) {
          copiedMessageId.value = null;
        }
      }, 1500);
    })
    .catch(() => {
      copiedMessageId.value = null;
    });
}

function labelForRole(role: ChatRole) {
  switch (role) {
    case "user":
      return "You";
    case "assistant":
      return "Agent";
    case "tool":
      return "Tool";
    case "system":
      return "System";
    default:
      return "Status";
  }
}

const timeFormatter = new Intl.DateTimeFormat(undefined, {
  hour: "numeric",
  minute: "2-digit",
});

function formatTimestamp(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  return timeFormatter.format(date);
}

function snippet(content: string) {
  if (!content) return "";
  const trimmed = content.replace(/\s+/g, " ").trim();
  return trimmed.length > 80 ? `${trimmed.slice(0, 77)}…` : trimmed;
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    sendCurrentPrompt();
  }
}

function autoSizeComposer() {
  const el = composer.value;
  if (!el) return;
  // If draft is empty, reset to default (1 row) height
  if (!draft.value || !draft.value.trim()) {
    el.style.height = "";
    return;
  }
  // Otherwise autosize up to a max height
  el.style.height = "auto";
  el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
}

type ScrollToBottomOptions = {
  force?: boolean;
  behavior?: ScrollBehavior;
};

function scrollMessagesToBottom(options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    const container = messagesPane.value;
    if (!container) return;

    if (!options.force && !autoScrollEnabled.value) {
      return;
    }

    const behavior = options.behavior ?? (options.force ? "smooth" : "auto");
    const target = Math.max(container.scrollHeight - container.clientHeight, 0);
    container.scrollTo({ top: target, behavior });

    if (options.force) {
      autoScrollEnabled.value = true;
    }
  });
}

function scrollToolsToBottom(options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    const container = toolsPane.value;
    if (!container) return;

    if (!options.force && !toolAutoScrollEnabled.value) {
      return;
    }

    const behavior = options.behavior ?? (options.force ? "smooth" : "auto");
    const target = Math.max(container.scrollHeight - container.clientHeight, 0);
    container.scrollTo({ top: target, behavior });

    if (options.force) {
      toolAutoScrollEnabled.value = true;
    }
  });
}

function isNearBottom(container: HTMLElement) {
  const distance =
    container.scrollHeight - (container.scrollTop + container.clientHeight);
  return distance <= SCROLL_LOCK_THRESHOLD;
}

function handleMessagesScroll(event: Event) {
  const container = event.target as HTMLElement | null;
  if (!container) return;
  autoScrollEnabled.value = isNearBottom(container);
}

function handleScrollToLatest() {
  scrollMessagesToBottom({ force: true, behavior: "smooth" });
}

function handleToolsScroll(event: Event) {
  const container = event.target as HTMLElement | null;
  if (!container) return;
  toolAutoScrollEnabled.value = isNearBottom(container);
}

function handleScrollToolsToLatest() {
  scrollToolsToBottom({ force: true, behavior: "smooth" });
}

function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return items[i];
    }
  }
  return null;
}

function findLastIndex<T>(items: T[], predicate: (item: T) => boolean): number {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return i;
    }
  }
  return -1;
}

function goToDashboard() {
  router.push({ name: "overview" });
}

// --- Voice recording (microphone → WAV → /stt) ---
const isRecording = ref(false);
const canUseMic =
  typeof window !== "undefined" &&
  !!navigator.mediaDevices &&
  !!window.AudioContext;
let mediaStream: MediaStream | null = null;
let audioCtx: AudioContext | null = null;
let processor: ScriptProcessorNode | null = null;
let sourceNode: MediaStreamAudioSourceNode | null = null;
let recordedChunks: Float32Array[] = [];
let inputChannels = 1;
let inputSampleRate = 48000;

async function startRecording() {
  if (!canUseMic || isRecording.value) return;
  try {
    mediaStream = await navigator.mediaDevices.getUserMedia({ audio: true });
    audioCtx = new (window.AudioContext ||
      (window as any).webkitAudioContext)();
    inputSampleRate = audioCtx.sampleRate || 48000;
    sourceNode = audioCtx.createMediaStreamSource(mediaStream);
    // ScriptProcessorNode buffer size 4096, stereo support if available
    processor = audioCtx.createScriptProcessor(4096, 2, 1);
    inputChannels = sourceNode.channelCount || 1;
    recordedChunks = [];
    processor.onaudioprocess = (e: AudioProcessingEvent) => {
      const input0 = e.inputBuffer.getChannelData(0);
      let chunk: Float32Array;
      if (inputChannels > 1) {
        const input1 = e.inputBuffer.getChannelData(1);
        const mono = new Float32Array(input0.length);
        for (let i = 0; i < input0.length; i++)
          mono[i] = (input0[i] + input1[i]) / 2;
        chunk = mono;
      } else {
        // copy to avoid referencing backing buffer
        chunk = new Float32Array(input0.length);
        chunk.set(input0);
      }
      recordedChunks.push(chunk);
    };
    sourceNode.connect(processor);
    processor.connect(audioCtx.destination);
    isRecording.value = true;
  } catch (err) {
    console.warn("Mic access failed", err);
    cleanupRecording();
  }
}

function cleanupRecording() {
  try {
    processor?.disconnect();
    sourceNode?.disconnect();
  } catch {}
  try {
    mediaStream?.getTracks().forEach((t) => t.stop());
  } catch {}
  try {
    audioCtx?.close();
  } catch {}
  mediaStream = null;
  processor = null;
  sourceNode = null;
  audioCtx = null;
}

async function stopRecording() {
  if (!isRecording.value) return;
  isRecording.value = false;
  cleanupRecording();
  // Merge chunks
  const totalLen = recordedChunks.reduce((sum, c) => sum + c.length, 0);
  const merged = new Float32Array(totalLen);
  let offset = 0;
  for (const c of recordedChunks) {
    merged.set(c, offset);
    offset += c.length;
  }
  recordedChunks = [];
  // Resample to 16kHz mono
  const targetRate = 16000;
  const resampled = resampleLinear(merged, inputSampleRate, targetRate);
  const wavBlob = encodeWAV(resampled, targetRate);
  try {
    const text = await transcribeBlob(wavBlob);
    if (text) {
      // Append to composer with a space if needed
      const needsSpace = draft.value && !/\s$/.test(draft.value);
      draft.value = (draft.value || "") + (needsSpace ? " " : "") + text;
      nextTick(() => autoSizeComposer());
    }
  } catch (err) {
    console.warn("STT failed", err);
  }
}

function resampleLinear(
  input: Float32Array,
  inRate: number,
  outRate: number,
): Float32Array {
  if (inRate === outRate) return input;
  const ratio = inRate / outRate;
  const outLen = Math.floor(input.length / ratio);
  const out = new Float32Array(outLen);
  let pos = 0;
  for (let i = 0; i < outLen; i++) {
    const idx = i * ratio;
    const i0 = Math.floor(idx);
    const i1 = Math.min(i0 + 1, input.length - 1);
    const frac = idx - i0;
    out[i] = input[i0] * (1 - frac) + input[i1] * frac;
    pos += ratio;
  }
  return out;
}

function encodeWAV(samples: Float32Array, sampleRate: number): Blob {
  // Convert float32 [-1,1] to 16-bit PCM
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  // RIFF header
  writeString(view, 0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  writeString(view, 8, "WAVE");
  // fmt chunk
  writeString(view, 12, "fmt ");
  view.setUint32(16, 16, true); // PCM chunk size
  view.setUint16(20, 1, true); // audio format = PCM
  view.setUint16(22, 1, true); // channels = 1
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, sampleRate * 2, true); // byte rate = sampleRate * blockAlign
  view.setUint16(32, 2, true); // block align = channels * bytesPerSample
  view.setUint16(34, 16, true); // bits per sample
  // data chunk
  writeString(view, 36, "data");
  view.setUint32(40, samples.length * 2, true);
  // PCM samples
  let offset = 44;
  for (let i = 0; i < samples.length; i++, offset += 2) {
    let s = Math.max(-1, Math.min(1, samples[i]));
    view.setInt16(offset, s < 0 ? s * 0x8000 : s * 0x7fff, true);
  }
  return new Blob([view], { type: "audio/wav" });
}

function writeString(view: DataView, offset: number, s: string) {
  for (let i = 0; i < s.length; i++) view.setUint8(offset + i, s.charCodeAt(i));
}

async function transcribeBlob(blob: Blob): Promise<string> {
  const form = new FormData();
  form.set("audio", blob, "prompt.wav");
  // Prefer same-origin /stt so it works in production with embedded UI.
  // In dev, Vite proxy forwards /stt to agentd when VITE_DEV_SERVER_PROXY is set.
  const url = "/stt";
  const resp = await fetch(url, { method: "POST", body: form });
  if (!resp.ok) throw new Error(`stt failed (${resp.status})`);
  const data = (await resp.json()) as { text?: string };
  return data?.text || "";
}
</script>

<style scoped>
.chat-markdown {
  white-space: normal;
  overflow-wrap: anywhere; /* allow breaking long tokens */
  word-break: break-word; /* legacy support */
}

:deep(.chat-markdown p) {
  margin: 0 0 0.75rem;
}

:deep(.chat-markdown p:last-child) {
  margin-bottom: 0;
}

:deep(.chat-markdown ul),
:deep(.chat-markdown ol) {
  margin: 0 0 0.75rem 1.25rem;
  padding: 0 0 0 1rem;
  list-style-position: outside;
}

:deep(.chat-markdown li) {
  margin-bottom: 0.25rem;
}

:deep(.chat-markdown ul) {
  list-style-type: disc;
}

:deep(.chat-markdown ol) {
  list-style-type: decimal;
}

:deep(.chat-markdown h1),
:deep(.chat-markdown h2),
:deep(.chat-markdown h3),
:deep(.chat-markdown h4),
:deep(.chat-markdown h5),
:deep(.chat-markdown h6) {
  margin: 1.25rem 0 0.75rem;
  font-weight: 600;
  line-height: 1.2;
}

:deep(.chat-markdown h1) {
  font-size: 1.6rem;
}

:deep(.chat-markdown h2) {
  font-size: 1.4rem;
}

:deep(.chat-markdown h3) {
  font-size: 1.2rem;
}

:deep(.chat-markdown h4) {
  font-size: 1.1rem;
}

:deep(.chat-markdown h5),
:deep(.chat-markdown h6) {
  font-size: 1rem;
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

:deep(.chat-markdown ul) {
  list-style-type: disc;
}

:deep(.chat-markdown ol) {
  list-style-type: decimal;
}

:deep(.chat-markdown h1),
:deep(.chat-markdown h2),
:deep(.chat-markdown h3),
:deep(.chat-markdown h4),
:deep(.chat-markdown h5),
:deep(.chat-markdown h6) {
  margin: 1rem 0 0.5rem;
  font-weight: 600;
  line-height: 1.25;
}

:deep(.chat-markdown h1) {
  font-size: 1.5rem;
}

:deep(.chat-markdown h2) {
  font-size: 1.3rem;
}

:deep(.chat-markdown h3) {
  font-size: 1.15rem;
}

:deep(.chat-markdown pre) {
  margin: 0 0 0.75rem;
}

:deep(.chat-markdown code) {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 0.875rem;
}

.chat-markdown :deep(pre.hljs) {
  border-radius: 0.5rem;
  overflow-x: auto;
  padding: 0.75rem;
  background-color: rgb(var(--color-surface-muted) / 0.9);
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
  background: linear-gradient(
    rgb(var(--color-surface-muted) / 0.95),
    rgb(var(--color-surface-muted) / 0.6)
  );
  border-top-left-radius: 0.5rem;
  border-top-right-radius: 0.5rem;
  z-index: 1;
}

.chat-markdown :deep(.md-codeblock .hljs) {
  margin-top: 0; /* snug under toolbar */
}

.chat-markdown :deep(.md-lang) {
  font-size: 0.75rem;
  color: rgb(var(--color-subtle-foreground));
}

.chat-markdown :deep(.md-copy-btn) {
  font-size: 0.75rem;
  line-height: 1;
  color: rgb(var(--color-foreground));
  background: rgb(var(--color-surface-muted) / 0.8);
  border: 1px solid rgb(var(--color-border));
  padding: 0.25rem 0.5rem;
  border-radius: 0.375rem;
  cursor: pointer;
}

.chat-markdown :deep(.md-copy-btn:hover) {
  color: rgb(var(--color-accent));
  border-color: rgb(var(--color-accent));
}

.chat-markdown :deep(.md-copy-btn.copied) {
  color: rgb(var(--color-success));
  border-color: rgb(var(--color-success));
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
