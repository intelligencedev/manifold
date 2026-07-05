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
  AgentThread,
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
import { useChatTargeting, type Participant } from "./chatTargeting";

export function useChatViewController() {
  const router = useRouter();
  const isBrowser = typeof window !== "undefined";
  const COCKPIT_TIMELINE_TICK_MS = 5_000;
  const COCKPIT_TIMELINE_MIN_WINDOW_MS = 30_000;
  const COCKPIT_TIMELINE_LIVE_UPDATE_MS = 100;
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
  const selectedActivityId = ref<string | null>(null);
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
  const toolActivityMsById = ref<Record<string, number>>({});
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
  const cockpitTimelineNowMs = ref(Date.now());
  let cockpitTimelineLiveInterval: number | null = null;

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

  type ActivityStatus = "running" | "done" | "error" | "idle";
  type SpecialistActivityItem = {
    id: string;
    assistantMessageId?: string;
    name: string;
    team?: string;
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
  type CockpitToolRow = {
    id: string;
    name: string;
    status: string;
    statusTone: ActivityStatus;
    args: string;
    output: string;
  };
  type CockpitTimelineTick = {
    id: string;
    label: string;
    position: string;
  };
  type CockpitTimelineSegment = {
    id: string;
    label: string;
    status: ActivityStatus;
    left: string;
    width: string;
    durationLabel: string;
  };
  type CockpitTimelineLane = {
    id: string;
    name: string;
    status: ActivityStatus;
    statusLabel: string;
    segments: CockpitTimelineSegment[];
  };
  type Participant = {
    id: string;
    name: string;
    model: string;
    kind: "specialist" | "team_orchestrator";
    routeName: string;
    mentionName: string;
    teamName?: string;
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

  function activityStartMs(item: SpecialistActivityItem) {
    return safeTimestampMs(item.startedAt) || item.updatedAt || Date.now();
  }

  function activityEndMs(item: SpecialistActivityItem, fallbackEndMs: number) {
    const start = activityStartMs(item);
    if (item.finishedAt) {
      const finished = safeTimestampMs(item.finishedAt);
      if (finished) return Math.max(start, finished);
    }
    if (item.status === "running") return Math.max(start, fallbackEndMs);
    return Math.max(start, fallbackEndMs, item.updatedAt || 0);
  }

  function roundedTimelineStart(ms: number) {
    return Math.floor(ms / COCKPIT_TIMELINE_TICK_MS) * COCKPIT_TIMELINE_TICK_MS;
  }

  function formatTimelineOffset(ms: number) {
    const totalSeconds = Math.max(0, Math.round(ms / 1000));
    const minutes = Math.floor(totalSeconds / 60);
    const seconds = totalSeconds % 60;
    return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
  }

  function percentBetween(value: number, start: number, span: number) {
    if (span <= 0) return "0%";
    const pct = Math.min(100, Math.max(0, ((value - start) / span) * 100));
    return `${pct.toFixed(2)}%`;
  }

  function statusToneFromLabel(label: string): ActivityStatus {
    const normalized = label.toLowerCase();
    if (normalized.includes("error") || normalized.includes("fail")) {
      return "error";
    }
    if (normalized.includes("running") || normalized.includes("live")) {
      return "running";
    }
    if (normalized.includes("queue") || normalized.includes("pending")) {
      return "idle";
    }
    return "done";
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
    const latestThought =
      thread.thoughtSummaries[thread.thoughtSummaries.length - 1];
    if (latestThought) return snippet(latestThought, 96);
    if (thread.content) return snippet(thread.content, 96);
    if (thread.prompt) return snippet(thread.prompt, 96);
    return thread.status === "running" ? "Working" : "No details yet";
  }

  function activityItemFromThread(thread: AgentThread): SpecialistActivityItem {
    const name =
      (thread.agent || "Delegated agent").trim() || "Delegated agent";
    const status = thread.status as ActivityStatus;
    const toolEntries = thread.entries.filter((entry) => entry.type === "tool");
    const team = (thread.team || "").trim() || undefined;
    return {
      assistantMessageId: thread.assistantMessageId,
      id: [thread.assistantMessageId, thread.callId].filter(Boolean).join(":"),
      name,
      team,
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
    const teamName = selectedTeamConfig.value?.name?.trim() || undefined;
    const assistant = lastAssistant.value;
    const status: ActivityStatus = assistant?.error
      ? "error"
      : isStreaming.value
        ? "running"
        : assistant?.content
          ? "done"
          : "idle";
    const latestThought =
      activeThoughtSummaries.value[activeThoughtSummaries.value.length - 1];
    const now = cockpitTimelineNowMs.value;
    const assistantStartMs = assistant
      ? safeTimestampMs(assistant.createdAt) || now
      : now;
    const assistantElapsedMs = assistant ? responseElapsedMs(assistant) : 0;
    const assistantEndMs =
      assistant &&
      status !== "running" &&
      status !== "idle" &&
      assistantElapsedMs > 0
        ? assistantStartMs + assistantElapsedMs
        : undefined;
    const startedAt = new Date(assistantStartMs).toISOString();
    const finishedAt = assistantEndMs
      ? new Date(assistantEndMs).toISOString()
      : undefined;
    return {
      id: "orchestrator",
      name: agentName || "orchestrator",
      team: teamName,
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
      startedAt,
      finishedAt,
      updatedAt:
        status === "running" ? now : (assistantEndMs ?? assistantStartMs),
      depth: 0,
      isOrchestrator: true,
    };
  }

  function sortActivityItems(items: SpecialistActivityItem[]) {
    return items.sort((a, b) => {
      if (a.status === "running" && b.status !== "running") return -1;
      if (a.status !== "running" && b.status === "running") return 1;
      return b.updatedAt - a.updatedAt;
    });
  }

  function runActivityItemsForMessage(messageId: string) {
    if (!messageId) return [];
    const items = agentThreads.value
      .filter((thread) => thread.assistantMessageId === messageId)
      .map(activityItemFromThread);
    const isLastAssistantMessage = messageId === lastAssistantId.value;
    const shouldShowOrchestrator =
      isLastAssistantMessage &&
      (activeThoughtSummaries.value.length > 0 ||
        (isStreaming.value && items.length === 0) ||
        (Boolean(selectedTeamConfig.value) && Boolean(lastAssistant.value)));
    if (shouldShowOrchestrator) items.unshift(orchestratorActivityItem());

    return sortActivityItems(items);
  }

  const runActivityItems = computed<SpecialistActivityItem[]>(() =>
    runActivityItemsForMessage(lastAssistantId.value),
  );

  const visibleParticipantActivityItems = computed(() =>
    runActivityItems.value.filter((item) => !item.isOrchestrator),
  );

  function visibleParticipantActivityItemsForMessage(messageId: string) {
    return runActivityItemsForMessage(messageId).filter(
      (item) => !item.isOrchestrator,
    );
  }

  const runActivityCounts = computed(() => {
    const counts = { running: 0, done: 0, error: 0, idle: 0 };
    for (const item of runActivityItems.value) counts[item.status] += 1;
    return counts;
  });

  const runActivityState = computed<ActivityStatus>(() => {
    if (runActivityCounts.value.error > 0) return "error";
    if (runActivityCounts.value.running > 0 || isStreaming.value)
      return "running";
    if (runActivityCounts.value.done > 0) return "done";
    return "idle";
  });

  const runActivityStateLabel = computed(() =>
    activityStateLabel(runActivityState.value),
  );

  const runActivitySidebarLabel = computed(() => {
    const count = runActivityItems.value.length;
    if (!count) return "Idle";
    return `${count} thread${count === 1 ? "" : "s"}`;
  });

  const selectedActivityItem = computed(() => {
    const selected = selectedActivityId.value;
    return (
      visibleParticipantActivityItems.value.find(
        (item) => item.id === selected,
      ) ||
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

  function hasMemoryContext(message: ChatMessage) {
    return Boolean(message.memoryContext?.text?.trim());
  }

  function returnedMemoryLaneCount(message: ChatMessage) {
    const lanes = message.memoryContext?.lanes;
    if (!lanes) return 0;
    return Object.values(lanes).filter(
      (lane) => lane.returned || (lane.items ?? 0) > 0,
    ).length;
  }

  function memoryContextPillMeta(message: ChatMessage) {
    const context = message.memoryContext;
    if (!context) return "";
    const parts: string[] = [];
    const laneCount = returnedMemoryLaneCount(message);
    if (laneCount > 0) {
      parts.push(`${laneCount} ${laneCount === 1 ? "lane" : "lanes"}`);
    }
    if (context.tokenEstimate && context.tokenEstimate > 0) {
      parts.push(`${context.tokenEstimate.toLocaleString()} tokens`);
    }
    if (context.truncated) {
      parts.push("truncated");
    }
    return parts.join(" · ");
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

  const expandedMemoryContextIds = ref<Set<string>>(new Set());

  function isMemoryContextExpanded(id: string): boolean {
    return expandedMemoryContextIds.value.has(id);
  }

  function collapseMemoryContext(id: string) {
    const next = new Set(expandedMemoryContextIds.value);
    next.delete(id);
    expandedMemoryContextIds.value = next;
  }

  function expandMemoryContext(id: string) {
    expandedMemoryContextIds.value = new Set([
      ...expandedMemoryContextIds.value,
      id,
    ]);
  }

  // Drawer JS transition hooks
  function drawerBeforeEnter(el: Element) {
    const e = el as HTMLElement;
    e.style.height = "0";
    e.style.overflow = "hidden";
  }
  function drawerEnter(el: Element, done: () => void) {
    const e = el as HTMLElement;
    const h = e.scrollHeight;
    e.style.transition = "height 0.28s cubic-bezier(0.4, 0, 0.2, 1)";
    e.style.height = h + "px";
    e.addEventListener("transitionend", done, { once: true });
  }
  function drawerAfterEnter(el: Element) {
    const e = el as HTMLElement;
    e.style.height = "auto";
    e.style.overflow = "";
    e.style.transition = "";
  }
  function drawerBeforeLeave(el: Element) {
    const e = el as HTMLElement;
    e.style.height = e.scrollHeight + "px";
    e.style.overflow = "hidden";
  }
  function drawerLeave(el: Element, done: () => void) {
    const e = el as HTMLElement;
    requestAnimationFrame(() => {
      e.style.transition = "height 0.22s cubic-bezier(0.4, 0, 0.2, 1)";
      e.style.height = "0";
      e.addEventListener("transitionend", done, { once: true });
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
    () =>
      visibleParticipantActivityItems.value.map(
        (i) =>
          `${i.id}:${i.description}:${i.thoughtSummaries.length}:${i.response.length}`,
      ),
    () => {
      for (const item of visibleParticipantActivityItems.value) {
        scrollThreadBodyToBottom(item.id);
      }
    },
    { flush: "post" },
  );

  // Auto-collapse parallel activity cards only after all running threads finish
  watch(
    () =>
      visibleParticipantActivityItems.value.map((i) => `${i.id}:${i.status}`),
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

  function participantActivityKey(participant: Participant) {
    if (participant.kind === "team_orchestrator") {
      const teamName = (participant.teamName || participant.mentionName)
        .trim()
        .toLowerCase();
      return `team:${teamName}:orchestrator`;
    }
    return `specialist:${participant.routeName.trim().toLowerCase()}`;
  }

  function activityItemKey(item: SpecialistActivityItem) {
    const team = (item.team || "").trim();
    const name = item.name.trim();
    const teamConfig = teamsByName.value.get(team.toLowerCase());
    const orchestrator = teamConfig
      ? teamOrchestratorDisplayName(teamConfig).toLowerCase()
      : "";
    if (
      team &&
      (name.toLowerCase() === "orchestrator" ||
        name.toLowerCase() === orchestrator)
    ) {
      return `team:${team.toLowerCase()}:orchestrator`;
    }
    if (item.isOrchestrator && team) {
      return `team:${team.toLowerCase()}:orchestrator`;
    }
    return `specialist:${name.toLowerCase()}`;
  }

  function participantActivityItems(participant: Participant) {
    const key = participantActivityKey(participant);
    return runActivityItems.value.filter(
      (item) => activityItemKey(item) === key,
    );
  }

  const selectedParticipantActivity = computed(() => {
    const key = selectedParticipantActivityName.value;
    if (!key) return null;
    return (
      participantList.value.find(
        (participant) => participantActivityKey(participant) === key,
      ) || null
    );
  });

  const selectedParticipantActivityItems = computed(() => {
    const participant = selectedParticipantActivity.value;
    return participant ? participantActivityItems(participant) : [];
  });

  function openParticipantActivity(participant: Participant) {
    selectedParticipantActivityName.value = participantActivityKey(participant);
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
      "activity-monitor-row--selected":
        selectedActivityItem.value?.id === item.id,
      "activity-monitor-row--running": item.status === "running",
      "activity-monitor-row--error": item.status === "error",
    };
  }

  function participantIsActive(participant: Participant) {
    const key = participantActivityKey(participant);

    // Find the currently streaming assistant message to determine who is live.
    const streamingMsg = activeMessages.value.find(
      (m) => m.role === "assistant" && m.streaming,
    );

    if (streamingMsg) {
      const liveTeam = selectedTeam.value.trim();
      const liveAgent = (streamingMsg.agentName || streamingMsg.agent || "")
        .trim()
        .toLowerCase();
      const liveTeamConfig = teamsByName.value.get(liveTeam.toLowerCase());
      const liveTeamOrchestrator = liveTeamConfig
        ? teamOrchestratorDisplayName(liveTeamConfig).toLowerCase()
        : "";
      const liveKey =
        liveTeam &&
        (liveAgent === liveTeamOrchestrator || liveAgent === "orchestrator")
          ? `team:${liveTeam.toLowerCase()}:orchestrator`
          : `specialist:${(
              liveAgent ||
              selectedSpecialist.value ||
              "orchestrator"
            ).toLowerCase()}`;

      // During streaming, only the agent whose name matches is live.
      // Never mark orchestrator live just because runActivityCounts > 0 here —
      // that count includes the specialist itself and causes false positives.
      return liveKey === key;
    }

    // No active stream: fall back to agent-thread activity counts.
    if (
      participant.kind === "specialist" &&
      participant.routeName.toLowerCase() === "orchestrator"
    ) {
      return runActivityCounts.value.running > 0;
    }
    return visibleParticipantActivityItems.value.some(
      (item) => item.status === "running" && activityItemKey(item) === key,
    );
  }

  function participantStatusLabel(participant: Participant) {
    const active = participantIsActive(participant);
    if (active) return "Live";
    if (
      participant.kind === "specialist" &&
      participant.routeName.toLowerCase() === "orchestrator"
    ) {
      const label = runActivityStateLabel.value;
      return label && label.toLowerCase() !== "completed" ? label : "Idle";
    }
    const key = participantActivityKey(participant);
    const item = runActivityItems.value.find(
      (activity) => activityItemKey(activity) === key,
    );
    if (!item) return "Idle";
    const lbl = item.statusLabel;
    return lbl && lbl.toLowerCase() !== "completed" ? lbl : "Idle";
  }

  function participantRowClasses(participant: Participant) {
    return {
      "participant-row--active": participantIsActive(participant),
    };
  }

  function participantDotClasses(participant: Participant) {
    const active = participantIsActive(participant);
    return {
      "participant-dot--active": active,
      "participant-dot--idle": !active,
    };
  }

  const cockpitActivityToolCount = computed(() =>
    runActivityItems.value.reduce(
      (count, item) => count + item.toolEntries.length,
      0,
    ),
  );
  const cockpitToolCount = computed(
    () => cockpitActivityToolCount.value || toolMessages.value.length,
  );
  const cockpitAgentContext = computed(() => resolveAgentContext());
  const cockpitTimelineItems = computed(() =>
    [...runActivityItems.value]
      .sort((a, b) => activityStartMs(a) - activityStartMs(b))
      .slice(0, 5),
  );
  const hasLiveCockpitTimelineItems = computed(() =>
    cockpitTimelineItems.value.some((item) => item.status === "running"),
  );

  function cockpitTimelineFallbackEndMs(
    item: SpecialistActivityItem,
    now: number,
  ) {
    if (item.status === "running") return now;
    const latestEntry = Math.max(
      0,
      ...item.toolEntries.map((entry) => safeTimestampMs(entry.createdAt)),
    );
    return Math.max(latestEntry, item.updatedAt || 0);
  }

  const cockpitTimelineWindow = computed(() => {
    const now = cockpitTimelineNowMs.value;
    const items = cockpitTimelineItems.value;
    if (!items.length) {
      const startMs = roundedTimelineStart(now);
      return {
        startMs,
        spanMs: COCKPIT_TIMELINE_MIN_WINDOW_MS,
        stepMs: COCKPIT_TIMELINE_TICK_MS,
      };
    }

    const starts = items.map(activityStartMs);
    const earliest = Math.min(...starts);
    const startMs = roundedTimelineStart(earliest);
    const latest = Math.max(
      ...items.map((item) =>
        activityEndMs(item, cockpitTimelineFallbackEndMs(item, now)),
      ),
    );
    const rawSpan = Math.max(
      COCKPIT_TIMELINE_MIN_WINDOW_MS,
      latest - startMs + COCKPIT_TIMELINE_TICK_MS,
    );
    const stepMs =
      Math.ceil(rawSpan / 6 / COCKPIT_TIMELINE_TICK_MS) *
      COCKPIT_TIMELINE_TICK_MS;
    return {
      startMs,
      spanMs: stepMs * 6,
      stepMs,
    };
  });
  const cockpitTimelineTicks = computed<CockpitTimelineTick[]>(() => {
    const window = cockpitTimelineWindow.value;
    return Array.from({ length: 7 }, (_, index) => {
      const offset = index * window.stepMs;
      return {
        id: String(index),
        label: formatTimelineOffset(offset),
        position: percentBetween(
          window.startMs + offset,
          window.startMs,
          window.spanMs,
        ),
      };
    });
  });
  const cockpitTimelineLanes = computed<CockpitTimelineLane[]>(() => {
    const now = cockpitTimelineNowMs.value;
    const window = cockpitTimelineWindow.value;
    return cockpitTimelineItems.value.map((item) => {
      const start = activityStartMs(item);
      const end = activityEndMs(item, cockpitTimelineFallbackEndMs(item, now));
      const left = percentBetween(start, window.startMs, window.spanMs);
      const durationPercent = Math.max(
        2.6,
        ((Math.max(end, start + 800) - start) / window.spanMs) * 100,
      );
      return {
        id: item.id,
        name: item.name,
        status: item.status,
        statusLabel: item.statusLabel,
        segments: [
          {
            id: `${item.id}:activity`,
            label: `${item.name} ${item.statusLabel} ${formatDuration(end - start)}`,
            status: item.status,
            left,
            width: `${Math.min(100, durationPercent).toFixed(2)}%`,
            durationLabel: formatDuration(end - start),
          },
        ],
      };
    });
  });
  const cockpitContextPercent = computed(() => {
    const metrics = sessionContextMetrics.value;
    if (!metrics.contextWindow) return 0;
    return Math.min(
      100,
      Math.round((metrics.inputTokens / metrics.contextWindow) * 100),
    );
  });
  const cockpitContextDegrees = computed(
    () => `${cockpitContextPercent.value * 3.6}deg`,
  );
  const cockpitContextLabel = computed(() => {
    const metrics = sessionContextMetrics.value;
    if (!metrics.contextWindow) return "Unknown";
    return `${metrics.inputTokens.toLocaleString()} / ${metrics.contextWindow.toLocaleString()}`;
  });
  const cockpitToolRows = computed<CockpitToolRow[]>(() => {
    const traceRows = runActivityItems.value.flatMap((item) =>
      item.toolEntries.map((entry) => ({
        id: `${item.id}:${entry.id}`,
        name: entry.title || "Tool call",
        status: item.statusLabel,
        statusTone: item.status,
        args: entry.args || "",
        output: entry.content || entry.data || "",
      })),
    );
    if (traceRows.length) return traceRows.slice(-25);

    return toolMessages.value.slice(-25).map((message) => {
      const status = message.error
        ? "Error"
        : message.streaming
          ? "Running"
          : "Done";
      return {
        id: message.id,
        name: message.activityToolTitle || message.title || "Tool call",
        status,
        statusTone: statusToneFromLabel(status),
        args: message.toolArgs || "",
        output: message.error || message.content || "",
      };
    });
  });

  function stopCockpitTimelineLiveUpdates() {
    if (cockpitTimelineLiveInterval == null) return;
    if (isBrowser) window.clearInterval(cockpitTimelineLiveInterval);
    cockpitTimelineLiveInterval = null;
  }

  function startCockpitTimelineLiveUpdates() {
    cockpitTimelineNowMs.value = Date.now();
    if (!isBrowser || cockpitTimelineLiveInterval != null) return;
    cockpitTimelineLiveInterval = window.setInterval(() => {
      cockpitTimelineNowMs.value = Date.now();
    }, COCKPIT_TIMELINE_LIVE_UPDATE_MS);
  }

  watch(
    hasLiveCockpitTimelineItems,
    (hasLiveItems) => {
      if (hasLiveItems) {
        startCockpitTimelineLiveUpdates();
      } else {
        cockpitTimelineNowMs.value = Date.now();
        stopCockpitTimelineLiveUpdates();
      }
    },
    { immediate: true, flush: "post" },
  );

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
      selectedActivityId.value =
        visibleParticipantActivityItems.value[0]?.id || null;
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
