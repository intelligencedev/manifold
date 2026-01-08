<template>
  <div class="flex h-full min-h-0 flex-1 overflow-hidden chat-modern">
    <section
      class="grid h-full flex-1 min-h-0 overflow-hidden gap-3 lg:grid-cols-[280px_1fr] xl:grid-cols-[300px_1fr] chat-grid"
    >
      <!-- Sessions sidebar -->
      <aside
        class="glass-surface hidden h-full min-h-0 lg:flex flex-col gap-3 overflow-hidden rounded-[var(--radius-lg,26px)] border border-white/12 bg-surface/70 p-4"
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
              <div class="flex items-center gap-2">
                <span
                  class="rounded-full border border-border/60 bg-surface px-2 py-0.5 text-[10px] text-subtle-foreground"
                >
                  {{ messageCountFor(session.id) }} msg{{
                    messageCountFor(session.id) === 1 ? "" : "s"
                  }}
                </span>
                <span>{{ formatTimestamp(session.updatedAt) }}</span>
              </div>
              <div class="flex items-center gap-2">
                <span
                  v-if="sessionIsStreaming(session.id)"
                  class="flex items-center gap-1 text-xs text-accent"
                >
                  <span
                    class="h-1.5 w-1.5 animate-pulse rounded-full bg-accent"
                  ></span>
                  Streaming
                </span>
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
        </div>
      </aside>

      <!-- Chat pane -->
      <section
        class="glass-surface relative flex h-full min-h-0 flex-col overflow-hidden rounded-[var(--radius-lg,26px)] border border-white/12 bg-surface/80 chat-pane"
      >
        <header
          class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3"
        >
          <div>
            <h1 class="text-base font-semibold text-foreground">
              {{ activeSession?.name || "Conversation" }}
            </h1>
          </div>
          <div class="flex items-center gap-2 text-xs text-subtle-foreground">
            <!-- Summary triggered indicator -->
            <span
              v-if="activeSummaryEvent"
              class="flex items-center gap-1.5 rounded-full bg-warning/10 dark:bg-warning/20 border border-warning/30 px-2.5 py-1 text-warning dark:text-warning-foreground transition-all duration-300"
              :title="`Summarized ${activeSummaryEvent.summarizedCount} of ${activeSummaryEvent.messageCount} messages (${activeSummaryEvent.inputTokens.toLocaleString()} tokens exceeded ${activeSummaryEvent.tokenBudget.toLocaleString()} budget)`"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-3 w-3"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fill-rule="evenodd"
                  d="M4 4a2 2 0 012-2h4.586A2 2 0 0112 2.586L15.414 6A2 2 0 0116 7.414V16a2 2 0 01-2 2H6a2 2 0 01-2-2V4zm2 6a1 1 0 011-1h6a1 1 0 110 2H7a1 1 0 01-1-1zm1 3a1 1 0 100 2h6a1 1 0 100-2H7z"
                  clip-rule="evenodd"
                />
              </svg>
              <span class="font-medium">Context summarized</span>
              <button
                type="button"
                class="ml-0.5 rounded-full p-0.5 hover:bg-warning/20 dark:hover:bg-warning/30 transition"
                title="Dismiss"
                @click.stop="chat.clearSummaryEvent()"
              >
                <svg
                  xmlns="http://www.w3.org/2000/svg"
                  class="h-3 w-3"
                  viewBox="0 0 20 20"
                  fill="currentColor"
                >
                  <path
                    fill-rule="evenodd"
                    d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                    clip-rule="evenodd"
                  />
                </svg>
              </button>
            </span>
            <div class="flex items-center gap-2">
              <DropdownSelect
                v-model="selectedSpecialist"
                :options="specialistOptions"
                size="xs"
                title="Choose specialist for this chat"
                aria-label="Specialist override"
              />
              <!-- Project selection is global in the header; moved to App.vue -->
              <DropdownSelect
                v-model="renderMode"
                :options="renderModeOptions"
                size="xs"
                title="Render mode for assistant responses"
                aria-label="Render mode"
              />
            </div>
          </div>
        </header>

        <div
          ref="messagesPane"
          class="flex-1 min-h-0 space-y-5 overflow-y-auto overflow-x-hidden overscroll-contain px-4 py-4 pb-3"
          @scroll="handleMessagesScroll"
          @click="handleMarkdownClick"
        >
          <div
            v-if="!chatMessages.length"
            class="ap-card flex h-full flex-col items-center justify-center gap-2 rounded-5 border border-dashed border-border bg-surface p-8 text-center text-sm text-subtle-foreground"
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
            class="relative max-w-[72ch] glass-surface rounded-[var(--radius,18px)] border border-white/12 p-5"
            :class="message.role === 'user' ? 'ml-auto' : ''"
          >
            <header class="flex flex-wrap items-center gap-2">
              <template v-if="message.role === 'assistant'">
                <span
                  class="rounded-full bg-accent/10 px-2 py-1 text-xs font-semibold text-accent"
                >
                  {{ agentNameFor(message) }}
                </span>
              </template>
              <span
                v-else
                class="rounded-full bg-surface-muted px-2 py-1 text-xs font-semibold text-muted-foreground"
              >
                {{ labelForRole(message.role) }}
              </span>
              <span class="text-xs text-faint-foreground">{{
                formatTimestamp(message.createdAt)
              }}</span>
              <span
                v-if="shouldShowResponseTimer(message)"
                class="inline-flex items-center gap-1 rounded-full border px-2 py-1 text-[11px] font-semibold tabular-nums"
                :class="
                  message.streaming
                    ? 'border-accent/30 bg-accent/10 text-accent'
                    : 'border-border/60 bg-surface-muted/40 text-faint-foreground'
                "
                :title="
                  message.streaming
                    ? 'Response time (running)'
                    : 'Response time'
                "
              >
                {{ formatDuration(responseElapsedMs(message.id)) }}
              </span>
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
              <div
                v-if="shouldShowResponseStatus(message)"
                class="response-status"
                :class="responseStatusClasses"
              >
                <div class="response-status__dot"></div>
                <div class="response-status__body">
                  <p class="response-status__title">
                    {{ responseStatus?.title }}
                  </p>
                  <p
                    v-if="responseStatus?.detail"
                    class="response-status__detail"
                  >
                    {{ responseStatus.detail }}
                  </p>
                </div>
                <span
                  class="response-status__pill"
                  :class="responseStatusPillClasses"
                >
                  {{ responseStatus?.stateLabel }}
                </span>
              </div>
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
                    class="h-16 w-16 rounded object-cover border border-border cursor-zoom-in"
                    @click="openImageModal(img)"
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
          class="absolute bottom-28 left-1/2 -translate-x-1/2 z-10 flex items-center gap-2 rounded-full bg-surface px-3 py-2 text-xs font-semibold text-foreground shadow-2 ring-1 ring-border/50 transform transition-all duration-200"
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

        <footer class="ap-hairline-b px-4 pt-2 pb-4">
          <form
            class="space-y-3"
            @submit.prevent="sendCurrentPrompt"
            @dragover.prevent
            @drop.prevent="handleDrop"
          >
            <p
              v-if="requiresProjectSelection"
              class="rounded-4 border border-danger/40 bg-danger/10 px-3 py-2 text-xs text-danger"
            >
              Select a project to run the agent. If you don't see any projects,
              contact an administrator.
            </p>
            <div
              class="ap-input chat-prompt-input relative rounded-4 bg-surface-muted/70 p-3 etched-dark"
            >
              <div
                class="flex flex-wrap items-center gap-2 sm:gap-3 sm:flex-nowrap"
              >
                <textarea
                  ref="composer"
                  v-model="draft"
                  rows="1"
                  class="flex-1 min-w-0 resize-none bg-transparent py-1.5 text-sm leading-6 text-foreground outline-none placeholder:text-faint-foreground"
                  :placeholder="
                    projectSelected
                      ? 'Message the agent...'
                      : 'Select a project to enable the chat.'
                  "
                  :disabled="!projectSelected"
                  @keydown="handleComposerKeydown"
                  @input="autoSizeComposer"
                ></textarea>

                <!-- Inline actions (right aligned) -->
                <div class="flex items-end gap-1 shrink-0">
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
                    class="inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline"
                    title="Attach files"
                    aria-label="Attach files"
                    :disabled="!projectSelected"
                    :class="
                      !projectSelected
                        ? 'opacity-50 cursor-not-allowed text-foreground/40'
                        : 'text-foreground/80 hover:text-accent'
                    "
                    @click="projectSelected ? fileInput?.click() : undefined"
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
                      isStreaming || !canUseMic || !projectSelected
                        ? 'opacity-50 cursor-not-allowed'
                        : '',
                    ]"
                    :disabled="isStreaming || !canUseMic || !projectSelected"
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

                  <!-- Toggle Image Generation -->
                  <button
                    type="button"
                    class="inline-flex h-8 w-8 items-center justify-center rounded-3 focus-visible:shadow-outline transition"
                    :class="[
                      imagePrompt
                        ? 'bg-accent/20 text-accent hover:bg-accent/30'
                        : 'text-foreground/80 hover:text-accent',
                      isStreaming || !projectSelected
                        ? 'opacity-50 cursor-not-allowed'
                        : '',
                    ]"
                    :disabled="isStreaming || !projectSelected"
                    title="Generate image response"
                    aria-label="Generate image response"
                    @click="imagePrompt = !imagePrompt"
                  >
                    <Camera class="h-5 w-5" />
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
                    :aria-label="
                      isStreaming ? 'Stop generating' : 'Send message'
                    "
                    @click="isStreaming ? stopStreaming() : sendCurrentPrompt()"
                    :disabled="
                      !isStreaming &&
                      (!projectSelected ||
                        (!draft.trim() && !pendingAttachments.length))
                    "
                  >
                    <SolarStopBold v-if="isStreaming" class="h-4 w-4" />
                    <SolarArrowToTopLeftBold v-else class="h-4 w-4" />
                  </button>
                </div>
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

      <!-- Image modal -->
      <div
        v-if="showImageModal && modalImage"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
        @click.self="closeImageModal"
      >
        <div
          class="relative max-h-[90vh] max-w-[90vw] rounded-5 bg-surface p-4 shadow-3 ring-1 ring-border/60"
        >
          <button
            type="button"
            class="absolute right-3 top-3 rounded-full bg-surface-muted px-2 py-1 text-sm text-foreground shadow hover:bg-surface"
            @click="closeImageModal"
          >
            ×
          </button>
          <div class="flex flex-col items-center gap-3">
            <img
              :src="modalImageSrc"
              :alt="modalImage.name"
              class="max-h-[70vh] max-w-[80vw] rounded border border-border object-contain"
            />
            <div class="text-center text-xs text-subtle-foreground">
              <p class="font-semibold text-foreground">{{ modalImage.name }}</p>
              <p v-if="modalImage.path">Saved at: {{ modalImage.path }}</p>
            </div>
          </div>
        </div>
      </div>

    </section>
  </div>
