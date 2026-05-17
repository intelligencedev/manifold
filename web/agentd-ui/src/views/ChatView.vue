<template>
  <div class="flex h-full min-h-0 flex-1 overflow-hidden chat-modern">
    <section
      class="grid h-full min-h-0 flex-1 grid-cols-[300px_minmax(0,1fr)_260px] overflow-hidden chat-grid"
    >
      <!-- Sessions sidebar -->
      <aside
        class="flex h-full min-h-0 flex-col gap-3 overflow-hidden border-r border-border/60 p-4 pr-5"
      >
        <header class="flex items-center justify-between gap-2">
          <h2 class="text-sm font-semibold text-foreground">Conversations</h2>
          <div class="flex items-center gap-1.5">
            <template v-if="!sessionSelectMode">
              <button
                type="button"
                class="rounded-4 border border-border px-2 py-1 text-xs font-semibold text-foreground transition hover:border-border/80 hover:text-foreground/80"
                title="Select conversations to delete"
                @click="enterSessionSelectMode"
              >
                Select
              </button>
              <button
                type="button"
                class="rounded-4 border border-border px-2 py-1 text-xs font-semibold text-foreground transition hover:border-accent hover:text-accent"
                @click="createSession()"
              >
                New
              </button>
            </template>
            <template v-else>
              <button
                type="button"
                class="rounded-4 border px-2 py-1 text-xs font-semibold transition"
                :class="selectedSessionIds.length > 0
                  ? 'border-danger/60 bg-danger/10 text-danger hover:bg-danger/20'
                  : 'border-border/40 text-faint-foreground cursor-not-allowed opacity-50'"
                :disabled="selectedSessionIds.length === 0"
                @click="openBulkDeleteDialog"
              >
                Delete{{ selectedSessionIds.length > 0 ? ` (${selectedSessionIds.length})` : '' }}
              </button>
              <button
                type="button"
                class="rounded-4 border border-border px-2 py-1 text-xs font-semibold text-foreground transition hover:border-accent hover:text-accent"
                @click="exitSessionSelectMode"
              >
                Cancel
              </button>
            </template>
          </div>
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
              sessionSelectMode && selectedSessionIds.includes(session.id)
                ? 'border-danger/70 bg-danger/20'
                : !sessionSelectMode && session.id === activeSessionId
                  ? 'border-accent/70 bg-surface-muted/60'
                  : 'hover:border-border hover:bg-surface-muted/40'
            "
            @click="sessionSelectMode ? toggleSessionSelection(session.id) : selectSession(session.id)"
          >
            <div class="flex items-center justify-between gap-2">
              <template v-if="sessionSelectMode">
                <p class="truncate font-medium text-foreground flex-1">{{ session.name }}</p>
              </template>
              <template v-else-if="renamingSessionId === session.id">
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
              v-if="!sessionSelectMode"
              class="mt-2 flex items-center justify-between text-[10px] text-faint-foreground"
            >
              <div class="flex items-center gap-2">
                <span
                  class="whitespace-nowrap rounded-full border border-border/60 bg-surface px-2 py-0.5 text-[10px] text-subtle-foreground"
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
                  class="rounded px-1 text-[10px] text-subtle-foreground opacity-0 transition group-hover:opacity-100 hover:text-accent"
                  title="Export conversation"
                  @click.stop="exportSession(session.id)"
                >
                  <SolarDownloadIcon class="inline-block h-3 w-3" />
                </button>
                <button
                  type="button"
                  class="inline-flex h-6 w-6 items-center justify-center text-danger opacity-0 transition group-hover:opacity-100 hover:text-danger/80"
                  :title="`Delete conversation ${session.name}`"
                  :aria-label="`Delete conversation ${session.name}`"
                  @click.stop="openDeleteSessionDialog(session)"
                >
                  <SolarTrashIcon class="h-3.5 w-3.5" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </aside>

      <!-- Chat pane -->
      <section
        class="relative flex h-full min-h-0 flex-col overflow-hidden px-5 chat-pane"
      >
        <header
          class="flex flex-wrap items-center justify-between gap-3 border-b border-border px-4 pb-4 pt-1"
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
            <div class="flex items-center gap-2 -mt-1.5">
              <div class="flex flex-col items-start gap-1">
                <span
                  class="ml-1 text-[10px] font-medium leading-none text-faint-foreground"
                >
                  Project
                </span>
                <DropdownSelect
                  v-model="selectedProjectId"
                  :options="projectOptions"
                  size="xs"
                  title="Project for this conversation"
                  aria-label="Project"
                  class="min-w-[180px]"
                />
              </div>
              <!-- Render mode dropdown retained for future use.
              <div class="flex flex-col items-start gap-1">
                <span
                  class="ml-1 text-[10px] font-medium leading-none text-faint-foreground"
                >
                  Render mode
                </span>
                <DropdownSelect
                  v-model="renderMode"
                  :options="renderModeOptions"
                  size="xs"
                  title="Render mode for assistant responses"
                  aria-label="Render mode"
                />
              </div>
              -->
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
            <p class="text-xl font-medium text-foreground">
              Hello {{ displayUsername }}. Ready to dive in?
            </p>
          </div>

          <div
            v-for="message in chatMessages"
            :key="message.id"
            class="group/msg relative flex w-full flex-col"
            :class="message.role === 'user' ? 'items-end' : 'items-center'"
          >
            <article
              class="relative w-full max-w-[72ch] rounded-[var(--radius,18px)] p-5"
              :class="message.role === 'user' ? 'glass-surface border border-white/12' : ''"
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
                <!-- Parallel specialist activity (sub-agents invoked concurrently) -->
                <div
                  v-if="message.id === lastAssistantId && visibleParticipantActivityItems.length > 0"
                  class="parallel-activity-grid"
                  :class="visibleParticipantActivityItems.length <= 2 ? 'parallel-activity-grid--row' : 'parallel-activity-grid--col'"
                >
                  <div
                    v-for="thread in visibleParticipantActivityItems"
                    :key="thread.id"
                    class="parallel-activity-card"
                  >
                    <Transition name="activity-pill">
                      <button
                        v-if="isActivityCollapsed(thread.id)"
                        type="button"
                        class="direct-activity-pill"
                        @click="expandActivity(thread.id)"
                      >
                        <span
                          class="direct-activity-pill-dot"
                          :class="thread.status === 'running' ? 'direct-activity-pill-dot--live' : ''"
                        ></span>
                        <span class="direct-activity-pill-label">{{ thread.name }}</span>
                        <span class="direct-activity-pill-chevron">›</span>
                      </button>
                    </Transition>
                    <Transition
                      @before-enter="drawerBeforeEnter"
                      @enter="drawerEnter"
                      @after-enter="drawerAfterEnter"
                      @before-leave="drawerBeforeLeave"
                      @leave="drawerLeave"
                    >
                      <div
                        v-if="!isActivityCollapsed(thread.id)"
                        class="direct-activity"
                      >
                        <div class="direct-activity-header">
                          <span class="direct-activity-label">{{ thread.name }}</span>
                          <button
                            v-if="thread.status !== 'running'"
                            type="button"
                            class="direct-activity-collapse-btn"
                            @click="collapseActivity(thread.id)"
                            title="Collapse"
                          >collapse ›</button>
                          <span v-else class="direct-activity-streaming-dot"></span>
                        </div>
                        <div
                          class="direct-activity-body"
                          :ref="(el) => registerThreadBody(el as Element | null, thread.id)"
                          @scroll="handleThreadBodyScroll($event, thread.id)"
                        >
                        <div
                          v-if="thread.toolEntries.length"
                          class="direct-activity-row"
                        >
                          <span class="direct-activity-label">Tool</span>
                          <span class="direct-activity-value">
                            {{ thread.toolEntries[thread.toolEntries.length - 1]?.title || '' }}
                          </span>
                        </div>
                        <div
                          v-if="thread.thoughtSummaries.length"
                          class="direct-activity-thought"
                        >
                          <span class="direct-activity-label">Thought summary</span>
                          <div
                            class="chat-markdown direct-activity-summary"
                            v-html="renderMarkdownOrHtml(thread.thoughtSummaries[thread.thoughtSummaries.length - 1] || '')"
                          ></div>
                        </div>
                        <div
                          v-if="thread.response && thread.status !== 'running'"
                          class="direct-activity-thought"
                        >
                          <span class="direct-activity-label">Response</span>
                          <div
                            class="chat-markdown direct-activity-summary"
                            v-html="renderMarkdownOrHtml(thread.response)"
                          ></div>
                        </div>
                        </div>
                      </div>
                    </Transition>
                  </div>
                </div>

                <div
                  v-if="shouldShowDirectActivity(message)"
                  class="direct-activity-wrapper"
                >
                  <!-- Collapsed pill: click to expand -->
                  <Transition name="activity-pill">
                  <button
                    v-if="isActivityCollapsed(message.id)"
                    type="button"
                    class="direct-activity-pill"
                    @click="expandActivity(message.id)"
                  >
                    <span class="direct-activity-pill-dot"></span>
                    <span class="direct-activity-pill-label">{{ agentNameFor(message) }} activity</span>
                    <span class="direct-activity-pill-chevron">›</span>
                  </button>
                  </Transition>

                  <!-- Expanded panel (drawer animation) -->
                  <Transition
                    @before-enter="drawerBeforeEnter"
                    @enter="drawerEnter"
                    @after-enter="drawerAfterEnter"
                    @before-leave="drawerBeforeLeave"
                    @leave="drawerLeave"
                  >
                  <div
                    v-if="!isActivityCollapsed(message.id)"
                    class="direct-activity"
                  >
                    <!-- Header row: name + collapse button -->
                    <div class="direct-activity-header">
                      <span class="direct-activity-label">{{ agentNameFor(message) }} activity</span>
                      <button
                        v-if="!message.streaming"
                        type="button"
                        class="direct-activity-collapse-btn"
                        @click="collapseActivity(message.id)"
                        title="Collapse"
                      >collapse ›</button>
                      <span v-else class="direct-activity-streaming-dot"></span>
                    </div>
                    <div class="direct-activity-body">
                    <div
                      v-if="message.activityToolTitle"
                      class="direct-activity-row"
                    >
                      <span class="direct-activity-label">Tool</span>
                      <span class="direct-activity-value">
                        {{ message.activityToolTitle }}
                      </span>
                    </div>
                    <div
                      v-if="shouldShowDirectThought(message)"
                      class="direct-activity-thought"
                    >
                      <span class="direct-activity-label">Thought summary</span>
                      <div
                        class="chat-markdown direct-activity-summary"
                        v-html="renderMarkdownOrHtml(message.activityThoughtSummary || '')"
                      ></div>
                    </div>
                    </div>
                  </div>
                  </Transition>
                </div>
                <p v-if="message.title" class="font-semibold text-foreground">
                  {{ message.title }}
                </p>
                <div
                  v-if="message.inputRequests?.length"
                  class="input-request-list"
                >
                  <form
                    v-for="request in message.inputRequests"
                    :key="request.id"
                    class="input-request-card"
                    :class="inputRequestCardClasses(request)"
                    @submit.prevent="submitInputRequest(message, request)"
                  >
                    <div class="input-request-header">
                      <div class="min-w-0">
                        <p class="input-request-kicker">
                          {{ inputRequestStatusLabel(request) }}
                        </p>
                        <p class="input-request-agent">
                          {{ request.agent || agentNameFor(message) }}
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
                      v-if="request.choices.length && isInputRequestRespondable(request)"
                      class="input-request-choices"
                    >
                      <label
                        v-for="choice in request.choices"
                        :key="choice.id"
                        class="input-request-choice"
                      >
                        <input
                          :type="request.multiple ? 'checkbox' : 'radio'"
                          :name="inputRequestFieldName(message, request)"
                          :checked="inputRequestChoiceSelected(message, request, choice.id)"
                          :disabled="isInputRequestSubmitting(message, request)"
                          @change="toggleInputRequestChoice(message, request, choice.id)"
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
                      v-if="request.allowFreeText && isInputRequestRespondable(request)"
                      v-model="inputRequestDrafts[inputRequestKey(message, request)]"
                      class="input-request-textarea"
                      rows="3"
                      placeholder="Type your response..."
                      :disabled="isInputRequestSubmitting(message, request)"
                    ></textarea>

                    <p
                      v-if="inputRequestLocalError(message, request) || request.error"
                      class="input-request-error"
                    >
                      {{ inputRequestLocalError(message, request) || request.error }}
                    </p>

                    <div
                      v-if="request.status === 'answered'"
                      class="input-request-answer"
                    >
                      <span class="input-request-answer-label">Answered</span>
                      <span class="input-request-answer-text">
                        {{ inputRequestAnswerSummary(request) }}
                      </span>
                    </div>

                    <div
                      v-if="isInputRequestRespondable(request)"
                      class="input-request-actions"
                    >
                      <button
                        type="submit"
                        class="input-request-submit"
                        :disabled="!canSubmitInputRequest(message, request)"
                      >
                        {{
                          isInputRequestSubmitting(message, request)
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
                <!-- Thought summaries are streamed into the Active Specialist panel. -->
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
            </article>

            <!-- Action toolbar: outside article, visible only on hover -->
            <div
              class="flex items-center gap-1 text-xs text-subtle-foreground mt-1 opacity-0 group-hover/msg:opacity-100 transition-opacity duration-150"
            >
              <button
                v-if="message.role === 'assistant'"
                type="button"
                class="rounded-4 px-2 py-1 transition hover:text-accent"
                :disabled="isStreaming || message.streaming"
                @click="regenerateAssistant(message)"
                title="Regenerate"
                aria-label="Regenerate response"
              >
                <SolarRefreshIcon class="h-4 w-4" />
              </button>
              <button
                v-if="message.role === 'assistant' && message.content"
                type="button"
                class="rounded-4 px-2 py-1 transition hover:text-accent"
                @click="copyMessage(message)"
                :title="copiedMessageId === message.id ? 'Copied' : 'Copy'"
                :aria-label="
                  copiedMessageId === message.id
                    ? 'Copied message'
                    : 'Copy message'
                "
              >
                <SolarCopyIcon class="h-4 w-4" />
              </button>
              <button
                v-if="message.role === 'user' && message.content"
                type="button"
                class="rounded-4 px-2 py-1 transition hover:text-accent"
                @click="copyMessage(message)"
                :title="copiedMessageId === message.id ? 'Copied' : 'Copy'"
                :aria-label="
                  copiedMessageId === message.id
                    ? 'Copied message'
                    : 'Copy message'
                "
              >
                <SolarCopyIcon class="h-4 w-4" />
              </button>
              <button
                v-if="
                  (message.role === 'assistant' || message.role === 'user') &&
                  message.id
                "
                type="button"
                class="rounded-4 px-2 py-1 transition hover:text-accent"
                :disabled="isStreaming || message.streaming"
                @click="deleteChatMessage(message)"
                title="Delete"
                aria-label="Delete message"
              >
                <SolarTrashIcon class="h-4 w-4" />
              </button>
            </div>
          </div>
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
                v-if="mentionMenuOpen"
                class="absolute bottom-full left-3 mb-2 w-72 overflow-hidden rounded-4 border border-border bg-surface shadow-3 ring-1 ring-border/50 z-20"
              >
                <div class="max-h-60 overflow-y-auto py-1">
                  <button
                    v-for="(cand, i) in mentionCandidates"
                    :key="cand.name"
                    type="button"
                    class="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-xs transition"
                    :class="
                      i === mentionActiveIndex
                        ? 'bg-surface-muted/60 text-foreground'
                        : 'text-subtle-foreground hover:bg-surface-muted/40 hover:text-foreground'
                    "
                    @mousedown.prevent="selectMentionCandidate(cand.name)"
                  >
                    <span class="truncate font-medium">@{{ cand.name }}</span>
                    <span class="shrink-0 text-[10px] text-faint-foreground">
                      {{ cand.model ? `Model ${cand.model}` : "" }}
                    </span>
                  </button>

                  <div
                    v-if="!mentionCandidates.length"
                    class="px-3 py-2 text-xs text-faint-foreground"
                  >
                    No matching specialists
                  </div>
                </div>
              </div>

              <div
                class="flex items-center gap-3"
              >
                <textarea
                  ref="composer"
                  v-model="draft"
                  rows="1"
                  class="flex-1 min-w-0 resize-none bg-transparent py-1.5 text-sm leading-6 text-foreground outline-none placeholder:text-faint-foreground"
                  :placeholder="
                    hasPendingInputRequest
                      ? 'Answer the request above to continue.'
                      : projectSelected
                      ? 'Message the agent...'
                      : 'Select a project to enable the chat.'
                  "
                  :disabled="!projectSelected || hasPendingInputRequest"
                  @keydown="handleComposerKeydown"
                  @input="handleComposerInput"
                  @keyup="handleComposerKeyup"
                  @click="updateMentionState"
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
                    :disabled="!projectSelected || hasPendingInputRequest"
                    :class="
                      !projectSelected || hasPendingInputRequest
                        ? 'opacity-50 cursor-not-allowed text-foreground/40'
                        : 'text-foreground/80 hover:text-accent'
                    "
                    @click="projectSelected && !hasPendingInputRequest ? fileInput?.click() : undefined"
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
                      isStreaming ||
                      !canUseMic ||
                      !projectSelected ||
                      hasPendingInputRequest
                        ? 'opacity-50 cursor-not-allowed'
                        : '',
                    ]"
                    :disabled="isStreaming || !canUseMic || !projectSelected || hasPendingInputRequest"
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
                      isStreaming || !projectSelected || hasPendingInputRequest
                        ? 'opacity-50 cursor-not-allowed'
                        : '',
                    ]"
                    :disabled="isStreaming || !projectSelected || hasPendingInputRequest"
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
                    :title="
                      isStreaming && !hasPendingInputRequest && (draft.trim() || pendingAttachments.length)
                        ? 'Send message'
                        : isStreaming
                          ? 'Stop generating'
                          : 'Send message'
                    "
                    :aria-label="
                      isStreaming && !hasPendingInputRequest && (draft.trim() || pendingAttachments.length)
                        ? 'Send message'
                        : isStreaming
                          ? 'Stop generating'
                          : 'Send message'
                    "
                    @click="
                      isStreaming && !hasPendingInputRequest && (draft.trim() || pendingAttachments.length)
                        ? sendCurrentPrompt()
                        : isStreaming
                          ? stopStreaming()
                          : sendCurrentPrompt()
                    "
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

      <div
        v-if="showDeleteSessionDialog && deleteSessionTarget"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-session-title"
        @click.self="closeDeleteSessionDialog"
        @keydown.esc.prevent="closeDeleteSessionDialog"
      >
        <div
          class="w-full max-w-md rounded-5 bg-surface p-5 shadow-3 ring-1 ring-border/60"
        >
          <h2
            id="delete-session-title"
            class="text-base font-semibold text-danger"
          >
            Delete Conversation
          </h2>
          <p class="mt-2 text-sm text-subtle-foreground">
            This permanently removes
            <span class="font-semibold text-foreground">{{
              deleteSessionTarget.name
            }}</span>
            and all messages in it.
          </p>
          <form class="mt-4 space-y-3" @submit.prevent="confirmDeleteSession">
            <p class="text-xs text-faint-foreground">
              This action cannot be undone.
            </p>
            <p v-if="deleteSessionError" class="text-xs text-danger">
              {{ deleteSessionError }}
            </p>
            <div class="flex items-center justify-end gap-2">
              <button
                type="button"
                class="h-9 rounded-full border border-white/15 px-3 text-sm text-subtle-foreground transition hover:border-white/30 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="deleteSessionPending"
                @click="closeDeleteSessionDialog"
              >
                Cancel
              </button>
              <button
                type="submit"
                class="h-9 rounded-full border border-danger/60 bg-danger/10 px-3 text-sm font-semibold text-danger transition hover:bg-danger/20 disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="!canConfirmDeleteSession"
              >
                {{
                  deleteSessionPending ? "Deleting..." : "Delete Conversation"
                }}
              </button>
            </div>
          </form>
        </div>
      </div>

      <!-- Bulk delete dialog -->
      <div
        v-if="showBulkDeleteDialog"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
        role="dialog"
        aria-modal="true"
        aria-labelledby="bulk-delete-title"
        @click.self="closeBulkDeleteDialog"
        @keydown.esc.prevent="closeBulkDeleteDialog"
      >
        <div class="w-full max-w-md rounded-5 bg-surface p-5 shadow-3 ring-1 ring-border/60">
          <h2 id="bulk-delete-title" class="text-base font-semibold text-danger">
            Delete {{ selectedSessionIds.length }} Conversation{{ selectedSessionIds.length === 1 ? '' : 's' }}
          </h2>
          <p class="mt-2 text-sm text-subtle-foreground">
            This permanently removes the selected
            {{ selectedSessionIds.length === 1 ? 'conversation' : `${selectedSessionIds.length} conversations` }}
            and all messages in them.
          </p>
          <form class="mt-4 space-y-3" @submit.prevent="confirmBulkDelete">
            <p class="text-xs text-faint-foreground">This action cannot be undone.</p>
            <p v-if="bulkDeleteError" class="text-xs text-danger">{{ bulkDeleteError }}</p>
            <div class="flex items-center justify-end gap-2">
              <button
                type="button"
                class="h-9 rounded-full border border-white/15 px-3 text-sm text-subtle-foreground transition hover:border-white/30 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="bulkDeletePending"
                @click="closeBulkDeleteDialog"
              >
                Cancel
              </button>
              <button
                type="submit"
                class="h-9 rounded-full border border-danger/60 bg-danger/10 px-3 text-sm font-semibold text-danger transition hover:bg-danger/20 disabled:cursor-not-allowed disabled:opacity-60"
                :disabled="bulkDeletePending"
              >
                {{ bulkDeletePending ? 'Deleting...' : `Delete ${selectedSessionIds.length === 1 ? 'Conversation' : 'Conversations'}` }}
              </button>
            </div>
          </form>
        </div>
      </div>

      <!-- Participants sidebar -->
      <aside
        class="flex h-full min-h-0 flex-col border-l border-border/60 pl-5 text-sm text-subtle-foreground chat-side"
      >
        <div class="flex min-h-0 flex-1 flex-col">
          <GlassCard flat class="flex min-h-0 flex-1 flex-col overflow-hidden">
            <div class="flex min-h-0 flex-1 flex-col">
              <header class="flex items-center justify-between">
                <h2 class="text-sm font-semibold text-foreground">
                  Participants
                </h2>
                <span class="text-[11px] text-faint-foreground">
                  {{ participantList.length }} available
                </span>
              </header>
              <div class="mt-2">
                <DropdownSelect
                  v-model="selectedTeam"
                  :options="teamOptions"
                  size="xs"
                  title="Team for this conversation"
                  aria-label="Specialist team"
                  class="w-full"
                />
              </div>
              <div class="mt-2 flex-1 min-h-0 overflow-y-auto">
                <div
                  v-if="!participantList.length"
                  class="rounded-4 border border-dashed border-border bg-surface p-3 text-xs text-subtle-foreground"
                >
                  No specialists available
                </div>
                <ul v-else class="participant-list">
                  <li
                    v-for="participant in participantList"
                    :key="participant.name"
                    class="participant-list-item"
                  >
                      <button
                        type="button"
                        class="participant-row"
                        :class="participantRowClasses(participant.name)"
                        :aria-label="`Open activity for ${participant.name}`"
                        @click="openParticipantActivity(participant.name)"
                      >
                        <span
                          class="participant-dot"
                          :class="participantDotClasses(participant.name)"
                        ></span>
                        <span class="participant-body">
                          <span class="participant-name">{{ participant.name }}</span>
                          <span class="participant-model">
                            {{
                              participant.model
                                ? `${participant.model}`
                                : "Model pending"
                            }}
                          </span>
                        </span>
                        <span class="participant-status">
                          {{ participantStatusLabel(participant.name) }}
                        </span>
                        <span
                          v-if="participantActivityItems(participant.name).length"
                          class="participant-activity-action"
                        >
                          View activity
                        </span>
                      </button>
                  </li>
                </ul>
              </div>
            </div>
          </GlassCard>
        </div>
      </aside>
    </section>

    <div
      v-if="selectedParticipantActivityName"
      class="activity-modal-backdrop"
      role="dialog"
      aria-modal="true"
      :aria-label="`${selectedParticipantActivity?.name || 'Specialist'} activity`"
      @click.self="closeParticipantActivity"
    >
      <div class="activity-modal">
        <header class="activity-modal-header">
          <div class="min-w-0">
            <p class="activity-modal-title">
              {{ selectedParticipantActivity?.name || "Specialist activity" }}
            </p>
            <p class="activity-modal-model">
              {{ selectedParticipantActivity?.model || "Model pending" }}
            </p>
          </div>
          <button
            type="button"
            class="activity-modal-close"
            aria-label="Close specialist activity"
            @click="closeParticipantActivity"
          >
            Close
          </button>
        </header>

        <div
          ref="participantActivityPane"
          class="activity-detail-scroll activity-modal-scroll"
          @scroll="handleActivityPaneScroll"
        >
          <section
            v-for="item in selectedParticipantActivityItems"
            :key="item.id"
            class="activity-detail-section"
          >
            <div v-if="item.toolEntries.length">
              <h3 class="activity-detail-section-title">
                Tool activity
              </h3>
              <ul class="activity-tool-list">
                <li
                  v-for="entry in item.toolEntries"
                  :key="entry.id"
                  class="activity-tool-item"
                >
                  <p class="activity-tool-title">
                    {{ entry.title || "Tool" }}
                  </p>
                </li>
              </ul>
            </div>

            <div
              v-if="item.thoughtSummaries.length"
              class="activity-detail-subsection"
            >
              <h3 class="activity-detail-section-title">
                Thought summaries
              </h3>
              <ul class="activity-thought-list text-foreground">
                <li
                  v-for="(summary, idx) in item.thoughtSummaries"
                  :key="`${item.id}:summary:${idx}:${summary}`"
                  class="activity-thought-item"
                >
                  <div
                    class="chat-markdown min-w-0 flex-1 break-words"
                    v-html="renderMarkdownOrHtml(summary)"
                  ></div>
                </li>
              </ul>
            </div>

            <div
              v-if="item.response"
              class="activity-detail-subsection"
            >
              <h3 class="activity-detail-section-title">
                Response stream
              </h3>
              <div
                class="chat-markdown activity-response"
                v-html="renderMarkdownOrHtml(item.response)"
              ></div>
            </div>

            <div
              v-if="item.error"
              class="activity-detail-subsection"
            >
              <h3 class="activity-detail-section-title">
                Error
              </h3>
              <p class="activity-error-text">
                {{ item.error }}
              </p>
            </div>

            <div
              v-if="!item.toolEntries.length && !item.thoughtSummaries.length && !item.response && !item.error"
              class="activity-detail-empty"
            >
              No activity details yet.
            </div>
          </section>

          <div
            v-if="!selectedParticipantActivityItems.length"
            class="activity-detail-empty"
          >
            No specialist activity yet.
          </div>
        </div>
      </div>
    </div>
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
  ChatInputRequest,
  ChatMessage,
  ChatSessionMeta,
  ChatRole,
} from "@/types/chat";
import { useQuery } from "@tanstack/vue-query";
import {
  listTeams,
  listSpecialists,
  type Specialist,
  type SpecialistTeam,
} from "@/api/client";
import { renderMarkdown } from "@/utils/markdown";
import { resolveLeadingSpecialistMention } from "@/utils/chatMentions";
import "highlight.js/styles/github-dark-dimmed.css";
import SolarPaperclip2Bold from "@/components/icons/SolarPaperclip2Bold.vue";
import SolarMicrophone3Bold from "@/components/icons/SolarMicrophone3Bold.vue";
import SolarArrowToTopLeftBold from "@/components/icons/SolarArrowToTopLeftBold.vue";
import SolarStopBold from "@/components/icons/SolarStopBold.vue";
import SolarCopyIcon from "@/components/icons/SolarCopy.vue";
import SolarTrashIcon from "@/components/icons/SolarTrash.vue";
import SolarRefreshIcon from "@/components/icons/SolarRefresh.vue";
import SolarDownloadIcon from "@/components/icons/SolarDownload.vue";
import Camera from "@/components/icons/Camera.vue";
import DropdownSelect from "@/components/DropdownSelect.vue";
import GlassCard from "@/components/ui/GlassCard.vue";
import { useChatStore } from "@/stores/chat";
import { useProjectsStore } from "@/stores/projects";
import type { DropdownOption } from "@/types/dropdown";

