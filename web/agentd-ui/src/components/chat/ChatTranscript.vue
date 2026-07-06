<template>
  <div
    ref="messagesPaneEl"
    class="chat-message-scroll flex-1 min-h-0 space-y-3 overflow-y-auto overflow-x-hidden overscroll-contain px-4 py-3 pb-2 xl:px-5"
    @scroll="model.handleMessagesScroll"
    @click="model.handleMarkdownClick"
  >
    <div
      v-if="model.hasOlderMessages || model.olderMessagesLoading"
      class="flex justify-center pb-1"
    >
      <button
        type="button"
        class="rounded-3 border border-border bg-surface-muted px-3 py-1.5 text-xs text-subtle-foreground transition hover:bg-surface-muted/80 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="model.olderMessagesLoading"
        @click="model.loadOlderMessages"
      >
        {{ model.olderMessagesLoading ? "Loading..." : "Load older messages" }}
      </button>
    </div>
    <p v-if="model.olderMessagesError" class="pb-1 text-center text-xs text-danger">
      {{ model.olderMessagesError }}
    </p>

    <div
      v-if="!model.chatMessages.length"
      class="flex h-full flex-col items-center justify-center gap-2 p-8 text-center text-sm text-muted-foreground"
    >
      <p class="text-xl font-medium text-foreground">
        Hello {{ model.displayUsername }}. Ready to dive in?
      </p>
    </div>

    <div
      v-for="message in model.chatMessages"
      :key="message.id"
      class="group/msg relative w-full"
    >
      <article
        :class="[
          'relative min-w-0',
          message.role === 'user'
            ? 'user-message-card ml-auto max-w-[66%] rounded-lg border px-3.5 py-2.5'
            : 'assistant-message-card py-1 pr-6',
        ]"
      >
        <header class="flex flex-wrap items-center gap-2">
          <template v-if="message.role === 'assistant'">
            <span
              class="rounded-sm border border-[rgb(124_134_255_/_0.3)] bg-[rgb(124_134_255_/_0.08)] px-2 py-[3px] font-mono text-[11px] text-[rgb(var(--accent-hi))]"
            >
              {{ model.agentNameFor(message) }}
            </span>
          </template>
          <span
            v-else
            class="rounded-sm border border-[rgb(var(--line-strong))] bg-surface-muted px-2 py-[3px] font-mono text-[11px] text-muted-foreground"
          >
            {{ model.labelForRole(message.role) }}
          </span>
          <span
            v-if="model.shouldShowResponseTimer(message)"
            class="inline-flex items-center gap-1 rounded-sm border px-2 py-[3px] font-mono text-[11px] tabular-nums"
            :class="
              message.streaming
                ? 'border-[rgb(124_134_255_/_0.3)] bg-[rgb(124_134_255_/_0.08)] text-[rgb(var(--accent-hi))]'
                : 'border-border bg-surface-muted text-faint-foreground'
            "
            :title="message.streaming ? 'Response time (running)' : 'Response time'"
          >
            {{ model.formatDuration(model.responseElapsedMs(message)) }}
          </span>
          <span
            v-if="message.streaming"
            class="flex items-center gap-1 font-mono text-[11px] uppercase tracking-[0.08em] text-[rgb(var(--accent-hi))]"
          >
            <span class="halo-pulse h-1.5 w-1.5 rounded-full bg-accent"></span>
            Streaming
          </span>
          <span
            v-if="message.error"
            class="rounded bg-danger px-2 py-0.5 text-[11px] font-semibold text-danger-foreground"
          >
            {{ message.error }}
          </span>
        </header>

        <div class="mt-3 space-y-3 break-words text-sm leading-relaxed text-foreground">
          <div
            v-if="model.hasDelegatedActivityForMessage(message.id)"
            class="parallel-activity-redirect"
          >
            Delegated specialist activity is shown in the participant list.
          </div>

          <div v-if="model.shouldShowDirectActivity(message)" class="direct-activity-wrapper">
            <Transition name="activity-pill">
              <button
                v-if="model.isActivityCollapsed(message.id)"
                type="button"
                class="direct-activity-pill"
                @click="model.expandActivity(message.id)"
              >
                <span class="direct-activity-pill-dot"></span>
                <span class="direct-activity-pill-label">{{ model.agentNameFor(message) }} activity</span>
                <span class="direct-activity-pill-chevron">›</span>
              </button>
            </Transition>

            <Transition
              @before-enter="model.drawerBeforeEnter"
              @enter="model.drawerEnter"
              @after-enter="model.drawerAfterEnter"
              @before-leave="model.drawerBeforeLeave"
              @leave="model.drawerLeave"
            >
              <div
                v-if="!model.isActivityCollapsed(message.id)"
                class="direct-activity"
              >
                <div class="direct-activity-header">
                  <span class="direct-activity-label">{{ model.agentNameFor(message) }} activity</span>
                  <button
                    v-if="!message.streaming"
                    type="button"
                    class="direct-activity-collapse-btn"
                    title="Collapse"
                    @click="model.collapseActivity(message.id)"
                  >
                    collapse ›
                  </button>
                  <span v-else class="direct-activity-streaming-dot"></span>
                </div>
                <div class="direct-activity-body">
                  <div v-if="message.activityToolTitle" class="direct-activity-row">
                    <span class="direct-activity-label">Tool</span>
                    <span class="direct-activity-value">
                      {{ message.activityToolTitle }}
                    </span>
                  </div>
                  <div v-if="model.shouldShowDirectThought(message)" class="direct-activity-thought">
                    <span class="direct-activity-label">Thought summary</span>
                    <div
                      class="chat-markdown direct-activity-summary"
                      v-html="model.renderMarkdownOrHtml(message.activityThoughtSummary || '')"
                    ></div>
                  </div>
                </div>
              </div>
            </Transition>
          </div>

          <div v-if="model.hasMemoryContext(message)" class="direct-activity-wrapper memory-context-wrapper">
            <Transition name="activity-pill">
              <button
                v-if="!model.isMemoryContextExpanded(message.id)"
                type="button"
                class="direct-activity-pill"
                :aria-expanded="false"
                @click="model.expandMemoryContext(message.id)"
              >
                <span class="direct-activity-pill-dot"></span>
                <span class="direct-activity-pill-label">
                  Retrieved memories
                  <span v-if="model.memoryContextPillMeta(message)" class="memory-context-pill-meta">
                    {{ model.memoryContextPillMeta(message) }}
                  </span>
                </span>
                <span class="direct-activity-pill-chevron">›</span>
              </button>
            </Transition>

            <Transition
              @before-enter="model.drawerBeforeEnter"
              @enter="model.drawerEnter"
              @after-enter="model.drawerAfterEnter"
              @before-leave="model.drawerBeforeLeave"
              @leave="model.drawerLeave"
            >
              <div
                v-if="model.isMemoryContextExpanded(message.id)"
                class="direct-activity memory-context-card"
              >
                <div class="direct-activity-header">
                  <span class="direct-activity-label">Retrieved memories</span>
                  <button
                    type="button"
                    class="direct-activity-collapse-btn"
                    :aria-expanded="true"
                    title="Collapse"
                    @click="model.collapseMemoryContext(message.id)"
                  >
                    collapse ›
                  </button>
                </div>
                <div class="direct-activity-body memory-context-body">
                  <div v-if="model.memoryContextPillMeta(message)" class="direct-activity-row">
                    <span class="direct-activity-label">Context</span>
                    <span class="direct-activity-value">
                      {{ model.memoryContextPillMeta(message) }}
                    </span>
                  </div>
                  <div class="direct-activity-thought">
                    <span class="direct-activity-label">Prompt memory</span>
                    <div
                      class="chat-markdown direct-activity-summary"
                      v-html="model.renderMarkdownOrHtml(message.memoryContext?.text || '')"
                    ></div>
                  </div>
                </div>
              </div>
            </Transition>
          </div>
          <p v-if="message.title" class="font-semibold text-foreground">
            {{ message.title }}
          </p>
          <div v-if="message.inputRequests?.length" class="input-request-list">
            <form
              v-for="request in message.inputRequests"
              :key="request.id"
              class="input-request-card"
              :class="model.inputRequestCardClasses(request)"
              @submit.prevent="model.submitInputRequest(message, request)"
            >
              <div class="input-request-header">
                <div class="min-w-0">
                  <p class="input-request-kicker">
                    {{ model.inputRequestStatusLabel(request) }}
                  </p>
                  <p class="input-request-agent">
                    {{ request.agent || model.agentNameFor(message) }}
                  </p>
                </div>
                <span
                  v-if="request.status === 'pending'"
                  class="input-request-live-dot"
                ></span>
              </div>
              <p class="input-request-question">
                {{ request.question }}
              </p>
              <p v-if="request.reason" class="input-request-reason">
                {{ request.reason }}
              </p>

              <div
                v-if="request.choices.length && model.isInputRequestRespondable(request)"
                class="input-request-choices"
              >
                <label
                  v-for="choice in request.choices"
                  :key="choice.id"
                  class="input-request-choice"
                >
                  <input
                    :type="request.multiple ? 'checkbox' : 'radio'"
                    :name="model.inputRequestFieldName(message, request)"
                    :checked="model.inputRequestChoiceSelected(message, request, choice.id)"
                    :disabled="model.isInputRequestSubmitting(message, request)"
                    @change="model.toggleInputRequestChoice(message, request, choice.id)"
                  />
                  <span class="min-w-0">
                    <span class="input-request-choice-label">
                      {{ choice.label }}
                    </span>
                    <span
                      v-if="choice.description"
                      class="input-request-choice-description"
                    >
                      {{ choice.description }}
                    </span>
                  </span>
                </label>
              </div>

              <textarea
                v-if="request.allowFreeText && model.isInputRequestRespondable(request)"
                :value="model.inputRequestDraft(message, request)"
                class="input-request-textarea"
                rows="3"
                placeholder="Tell the model what to do..."
                :disabled="model.isInputRequestSubmitting(message, request)"
                @input="
                  model.setInputRequestDraft(
                    message,
                    request,
                    (($event.target as HTMLTextAreaElement | null)?.value || ''),
                  )
                "
              ></textarea>

              <p
                v-if="model.inputRequestLocalError(message, request) || request.error"
                class="input-request-error"
              >
                {{ model.inputRequestLocalError(message, request) || request.error }}
              </p>

              <div v-if="request.status === 'answered'" class="input-request-answer">
                <span class="input-request-answer-label">Answered</span>
                <span class="input-request-answer-text">
                  {{ model.inputRequestAnswerSummary(request) }}
                </span>
              </div>

              <div v-if="model.isInputRequestRespondable(request)" class="input-request-actions">
                <button
                  type="submit"
                  class="input-request-submit"
                  :disabled="!model.canSubmitInputRequest(message, request)"
                >
                  {{
                    model.isInputRequestSubmitting(message, request)
                      ? "Submitting..."
                      : "Continue"
                  }}
                </button>
              </div>
            </form>
          </div>
          <pre
            v-if="message.toolArgs"
            class="whitespace-pre-wrap rounded-4 border border-border bg-surface-muted/60 p-3 text-xs text-subtle-foreground"
            >{{ message.toolArgs }}</pre
          >
          <div
            v-if="message.content"
            class="chat-markdown"
            v-html="model.renderMarkdownOrHtml(message.content)"
          ></div>
          <div v-if="message.attachments?.length" class="space-y-2">
            <div
              v-if="message.attachments.some((a) => a.kind === 'image')"
              class="flex gap-2 overflow-x-auto pb-1"
            >
              <img
                v-for="img in message.attachments.filter((a) => a.kind === 'image')"
                :key="img.id"
                :src="img.previewUrl"
                :alt="img.name"
                class="h-16 w-16 cursor-zoom-in rounded border border-border object-cover"
                @click="model.openImageModal(img)"
              />
            </div>
            <div
              v-if="message.attachments.some((a) => a.kind === 'video')"
              class="space-y-2"
            >
              <video
                v-for="video in message.attachments.filter((a) => a.kind === 'video')"
                :key="video.id"
                :src="video.previewUrl"
                controls
                preload="metadata"
                class="max-h-[360px] w-full rounded-4 border border-border bg-black"
              ></video>
            </div>
            <div
              v-if="message.attachments.some((a) => a.kind === 'text')"
              class="flex flex-wrap gap-2"
            >
              <span
                v-for="t in message.attachments.filter((a) => a.kind === 'text')"
                :key="t.id"
                class="inline-flex items-center gap-1 rounded-full border border-border bg-surface px-2 py-1 text-[11px]"
              >
                <span class="max-w-[180px] truncate">{{ t.name }}</span>
              </span>
            </div>
          </div>
          <audio v-if="message.audioUrl" :src="message.audioUrl" controls class="w-full"></audio>
          <video
            v-if="message.videoUrl"
            :src="message.videoUrl"
            controls
            preload="metadata"
            class="max-h-[360px] w-full rounded-4 border border-border bg-black"
          ></video>
        </div>
      </article>

      <div
        class="mt-1 flex items-center gap-1 text-xs text-subtle-foreground opacity-0 transition-opacity duration-150 group-hover/msg:opacity-100"
        :class="message.role === 'user' ? 'justify-end pr-1' : ''"
      >
        <button
          v-if="message.role === 'assistant'"
          type="button"
          class="rounded-4 px-2 py-1 transition hover:text-accent"
          :disabled="model.isStreaming || message.streaming"
          title="Regenerate"
          aria-label="Regenerate response"
          @click="model.regenerateAssistant(message)"
        >
          <SolarRefreshIcon class="h-4 w-4" />
        </button>
        <button
          v-if="model.canResumeDurableRun(message)"
          type="button"
          class="rounded-4 px-2 py-1 transition hover:text-accent"
          title="Resume run"
          aria-label="Resume run"
          @click="model.resumeDurableRun(message)"
        >
          Resume
        </button>
        <button
          v-if="message.role === 'assistant' && message.content"
          type="button"
          class="rounded-4 px-2 py-1 transition hover:text-accent"
          :title="model.copiedMessageId === message.id ? 'Copied' : 'Copy'"
          :aria-label="
            model.copiedMessageId === message.id ? 'Copied message' : 'Copy message'
          "
          @click="model.copyMessage(message)"
        >
          <SolarCopyIcon class="h-4 w-4" />
        </button>
        <button
          v-if="message.role === 'user' && message.content"
          type="button"
          class="rounded-4 px-2 py-1 transition hover:text-accent"
          :title="model.copiedMessageId === message.id ? 'Copied' : 'Copy'"
          :aria-label="
            model.copiedMessageId === message.id ? 'Copied message' : 'Copy message'
          "
          @click="model.copyMessage(message)"
        >
          <SolarCopyIcon class="h-4 w-4" />
        </button>
        <button
          v-if="(message.role === 'assistant' || message.role === 'user') && message.id"
          type="button"
          class="rounded-4 px-2 py-1 transition hover:text-accent"
          :disabled="model.isStreaming || message.streaming"
          title="Delete"
          aria-label="Delete message"
          @click="model.deleteChatMessage(message)"
        >
          <SolarTrashIcon class="h-4 w-4" />
        </button>
      </div>
    </div>
  </div>

  <button
    type="button"
    class="absolute bottom-20 right-8 z-10 inline-flex h-8 w-8 items-center justify-center rounded-full border border-border/60 bg-surface/95 text-subtle-foreground shadow-1 transition-all duration-150 hover:border-accent/50 hover:text-accent focus-visible:shadow-outline"
    :class="
      model.showScrollToBottom
        ? 'pointer-events-auto translate-y-0 opacity-100'
        : 'pointer-events-none translate-y-1 opacity-0'
    "
    title="Scroll to latest"
    aria-label="Scroll to latest"
    @click="model.handleScrollToLatest"
  >
    <SolarListArrowDownIcon class="h-4 w-4" />
  </button>
</template>

<script setup lang="ts">
import { computed } from "vue";
import SolarCopyIcon from "@/components/icons/SolarCopy.vue";
import SolarListArrowDownIcon from "@/components/icons/Expand.vue";
import SolarRefreshIcon from "@/components/icons/SolarRefresh.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import type { ChatTranscriptModel } from "@/composables/chat/useChatViewController";

const props = defineProps<{
  model: ChatTranscriptModel;
}>();

const messagesPaneEl = computed({
  get: () => null,
  set: (value) => props.model.setMessagesPaneRef(value as Element | null),
});
</script>