</template>

<script setup lang="ts">
import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import { useRouter } from "vue-router";
import axios from "axios";
import type {
  AgentThread,
  ChatAttachment,
  ChatMessage,
  ChatSessionMeta,
  ChatRole,
} from "@/types/chat";
import { useQuery } from "@tanstack/vue-query";
import { listSpecialists, type Specialist } from "@/api/client";
import { renderMarkdown } from "@/utils/markdown";
import "highlight.js/styles/github-dark-dimmed.css";
import SolarPaperclip2Bold from "@/components/icons/SolarPaperclip2Bold.vue";
import SolarMicrophone3Bold from "@/components/icons/SolarMicrophone3Bold.vue";
import SolarArrowToTopLeftBold from "@/components/icons/SolarArrowToTopLeftBold.vue";
import SolarStopBold from "@/components/icons/SolarStopBold.vue";
import Camera from "@/components/icons/Camera.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import { useChatStore } from "@/stores/chat";
import { useProjectsStore } from "@/stores/projects";
import type { DropdownOption } from "@/types/dropdown";

const router = useRouter();
const isBrowser = typeof window !== "undefined";
const SCROLL_LOCK_THRESHOLD = 80;
let previousBodyOverflow: string | null = null;

const chat = useChatStore();
const proj = useProjectsStore();
onMounted(() => {
  void proj.refresh();
  if (isBrowser) {
    previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
});
const projects = computed(() => proj.projects);
const selectedProjectId = computed({
  get: () => proj.currentProjectId || "",
  set: (v: string) => (proj.currentProjectId = v),
});
const sessions = computed(() => chat.sessions);
const messagesBySession = computed(() => chat.messagesBySession);
const sessionsLoading = computed(() => chat.sessionsLoading);
const sessionsError = computed(() => chat.sessionsError);
const agentThreads = computed(() => chat.agentThreads);

const activeSessionId = computed({
  get: () => chat.activeSessionId,
  set: (v: string) => (chat.activeSessionId = v),
});
const draft = ref("");
const isStreaming = computed(() => chat.isStreaming);
const renamingSessionId = ref<string | null>(null);
const renamingName = ref("");
const renameInput = ref<HTMLInputElement | null>(null);
const messagesPane = ref<HTMLDivElement | null>(null);
const composer = ref<HTMLTextAreaElement | null>(null);
const copiedMessageId = ref<string | null>(null);
const autoScrollEnabled = ref(true);
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
// Toggle to request image generation from providers that support it (e.g., Google Gemini)
const imagePrompt = ref(false);
// Image modal state
const showImageModal = ref(false);
const modalImage = ref<ChatAttachment | null>(null);
const modalImageSrc = computed(() => {
  const img = modalImage.value;
  if (!img) return "";
  return img.previewUrl || img.path || "";
});

// Specialists dropdown state
const { data: specialistsData } = useQuery({
  queryKey: ["specialists"],
  queryFn: listSpecialists,
  staleTime: 5_000,
});
const specialistsByName = computed(() => {
  const map = new Map<string, Specialist>();
  (specialistsData?.value || []).forEach((s: Specialist) => {
    const key = s.name?.trim().toLowerCase();
    if (key) map.set(key, s);
  });
  return map;
});
const specialistNames = computed(() =>
  (specialistsData?.value || [])
    .map((s: Specialist) => s.name)
    .filter((n: string) => n && n.trim().toLowerCase() !== "orchestrator")
    .sort((a: string, b: string) =>
      a.localeCompare(b, undefined, { sensitivity: "base" }),
    ),
);

// Transform specialists data for dropdown
const specialistOptions = computed<DropdownOption[]>(() => [
  { id: "orchestrator", label: "orchestrator", value: "orchestrator" },
  ...specialistNames.value.map((name: string) => ({
    id: name,
    label: name,
    value: name,
  })),
]);

// Transform projects data for dropdown
const projectOptions = computed<DropdownOption[]>(() => {
  const projectEntries = projects.value.map((project) => ({
    id: project.id,
    label: project.name,
    value: project.id,
  }));
  if (!projectEntries.length) {
    return [{ id: "", label: "no project available", value: "" }];
  }
  return [
    {
      id: "",
      label: "Select a project",
      value: "",
      disabled: true,
    },
    ...projectEntries,
  ];
});

// Transform render mode options for dropdown
const renderModeOptions = computed<DropdownOption[]>(() => [
  { id: "markdown", label: "markdown", value: "markdown" },
  { id: "html", label: "html", value: "html" },
]);

const selectedSpecialist = ref<string>("orchestrator");
const projectSelected = computed(() => Boolean(selectedProjectId.value));
const requiresProjectSelection = computed(() => !projectSelected.value);

function httpStatus(error: unknown): number | null {
  if (axios.isAxiosError(error)) {
    return error.response?.status ?? null;
  }
  return null;
}

const refreshSessionsFromServer = chat.refreshSessionsFromServer;
const loadMessagesFromServer = chat.loadMessagesFromServer;

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

const activeSession = computed(() => chat.activeSession);
const activeMessages = computed(() => chat.activeMessages);
const chatMessages = computed(() => chat.chatMessages);
const toolMessages = computed(() => chat.toolMessages);
const toolActivityMsById = ref<Record<string, number>>({});
const activeSummaryEvent = computed(() => chat.activeSummaryEvent);
const sessionAgentDefaults = computed(() =>
  parseAgentModelLabel(activeSession.value?.model || ""),
);
const showScrollToBottom = computed(
  () => !autoScrollEnabled.value && chatMessages.value.length > 0,
);
const sessionMessageCounts = computed<Record<string, number>>(() => {
  const counts: Record<string, number> = {};
  for (const session of sessions.value) {
    const local = messagesBySession.value[session.id];
    const metaCount = session.messageCount ?? 0;
    if (Array.isArray(local) && local.length) {
      counts[session.id] = local.length;
    } else {
      counts[session.id] = metaCount;
    }
  }
  return counts;
});

function messageCountFor(sessionId: string) {
  return sessionMessageCounts.value[sessionId] ?? 0;
}

function sessionIsStreaming(sessionId: string) {
  return chat.isSessionStreaming(sessionId);
}

// --- Response timer (elapsed while streaming; frozen when stream completes) ---
// Note: historical messages loaded from the server won't have timing info; we only
// show timers for messages created/streamed during this UI session.
const responseStartMsByMessageId = new Map<string, number>();
const responseElapsedMsByMessageId = ref<Record<string, number>>({});
const responseIntervalByMessageId = new Map<string, number>();

function safeParseIsoMs(iso: string) {
  const ms = Date.parse(iso);
  return Number.isFinite(ms) ? ms : null;
}

function responseElapsedMs(messageId: string) {
  return responseElapsedMsByMessageId.value[messageId] ?? 0;
}

function formatDuration(ms: number) {
  const clamped = Math.max(0, ms);
  const seconds = clamped / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${minutes}:${String(secs).padStart(2, "0")}`;
}

function ensureResponseTimer(message: ChatMessage) {
  const id = message.id;
  if (!id) return;

  if (!responseStartMsByMessageId.has(id)) {
    const start = safeParseIsoMs(message.createdAt) ?? Date.now();
    responseStartMsByMessageId.set(id, start);
  }

  const startMs = responseStartMsByMessageId.get(id);
  if (!startMs) return;

  responseElapsedMsByMessageId.value[id] = Math.max(0, Date.now() - startMs);

  if (isBrowser && !responseIntervalByMessageId.has(id)) {
    const handle = window.setInterval(() => {
      const start = responseStartMsByMessageId.get(id);
      if (!start) return;
      responseElapsedMsByMessageId.value[id] = Math.max(0, Date.now() - start);
    }, 100);
    responseIntervalByMessageId.set(id, handle);
  }
}

function stopResponseTimer(messageId: string) {
  const start = responseStartMsByMessageId.get(messageId);
  if (start) {
    responseElapsedMsByMessageId.value[messageId] = Math.max(
      0,
      Date.now() - start,
    );
  }
  const handle = responseIntervalByMessageId.get(messageId);
  if (handle != null) {
    if (isBrowser) window.clearInterval(handle);
    responseIntervalByMessageId.delete(messageId);
  }
}

function stopAllResponseTimers() {
  // Iterate a snapshot since stopResponseTimer mutates the map.
  for (const id of Array.from(responseIntervalByMessageId.keys())) {
    stopResponseTimer(id);
  }
}

function shouldShowResponseTimer(message: ChatMessage) {
  if (message.role !== "assistant") return false;
  if (message.streaming) return true;
  return message.id in responseElapsedMsByMessageId.value;
}

type ResponseStatusState = "running" | "done" | "error";
type ResponseStatus = {
  title: string;
  detail: string;
  state: ResponseStatusState;
  stateLabel: string;
};

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

function safeTimestampMs(value?: string) {
  if (!value) return 0;
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : 0;
}

function agentThreadTimestamp(thread: AgentThread) {
  const lastEntry = thread.entries[thread.entries.length - 1];
  const stamp = lastEntry?.createdAt || thread.finishedAt || thread.startedAt;
  return safeTimestampMs(stamp);
}

function responseStateLabel(state: ResponseStatusState) {
  switch (state) {
    case "running":
      return "Running";
    case "done":
      return "Complete";
    case "error":
      return "Error";
    default:
      return "Running";
  }
}

function statusFromTool(tool: ChatMessage): ResponseStatus {
  const state: ResponseStatusState = tool.error
    ? "error"
    : tool.streaming
      ? "running"
      : "done";
  const name = (tool.title || "Tool").trim() || "Tool";
  const title =
    state === "running"
      ? `Using ${name}...`
      : state === "done"
        ? `Used ${name}`
        : `${name} failed`;
  const argDetail = tool.toolArgs ? snippet(tool.toolArgs) : "";
  return {
    title,
    detail: argDetail ? `Args: ${argDetail}` : "Tool call",
    state,
    stateLabel: responseStateLabel(state),
  };
}

function statusFromThread(thread: AgentThread): ResponseStatus {
  const state = thread.status;
  const name = (thread.agent || "Delegated agent").trim() || "Delegated agent";
  const title =
    state === "running"
      ? `Delegating to ${name}...`
      : state === "done"
        ? `${name} responded`
        : `${name} error`;
  const detail = thread.model ? `Model ${thread.model}` : "Delegation";
  return {
    title,
    detail,
    state,
    stateLabel: responseStateLabel(state),
  };
}

const latestToolMessage = computed(() => {
  const assistant = lastAssistant.value;
  if (!assistant) return null;
  const cutoff = safeTimestampMs(assistant.createdAt);
  let latest: ChatMessage | null = null;
  let latestStamp = 0;
  for (const tool of toolMessages.value) {
    const createdStamp = safeTimestampMs(tool.createdAt);
    if (createdStamp < cutoff) continue;
    const activityStamp =
      toolActivityMsById.value[tool.id] || createdStamp || 0;
    if (!latest || activityStamp >= latestStamp) {
      latest = tool;
      latestStamp = activityStamp;
    }
  }
  return latest;
});

const latestAgentThread = computed(() => {
  if (!agentThreads.value.length) return null;
  return agentThreads.value.reduce((latest, thread) =>
    agentThreadTimestamp(thread) >= agentThreadTimestamp(latest)
      ? thread
      : latest,
  );
});

const responseStatus = computed<ResponseStatus | null>(() => {
  const assistant = lastAssistant.value;
  if (!assistant || !assistant.streaming) return null;

  const tool = latestToolMessage.value;
  const thread = latestAgentThread.value;
  const toolTs = tool
    ? toolActivityMsById.value[tool.id] || safeTimestampMs(tool.createdAt)
    : 0;
  const threadTs = thread ? agentThreadTimestamp(thread) : 0;
  const toolRunning = tool?.streaming ?? false;
  const threadRunning = thread?.status === "running";

  if (tool && toolRunning && !threadRunning) return statusFromTool(tool);
  if (thread && threadRunning && !toolRunning) return statusFromThread(thread);
  if (toolTs || threadTs) {
    if (tool && toolTs >= threadTs) return statusFromTool(tool);
    if (thread) return statusFromThread(thread);
  }

  return {
    title: "Drafting response",
    detail: "Synthesizing output",
    state: "running",
    stateLabel: "Working",
  };
});

const responseStatusClasses = computed(() => {
  const state = responseStatus.value?.state || "running";
  return {
    "response-status--running": state === "running",
    "response-status--done": state === "done",
    "response-status--error": state === "error",
  };
});

const responseStatusPillClasses = computed(() => {
  const state = responseStatus.value?.state || "running";
  return {
    "response-status__pill--running": state === "running",
    "response-status__pill--done": state === "done",
    "response-status__pill--error": state === "error",
  };
});

function shouldShowResponseStatus(message: ChatMessage) {
  return (
    message.role === "assistant" &&
    message.id === lastAssistantId.value &&
    message.streaming &&
    Boolean(responseStatus.value)
  );
}

watch(
  () =>
    toolMessages.value.map((msg) => ({
      id: msg.id,
      signature: `${msg.content.length}:${msg.streaming ? 1 : 0}:${
        msg.error ? 1 : 0
      }`,
      createdAt: msg.createdAt,
    })),
  (next, prev) => {
    const now = Date.now();
    const prevMap = new Map<string, string>();
    (prev || []).forEach((item) => prevMap.set(item.id, item.signature));
    const updated: Record<string, number> = {};

    for (const item of next) {
      const priorSig = prevMap.get(item.id);
      if (!priorSig || priorSig !== item.signature) {
        updated[item.id] = now;
      } else {
        const baseStamp = safeTimestampMs(item.createdAt);
        updated[item.id] =
          toolActivityMsById.value[item.id] ?? (baseStamp || now);
      }
    }

    toolActivityMsById.value = updated;
  },
  { flush: "post" },
);

watch(
  () =>
    activeMessages.value.map(
      (msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`,
    ),
  () => scrollMessagesToBottom(),
  { flush: "post" },
);