const router = useRouter();
const isBrowser = typeof window !== "undefined";
const SCROLL_LOCK_THRESHOLD = 80;
let previousBodyOverflow: string | null = null;

const chat = useChatStore();
const proj = useProjectsStore();

type CurrentUser = { name?: string; email?: string; picture?: string };
const currentUser = ref<CurrentUser | null>(null);

async function loadCurrentUser() {
  try {
    const res = await fetch("/api/me", { credentials: "include" });
    if (res.ok) {
      currentUser.value = await res.json();
      return;
    }
  } catch (_) {
    // ignore
  }

  const g = (window as any).__MANIFOLD_USER__;
  if (g) currentUser.value = g;
}

function usernameFromUser(user: CurrentUser | null): string | null {
  const name = user?.name?.trim();
  if (name) return name;

  const email = user?.email?.trim();
  if (!email) return null;

  const at = email.indexOf("@");
  return at > 0 ? email.slice(0, at) : email;
}

const displayUsername = computed(
  () => usernameFromUser(currentUser.value) || "there",
);

onMounted(() => {
  void loadCurrentUser();
  void proj.refresh();
  if (isBrowser) {
    previousBodyOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
});
const projects = computed(() => proj.projects);
const selectedProjectBySession = ref<Record<string, string>>({});
const selectedProjectId = computed({
  get: () => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return "";
    return selectedProjectBySession.value[sessionId] || "";
  },
  set: (v: string) => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return;
    selectedProjectBySession.value = {
      ...selectedProjectBySession.value,
      [sessionId]: v,
    };
  },
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
const copiedThoughtSummaries = ref(false);
const selectedActivityId = ref<string | null>(null);
const autoScrollEnabled = ref(true);
const lastScrollTop = ref(0);
const activityAutoScrollEnabled = ref(true);
const activityLastScrollTop = ref(0);
// Per-thread scroll state for parallel activity cards
const threadBodyEls = new Map<string, HTMLElement>();
const threadScrollEnabled = new Map<string, boolean>();
const threadScrollLastTop = new Map<string, number>();
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
const inputRequestDrafts = ref<Record<string, string>>({});
const inputRequestSelections = ref<Record<string, string[]>>({});
const inputRequestSubmitting = ref<Record<string, boolean>>({});
const inputRequestErrors = ref<Record<string, string>>({});
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
const showDeleteSessionDialog = ref(false);
const deleteSessionTarget = ref<ChatSessionMeta | null>(null);
const deleteSessionPending = ref(false);
const deleteSessionError = ref("");
const canConfirmDeleteSession = computed(
  () => !!deleteSessionTarget.value?.id && !deleteSessionPending.value,
);

// Multi-select state
const sessionSelectMode = ref(false);
const selectedSessionIds = ref<string[]>([]);
const showBulkDeleteDialog = ref(false);
const bulkDeletePending = ref(false);
const bulkDeleteError = ref("");
const participantActivityPane = ref<HTMLElement | null>(null);
const selectedParticipantActivityName = ref<string | null>(null);

// Specialists dropdown state
const { data: specialistsData } = useQuery({
  queryKey: ["specialists"],
  queryFn: listSpecialists,
  staleTime: 5_000,
});
const { data: teamsData } = useQuery({
  queryKey: ["teams"],
  queryFn: listTeams,
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
const teamsByName = computed(() => {
  const map = new Map<string, SpecialistTeam>();
  (teamsData?.value || []).forEach((t: SpecialistTeam) => {
    const key = t.name?.trim().toLowerCase();
    if (key) map.set(key, t);
  });
  return map;
});

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

watch(
  [activeSessionId, projects],
  ([sessionId, projectList]) => {
    if (!sessionId) return;
    if (selectedProjectBySession.value[sessionId]) return;
    const fallback = projectList[0]?.id;
    if (!fallback) return;
    selectedProjectBySession.value = {
      ...selectedProjectBySession.value,
      [sessionId]: fallback,
    };
  },
  { immediate: true },
);

// Transform render mode options for dropdown
const renderModeOptions = computed<DropdownOption[]>(() => [
  { id: "markdown", label: "markdown", value: "markdown" },
  { id: "html", label: "html", value: "html" },
]);

const teamOptions = computed<DropdownOption[]>(() => {
  const teams = (teamsData?.value || [])
    .map((t) => ({
      id: t.name,
      label: t.name,
      value: t.name,
    }))
    .filter((t) => (t.value || "").trim())
    .sort((a, b) =>
      a.label.localeCompare(b.label, undefined, { sensitivity: "base" }),
    );
  return [{ id: "", label: "All specialists", value: "" }, ...teams];
});

const selectedSpecialistBySession = ref<Record<string, string>>({});
const selectedSpecialist = computed({
  get: () => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return "orchestrator";
    return selectedSpecialistBySession.value[sessionId] || "orchestrator";
  },
  set: (value: string) => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return;
    const next = (value || "orchestrator").trim() || "orchestrator";
    selectedSpecialistBySession.value = {
      ...selectedSpecialistBySession.value,
      [sessionId]: next,
    };
  },
});

const selectedTeamBySession = ref<Record<string, string>>({});
const selectedTeam = computed({
  get: () => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return "";
    return selectedTeamBySession.value[sessionId] || "";
  },
  set: (value: string) => {
    const sessionId = activeSessionId.value;
    if (!sessionId) return;
    const next = (value || "").trim();
    selectedTeamBySession.value = {
      ...selectedTeamBySession.value,
      [sessionId]: next,
    };
  },
});
const selectedTeamConfig = computed(() => {
  const name = (selectedTeam.value || "").trim().toLowerCase();
  if (!name) return null;
  return teamsByName.value.get(name) || null;
});
const selectedTeamMembers = computed(() => {
  const team = selectedTeamConfig.value;
  if (!team) return new Set<string>();
  return new Set(
    (team.members || []).map((m) => m.trim().toLowerCase()).filter(Boolean),
  );
});

