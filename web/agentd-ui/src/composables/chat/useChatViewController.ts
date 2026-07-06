import {
  computed,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import type { ComponentPublicInstance } from "vue";
import { useRouter } from "vue-router";
import axios from "axios";
import type {
  ChatAttachment,
  ChatMessage,
  ChatSessionMeta,
  ChatRole,
} from "@/types/chat";
import { useQuery } from "@tanstack/vue-query";
import {
  fetchAgentdSettings,
  listTeams,
  listSpecialists,
  type AgentdSettings,
} from "@/api/client";
import { fetchChatMessages } from "@/api/chat";
import { renderMarkdown } from "@/utils/markdown";
import { resolveLeadingChatMention } from "@/utils/chatMentions";
import { useChatStore } from "@/stores/chat";
import {
  contextMetricsFromSummaryEvent,
  localContextMetricsForMessages,
} from "@/stores/chatHelpers";
import { useProjectsStore } from "@/stores/projects";
import type { ChatContextMetrics } from "@/types/chat";
import type { DropdownOption } from "@/types/dropdown";
import { useChatInputRequests } from "./chatInputRequests";
import { useChatScroll } from "./chatScroll";
import { useChatVoiceRecording } from "./chatVoiceRecording";
import { useChatTargeting } from "./chatTargeting";
import { useChatActivity } from "./chatActivity";

export function useChatViewController() {
  const router = useRouter();
  const isBrowser = typeof window !== "undefined";
  let previousBodyOverflow: string | null = null;

  const chat = useChatStore();
  const proj = useProjectsStore();
  const summarySettingsQuery = useQuery({
    queryKey: ["agentd-summary-settings"],
    queryFn: fetchAgentdSettings,
    staleTime: 60_000,
    retry: false,
  });

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
    void proj.refresh({ includeUsage: false });
    if (isBrowser) {
      previousBodyOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    }
  });
  const projects = computed(() => proj.projects);
  const selectedProjectBySession = ref<Record<string, string>>({});

  function hasSelectedProjectOverride(sessionId: string) {
    return Object.prototype.hasOwnProperty.call(
      selectedProjectBySession.value,
      sessionId,
    );
  }

  function projectIdForSession(sessionId: string) {
    return (
      sessions.value.find((session) => session.id === sessionId)?.projectId ||
      ""
    ).trim();
  }

  function setSelectedProjectOverride(
    sessionId: string,
    projectId: string | null,
  ) {
    const next = { ...selectedProjectBySession.value };
    if (projectId === null) delete next[sessionId];
    else next[sessionId] = projectId;
    selectedProjectBySession.value = next;
  }

  const selectedProjectId = computed({
    get: () => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return "";
      if (hasSelectedProjectOverride(sessionId)) {
        return selectedProjectBySession.value[sessionId] || "";
      }
      return projectIdForSession(sessionId);
    },
    set: (v: string) => {
      const sessionId = activeSessionId.value;
      if (!sessionId) return;
      const projectId = (v || "").trim();
      const previousProjectId = selectedProjectId.value;
      if (previousProjectId === projectId) return;
      setSelectedProjectOverride(sessionId, projectId);
      void chat
        .updateSessionProject(sessionId, projectId)
        .then(() => {
          if (selectedProjectBySession.value[sessionId] === projectId) {
            setSelectedProjectOverride(sessionId, null);
          }
        })
        .catch((error) => {
          if (selectedProjectBySession.value[sessionId] === projectId) {
            const storedProjectId = projectIdForSession(sessionId);
            setSelectedProjectOverride(
              sessionId,
              previousProjectId === storedProjectId ? null : previousProjectId,
            );
          }
          console.warn("Failed to persist chat session project:", error);
        });
    },
  });
  const sessions = computed(() => chat.sessions);
  const messagesBySession = computed(() => chat.messagesBySession);
  const sessionsLoading = computed(() => chat.sessionsLoading);
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
  const composer = ref<HTMLTextAreaElement | null>(null);
  const copiedMessageId = ref<string | null>(null);
  const copiedThoughtSummaries = ref(false);
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
  const {
    inputRequestCardClasses,
    inputRequestStatusLabel,
    submitInputRequest,
    inputRequestChoiceSelected,
    toggleInputRequestChoice,
    isInputRequestSubmitting,
    inputRequestLocalError,
    inputRequestKey,
    inputRequestFieldName,
    inputRequestDraft,
    setInputRequestDraft,
    isInputRequestRespondable,
    canSubmitInputRequest,
    inputRequestAnswerSummary,
  } = useChatInputRequests({
    activeSessionId,
    chat,
  });
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
  const pinningSessionIds = ref<Record<string, boolean>>({});
  const canConfirmDeleteSession = computed(
    () => !!deleteSessionTarget.value?.id && !deleteSessionPending.value,
  );

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
  const specialists = computed(() => specialistsData?.value || []);
  const teams = computed(() => teamsData?.value || []);
  const teamsReady = computed(() => Boolean(teamsData?.value));

  // Transform projects data for dropdown
  const projectOptions = computed<DropdownOption[]>(() => {
    const projectEntries: DropdownOption[] = projects.value.map((project) => ({
      id: project.id,
      label: project.name,
      value: project.id,
    }));
    const lockedProjectID = selectedProjectId.value;
    if (
      lockedProjectID &&
      !projectEntries.some((project) => project.value === lockedProjectID)
    ) {
      projectEntries.unshift({
        id: lockedProjectID,
        label: "Temporary project",
        value: lockedProjectID,
        disabled: true,
      });
    }
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

  const projectSelected = computed(() => Boolean(activeSessionId.value));
  const requiresProjectSelection = computed(() => false);

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
    if (
      name.endsWith(".png") ||
      name.endsWith(".jpg") ||
      name.endsWith(".jpeg")
    )
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
  const activeMessagePaging = computed(() => chat.activeMessagePaging);
  const hasOlderMessages = computed(() =>
    Boolean(activeMessagePaging.value?.hasOlder),
  );
  const olderMessagesLoading = computed(() =>
    Boolean(activeMessagePaging.value?.loadingOlder),
  );
  const olderMessagesError = computed(
    () => activeMessagePaging.value?.error || "",
  );
  const {
    autoScrollEnabled,
    activityAutoScrollEnabled,
    activityLastScrollTop,
    setMessagesPaneRef,
    setParticipantActivityPaneRef,
    scrollMessagesToBottom,
    preserveMessagesScrollWhileLoadingOlder: loadOlderMessages,
    scrollActivityPaneToBottom,
    registerThreadBody,
    scrollThreadBodyToBottom,
    handleThreadBodyScroll,
    handleMessagesScroll,
    handleActivityPaneScroll,
    handleScrollToLatest,
  } = useChatScroll({
    activeSessionId,
    hasOlderMessages,
    olderMessagesLoading,
    loadOlderMessages: (sessionId: string) => chat.loadOlderMessages(sessionId),
  });
  const activeSummaryEvent = computed(() => chat.activeSummaryEvent);
  const configuredSummaryBudget = computed(() =>
    summaryBudgetFromAgentdSettings(summarySettingsQuery.data.value),
  );
  const sessionContextMetrics = computed(() => {
    const latestMetrics = findLast(activeMessages.value, (message) =>
      Boolean(message.contextMetrics),
    )?.contextMetrics;
    const streamingMetrics = findLast(activeMessages.value, (message) =>
      Boolean(message.streaming && message.contextMetrics),
    )?.contextMetrics;
    if (streamingMetrics) return streamingMetrics;
    const serverMetrics = findLast(
      activeMessages.value,
      (message) =>
        !!message.contextMetrics &&
        message.contextMetrics.phase !== "client_estimate",
    )?.contextMetrics;
    if (latestMetrics?.phase === "client_estimate") {
      return withKnownContextBudget(latestMetrics, serverMetrics);
    }
    if (serverMetrics) return serverMetrics;
    if (activeSummaryEvent.value) {
      const summaryMetrics = contextMetricsFromSummaryEvent(
        activeSummaryEvent.value,
        configuredSummaryBudget.value ?? undefined,
      );
      if (summaryMetrics) return summaryMetrics;
    }
    return localContextMetricsForMessages(
      activeMessages.value,
      configuredSummaryBudget.value ?? undefined,
    );
  });
  function summaryBudgetFromAgentdSettings(
    settings?: AgentdSettings,
  ): Pick<
    ChatContextMetrics,
    "contextWindow" | "reserveTokens" | "summaryThreshold"
  > | null {
    if (!settings) return null;
    const contextWindow = positiveMetricValue(
      settings.summaryContextWindowTokens,
    );
    const reserveTokens = positiveMetricValue(
      settings.summaryReserveBufferTokens,
    );
    const summaryThreshold =
      positiveMetricValue(settings.summaryTokenBudget) ??
      (contextWindow && reserveTokens
        ? summaryThresholdForBudget(contextWindow, reserveTokens)
        : null);
    if (!contextWindow || !summaryThreshold) return null;
    return {
      contextWindow,
      reserveTokens:
        reserveTokens ?? Math.max(contextWindow - summaryThreshold, 0),
      summaryThreshold,
    };
  }

  function positiveMetricValue(value: unknown): number | null {
    return typeof value === "number" && Number.isFinite(value) && value > 0
      ? value
      : null;
  }

  function summaryThresholdForBudget(
    contextWindow: number,
    reserveTokens: number,
  ): number {
    const budget = contextWindow - reserveTokens;
    return budget > 0 ? budget : Math.floor(contextWindow / 2);
  }

  function withKnownContextBudget(
    metrics: ChatContextMetrics,
    budgetSource?: ChatContextMetrics,
  ): ChatContextMetrics {
    if (!budgetSource) return metrics;
    return {
      ...metrics,
      contextWindow: budgetSource.contextWindow,
      summaryThreshold: budgetSource.summaryThreshold,
      reserveTokens: budgetSource.reserveTokens,
      willSummarize: metrics.inputTokens >= budgetSource.summaryThreshold,
    };
  }
  const toolMessages = computed(() => chat.toolMessages);
  const activeThoughtSummaries = computed(() => chat.activeThoughtSummaries);
  const memorySettingsSavingBySession = ref<Record<string, boolean>>({});
  const commandPolicyDisablePending = ref(false);
  const commandPolicyDisableError = ref("");
  const activeMemorySettingsSaving = computed(() => {
    const sessionId = activeSessionId.value;
    return Boolean(sessionId && memorySettingsSavingBySession.value[sessionId]);
  });
  const commandPolicyAllowAllActive = computed(() =>
    Boolean(activeSession.value?.commandPolicyAllowAll),
  );
  const memoryEnabled = computed(
    () =>
      activeSession.value?.memoryEnabled ??
      Boolean(
        activeSession.value?.evolvingMemoryEnabled &&
        activeSession.value?.beliefMemoryEnabled,
      ),
  );
  const hasPendingInputRequest = computed(() =>
    activeMessages.value.some((message) =>
      (message.inputRequests || []).some((request) =>
        isInputRequestRespondable(request),
      ),
    ),
  );
  const sessionAgentDefaults = computed(() =>
    parseAgentModelLabel(activeSession.value?.model || ""),
  );
  const {
    teamOptions,
    teamsByName,
    selectedSpecialist,
    selectedTeam,
    selectedTeamConfig,
    mentionMenuOpen,
    mentionCandidates,
    mentionActiveIndex,
    selectMentionCandidate,
    updateMentionState,
    closeMentionMenu,
    chatMentionTargets,
    participantList,
    resolveAgentContext,
    setSelectedTeamValue,
    teamOrchestratorDisplayName,
  } = useChatTargeting({
    activeSessionId,
    sessions,
    teams,
    teamsReady,
    specialists,
    draft,
    composer,
    autoSizeComposer,
    sessionAgentDefaults,
    updateSessionActiveTarget: (sessionId, specialist, team) =>
      chat.updateSessionActiveTarget(sessionId, specialist, team),
  });
  const composerPlaceholder = computed(() => {
    if (hasPendingInputRequest.value) {
      return "Answer the request above to continue.";
    }
    if (!projectSelected.value) {
      return "Select a project to enable the chat.";
    }
    const { agentName } = resolveAgentContext();
    return `Message ${agentName || "orchestrator"}...`;
  });
  const showScrollToBottom = computed(
    () => !autoScrollEnabled.value && chatMessages.value.length > 0,
  );
  const sessionMessageCounts = computed<Record<string, number>>(() => {
    const counts: Record<string, number> = {};
    for (const session of sessions.value) {
      const local = messagesBySession.value[session.id];
      const metaCount =
        typeof session.messageCount === "number" ? session.messageCount : 0;
      counts[session.id] =
        typeof session.messageCount === "number"
          ? metaCount
          : Array.isArray(local)
            ? local.length
            : 0;
    }
    return counts;
  });
  const sessionsAwaitingInput = computed(() => {
    const ids = new Set<string>();
    for (const [sessionId, messages] of Object.entries(
      messagesBySession.value,
    )) {
      if (
        messages.some((message) =>
          (message.inputRequests || []).some((request) =>
            isInputRequestRespondable(request),
          ),
        )
      ) {
        ids.add(sessionId);
      }
    }
    return ids;
  });

  function messageCountFor(sessionId: string) {
    return sessionMessageCounts.value[sessionId] ?? 0;
  }

  function sessionAwaitingInput(sessionId: string) {
    return sessionsAwaitingInput.value.has(sessionId);
  }

  function sessionIsStreaming(sessionId: string) {
    return chat.isSessionStreaming(sessionId);
  }

  // --- Response timer (elapsed while streaming; frozen when stream completes) ---
  const responseStartMsByMessageId = new Map<string, number>();
  const responseElapsedMsByMessageId = ref<Record<string, number>>({});
  const responseIntervalByMessageId = new Map<string, number>();
  function safeParseIsoMs(iso: string) {
    const ms = Date.parse(iso);
    return Number.isFinite(ms) ? ms : null;
  }

  function persistedResponseDurationMs(message: ChatMessage) {
    const duration =
      typeof message.durationMs === "number" ? message.durationMs : Number.NaN;
    return Number.isFinite(duration) && duration >= 0 ? duration : undefined;
  }

  function responseElapsedMs(message: ChatMessage) {
    if (!message.streaming) {
      const persisted = persistedResponseDurationMs(message);
      if (persisted !== undefined) return persisted;
    }
    return responseElapsedMsByMessageId.value[message.id] ?? 0;
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
      const previousElapsed = responseElapsedMsByMessageId.value[id];
      const start =
        typeof previousElapsed === "number" && previousElapsed > 0
          ? Date.now() - previousElapsed
          : (safeParseIsoMs(message.createdAt) ?? Date.now());
      responseStartMsByMessageId.set(id, start);
    }

    const startMs = responseStartMsByMessageId.get(id);
    if (!startMs) return;

    responseElapsedMsByMessageId.value[id] = Math.max(0, Date.now() - startMs);

    if (isBrowser && !responseIntervalByMessageId.has(id)) {
      const handle = window.setInterval(() => {
        const start = responseStartMsByMessageId.get(id);
        if (!start) return;
        responseElapsedMsByMessageId.value[id] = Math.max(
          0,
          Date.now() - start,
        );
      }, 100);
      responseIntervalByMessageId.set(id, handle);
    }
  }

  function updateLocalResponseElapsed(messageId: string) {
    const start = responseStartMsByMessageId.get(messageId);
    if (start) {
      responseElapsedMsByMessageId.value[messageId] = Math.max(
        0,
        Date.now() - start,
      );
    }
  }

  function clearResponseTimerInterval(messageId: string) {
    const handle = responseIntervalByMessageId.get(messageId);
    if (handle != null) {
      if (isBrowser) window.clearInterval(handle);
      responseIntervalByMessageId.delete(messageId);
    }
  }

  function suspendResponseTimer(messageId: string) {
    updateLocalResponseElapsed(messageId);
    clearResponseTimerInterval(messageId);
  }

  function pauseResponseTimer(messageId: string) {
    updateLocalResponseElapsed(messageId);
    responseStartMsByMessageId.delete(messageId);
    clearResponseTimerInterval(messageId);
  }

  function finalizeResponseTimer(message: ChatMessage) {
    const persisted = persistedResponseDurationMs(message);
    if (persisted !== undefined) {
      responseElapsedMsByMessageId.value[message.id] = persisted;
    } else {
      updateLocalResponseElapsed(message.id);
    }
    responseStartMsByMessageId.delete(message.id);
    clearResponseTimerInterval(message.id);
  }

  function stopAllResponseTimers() {
    // Iterate a snapshot since suspending mutates the interval map.
    for (const id of Array.from(responseIntervalByMessageId.keys())) {
      suspendResponseTimer(id);
    }
  }

  function shouldShowResponseTimer(message: ChatMessage) {
    if (message.role !== "assistant") return false;
    if (message.streaming) return true;
    if (persistedResponseDurationMs(message) !== undefined) return true;
    return message.id in responseElapsedMsByMessageId.value;
  }

  const {
    cockpitContextDegrees,
    cockpitContextPercent,
    cockpitContextLabel,
    cockpitToolCount,
    cockpitToolRows,
    cockpitTimelineLanes,
    cockpitTimelineTicks,
    runActivitySidebarLabel,
    visibleParticipantActivityItemsForMessage,
    isActivityCollapsed,
    expandActivity,
    collapseActivity,
    drawerBeforeEnter,
    drawerEnter,
    drawerAfterEnter,
    drawerBeforeLeave,
    drawerLeave,
    shouldShowDirectActivity,
    shouldShowDirectThought,
    hasMemoryContext,
    isMemoryContextExpanded,
    expandMemoryContext,
    collapseMemoryContext,
    memoryContextPillMeta,
    selectedActivityThoughtSummaries,
    stopCockpitTimelineLiveUpdates,
    participantRowClasses,
    participantDotClasses,
    participantStatusLabel,
    openParticipantActivity,
    selectedParticipantActivityName,
    selectedParticipantActivity,
    selectedParticipantActivityItems,
    closeParticipantActivity,
  } = useChatActivity({
    isBrowser,
    activeMessages,
    agentThreads,
    isStreaming,
    activeThoughtSummaries,
    selectedTeam,
    selectedSpecialist,
    selectedTeamConfig,
    teamsByName,
    participantList,
    toolMessages,
    sessionContextMetrics,
    resolveAgentContext,
    teamOrchestratorDisplayName,
    responseElapsedMs,
    scrollThreadBodyToBottom,
    scrollActivityPaneToBottom,
    activityAutoScrollEnabled,
    activityLastScrollTop,
  });

  watch(
    sessions,
    (next) => {
      const keep = new Set(next.map((s) => s.id));
      const projectCurrent = selectedProjectBySession.value;
      let projectChanged = false;
      const projectPruned: Record<string, string> = {};
      for (const session of next) {
        if (!hasSelectedProjectOverride(session.id)) continue;
        const overrideProjectID = projectCurrent[session.id] || "";
        if (overrideProjectID === (session.projectId || "").trim()) {
          projectChanged = true;
          continue;
        }
        projectPruned[session.id] = overrideProjectID;
      }
      for (const id of Object.keys(projectCurrent)) {
        if (!keep.has(id)) {
          projectChanged = true;
          break;
        }
      }
      if (projectChanged) selectedProjectBySession.value = projectPruned;

      const pinningCurrent = pinningSessionIds.value;
      let pinningChanged = false;
      const pinningPruned: Record<string, boolean> = {};
      for (const [id, value] of Object.entries(pinningCurrent)) {
        if (keep.has(id)) {
          pinningPruned[id] = value;
        } else {
          pinningChanged = true;
        }
      }
      if (pinningChanged) pinningSessionIds.value = pinningPruned;
    },
    { immediate: true },
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
      activeMessages.value.map(
        (m) => `${m.id}:${m.role}:${m.streaming ? 1 : 0}:${m.durationMs ?? ""}`,
      ),
    () => {
      for (const msg of activeMessages.value) {
        if (msg.role !== "assistant") continue;
        if (msg.streaming) ensureResponseTimer(msg);
        else if (
          persistedResponseDurationMs(msg) !== undefined ||
          msg.id in responseElapsedMsByMessageId.value
        ) {
          if (msg.error) pauseResponseTimer(msg.id);
          else finalizeResponseTimer(msg);
        }
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
    commandPolicyDisableError.value = "";
    if (sessionId) {
      void loadMessagesFromServer(sessionId);
    }
    // Switching sessions: ensure we don't leave any intervals running.
    stopAllResponseTimers();
  });

  watch(renamingSessionId, (value) => {
    if (!value) return;
    nextTick(() => {
      const input = renameInput.value;
      if (!(input instanceof HTMLInputElement)) return;
      input.focus();
      input.select();
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
    stopCockpitTimelineLiveUpdates();
    cleanupRecording();
    if (isBrowser && previousBodyOverflow !== null) {
      document.body.style.overflow = previousBodyOverflow;
    }
  });

  function setRenameInput(el: Element | ComponentPublicInstance | null) {
    renameInput.value = el instanceof HTMLInputElement ? el : null;
  }

  function conversationOptionLabel(session: ChatSessionMeta) {
    const messageCount = messageCountFor(session.id);
    const status = sessionAwaitingInput(session.id)
      ? " · awaiting input"
      : sessionIsStreaming(session.id)
        ? " · streaming"
        : "";
    const pinned = session.pinned ? "★ " : "";
    return `${pinned}${session.name} (${messageCount})${status}`;
  }

  const conversationOptions = computed<DropdownOption[]>(() => {
    if (sessionsLoading.value) {
      return [
        {
          id: "",
          label: "Loading conversations...",
          value: "",
          disabled: true,
        },
      ];
    }
    if (!sessions.value.length) {
      return [
        { id: "", label: "No conversations yet", value: "", disabled: true },
      ];
    }
    return sessions.value.map((session) => ({
      id: session.id,
      label: conversationOptionLabel(session),
      value: session.id,
    }));
  });

  function selectSession(sessionId: string) {
    if (!sessionId) return;
    chat.selectSession(sessionId);
    autoScrollEnabled.value = true;
    nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
  }

  async function createSession(name = "New Chat") {
    try {
      await chat.createSession(name);
      autoScrollEnabled.value = true;
      nextTick(() => scrollMessagesToBottom({ force: true, behavior: "auto" }));
    } catch (error) {
      const status = httpStatus(error);
      if (status === 403) {
        // readonly
      }
    }
  }

  function sessionPinPending(sessionId: string) {
    return Boolean(pinningSessionIds.value[sessionId]);
  }

  function setSessionPinPending(sessionId: string, pending: boolean) {
    const next = { ...pinningSessionIds.value };
    if (pending) next[sessionId] = true;
    else delete next[sessionId];
    pinningSessionIds.value = next;
  }

  async function toggleSessionPinned(session: ChatSessionMeta) {
    if (!session?.id || sessionPinPending(session.id)) return;
    const nextPinned = !Boolean(session.pinned);
    setSessionPinPending(session.id, true);
    try {
      await chat.updateSessionPinned(session.id, nextPinned);
    } catch (error) {
      console.warn("Failed to update conversation pin state", error);
    } finally {
      setSessionPinPending(session.id, false);
    }
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

    const messages = await fetchChatMessages(sessionId);

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

  async function setSessionMemorySetting(event: Event) {
    const sessionId = activeSessionId.value;
    const checked = Boolean((event.target as HTMLInputElement | null)?.checked);
    if (!sessionId || isStreaming.value) return;
    memorySettingsSavingBySession.value = {
      ...memorySettingsSavingBySession.value,
      [sessionId]: true,
    };
    try {
      await chat.updateSessionMemorySettings(sessionId, {
        memoryEnabled: checked,
      });
    } catch (error) {
      console.warn("Failed to persist chat memory settings:", error);
    } finally {
      const { [sessionId]: _removed, ...rest } =
        memorySettingsSavingBySession.value;
      memorySettingsSavingBySession.value = rest;
    }
  }

  async function disableSessionCommandPolicyAllowAll() {
    const sessionId = activeSessionId.value;
    if (!sessionId || commandPolicyDisablePending.value) return;
    commandPolicyDisablePending.value = true;
    commandPolicyDisableError.value = "";
    try {
      await chat.updateSessionCommandPolicyAllowAll(sessionId, false);
    } catch (error) {
      console.warn("Failed to disable session command policy override:", error);
      commandPolicyDisableError.value = "Could not disable command override.";
    } finally {
      commandPolicyDisablePending.value = false;
    }
  }

  async function sendCurrentPrompt() {
    if (hasPendingInputRequest.value) return;
    await sendPrompt(draft.value);
  }

  async function sendPrompt(
    text: string,
    options: { echoUser?: boolean } = {},
  ) {
    const content = text.trim();
    if (!projectSelected.value) return;
    if (!content && !pendingAttachments.value.length) return;

    // New prompt: ensure any prior timer intervals are stopped before we start a new stream.
    stopAllResponseTimers();

    const previousDraft = draft.value;
    try {
      const sessionId = activeSessionId.value;
      const projectId = selectedProjectId.value.trim();
      if (sessionId && projectId) {
        void chat.updateSessionProject(sessionId, projectId).catch((error) => {
          console.warn("Failed to persist chat session project:", error);
        });
      }

      autoScrollEnabled.value = true;
      draft.value = options.echoUser === false ? draft.value : "";
      nextTick(() => autoSizeComposer());
      const attachmentsToSend = [...pendingAttachments.value];
      const filesByAttachmentSnapshot = new Map(filesByAttachment);
      if (attachmentsToSend.some((att) => att.kind === "image")) {
        pendingAttachments.value = pendingAttachments.value.filter(
          (att) => att.kind !== "image",
        );
      }
      const mentioned = resolveLeadingChatMention(
        content,
        chatMentionTargets.value,
      );
      let teamName = (selectedTeam.value || "").trim() || undefined;
      let routingSpecialist =
        (selectedSpecialist.value || "orchestrator").trim() || "orchestrator";
      let routingTargetName = routingSpecialist;
      if (mentioned.kind === "team" && mentioned.name) {
        teamName = mentioned.name;
        selectedTeam.value = teamName;
        selectedSpecialist.value = "orchestrator";
        routingSpecialist = "orchestrator";
        routingTargetName = teamName;
      } else if (mentioned.kind === "specialist" && mentioned.name) {
        routingSpecialist = mentioned.name;
        selectedSpecialist.value = routingSpecialist;
        routingTargetName = routingSpecialist;
      } else if (
        teamName &&
        routingSpecialist.toLowerCase() === "orchestrator"
      ) {
        routingTargetName = teamName;
      }
      const specialist =
        routingSpecialist.toLowerCase() !== "orchestrator"
          ? routingSpecialist
          : undefined;
      const { agentName, agentModel } = resolveAgentContext();
      await chat.sendPrompt(
        content,
        attachmentsToSend,
        filesByAttachmentSnapshot,
        {
          ...options,
          specialist,
          routingSpecialist,
          routingTargetName,
          teamName,
          projectId: projectId || undefined,
          memoryEnabled: memoryEnabled.value,
          image: imagePrompt.value,
          imageSize: "1K",
          agentName,
          agentModel,
        },
      );
    } catch (error) {
      if (options.echoUser !== false) {
        draft.value = previousDraft;
        nextTick(() => autoSizeComposer());
      }
      console.warn("Failed to send chat prompt:", error);
    } finally {
      pendingAttachments.value = [];
      filesByAttachment.clear();
    }
  }

  function stopStreaming() {
    chat.stopStreaming();
  }

  function canResumeDurableRun(message: ChatMessage) {
    return Boolean(
      message.role === "assistant" &&
      message.error &&
      message.runId &&
      !message.streaming &&
      !isStreaming.value,
    );
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
    const routingTargetName =
      teamName && !specialist ? teamName : routingSpecialist;
    const { agentName, agentModel } = resolveAgentContext();
    const sessionId = activeSessionId.value;
    const projectId = selectedProjectId.value.trim();
    if (sessionId && projectId) {
      await chat.updateSessionProject(sessionId, projectId);
    }
    await chat.regenerateAssistant({
      specialist,
      routingSpecialist,
      routingTargetName,
      teamName,
      projectId,
      memoryEnabled: memoryEnabled.value,
      agentName,
      agentModel,
      messageId: message.id,
    });
  }

  async function resumeDurableRun(message: ChatMessage) {
    const runId = message.runId?.trim();
    const sessionId = activeSessionId.value;
    if (!runId || !sessionId || message.role !== "assistant") return;
    await chat.resumeDurableRun(sessionId, message.id, runId);
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
      (message.agentModel || message.model || "").trim() ||
      defaults.model ||
      "";
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
        if (cand) selectMentionCandidate(cand);
        return;
      }
      if (event.key === "Tab") {
        const cand = mentionCandidates.value[mentionActiveIndex.value];
        if (cand) {
          event.preventDefault();
          selectMentionCandidate(cand);
        }
        return;
      }
    }

    if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
      event.preventDefault();
      sendCurrentPrompt();
    }
  }

  function handleComposerInput(nextDraft?: string) {
    if (typeof nextDraft === "string") draft.value = nextDraft;
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

  function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
    for (let i = items.length - 1; i >= 0; i -= 1) {
      if (predicate(items[i])) {
        return items[i];
      }
    }
    return null;
  }

  const {
    isRecording,
    canUseMic,
    startRecording,
    stopRecording,
    cleanupRecording,
  } = useChatVoiceRecording({
    draft,
    autoSizeComposer,
  });

  function setRenamingName(value: string) {
    renamingName.value = value;
  }

  function setSelectedProjectId(value: string) {
    selectedProjectId.value = value;
  }

  function setDraftValue(value: string) {
    draft.value = value;
  }

  function setImagePromptValue(value: boolean) {
    imagePrompt.value = value;
  }

  function setComposerRef(el: Element | ComponentPublicInstance | null) {
    composer.value = el as HTMLTextAreaElement | null;
  }

  function setFileInputRef(el: Element | ComponentPublicInstance | null) {
    fileInput.value = el as HTMLInputElement | null;
  }

  function triggerFilePicker() {
    fileInput.value?.click();
  }

  const sessionPanel = computed(() => ({
    activeSessionId: activeSessionId.value,
    activeSession: activeSession.value,
    renamingSessionId: renamingSessionId.value,
    renamingName: renamingName.value,
    sessionsLoading: sessionsLoading.value,
    sessions: sessions.value,
    conversationOptions: conversationOptions.value,
    setRenameInput,
    setRenamingName,
    cockpitContextDegrees: cockpitContextDegrees.value,
    cockpitContextPercent: cockpitContextPercent.value,
    cockpitContextLabel: cockpitContextLabel.value,
    cockpitToolCount: cockpitToolCount.value,
    cockpitToolRows: cockpitToolRows.value,
    sessionPinPending,
    selectSession,
    createSession,
    startRename,
    commitRename,
    cancelRename,
    toggleSessionPinned,
    exportSession,
    openDeleteSessionDialog,
  }));

  const headerPanel = computed(() => ({
    activeSession: activeSession.value,
    activeSummaryEvent: activeSummaryEvent.value,
    clearSummaryEvent: chat.clearSummaryEvent,
    commandPolicyAllowAllActive: commandPolicyAllowAllActive.value,
    commandPolicyDisablePending: commandPolicyDisablePending.value,
    commandPolicyDisableError: commandPolicyDisableError.value,
    disableSessionCommandPolicyAllowAll,
    projectOptions: projectOptions.value,
    selectedProjectId: selectedProjectId.value,
    setSelectedProjectId,
    memoryEnabled: memoryEnabled.value,
    activeMemorySettingsSaving: activeMemorySettingsSaving.value,
    isStreaming: isStreaming.value,
    setSessionMemorySetting,
  }));

  const timelinePanel = computed(() => ({
    cockpitTimelineLanes: cockpitTimelineLanes.value,
    cockpitTimelineTicks: cockpitTimelineTicks.value,
    runActivitySidebarLabel: runActivitySidebarLabel.value,
  }));

  const transcript = computed(() => ({
    displayUsername: displayUsername.value,
    hasOlderMessages: hasOlderMessages.value,
    olderMessagesLoading: olderMessagesLoading.value,
    olderMessagesError: olderMessagesError.value,
    chatMessages: chatMessages.value,
    loadOlderMessages,
    setMessagesPaneRef,
    handleMessagesScroll,
    handleMarkdownClick,
    visibleParticipantActivityItemsForMessage,
    isActivityCollapsed,
    expandActivity,
    collapseActivity,
    registerThreadBody,
    handleThreadBodyScroll,
    drawerBeforeEnter,
    drawerEnter,
    drawerAfterEnter,
    drawerBeforeLeave,
    drawerLeave,
    renderMarkdownOrHtml,
    shouldShowDirectActivity,
    shouldShowDirectThought,
    agentNameFor,
    hasMemoryContext,
    isMemoryContextExpanded,
    expandMemoryContext,
    collapseMemoryContext,
    memoryContextPillMeta,
    inputRequestCardClasses,
    inputRequestStatusLabel,
    submitInputRequest,
    inputRequestChoiceSelected,
    toggleInputRequestChoice,
    isInputRequestSubmitting,
    inputRequestLocalError,
    inputRequestKey,
    inputRequestFieldName,
    inputRequestDraft,
    setInputRequestDraft,
    isInputRequestRespondable,
    canSubmitInputRequest,
    inputRequestAnswerSummary,
    openImageModal,
    canResumeDurableRun,
    resumeDurableRun,
    copiedMessageId: copiedMessageId.value,
    copyMessage,
    regenerateAssistant,
    deleteChatMessage,
    isStreaming: isStreaming.value,
    labelForRole,
    shouldShowResponseTimer,
    responseElapsedMs,
    formatDuration,
    showScrollToBottom: showScrollToBottom.value,
    handleScrollToLatest,
  }));

  const composerPanel = computed(() => ({
    requiresProjectSelection: requiresProjectSelection.value,
    projectSelected: projectSelected.value,
    hasPendingInputRequest: hasPendingInputRequest.value,
    mentionMenuOpen: mentionMenuOpen.value,
    mentionCandidates: mentionCandidates.value,
    mentionActiveIndex: mentionActiveIndex.value,
    selectMentionCandidate,
    draft: draft.value,
    setDraftValue,
    composerPlaceholder: composerPlaceholder.value,
    setComposerRef,
    handleComposerKeydown,
    handleComposerInput,
    handleComposerKeyup,
    updateMentionState,
    setFileInputRef,
    triggerFilePicker,
    handleFileInputChange,
    handleDrop,
    pendingAttachments: pendingAttachments.value,
    imageAttachments: imageAttachments.value,
    textAttachments: textAttachments.value,
    removeAttachment,
    isRecording: isRecording.value,
    canUseMic,
    startRecording,
    stopRecording,
    imagePrompt: imagePrompt.value,
    setImagePromptValue,
    sendCurrentPrompt,
    stopStreaming,
    isStreaming: isStreaming.value,
  }));

  const participantsPanel = computed(() => ({
    selectedTeam: selectedTeam.value,
    setSelectedTeamValue,
    teamOptions: teamOptions.value,
    participantList: participantList.value,
    participantRowClasses,
    participantDotClasses,
    participantStatusLabel,
    openParticipantActivity,
  }));

  const modals = computed(() => ({
    showImageModal: showImageModal.value,
    modalImage: modalImage.value,
    modalImageSrc: modalImageSrc.value,
    closeImageModal,
    showDeleteSessionDialog: showDeleteSessionDialog.value,
    deleteSessionTarget: deleteSessionTarget.value,
    deleteSessionPending: deleteSessionPending.value,
    deleteSessionError: deleteSessionError.value,
    canConfirmDeleteSession: canConfirmDeleteSession.value,
    closeDeleteSessionDialog,
    confirmDeleteSession,
    selectedParticipantActivityName: selectedParticipantActivityName.value,
    selectedParticipantActivity: selectedParticipantActivity.value,
    selectedParticipantActivityItems: selectedParticipantActivityItems.value,
    setParticipantActivityPaneRef,
    handleActivityPaneScroll,
    closeParticipantActivity,
    renderMarkdownOrHtml,
  }));

  return {
    sessionPanel,
    headerPanel,
    timelinePanel,
    transcript,
    composerPanel,
    participantsPanel,
    modals,
  };
}

export type ChatViewModel = ReturnType<typeof useChatViewController>;
export type ChatSessionPanelModel = ChatViewModel["sessionPanel"]["value"];
export type ChatHeaderPanelModel = ChatViewModel["headerPanel"]["value"];
export type ChatTimelinePanelModel = ChatViewModel["timelinePanel"]["value"];
export type ChatTranscriptModel = ChatViewModel["transcript"]["value"];
export type ChatComposerPanelModel = ChatViewModel["composerPanel"]["value"];
export type ChatParticipantsPanelModel =
  ChatViewModel["participantsPanel"]["value"];
export type ChatModalsModel = ChatViewModel["modals"]["value"];