// Keep response timers in sync with streaming lifecycle.
watch(
  () =>
    activeMessages.value.map((m) => `${m.id}:${m.role}:${m.streaming ? 1 : 0}`),
  () => {
    for (const msg of activeMessages.value) {
      if (msg.role !== "assistant") continue;
      if (msg.streaming) ensureResponseTimer(msg);
      else if (msg.id in responseElapsedMsByMessageId.value)
        stopResponseTimer(msg.id);
    }
  },
  { flush: "post" },
);

// Auto-dismiss summary event after 8 seconds
watch(activeSummaryEvent, (event) => {
  if (event) {
    setTimeout(() => {
      chat.clearSummaryEvent();
    }, 8000);
  }
});

watch(activeSessionId, (sessionId) => {
  if (sessionId) {
    void loadMessagesFromServer(sessionId);
  }
  // Switching sessions: ensure we don't leave any intervals running.
  stopAllResponseTimers();
});

watch(renamingSessionId, (value) => {
  if (!value) return;
  nextTick(() => {
    renameInput.value?.focus();
    renameInput.value?.select();
  });
});

onMounted(() => {
  void chat.init();
  nextTick(() => {
    autoSizeComposer();
    scrollMessagesToBottom({ force: true, behavior: "auto" });
  });
});

onBeforeUnmount(() => {
  stopAllResponseTimers();
  if (isBrowser && previousBodyOverflow !== null) {
    document.body.style.overflow = previousBodyOverflow;
  }
});