// --- @mention specialist picker (Slack-like) ---
const mentionQuery = ref("");
const mentionTokenStart = ref<number | null>(null);
const mentionTokenEnd = ref<number | null>(null);
const mentionActiveIndex = ref(0);

const mentionCandidates = computed<Participant[]>(() => {
  const q = (mentionQuery.value || "").trim().toLowerCase();
  const base = participantList.value;
  if (!q) return base;
  return base.filter((p) => p.name.toLowerCase().includes(q));
});

const mentionMenuOpen = computed(() => {
  if (!projectSelected.value) return false;
  return mentionTokenStart.value != null && mentionTokenEnd.value != null;
});

function closeMentionMenu() {
  mentionQuery.value = "";
  mentionTokenStart.value = null;
  mentionTokenEnd.value = null;
  mentionActiveIndex.value = 0;
}

function updateMentionState() {
  const el = composer.value;
  if (!el) return;

  const value = draft.value || "";
  const cursor =
    typeof el.selectionStart === "number" ? el.selectionStart : value.length;

  const before = value.slice(0, cursor);
  const lastBoundary = Math.max(
    before.lastIndexOf(" "),
    before.lastIndexOf("\n"),
    before.lastIndexOf("\t"),
  );
  const tokenStart = lastBoundary + 1;
  const token = before.slice(tokenStart);

  if (!token.startsWith("@")) {
    closeMentionMenu();
    return;
  }

  // If the token contains another @ later, treat only the last one as mention start.
  const lastAt = token.lastIndexOf("@");
  const start = tokenStart + lastAt;
  const query = before.slice(start + 1);

  // If the mention contains spaces, it's no longer an active mention.
  if (/\s/.test(query)) {
    closeMentionMenu();
    return;
  }

  const prevStart = mentionTokenStart.value;
  const prevQuery = mentionQuery.value;

  mentionTokenStart.value = start;
  mentionTokenEnd.value = cursor;
  mentionQuery.value = query;

  // Only reset active index if the mention token changed (not just cursor movement)
  if (prevStart !== start || prevQuery !== query) {
    mentionActiveIndex.value = 0;
  }
}

function selectMentionCandidate(name: string) {
  const start = mentionTokenStart.value;
  const end = mentionTokenEnd.value;
  if (start == null || end == null) return;

  const value = draft.value || "";
  const before = value.slice(0, start);
  const after = value.slice(end);
  const insert = `@${name} `;

  selectedSpecialist.value = name.trim() || "orchestrator";

  draft.value = `${before}${insert}${after}`;
  closeMentionMenu();

  nextTick(() => {
    const el = composer.value;
    if (!el) return;
    el.focus();
    const pos = (before + insert).length;
    el.setSelectionRange(pos, pos);
    autoSizeComposer();
  });
}

watch([selectedTeam, teamsData], ([teamName]) => {
  const name = (teamName || "").trim();
  if (!name) return;
  if (!teamsByName.value.has(name.toLowerCase())) {
    selectedTeam.value = "";
  }
});

watch([selectedTeam, selectedSpecialist, selectedTeamMembers], () => {
  if (!selectedTeam.value) return;
  const selected = (selectedSpecialist.value || "").trim();
  if (!selected || selected.toLowerCase() === "orchestrator") return;
  if (!selectedTeamMembers.value.has(selected.toLowerCase())) {
    selectedSpecialist.value = "orchestrator";
  }
});
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
const activeThoughtSummaries = computed(() => chat.activeThoughtSummaries);
const hasPendingInputRequest = computed(() =>
  activeMessages.value.some((message) =>
    (message.inputRequests || []).some((request) =>
      isInputRequestRespondable(request),
    ),
  ),
);
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

type ActivityStatus = "running" | "done" | "error" | "idle";
type SpecialistActivityItem = {
  id: string;
  name: string;
  model: string;
  status: ActivityStatus;
  statusLabel: string;
  description: string;
  initials: string;
  thoughtSummaries: string[];
  response: string;
  toolEntries: AgentThread["entries"];
  error: string;
  startedAt: string;
  finishedAt?: string;
  updatedAt: number;
  depth: number;
  isOrchestrator: boolean;
};
type Participant = {
  name: string;
  model: string;
};

const lastAssistant = computed(() =>
  findLast(activeMessages.value, (msg) => msg.role === "assistant"),
);
const lastAssistantId = computed(() => lastAssistant.value?.id || "");

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

function activityStateLabel(state: ActivityStatus) {
  switch (state) {
    case "running":
      return "Live";
    case "done":
      return "Complete";
    case "error":
      return "Error";
    default:
      return "Ready";
  }
}

function initialsForName(name: string) {
  const parts = (name || "Agent").split(/[\s_-]+/).filter(Boolean);
  if (!parts.length) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return `${parts[0][0]}${parts[1][0]}`.toUpperCase();
}

function latestThreadEntry(thread: AgentThread) {
  return thread.entries[thread.entries.length - 1] || null;
}

function activityDescriptionForThread(thread: AgentThread) {
  const latestEntry = latestThreadEntry(thread);
  if (thread.error) return thread.error;
  if (latestEntry?.type === "tool") {
    return latestEntry.title ? `Tool: ${latestEntry.title}` : "Using a tool";
  }
  const latestThought = thread.thoughtSummaries[thread.thoughtSummaries.length - 1];
  if (latestThought) return snippet(latestThought, 96);
  if (thread.content) return snippet(thread.content, 96);
  if (thread.prompt) return snippet(thread.prompt, 96);
  return thread.status === "running" ? "Working" : "No details yet";
}

function activityItemFromThread(thread: AgentThread): SpecialistActivityItem {
  const name = (thread.agent || "Delegated agent").trim() || "Delegated agent";
  const status = thread.status as ActivityStatus;
  const toolEntries = thread.entries.filter((entry) => entry.type === "tool");
  return {
    id: thread.callId,
    name,
    model: (thread.model || "").trim(),
    status,
    statusLabel: activityStateLabel(status),
    description: activityDescriptionForThread(thread),
    initials: initialsForName(name),
    thoughtSummaries: thread.thoughtSummaries || [],
    response: thread.content || "",
    toolEntries,
    error: thread.error || "",
    startedAt: thread.startedAt,
    finishedAt: thread.finishedAt,
    updatedAt: agentThreadTimestamp(thread),
    depth: thread.depth,
    isOrchestrator: false,
  };
}

function orchestratorActivityItem(): SpecialistActivityItem {
  const { agentName, agentModel } = resolveAgentContext();
  const assistant = lastAssistant.value;
  const status: ActivityStatus = assistant?.error
    ? "error"
    : isStreaming.value
      ? "running"
      : assistant?.content
        ? "done"
        : "idle";
  const latestThought = activeThoughtSummaries.value[activeThoughtSummaries.value.length - 1];
  return {
    id: "orchestrator",
    name: agentName || "orchestrator",
    model: agentModel || "",
    status,
    statusLabel: activityStateLabel(status),
    description: latestThought
      ? snippet(latestThought, 96)
      : assistant?.content
        ? snippet(assistant.content, 96)
        : status === "running"
          ? "Coordinating response"
          : "Ready",
    initials: initialsForName(agentName || "orchestrator"),
    thoughtSummaries: activeThoughtSummaries.value,
    response: assistant?.content || "",
    toolEntries: [],
    error: assistant?.error || "",
    startedAt: assistant?.createdAt || new Date().toISOString(),
    finishedAt: status === "done" ? assistant?.createdAt : undefined,
    updatedAt: assistant ? safeTimestampMs(assistant.createdAt) || Date.now() : Date.now(),
    depth: 0,
    isOrchestrator: true,
  };
}

const runActivityItems = computed<SpecialistActivityItem[]>(() => {
  const items = agentThreads.value.map(activityItemFromThread);
  const shouldShowOrchestrator =
    activeThoughtSummaries.value.length > 0 ||
    (isStreaming.value && items.length === 0);
  if (shouldShowOrchestrator) items.unshift(orchestratorActivityItem());

  return items.sort((a, b) => {
    if (a.status === "running" && b.status !== "running") return -1;
    if (a.status !== "running" && b.status === "running") return 1;
    return b.updatedAt - a.updatedAt;
  });
});

const visibleParticipantActivityItems = computed(() =>
  runActivityItems.value.filter((item) => !item.isOrchestrator),
);

const runActivityCounts = computed(() => {
  const counts = { running: 0, done: 0, error: 0, idle: 0 };
  for (const item of runActivityItems.value) counts[item.status] += 1;
  return counts;
});

const runActivityState = computed<ActivityStatus>(() => {
  if (runActivityCounts.value.error > 0) return "error";
  if (runActivityCounts.value.running > 0 || isStreaming.value) return "running";
  if (runActivityCounts.value.done > 0) return "done";
  return "idle";
});

const runActivityStateLabel = computed(() =>
  activityStateLabel(runActivityState.value),
);

const runActivityTitle = computed(() => {
  const count = runActivityItems.value.length;
  if (!count) return "Drafting response";
  if (runActivityCounts.value.running > 0) {
    return `${runActivityCounts.value.running} specialist${runActivityCounts.value.running === 1 ? "" : "s"} working`;
  }
  if (runActivityCounts.value.error > 0) return "Specialist work needs attention";
  return `${count} specialist${count === 1 ? "" : "s"} complete`;
});

const runActivityDetail = computed(() => {
  const parts = [];
  if (runActivityCounts.value.done) parts.push(`${runActivityCounts.value.done} complete`);
  if (runActivityCounts.value.running) parts.push(`${runActivityCounts.value.running} running`);
  if (runActivityCounts.value.error) parts.push(`${runActivityCounts.value.error} error`);
  return parts.length ? parts.join(" / ") : "Synthesizing output";
});

const runActivitySidebarLabel = computed(() => {
  const count = runActivityItems.value.length;
  if (!count) return "Idle";
  return `${count} thread${count === 1 ? "" : "s"}`;
});

const runActivityClasses = computed(() => ({
  "run-activity--running": runActivityState.value === "running",
  "run-activity--done": runActivityState.value === "done",
  "run-activity--error": runActivityState.value === "error",
}));

const runActivityPillClasses = computed(() => ({
  "run-activity-pill--running": runActivityState.value === "running",
  "run-activity-pill--done": runActivityState.value === "done",
  "run-activity-pill--error": runActivityState.value === "error",
}));

const selectedActivityItem = computed(() => {
  const selected = selectedActivityId.value;
  return (
    visibleParticipantActivityItems.value.find((item) => item.id === selected) ||
    visibleParticipantActivityItems.value[0] ||
    null
  );
});

const selectedActivityThoughtSummaries = computed(
  () => selectedActivityItem.value?.thoughtSummaries || [],
);