watch(draft, () => autoSizeComposer());

function setRenameInput(el: HTMLInputElement | null) {
  renameInput.value = el;
}

function selectSession(sessionId: string) {
  chat.selectSession(sessionId);
  autoScrollEnabled.value = true;
  nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
}

async function createSession(name = "New Chat") {
  try {
    await chat.createSession(name);
    const session = chat.activeSession;
    if (session) {
      renamingSessionId.value = session.id;
      renamingName.value = session.name;
    }
    autoScrollEnabled.value = true;
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
  } catch (error) {
    const status = httpStatus(error);
    if (status === 403) {
      // readonly
    }
  }
}

async function deleteSession(sessionId: string) {
  try {
    await chat.deleteSession(sessionId);
    autoScrollEnabled.value = true;
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
  } catch (error) {
    // ignore
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
  try {
    await chat.renameSession(sessionId, name);
  } catch (error) {
    // ignore
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
  if (!projectSelected.value) return;
  if ((!content && !pendingAttachments.value.length) || isStreaming.value)
    return;

  // New prompt: ensure any prior timer intervals are stopped before we start a new stream.
  stopAllResponseTimers();

  autoScrollEnabled.value = true;
  draft.value = options.echoUser === false ? draft.value : "";
  try {
    const specialist =
      selectedSpecialist.value && selectedSpecialist.value !== "orchestrator"
        ? selectedSpecialist.value
        : undefined;
    const { agentName, agentModel } = resolveAgentContext();
    await chat.sendPrompt(
      content,
      pendingAttachments.value,
      filesByAttachment,
      {
        ...options,
        specialist,
        projectId: selectedProjectId.value || undefined,
        image: imagePrompt.value,
        imageSize: "1K",
        agentName,
        agentModel,
      },
    );
  } catch (error) {
    // handled in store
  } finally {
    pendingAttachments.value = [];
    filesByAttachment.clear();
  }
}

function stopStreaming() {
  chat.stopStreaming();
}

async function regenerateAssistant() {
  if (!projectSelected.value || !canRegenerate.value || !lastUser.value) return;
  const specialist =
    selectedSpecialist.value && selectedSpecialist.value !== "orchestrator"
      ? selectedSpecialist.value
      : undefined;
  const { agentName, agentModel } = resolveAgentContext();
  await chat.regenerateAssistant({
    specialist,
    projectId: selectedProjectId.value,
    agentName,
    agentModel,
  });
}

function resolveAgentContext() {
  const selected = (selectedSpecialist.value || "orchestrator").trim();
  const fallback = sessionAgentDefaults.value;
  const agentName = selected || fallback.agentName || "Agent";
  const spec = specialistsByName.value.get(agentName.toLowerCase());
  const agentModel = (spec?.model || "").trim() || fallback.model || "";
  return { agentName, agentModel };
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

function openImageModal(img: ChatAttachment) {
  modalImage.value = img;
  showImageModal.value = true;
}

function closeImageModal() {
  showImageModal.value = false;
  modalImage.value = null;
}

function parseAgentModelLabel(label?: string) {
  const raw = (label || "").trim();
  if (!raw) return { agentName: "", model: "" };
  const [maybeAgent, ...rest] = raw.split(":");
  if (rest.length) {
    return { agentName: maybeAgent, model: rest.join(":") };
  }
  return { agentName: "", model: raw };
}

function agentMetaForMessage(message: ChatMessage) {
  if (message.role !== "assistant") return null;
  const defaults = sessionAgentDefaults.value;
  const agentName =
    (message.agentName || message.agent || "").trim() ||
    defaults.agentName ||
    "Agent";
  const agentModel =
    (message.agentModel || message.model || "").trim() || defaults.model || "";
  return { agentName, agentModel };
}

function agentNameFor(message: ChatMessage) {
  const meta = agentMetaForMessage(message);
  if (!meta) return labelForRole(message.role);
  return meta.agentName || labelForRole(message.role);
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

function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return items[i];
    }
  }
  return null;
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
.chat-modern {
  width: 100%;
  height: 100%;
  max-height: 100%;
  min-height: 0;
  overflow: hidden;
  overscroll-behavior: contain;
}

.chat-grid {
  min-height: 0;
  height: 100%;
  max-height: 100%;
}

.chat-pane {
  min-height: 0;
  height: 100%;
  max-height: 100%;
}

.response-status {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.7rem 0.85rem;
  border-radius: 0.9rem;
  border: 1px solid rgb(var(--color-border) / 0.6);
  background: linear-gradient(
    135deg,
    rgb(var(--color-surface-muted) / 0.9),
    rgb(var(--color-surface) / 0.95)
  );
  box-shadow: 0 14px 32px -24px rgb(0 0 0 / 0.6);
}

.response-status__dot {
  width: 0.65rem;
  height: 0.65rem;
  border-radius: 999px;
  background: rgb(var(--color-accent));
}

.response-status__body {
  min-width: 0;
  flex: 1;
}

.response-status__title {
  font-weight: 600;
  font-size: 0.9rem;
  color: rgb(var(--color-foreground));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.response-status__detail {
  margin-top: 0.1rem;
  font-size: 0.75rem;
  color: rgb(var(--color-subtle-foreground));
}

.response-status__pill {
  flex-shrink: 0;
  font-size: 0.62rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  padding: 0.2rem 0.55rem;
  border-radius: 999px;
  border: 1px solid transparent;
}

.response-status--running {
  border-color: rgb(var(--color-accent) / 0.35);
}

.response-status--running .response-status__dot {
  background: rgb(var(--color-accent));
  box-shadow: 0 0 0 6px rgb(var(--color-accent) / 0.18);
  animation: statusPulse 1.8s ease-in-out infinite;
}

.response-status__pill--running {
  border-color: rgb(var(--color-accent) / 0.4);
  color: rgb(var(--color-accent));
  background: rgb(var(--color-accent) / 0.12);
}

.response-status--done {
  border-color: rgb(var(--color-success) / 0.35);
}

.response-status--done .response-status__dot {
  background: rgb(var(--color-success));
  box-shadow: 0 0 0 6px rgb(var(--color-success) / 0.18);
}

.response-status__pill--done {
  border-color: rgb(var(--color-success) / 0.35);
  color: rgb(var(--color-success));
  background: rgb(var(--color-success) / 0.12);
}

.response-status--error {
  border-color: rgb(var(--color-danger) / 0.35);
}

.response-status--error .response-status__dot {
  background: rgb(var(--color-danger));
  box-shadow: 0 0 0 6px rgb(var(--color-danger) / 0.2);
}

.response-status__pill--error {
  border-color: rgb(var(--color-danger) / 0.4);
  color: rgb(var(--color-danger));
  background: rgb(var(--color-danger) / 0.12);
}

@keyframes statusPulse {
  0% {
    transform: scale(0.85);
    opacity: 0.6;
  }
  50% {
    transform: scale(1);
    opacity: 1;
  }
  100% {
    transform: scale(0.85);
    opacity: 0.6;
  }
}

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

.chat-modern .chat-prompt-input.ap-input {
  border: 1px solid rgb(255 255 255 / 0.12);
}

.chat-modern .chat-prompt-input.ap-input:focus-within,
.chat-modern .chat-prompt-input.ap-input:focus {
  border-color: rgb(255 255 255 / 0.12);
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