function shouldShowDirectActivity(message: ChatMessage) {
  return (
    message.role === "assistant" &&
    Boolean(message.activityToolTitle || shouldShowDirectThought(message))
  );
}

function shouldShowDirectThought(message: ChatMessage) {
  return Boolean(message.activityThoughtSummary);
}

function inputRequestKey(message: ChatMessage, request: ChatInputRequest) {
  return `${message.id}:${request.id}`;
}

function inputRequestFieldName(message: ChatMessage, request: ChatInputRequest) {
  return `input-request-${inputRequestKey(message, request)}`;
}

function isInputRequestRespondable(request: ChatInputRequest) {
  return request.status === "pending" || request.status === "error";
}

function inputRequestStatusLabel(request: ChatInputRequest) {
  switch (request.status) {
    case "answered":
      return "Response submitted";
    case "cancelled":
      return "Request cancelled";
    case "error":
      return "Response required";
    default:
      return "Response required";
  }
}

function inputRequestCardClasses(request: ChatInputRequest) {
  return {
    "input-request-card--answered": request.status === "answered",
    "input-request-card--cancelled": request.status === "cancelled",
    "input-request-card--error": request.status === "error",
  };
}

function inputRequestSelection(
  message: ChatMessage,
  request: ChatInputRequest,
) {
  const key = inputRequestKey(message, request);
  return inputRequestSelections.value[key] || request.choiceIds || [];
}

function inputRequestChoiceSelected(
  message: ChatMessage,
  request: ChatInputRequest,
  choiceId: string,
) {
  return inputRequestSelection(message, request).includes(choiceId);
}

function toggleInputRequestChoice(
  message: ChatMessage,
  request: ChatInputRequest,
  choiceId: string,
) {
  if (!isInputRequestRespondable(request)) return;
  const key = inputRequestKey(message, request);
  const current = inputRequestSelection(message, request);
  let next: string[];
  if (request.multiple) {
    next = current.includes(choiceId)
      ? current.filter((id) => id !== choiceId)
      : [...current, choiceId];
  } else {
    next = [choiceId];
  }
  inputRequestSelections.value = {
    ...inputRequestSelections.value,
    [key]: next,
  };
  inputRequestErrors.value = { ...inputRequestErrors.value, [key]: "" };
}

function inputRequestDraft(message: ChatMessage, request: ChatInputRequest) {
  return inputRequestDrafts.value[inputRequestKey(message, request)] || "";
}

function isInputRequestSubmitting(
  message: ChatMessage,
  request: ChatInputRequest,
) {
  return Boolean(inputRequestSubmitting.value[inputRequestKey(message, request)]);
}

function inputRequestLocalError(
  message: ChatMessage,
  request: ChatInputRequest,
) {
  return inputRequestErrors.value[inputRequestKey(message, request)] || "";
}

function canSubmitInputRequest(
  message: ChatMessage,
  request: ChatInputRequest,
) {
  if (!isInputRequestRespondable(request)) return false;
  if (isInputRequestSubmitting(message, request)) return false;
  const selected = inputRequestSelection(message, request);
  const text = inputRequestDraft(message, request).trim();
  if (request.choices.length && selected.length > 0) return true;
  if (request.allowFreeText && text) return true;
  return false;
}

function inputRequestAnswerSummary(request: ChatInputRequest) {
  const labels = (request.choiceIds || [])
    .map((id) => request.choices.find((choice) => choice.id === id)?.label || id)
    .filter(Boolean);
  const parts = [...labels];
  if (request.answer) parts.push(request.answer);
  return parts.join(", ") || "Response submitted";
}

async function submitInputRequest(
  message: ChatMessage,
  request: ChatInputRequest,
) {
  const key = inputRequestKey(message, request);
  if (!activeSessionId.value || !canSubmitInputRequest(message, request)) {
    return;
  }
  inputRequestSubmitting.value = {
    ...inputRequestSubmitting.value,
    [key]: true,
  };
  inputRequestErrors.value = { ...inputRequestErrors.value, [key]: "" };
  try {
    await chat.submitInputRequest(
      activeSessionId.value,
      message.id,
      request.id,
      inputRequestDraft(message, request),
      inputRequestSelection(message, request),
    );
    const drafts = { ...inputRequestDrafts.value };
    const selections = { ...inputRequestSelections.value };
    delete drafts[key];
    delete selections[key];
    inputRequestDrafts.value = drafts;
    inputRequestSelections.value = selections;
  } catch (error) {
    inputRequestErrors.value = {
      ...inputRequestErrors.value,
      [key]:
        error instanceof Error ? error.message : "Failed to submit response",
    };
  } finally {
    inputRequestSubmitting.value = {
      ...inputRequestSubmitting.value,
      [key]: false,
    };
  }
}

// --- Collapsible activity panel per message ---
const collapsedActivityIds = ref<Set<string>>(new Set());

function isActivityCollapsed(id: string): boolean {
  return collapsedActivityIds.value.has(id);
}

function collapseActivity(id: string) {
  collapsedActivityIds.value = new Set([...collapsedActivityIds.value, id]);
}

function expandActivity(id: string) {
  const next = new Set(collapsedActivityIds.value);
  next.delete(id);
  collapsedActivityIds.value = next;
}

// Drawer JS transition hooks
function drawerBeforeEnter(el: Element) {
  const e = el as HTMLElement;
  e.style.height = '0';
  e.style.overflow = 'hidden';
}
function drawerEnter(el: Element, done: () => void) {
  const e = el as HTMLElement;
  const h = e.scrollHeight;
  e.style.transition = 'height 0.28s cubic-bezier(0.4, 0, 0.2, 1)';
  e.style.height = h + 'px';
  e.addEventListener('transitionend', done, { once: true });
}
function drawerAfterEnter(el: Element) {
  const e = el as HTMLElement;
  e.style.height = 'auto';
  e.style.overflow = '';
  e.style.transition = '';
}
function drawerBeforeLeave(el: Element) {
  const e = el as HTMLElement;
  e.style.height = e.scrollHeight + 'px';
  e.style.overflow = 'hidden';
}
function drawerLeave(el: Element, done: () => void) {
  const e = el as HTMLElement;
  requestAnimationFrame(() => {
    e.style.transition = 'height 0.22s cubic-bezier(0.4, 0, 0.2, 1)';
    e.style.height = '0';
    e.addEventListener('transitionend', done, { once: true });
  });
}

// Auto-collapse activity panel when streaming finishes
watch(
  () => activeMessages.value.map((m) => `${m.id}:${m.streaming ? 1 : 0}`),
  (cur, prev) => {
    if (!prev) return;
    for (let i = 0; i < cur.length; i++) {
      const [id, streaming] = (cur[i] || "").split(":");
      const [, prevStreaming] = (prev[i] || "").split(":");
      // Transitioned from streaming → done
      if (prevStreaming === "1" && streaming === "0" && id) {
        const msg = activeMessages.value.find((m) => m.id === id);
        if (msg && shouldShowDirectActivity(msg)) {
          collapseActivity(id);
        }
      }
    }
  },
  { flush: "post" },
);

// Auto-scroll parallel activity card bodies on content changes
watch(
  () => visibleParticipantActivityItems.value.map(
    (i) => `${i.id}:${i.description}:${i.thoughtSummaries.length}:${i.response.length}`,
  ),
  () => {
    for (const item of visibleParticipantActivityItems.value) {
      if (threadBodyEls.has(item.id)) {
        scrollThreadBodyToBottom(item.id);
      }
    }
  },
  { flush: 'post' },
);

// Auto-collapse parallel activity cards only after all running threads finish
watch(
  () => visibleParticipantActivityItems.value.map((i) => `${i.id}:${i.status}`),
  () => {
    const items = visibleParticipantActivityItems.value;
    if (!items.length || items.some((item) => item.status === "running")) {
      return;
    }
    for (const item of items) {
      if (item.status === "done") collapseActivity(item.id);
    }
  },
  { flush: "post", immediate: true },
);

function selectActivity(id: string) {
  selectedActivityId.value = id;
  activityAutoScrollEnabled.value = true;
  activityLastScrollTop.value = 0;
  scrollActivityPaneToBottom({ force: true });
}

function participantActivityItems(name: string) {
  const key = name.trim().toLowerCase();
  return visibleParticipantActivityItems.value.filter(
    (item) => item.name.toLowerCase() === key,
  );
}

function participantActivityKey(name: string) {
  return name.trim().toLowerCase();
}

const selectedParticipantActivity = computed(() => {
  const key = selectedParticipantActivityName.value;
  if (!key) return null;
  return (
    participantList.value.find(
      (participant) => participantActivityKey(participant.name) === key,
    ) || null
  );
});

const selectedParticipantActivityItems = computed(() => {
  const participant = selectedParticipantActivity.value;
  return participant ? participantActivityItems(participant.name) : [];
});

function openParticipantActivity(name: string) {
  selectedParticipantActivityName.value = participantActivityKey(name);
  activityAutoScrollEnabled.value = true;
  activityLastScrollTop.value = 0;
  nextTick(() => {
    scrollActivityPaneToBottom({ force: true, behavior: "auto" });
  });
}

function closeParticipantActivity() {
  selectedParticipantActivityName.value = null;
}

function activityStatusClasses(item: SpecialistActivityItem) {
  return {
    "activity-status--running": item.status === "running",
    "activity-status--done": item.status === "done",
    "activity-status--error": item.status === "error",
    "activity-status--idle": item.status === "idle",
  };
}

function activityMonitorRowClasses(item: SpecialistActivityItem) {
  return {
    "activity-monitor-row--selected": selectedActivityItem.value?.id === item.id,
    "activity-monitor-row--running": item.status === "running",
    "activity-monitor-row--error": item.status === "error",
  };
}

const participantList = computed<Participant[]>(() => {
  const list: Participant[] = [];
  const seen = new Set<string>();
  const add = (name: string, model?: string) => {
    const trimmed = (name || "").trim();
    if (!trimmed) return;
    const key = trimmed.toLowerCase();
    if (seen.has(key)) return;
    seen.add(key);
    list.push({ name: trimmed, model: (model || "").trim() });
  };
  const selectedTeamValue = selectedTeamConfig.value;
  if (selectedTeamValue) {
    const teamOrchestratorModel =
      (selectedTeamValue.orchestrator?.model || "").trim() ||
      sessionAgentDefaults.value.model ||
      "";
    add("orchestrator", teamOrchestratorModel);
    const members = (selectedTeamValue.members || [])
      .map((name) => name.trim())
      .filter(Boolean)
      .sort((a, b) => a.localeCompare(b, undefined, { sensitivity: "base" }));
    for (const name of members) {
      if (name.toLowerCase() === "orchestrator") continue;
      const spec = specialistsByName.value.get(name.toLowerCase());
      if (spec?.paused) continue;
      add(spec?.name || name, spec?.model || "");
    }
    return list;
  }

  const orchestratorModel =
    specialistsByName.value.get("orchestrator")?.model?.trim() ||
    sessionAgentDefaults.value.model ||
    "";
  const orchestratorSpec = specialistsByName.value.get("orchestrator");
  if (!orchestratorSpec?.paused) {
    add("orchestrator", orchestratorModel);
  }
  const extras = (specialistsData?.value || [])
    .filter((spec: Specialist) => !spec.paused)
    .map((spec: Specialist) => ({
      name: (spec.name || "").trim(),
      model: (spec.model || "").trim(),
    }))
    .filter((spec) => spec.name && spec.name.toLowerCase() !== "orchestrator")
    .sort((a, b) =>
      a.name.localeCompare(b.name, undefined, { sensitivity: "base" }),
    );
  extras.forEach((spec) => add(spec.name, spec.model));
  return list;
});

function participantIsActive(name: string) {
  const key = name.trim().toLowerCase();

  // Find the currently streaming assistant message to determine who is live.
  const streamingMsg = activeMessages.value.find(
    (m) => m.role === "assistant" && m.streaming,
  );

  if (streamingMsg) {
    // Prefer the agent name embedded in the message; fall back to selectedSpecialist.
    const liveAgent = (
      streamingMsg.agentName ||
      streamingMsg.agent ||
      selectedSpecialist.value ||
      "orchestrator"
    ).trim().toLowerCase();

    // During streaming, only the agent whose name matches is live.
    // Never mark orchestrator live just because runActivityCounts > 0 here —
    // that count includes the specialist itself and causes false positives.
    return liveAgent === key;
  }

  // No active stream: fall back to agent-thread activity counts.
  if (key === "orchestrator") {
    return runActivityCounts.value.running > 0;
  }
  return visibleParticipantActivityItems.value.some(
    (item) => item.status === "running" && item.name.toLowerCase() === key,
  );
}

function participantStatusLabel(name: string) {
  const key = name.trim().toLowerCase();
  const active = participantIsActive(name);
  if (active) return "Live";
  if (key === "orchestrator") {
    const label = runActivityStateLabel.value;
    return (label && label.toLowerCase() !== "completed") ? label : "Idle";
  }
  const item = visibleParticipantActivityItems.value.find(
    (activity) => activity.name.toLowerCase() === key,
  );
  if (!item) return "Idle";
  const lbl = item.statusLabel;
  return (lbl && lbl.toLowerCase() !== "completed") ? lbl : "Idle";
}

function participantRowClasses(name: string) {
  return {
    "participant-row--active": participantIsActive(name),
  };
}

function participantDotClasses(name: string) {
  const active = participantIsActive(name);
  return {
    "participant-dot--active": active,
    "participant-dot--idle": !active,
  };
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

watch(sessions, (next) => {
  const keep = new Set(next.map((s) => s.id));
  const current = selectedSpecialistBySession.value;
  let changed = false;
  const pruned: Record<string, string> = {};
  for (const [id, value] of Object.entries(current)) {
    if (keep.has(id)) {
      pruned[id] = value;
    } else {
      changed = true;
    }
  }
  if (changed) selectedSpecialistBySession.value = pruned;

  const teamCurrent = selectedTeamBySession.value;
  let teamChanged = false;
  const teamPruned: Record<string, string> = {};
  for (const [id, value] of Object.entries(teamCurrent)) {
    if (keep.has(id)) {
      teamPruned[id] = value;
    } else {
      teamChanged = true;
    }
  }
  if (teamChanged) selectedTeamBySession.value = teamPruned;

  const projectCurrent = selectedProjectBySession.value;
  let projectChanged = false;
  const projectPruned: Record<string, string> = {};
  for (const [id, value] of Object.entries(projectCurrent)) {
    if (keep.has(id)) {
      projectPruned[id] = value;
    } else {
      projectChanged = true;
    }
  }
  if (projectChanged) selectedProjectBySession.value = projectPruned;
});

watch(
  () =>
    activeMessages.value.map(
      (msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`,
    ),
  () => scrollMessagesToBottom(),
  { flush: "post" },
);

watch(
  () =>
    visibleParticipantActivityItems.value.map((item) => item.id).join(":"),
  () => {
    if (
      selectedActivityId.value &&
      visibleParticipantActivityItems.value.some(
        (item) => item.id === selectedActivityId.value,
      )
    ) {
      return;
    }
    selectedActivityId.value = visibleParticipantActivityItems.value[0]?.id || null;
  },
  { immediate: true },
);

watch(
  () =>
    visibleParticipantActivityItems.value
      .map((item) =>
        [
          item.id,
          item.thoughtSummaries.map((summary) => summary.length).join(","),
          item.response.length,
          item.toolEntries.length,
          item.error || "",
        ].join("/"),
      )
      .join(":"),
  () => {
    scrollActivityPaneToBottom();
  },
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

function enterSessionSelectMode() {
  selectedSessionIds.value = [];
  sessionSelectMode.value = true;
}

function exitSessionSelectMode() {
  sessionSelectMode.value = false;
  selectedSessionIds.value = [];
}

function toggleSessionSelection(id: string) {
  const idx = selectedSessionIds.value.indexOf(id);
  if (idx >= 0) {
    selectedSessionIds.value.splice(idx, 1);
  } else {
    selectedSessionIds.value.push(id);
  }
}

function openBulkDeleteDialog() {
  if (selectedSessionIds.value.length === 0) return;
  bulkDeleteError.value = "";
  bulkDeletePending.value = false;
  showBulkDeleteDialog.value = true;
}

function closeBulkDeleteDialog() {
  if (bulkDeletePending.value) return;
  showBulkDeleteDialog.value = false;
  bulkDeleteError.value = "";
}

async function confirmBulkDelete() {
  if (bulkDeletePending.value || selectedSessionIds.value.length === 0) return;
  bulkDeletePending.value = true;
  bulkDeleteError.value = "";
  const ids = selectedSessionIds.value.slice();
  let failed = 0;
  for (const id of ids) {
    try {
      await chat.deleteSession(id);
    } catch {
      failed++;
    }
  }
  bulkDeletePending.value = false;
  if (failed > 0) {
    bulkDeleteError.value = `Failed to delete ${failed} conversation${failed === 1 ? "" : "s"}.`;
    return;
  }
  showBulkDeleteDialog.value = false;
  exitSessionSelectMode();
  autoScrollEnabled.value = true;
  nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
}

function resetDeleteSessionDialogState() {
  deleteSessionTarget.value = null;
  deleteSessionPending.value = false;
  deleteSessionError.value = "";
}

function openDeleteSessionDialog(session: ChatSessionMeta) {
  if (!session?.id) return;
  deleteSessionTarget.value = session;
  deleteSessionPending.value = false;
  deleteSessionError.value = "";
  showDeleteSessionDialog.value = true;
}

function closeDeleteSessionDialog() {
  if (deleteSessionPending.value) return;
  showDeleteSessionDialog.value = false;
  resetDeleteSessionDialogState();
}

async function confirmDeleteSession() {
  const sessionId = deleteSessionTarget.value?.id;
  if (!sessionId || !canConfirmDeleteSession.value) return;
  deleteSessionPending.value = true;
  deleteSessionError.value = "";
  try {
    await chat.deleteSession(sessionId);
    showDeleteSessionDialog.value = false;
    resetDeleteSessionDialogState();
    autoScrollEnabled.value = true;
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
  } catch (error) {
    deleteSessionError.value = "Failed to delete conversation.";
  }
  deleteSessionPending.value = false;
}

async function exportSession(sessionId: string) {
  const session = sessions.value.find((s) => s.id === sessionId);
  if (!session) return;

  // Load messages for the session (force refresh to ensure we have all)
  await chat.loadMessagesFromServer(sessionId, { force: true });
  const messages = messagesBySession.value[sessionId] || [];

  // Build export payload
  const payload = {
    exportedAt: new Date().toISOString(),
    session: {
      id: session.id,
      name: session.name,
      createdAt: session.createdAt,
      updatedAt: session.updatedAt,
      model: session.model,
    },
    messages: messages.map((msg) => ({
      id: msg.id,
      role: msg.role,
      content: msg.content,
      createdAt: msg.createdAt,
      agent: msg.agent,
      agentName: msg.agentName,
      agentModel: msg.agentModel,
      model: msg.model,
      title: msg.title,
      toolArgs: msg.toolArgs,
      attachments: msg.attachments?.map((att) => ({
        id: att.id,
        name: att.name,
        kind: att.kind,
        path: att.path,
      })),
    })),
  };

  // Safe stringify with cycle protection
  const seen = new WeakSet();
  const json = JSON.stringify(
    payload,
    (_k, val) => {
      if (typeof val === "function" || typeof val === "symbol")
        return undefined;
      if (val && typeof val === "object") {
        if (seen.has(val)) return undefined;
        seen.add(val);
      }
      return val;
    },
    2,
  );

  // Create filename from session name
  const safeName = (session.name || "chat")
    .replace(/[^a-zA-Z0-9_-]/g, "_")
    .slice(0, 50);
  const ts = new Date().toISOString().replace(/[:]/g, "-").slice(0, 19);
  const filename = `${safeName}-${ts}.json`;

  // Trigger download
  const blob = new Blob([json], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
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
  if (hasPendingInputRequest.value) return;
  await sendPrompt(draft.value);
}

async function sendPrompt(text: string, options: { echoUser?: boolean } = {}) {
  const content = text.trim();
  if (!projectSelected.value) return;
  if (!content && !pendingAttachments.value.length) return;

  // New prompt: ensure any prior timer intervals are stopped before we start a new stream.
  stopAllResponseTimers();

  autoScrollEnabled.value = true;
  draft.value = options.echoUser === false ? draft.value : "";
  try {
    const attachmentsToSend = [...pendingAttachments.value];
    const filesByAttachmentSnapshot = new Map(filesByAttachment);
    if (attachmentsToSend.some((att) => att.kind === "image")) {
      pendingAttachments.value = pendingAttachments.value.filter(
        (att) => att.kind !== "image",
      );
    }
    const mentioned = resolveLeadingSpecialistMention(
      content,
      participantList.value.map((p) => p.name),
    );
    const mentionedSpecialist = (mentioned.specialist || "").trim();
    if (mentionedSpecialist) {
      selectedSpecialist.value = mentionedSpecialist;
    }
    const routingSpecialist =
      mentionedSpecialist ||
      (selectedSpecialist.value || "orchestrator").trim() ||
      "orchestrator";
    const specialist =
      routingSpecialist.toLowerCase() !== "orchestrator"
        ? routingSpecialist
        : undefined;
    const teamName = selectedTeam.value || undefined;
    const { agentName, agentModel } = resolveAgentContext();
    await chat.sendPrompt(
      content,
      attachmentsToSend,
      filesByAttachmentSnapshot,
      {
        ...options,
        specialist,
        routingSpecialist,
        teamName,
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

async function regenerateAssistant(message: ChatMessage) {
  if (!projectSelected.value || message.role !== "assistant" || !message.id)
    return;
  const routingSpecialist =
    (selectedSpecialist.value || "orchestrator").trim() || "orchestrator";
  const specialist =
    routingSpecialist && routingSpecialist.toLowerCase() !== "orchestrator"
      ? routingSpecialist
      : undefined;
  const teamName = selectedTeam.value || undefined;
  const { agentName, agentModel } = resolveAgentContext();
  await chat.regenerateAssistant({
    specialist,
    routingSpecialist,
    teamName,
    projectId: selectedProjectId.value,
    agentName,
    agentModel,
    messageId: message.id,
  });
}

function resolveAgentContext() {
  const selected = (selectedSpecialist.value || "orchestrator").trim();
  const fallback = sessionAgentDefaults.value;
  const agentName = selected || fallback.agentName || "Agent";
  const teamModel =
    selected.toLowerCase() === "orchestrator"
      ? (selectedTeamConfig.value?.orchestrator?.model || "").trim()
      : "";
  const spec = specialistsByName.value.get(agentName.toLowerCase());
  const agentModel =
    teamModel || (spec?.model || "").trim() || fallback.model || "";
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

async function deleteChatMessage(message: ChatMessage) {
  const sessionId = activeSessionId.value;
  if (!sessionId || !message?.id) return;
  if (isStreaming.value || message.streaming) return;
  const label = message.role === "user" ? "user" : "assistant";
  const ok = confirm(`Delete this ${label} message?`);
  if (!ok) return;
  try {
    await chat.deleteMessage(sessionId, message.id);
  } catch (error) {
    console.warn("Failed to delete message", error);
  }
}

function copyThoughtSummaries() {
  const summaries = selectedActivityThoughtSummaries.value || [];
  if (!summaries.length) return;

  const text = summaries
    .map((summary) => {
      const raw = (summary || "").trim();
      if (!raw) return "";
      if (renderMode.value !== "html") return raw;

      try {
        const doc = new DOMParser().parseFromString(raw, "text/html");
        return (doc.body?.textContent || "").trim();
      } catch {
        return raw;
      }
    })
    .filter(Boolean)
    .join("\n\n")
    .trim();

  if (!text) return;

  const setCopied = () => {
    copiedThoughtSummaries.value = true;
    setTimeout(() => {
      copiedThoughtSummaries.value = false;
    }, 1200);
  };

  if (navigator.clipboard?.writeText) {
    navigator.clipboard
      .writeText(text)
      .then(setCopied)
      .catch(() => {
        copiedThoughtSummaries.value = false;
      });
    return;
  }

  try {
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    textarea.style.top = "0";
    document.body.appendChild(textarea);
    textarea.select();
    textarea.setSelectionRange(0, textarea.value.length);
    const ok = document.execCommand("copy");
    document.body.removeChild(textarea);
    if (ok) setCopied();
  } catch {
    copiedThoughtSummaries.value = false;
  }
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

function snippet(content: string, maxLength = 80) {
  if (!content) return "";
  const trimmed = content.replace(/\s+/g, " ").trim();
  const safeLength = Math.max(4, maxLength);
  return trimmed.length > safeLength
    ? `${trimmed.slice(0, safeLength - 3)}...`
    : trimmed;
}

function handleComposerKeydown(event: KeyboardEvent) {
  if (mentionMenuOpen.value) {
    if (event.key === "Escape") {
      event.preventDefault();
      closeMentionMenu();
      return;
    }
    if (event.key === "ArrowDown") {
      event.preventDefault();
      if (mentionCandidates.value.length) {
        mentionActiveIndex.value =
          (mentionActiveIndex.value + 1) % mentionCandidates.value.length;
      }
      return;
    }
    if (event.key === "ArrowUp") {
      event.preventDefault();
      if (mentionCandidates.value.length) {
        mentionActiveIndex.value =
          (mentionActiveIndex.value - 1 + mentionCandidates.value.length) %
          mentionCandidates.value.length;
      }
      return;
    }
    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      const cand = mentionCandidates.value[mentionActiveIndex.value];
      if (cand) selectMentionCandidate(cand.name);
      return;
    }
    if (event.key === "Tab") {
      const cand = mentionCandidates.value[mentionActiveIndex.value];
      if (cand) {
        event.preventDefault();
        selectMentionCandidate(cand.name);
      }
      return;
    }
  }

  if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
    event.preventDefault();
    sendCurrentPrompt();
  }
}

function handleComposerInput() {
  autoSizeComposer();
  updateMentionState();
}

function handleComposerKeyup() {
  // Cursor movement without input (e.g., arrows) should update mention detection.
  updateMentionState();
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

function scrollPaneToBottom(
  container: HTMLElement | null,
  enabledRef: { value: boolean },
  options: ScrollToBottomOptions = {},
) {
  if (!container) return;
  if (!options.force && !enabledRef.value) {
    return;
  }

  const behavior = options.behavior ?? (options.force ? "smooth" : "auto");
  const target = Math.max(container.scrollHeight - container.clientHeight, 0);
  container.scrollTo({ top: target, behavior });

  if (options.force) {
    enabledRef.value = true;
  }
}

function scrollMessagesToBottom(options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    scrollPaneToBottom(messagesPane.value, autoScrollEnabled, options);
  });
}

function scrollActivityPaneToBottom(options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    scrollPaneToBottom(participantActivityPane.value, activityAutoScrollEnabled, options);
  });
}

function registerThreadBody(el: Element | null, threadId: string) {
  if (el instanceof HTMLElement) {
    if (threadBodyEls.get(threadId) === el) return;
    threadBodyEls.set(threadId, el);
    if (!threadScrollEnabled.has(threadId)) threadScrollEnabled.set(threadId, true);
    if (!threadScrollLastTop.has(threadId)) threadScrollLastTop.set(threadId, 0);
  } else {
    threadBodyEls.delete(threadId);
  }
}

function scrollThreadBodyToBottom(threadId: string, options: ScrollToBottomOptions = {}) {
  nextTick(() => {
    const el = threadBodyEls.get(threadId);
    if (!el) return;
    const enabledRef = {
      get value() { return threadScrollEnabled.get(threadId) ?? true; },
      set value(v: boolean) {
        threadScrollEnabled.set(threadId, v);
      },
    };
    scrollPaneToBottom(el, enabledRef, options);
  });
}

function handleThreadBodyScroll(event: Event, threadId: string) {
  const enabledRef = {
    get value() { return threadScrollEnabled.get(threadId) ?? true; },
    set value(v: boolean) {
      threadScrollEnabled.set(threadId, v);
    },
  };
  const lastTopRef = {
    get value() { return threadScrollLastTop.get(threadId) ?? 0; },
    set value(v: number) {
      threadScrollLastTop.set(threadId, v);
    },
  };
  handlePaneScroll(event, enabledRef, lastTopRef);
}

function isNearBottom(container: HTMLElement) {
  const distance =
    container.scrollHeight - (container.scrollTop + container.clientHeight);
  return distance <= SCROLL_LOCK_THRESHOLD;
}

function handleMessagesScroll(event: Event) {
  handlePaneScroll(event, autoScrollEnabled, lastScrollTop);
}

function handleActivityPaneScroll(event: Event) {
  handlePaneScroll(event, activityAutoScrollEnabled, activityLastScrollTop);
}

function handlePaneScroll(
  event: Event,
  enabledRef: { value: boolean },
  lastTopRef: { value: number },
) {
  const container = event.target as HTMLElement | null;
  if (!container) return;
  if (container.scrollHeight <= container.clientHeight) {
    enabledRef.value = true;
    lastTopRef.value = 0;
    return;
  }

  const currentTop = container.scrollTop;
  const delta = currentTop - lastTopRef.value;
  lastTopRef.value = currentTop;

  if (delta < -1) {
    enabledRef.value = false;
    return;
  }

  const nearBottom = isNearBottom(container);
  if (nearBottom) {
    enabledRef.value = true;
  } else if (delta > 0) {
    enabledRef.value = false;
  }
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
    audioCtx = new (
      window.AudioContext || (window as any).webkitAudioContext
    )();
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

.chat-side {
  min-height: 0;
}

.participant-status {
  flex: 0 0 auto;
  border-radius: 999px;
  border: 1px solid rgb(var(--color-border) / 0.55);
  padding: 0.15rem 0.45rem;
  font-size: 0.62rem;
  font-weight: 700;
  line-height: 1.2;
  color: rgb(var(--color-subtle-foreground));
  background: rgb(var(--color-surface) / 0.9);
}

.direct-activity-wrapper {
  width: 100%;
}

/* Parallel specialist activity grid */
.parallel-activity-grid {
  display: flex;
  gap: 0.6rem;
  width: 100%;
}
.parallel-activity-grid--row {
  flex-direction: row;
  align-items: flex-start;
}
.parallel-activity-grid--row .parallel-activity-card {
  flex: 1 1 0;
  min-width: 0;
}
.parallel-activity-grid--col {
  flex-direction: column;
}
.parallel-activity-grid--col .parallel-activity-card {
  width: 100%;
}

/* Live dot variant for running threads */
.direct-activity-pill-dot--live {
  background: rgb(var(--color-accent)) !important;
  animation: activityPulse 1.2s ease-in-out infinite;
}

/* Pill fade in/out */
.activity-pill-enter-active {
  transition: opacity 0.15s ease;
}
.activity-pill-leave-active {
  transition: opacity 0.1s ease;
}
.activity-pill-enter-from,
.activity-pill-leave-to {
  opacity: 0;
}

/* Collapsed pill */
.direct-activity-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  border-radius: 999px;
  border: 1px solid rgb(var(--color-border) / 0.5);
  background: rgb(var(--color-surface-muted) / 0.45);
  padding: 0.3rem 0.75rem 0.3rem 0.55rem;
  font-size: 0.72rem;
  font-weight: 600;
  color: rgb(var(--color-subtle-foreground));
  cursor: pointer;
  transition: border-color 0.15s, color 0.15s;
}
.direct-activity-pill:hover {
  border-color: rgb(var(--color-accent) / 0.5);
  color: rgb(var(--color-accent));
}
.direct-activity-pill-dot {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: rgb(var(--color-subtle-foreground));
  flex-shrink: 0;
}
.direct-activity-pill:hover .direct-activity-pill-dot {
  background: rgb(var(--color-accent));
}
.direct-activity-pill-label {
  flex: 1;
}
.direct-activity-pill-chevron {
  font-size: 0.85rem;
  line-height: 1;
  transform: rotate(0deg);
}

/* Expanded panel */
.direct-activity {
  display: flex;
  flex-direction: column;
  border-radius: 0.8rem;
  border: 1px solid rgb(var(--color-border) / 0.58);
  background: rgb(var(--color-surface-muted) / 0.62);
  overflow: hidden;
}

.direct-activity-header {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  padding: 0.6rem 0.75rem;
  border-bottom: 1px solid rgb(var(--color-border) / 0.35);
}

.direct-activity-body {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
  max-height: 14rem;
  overflow-y: auto;
  padding: 0.65rem 0.75rem 0.75rem;
}

.direct-activity-collapse-btn {
  font-size: 0.68rem;
  font-weight: 600;
  color: rgb(var(--color-subtle-foreground));
  background: none;
  border: none;
  cursor: pointer;
  padding: 0.1rem 0.3rem;
  border-radius: 0.3rem;
  transition: color 0.15s;
}
.direct-activity-collapse-btn:hover {
  color: rgb(var(--color-accent));
}

.direct-activity-streaming-dot {
  width: 0.45rem;
  height: 0.45rem;
  border-radius: 50%;
  background: rgb(var(--color-accent));
  animation: activityPulse 1.2s ease-in-out infinite;
}

@keyframes activityPulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.direct-activity-row {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  min-width: 0;
}

.direct-activity-label {
  flex: 0 0 auto;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.62rem;
  font-weight: 800;
  letter-spacing: 0.08em;
  line-height: 1.2;
  text-transform: uppercase;
}

.direct-activity-value {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgb(var(--color-foreground));
  font-size: 0.76rem;
  font-weight: 700;
}

.direct-activity-thought {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
}

.direct-activity-summary {
  color: rgb(var(--color-foreground));
  font-size: 0.78rem;
  line-height: 1.5;
}

.input-request-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.input-request-card {
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-warning) / 0.55);
  background: rgb(var(--color-warning) / 0.1);
  padding: 0.85rem;
  box-shadow: 0 16px 32px -28px rgb(0 0 0 / 0.75);
}

.input-request-card--answered {
  border-color: rgb(var(--color-success) / 0.45);
  background: rgb(var(--color-success) / 0.09);
}

.input-request-card--cancelled,
.input-request-card--error {
  border-color: rgb(var(--color-danger) / 0.45);
}

.input-request-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.input-request-kicker {
  color: rgb(var(--color-warning));
  font-size: 0.64rem;
  font-weight: 850;
  letter-spacing: 0;
  line-height: 1.2;
  text-transform: uppercase;
}

.input-request-agent {
  margin-top: 0.16rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.72rem;
  line-height: 1.25;
}

.input-request-live-dot {
  width: 0.55rem;
  height: 0.55rem;
  margin-top: 0.2rem;
  flex: 0 0 auto;
  border-radius: 999px;
  background: rgb(var(--color-warning));
  box-shadow: 0 0 0 4px rgb(var(--color-warning) / 0.16);
  animation: activityPulse 1.2s ease-in-out infinite;
}

.input-request-question {
  margin-top: 0.7rem;
  color: rgb(var(--color-foreground));
  font-size: 0.92rem;
  font-weight: 700;
  line-height: 1.45;
}

.input-request-reason {
  margin-top: 0.35rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.78rem;
  line-height: 1.45;
}

.input-request-choices {
  display: grid;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.input-request-choice {
  display: flex;
  align-items: flex-start;
  gap: 0.55rem;
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.58);
  background: rgb(var(--color-surface) / 0.68);
  padding: 0.55rem 0.65rem;
  color: rgb(var(--color-foreground));
  font-size: 0.8rem;
  line-height: 1.35;
}

.input-request-choice input {
  margin-top: 0.18rem;
  accent-color: rgb(var(--color-accent));
}

.input-request-choice-label {
  display: block;
  font-weight: 700;
}

.input-request-choice-description {
  display: block;
  margin-top: 0.12rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.72rem;
}

.input-request-textarea {
  margin-top: 0.75rem;
  width: 100%;
  resize: vertical;
  border-radius: 0.5rem;
  border: 1px solid rgb(var(--color-border) / 0.7);
  background: rgb(var(--color-surface) / 0.78);
  color: rgb(var(--color-foreground));
  font-size: 0.82rem;
  line-height: 1.45;
  outline: none;
  padding: 0.65rem;
}

.input-request-textarea:focus {
  border-color: rgb(var(--color-accent) / 0.75);
}

.input-request-error {
  margin-top: 0.5rem;
  color: rgb(var(--color-danger));
  font-size: 0.74rem;
  font-weight: 650;
}

.input-request-answer {
  display: flex;
  gap: 0.45rem;
  margin-top: 0.65rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.78rem;
}

.input-request-answer-label {
  color: rgb(var(--color-success));
  font-weight: 800;
}

.input-request-answer-text {
  min-width: 0;
  overflow-wrap: anywhere;
}

.input-request-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 0.75rem;
}

.input-request-submit {
  border-radius: 999px;
  border: 1px solid rgb(var(--color-accent) / 0.55);
  background: rgb(var(--color-accent));
  color: rgb(var(--color-accent-foreground));
  font-size: 0.78rem;
  font-weight: 800;
  line-height: 1;
  padding: 0.58rem 0.9rem;
}

.input-request-submit:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.activity-detail-empty {
  padding: 0.75rem;
  border: 1px dashed rgb(var(--color-border) / 0.65);
  border-radius: 0.75rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.78rem;
}

.activity-detail-scroll {
  height: 100%;
  min-height: 0;
  overflow: auto;
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}

.activity-detail-section {
  border-top: 1px solid rgb(var(--color-border) / 0.5);
  padding-top: 1rem;
}

.activity-detail-section:first-child {
  border-top: none;
  padding-top: 0;
}

.activity-detail-subsection {
  min-height: 0;
  margin-top: 1.15rem;
}

.activity-detail-section-title {
  margin-bottom: 0.62rem;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.62rem;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.activity-thought-item,
.activity-response,
.activity-tool-item,
.activity-error-text {
  border-radius: 0.65rem;
  border: 1px solid rgb(var(--color-border) / 0.55);
  background: rgb(var(--color-surface-muted) / 0.72);
  padding: 0.65rem;
}

.activity-thought-item {
  color: rgb(var(--color-foreground));
  font-size: 0.72rem;
  line-height: 1.42;
  white-space: pre-wrap;
}

.activity-thought-list {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.activity-response {
  color: rgb(var(--color-foreground));
  font-size: 0.71rem;
  line-height: 1.42;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.activity-tool-list {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  margin-bottom: 0.2rem;
}

.activity-tool-title {
  margin-bottom: 0.35rem;
  font-size: 0.7rem;
  font-weight: 650;
  color: rgb(var(--color-foreground));
}

.activity-error-text {
  color: rgb(var(--color-danger));
  font-size: 0.72rem;
  line-height: 1.4;
}

.participant-list {
  display: flex;
  flex-direction: column;
}

.participant-list-item {
  border-bottom: 1px solid rgb(var(--color-border) / 0.4);
}

.participant-list-item:last-child {
  border-bottom: none;
}

.participant-row {
  display: flex;
  width: 100%;
  align-items: flex-start;
  gap: 0.6rem;
  padding: 0.5rem 0.75rem;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    background 0.2s ease,
    border-color 0.2s ease;
}

.participant-row:hover,
.participant-row:focus-visible {
  background: rgb(var(--color-surface-muted) / 0.65);
  outline: none;
}

.participant-row--active {
  background: rgb(var(--color-accent) / 0.08);
}

.participant-dot {
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 999px;
  background: rgb(var(--color-border));
}

.participant-dot--active {
  background: rgb(var(--color-success));
  box-shadow: 0 0 0 4px rgb(var(--color-success) / 0.22);
}

.participant-dot--idle {
  background: rgb(var(--color-warning));
  box-shadow: 0 0 0 4px rgb(var(--color-warning) / 0.18);
}

.participant-body {
  min-width: 0;
  flex: 1;
}

.participant-name {
  display: block;
  font-size: 0.82rem;
  font-weight: 600;
  color: rgb(var(--color-foreground));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.participant-model {
  display: block;
  margin-top: 0.1rem;
  font-size: 0.7rem;
  color: rgb(var(--color-subtle-foreground));
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.participant-activity-action {
  flex: 0 0 auto;
  color: rgb(var(--color-accent));
  font-size: 0.66rem;
  font-weight: 700;
  line-height: 1.25;
  white-space: nowrap;
}

.activity-modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 60;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 1.5rem;
  background: rgb(0 0 0 / 0.62);
  backdrop-filter: blur(8px);
}

.activity-modal {
  display: flex;
  flex-direction: column;
  width: min(68rem, 94vw);
  height: min(44rem, 88vh);
  border-radius: 0.95rem;
  border: 1px solid rgb(var(--color-border) / 0.72);
  background: rgb(var(--color-surface) / 0.98);
  box-shadow: 0 28px 80px -36px rgb(0 0 0 / 0.9);
  overflow: hidden;
}

.activity-modal-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
  padding: 1rem 1.1rem;
  border-bottom: 1px solid rgb(var(--color-border) / 0.58);
}

.activity-modal-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgb(var(--color-foreground));
  font-size: 1rem;
  font-weight: 750;
}

.activity-modal-model {
  margin-top: 0.2rem;
  max-width: 44rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.76rem;
}

.activity-modal-close {
  flex: 0 0 auto;
  border-radius: 999px;
  border: 1px solid rgb(var(--color-border) / 0.72);
  background: rgb(var(--color-surface-muted) / 0.8);
  color: rgb(var(--color-foreground));
  padding: 0.35rem 0.7rem;
  font-size: 0.74rem;
  font-weight: 700;
}

.activity-modal-close:hover,
.activity-modal-close:focus-visible {
  border-color: rgb(var(--color-accent) / 0.7);
  outline: none;
}

.activity-modal-scroll {
  flex: 1;
  padding: 1rem 1.1rem 1.2rem;
}

.run-activity {
  margin-top: 0.75rem;
  padding: 0.85rem;
  border-radius: 0.85rem;
  border: 1px solid rgb(var(--color-border) / 0.6);
  background: rgb(var(--color-surface-muted) / 0.92);
  box-shadow: 0 14px 32px -24px rgb(0 0 0 / 0.6);
}

.run-activity-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.15rem 0.75rem;
  align-items: center;
}

.run-activity-title {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.9rem;
  font-weight: 700;
  color: rgb(var(--color-foreground));
}

.run-activity-detail {
  grid-column: 1 / 2;
  color: rgb(var(--color-subtle-foreground));
  font-size: 0.74rem;
}

.run-activity-pill {
  grid-column: 2 / 3;
  grid-row: 1 / 3;
  flex-shrink: 0;
  align-self: start;
  font-size: 0.62rem;
  font-weight: 800;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  padding: 0.2rem 0.55rem;
  border-radius: 999px;
  border: 1px solid rgb(var(--color-border) / 0.55);
  color: rgb(var(--color-subtle-foreground));
  background: rgb(var(--color-surface) / 0.88);
}

.run-activity--running {
  border-color: rgb(var(--color-accent) / 0.35);
}

.run-activity--done {
  border-color: rgb(var(--color-success) / 0.35);
}

.run-activity--error {
  border-color: rgb(var(--color-danger) / 0.35);
}

.activity-status--running,
.run-activity-pill--running {
  border-color: rgb(var(--color-accent) / 0.4);
  color: rgb(var(--color-accent));
  background: rgb(var(--color-accent) / 0.12);
}

.activity-status--running {
  animation: statusPulse 1.8s ease-in-out infinite;
}

.activity-status--done,
.run-activity-pill--done {
  border-color: rgb(var(--color-success) / 0.35);
  color: rgb(var(--color-success));
  background: rgb(var(--color-success) / 0.12);
}

.activity-status--error,
.run-activity-pill--error {
  border-color: rgb(var(--color-danger) / 0.4);
  color: rgb(var(--color-danger));
  background: rgb(var(--color-danger) / 0.12);
}

.activity-status--idle {
  border-color: rgb(var(--color-warning) / 0.35);
  color: rgb(var(--color-warning));
  background: rgb(var(--color-warning) / 0.12);
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
  max-width: 100%;
  overflow-x: auto;
  white-space: pre;
}

:deep(.chat-markdown code) {
  font-family:
    ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono",
    "Courier New", monospace;
  font-size: 0.875rem;
}

.chat-markdown :deep(pre.hljs) {
  border-top-left-radius: 0;
  border-top-right-radius: 0;
  border-bottom-left-radius: 0.5rem;
  border-bottom-right-radius: 0.5rem;
  overflow-x: auto;
  padding: 0.75rem;
  background-color: rgb(var(--color-surface-muted) / 0.78);
  border-color: rgb(var(--color-surface-muted) / 0.78);
  max-width: 100%;
}

.chat-markdown :deep(code.hljs) {
  display: block;
  white-space: pre;
  max-width: 100%;
  overflow-x: auto;
  background: transparent !important;
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
  background: rgb(var(--color-surface-muted) / 0.78);
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
