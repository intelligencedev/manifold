import {
  computed,
  nextTick,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import type { ComponentPublicInstance } from "vue";
import axios from "axios";
import { useQuery } from "@tanstack/vue-query";
import { fetchAgentdSettings, listTeams, listSpecialists } from "@/api/client";
import { fetchChatMessages } from "@/api/chat";
import { renderMarkdown } from "@/utils/markdown";
import { useChatStore } from "@/stores/chat";
import { useProjectsStore } from "@/stores/projects";
import type { DropdownOption } from "@/types/dropdown";
import { useChatInputRequests } from "./chatInputRequests";
import { useChatScroll } from "./chatScroll";
import { useChatVoiceRecording } from "./chatVoiceRecording";
import { useChatTargeting } from "./chatTargeting";
import { useChatActivity } from "./chatActivity";
import { useChatModals } from "./useChatModals";
import { useChatResponseTimers } from "./useChatResponseTimers";
import { useChatSessionPanel } from "./useChatSessionPanel";
import { useChatComposer } from "./useChatComposer";
import { useChatTranscript } from "./useChatTranscript";

export function useChatViewController() {
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

  // --- Current user ---
  type CurrentUser = { name?: string; email?: string; picture?: string };
  const currentUser = ref<CurrentUser | null>(null);

  async function loadCurrentUser() {
    try {
      const res = await fetch("/api/me", { credentials: "include" });
      if (res.ok) {
        currentUser.value = await res.json();
        return;
      }
    } catch {
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

  // --- Core store-derived state ---
  const projects = computed(() => proj.projects);
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
  const composer = ref<HTMLTextAreaElement | null>(null);

  // --- Project override state ---
  const selectedProjectBySession = ref<Record<string, string>>({});
  const pinningSessionIds = ref<Record<string, boolean>>({});

  function hasSelectedProjectOverride(sessionId: string) {
    return Object.prototype.hasOwnProperty.call(
      selectedProjectBySession.value,
      sessionId,
    );
  }

  function projectIdForSession(sessionId: string) {
    return (
      sessions.value.find((s) => s.id === sessionId)?.projectId || ""
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

  const projectSelected = computed(() => Boolean(activeSessionId.value));
  const requiresProjectSelection = computed(() => false);

  // --- Input requests ---
  const inputRequests = useChatInputRequests({
    activeSessionId,
    chat,
  });

  // --- Render mode ---
  const renderMode = ref<"markdown" | "html">("markdown");
  const imagePrompt = ref(false);

  // --- Specialists / Teams queries ---
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

  // --- Project / render mode dropdown options ---
  const projectOptions = computed<DropdownOption[]>(() => {
    const projectEntries: DropdownOption[] = projects.value.map((p) => ({
      id: p.id,
      label: p.name,
      value: p.id,
    }));
    const lockedProjectID = selectedProjectId.value;
    if (
      lockedProjectID &&
      !projectEntries.some((p) => p.value === lockedProjectID)
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
      { id: "", label: "Select a project", value: "", disabled: true },
      ...projectEntries,
    ];
  });

  const renderModeOptions = computed<DropdownOption[]>(() => [
    { id: "markdown", label: "markdown", value: "markdown" },
    { id: "html", label: "html", value: "html" },
  ]);

  // --- Active session/messages derived state ---
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
  const activeSummaryEvent = computed(() => chat.activeSummaryEvent);
  const toolMessages = computed(() => chat.toolMessages);
  const activeThoughtSummaries = computed(() => chat.activeThoughtSummaries);

  // --- Memory / command policy state ---
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
        inputRequests.isInputRequestRespondable(request),
      ),
    ),
  );

  // --- Response timers ---
  const responseTimers = useChatResponseTimers(isBrowser);

  // --- Scroll ---
  const scroll = useChatScroll({
    activeSessionId,
    hasOlderMessages,
    olderMessagesLoading,
    loadOlderMessages: (sessionId: string) => chat.loadOlderMessages(sessionId),
  });

  // --- Session agent defaults ---
  const sessionAgentDefaults = computed(() =>
    parseAgentModelLabel(activeSession.value?.model || ""),
  );

  function parseAgentModelLabel(label?: string) {
    const raw = (label || "").trim();
    if (!raw) return { agentName: "", model: "" };
    const [maybeAgent, ...rest] = raw.split(":");
    if (rest.length) {
      return { agentName: maybeAgent, model: rest.join(":") };
    }
    return { agentName: "", model: raw };
  }

  // --- Targeting ---
  const targeting = useChatTargeting({
    activeSessionId,
    sessions,
    teams,
    teamsReady,
    specialists,
    draft,
    composer,
    autoSizeComposer: () => composerActions.autoSizeComposer(),
    sessionAgentDefaults,
    updateSessionActiveTarget: (sessionId, specialist, team) =>
      chat.updateSessionActiveTarget(sessionId, specialist, team),
  });

  // --- Composer (needs targeting, so we create it after targeting) ---
  const composerActions = useChatComposer({
    chat,
    activeSessionId,
    draft,
    composer,
    selectedProjectId,
    projectSelected,
    hasPendingInputRequest,
    memoryEnabled,
    imagePrompt,
    renderMode,
    isStreaming,
    activeMessages,
    targeting,
    inputRequests,
    responseTimers,
    resolveAgentContext: targeting.resolveAgentContext,
    selectedTeam: targeting.selectedTeam,
    selectedSpecialist: targeting.selectedSpecialist,
    chatMentionTargets: targeting.chatMentionTargets,
    autoScrollEnabled: scroll.autoScrollEnabled,
  });

  // --- Voice recording (needs draft and autoSizeComposer) ---
  const voiceRecording = useChatVoiceRecording({
    draft,
    autoSizeComposer: composerActions.autoSizeComposer,
  });
  // --- Transcript helpers ---
  const transcriptHelpers = useChatTranscript({
    activeMessages,
    activeSummaryEvent,
    summarySettingsData: summarySettingsQuery.data,
    selectedProjectId,
    activeSessionId,
  });

  // --- Modals ---
  const modalsState = useChatModals({
    activeSessionId,
    scrollMessagesToBottom: scroll.scrollMessagesToBottom,
    autoScrollEnabled: scroll.autoScrollEnabled,
  });

  // --- Session panel ---
  const sessionPanelState = useChatSessionPanel({
    chat,
    activeSessionId,
    sessions,
    sessionsLoading,
    messagesBySession,
    inputRequests,
    scroll,
    pinningSessionIds,
    selectedProjectBySession,
  });

  // --- Activity (cockpit/timeline/participant activity) ---
  const {
    cockpitContextDegrees,
    cockpitContextPercent,
    cockpitContextLabel,
    cockpitContextLegend,
    cockpitContextGradient,
    cockpitContextTitle,
    cockpitToolCount,
    cockpitToolRows,
    cockpitTimelineLanes,
    cockpitTimelineTicks,
    runActivitySidebarLabel,
    visibleParticipantActivityItemsForMessage,
    hasDelegatedActivityForMessage,
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
    participantActivityGroupsFor,
    participantActivityGroupKey,
    participantActivityGroupPrimaryItem,
    toggleParticipantActivityGroup,
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
    selectedTeam: targeting.selectedTeam,
    selectedSpecialist: targeting.selectedSpecialist,
    selectedTeamConfig: targeting.selectedTeamConfig,
    teamsByName: targeting.teamsByName,
    participantList: targeting.participantList,
    toolMessages,
    sessionContextMetrics: transcriptHelpers.sessionContextMetrics,
    resolveAgentContext: targeting.resolveAgentContext,
    teamOrchestratorDisplayName: targeting.teamOrchestratorDisplayName,
    responseElapsedMs: responseTimers.responseElapsedMs,
    scrollThreadBodyToBottom: scroll.scrollThreadBodyToBottom,
    scrollActivityPaneToBottom: scroll.scrollActivityPaneToBottom,
    activityAutoScrollEnabled: scroll.activityAutoScrollEnabled,
    activityLastScrollTop: scroll.activityLastScrollTop,
  });

  // --- Derived state for composer placeholder ---
  const composerPlaceholder = computed(() => {
    if (hasPendingInputRequest.value) {
      return "Answer the request above to continue.";
    }
    if (!projectSelected.value) {
      return "Select a project to enable the chat.";
    }
    const { agentName } = targeting.resolveAgentContext();
    return `Message ${agentName || "orchestrator"}...`;
  });

  const showScrollToBottom = computed(
    () => !scroll.autoScrollEnabled.value && chatMessages.value.length > 0,
  );

  // --- Watchers ---
  watch(
    () =>
      activeMessages.value.map(
        (msg) => `${msg.id}:${msg.content.length}:${msg.streaming ? 1 : 0}`,
      ),
    () => scroll.scrollMessagesToBottom(),
    { flush: "post" },
  );

  watch(
    () =>
      activeMessages.value.map(
        (m) => `${m.id}:${m.role}:${m.streaming ? 1 : 0}:${m.durationMs ?? ""}`,
      ),
    () => {
      for (const msg of activeMessages.value) {
        if (msg.role !== "assistant") continue;
        if (msg.streaming) responseTimers.ensureResponseTimer(msg);
        else if (
          responseTimers.persistedResponseDurationMs(msg) !== undefined ||
          msg.id in responseTimers.responseElapsedMsByMessageId.value
        ) {
          if (msg.error) responseTimers.pauseResponseTimer(msg.id);
          else responseTimers.finalizeResponseTimer(msg);
        }
      }
    },
    { flush: "post" },
  );

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
      void chat.loadMessagesFromServer(sessionId);
    }
    responseTimers.stopAllResponseTimers();
  });

  // --- Lifecycle ---
  onMounted(() => {
    void loadCurrentUser();
    void proj.refresh({ includeUsage: false });
    void chat.init();
    if (isBrowser) {
      previousBodyOverflow = document.body.style.overflow;
      document.body.style.overflow = "hidden";
    }
    nextTick(() => {
      composerActions.autoSizeComposer();
      scroll.scrollMessagesToBottom({ force: true, behavior: "auto" });
    });
  });

  onBeforeUnmount(() => {
    responseTimers.stopAllResponseTimers();
    stopCockpitTimelineLiveUpdates();
    voiceRecording.cleanupRecording();
    if (isBrowser && previousBodyOverflow !== null) {
      document.body.style.overflow = previousBodyOverflow;
    }
  });

  // --- Remaining actions ---
  function httpStatus(error: unknown): number | null {
    if (axios.isAxiosError(error)) {
      return error.response?.status ?? null;
    }
    return null;
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

  async function exportSession(sessionId: string) {
    const session = sessions.value.find((s) => s.id === sessionId);
    if (!session) return;

    const messages = await fetchChatMessages(sessionId);

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

    const safeName = (session.name || "chat")
      .replace(/[^a-zA-Z0-9_-]/g, "_")
      .slice(0, 50);
    const ts = new Date().toISOString().replace(/[:]/g, "-").slice(0, 19);
    const filename = `${safeName}-${ts}.json`;

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
    return transcriptHelpers.renderMarkdownOrHtml(
      content,
      renderMode.value,
      renderMarkdown,
    );
  }

  function setSelectedProjectId(value: string) {
    selectedProjectId.value = value;
  }

  function setRenamingName(value: string) {
    sessionPanelState.renamingName.value = value;
  }

  // --- Panel computed returns ---
  const sessionPanel = computed(() => ({
    activeSessionId: activeSessionId.value,
    activeSession: activeSession.value,
    renamingSessionId: sessionPanelState.renamingSessionId.value,
    renamingName: sessionPanelState.renamingName.value,
    sessionsLoading: sessionsLoading.value,
    sessions: sessions.value,
    conversationOptions: sessionPanelState.conversationOptions.value,
    setRenameInput: sessionPanelState.setRenameInput,
    setRenamingName,
    cockpitContextDegrees: cockpitContextDegrees.value,
    cockpitContextPercent: cockpitContextPercent.value,
    cockpitContextLabel: cockpitContextLabel.value,
    cockpitContextLegend: cockpitContextLegend.value,
    cockpitContextGradient: cockpitContextGradient.value,
    cockpitContextTitle: cockpitContextTitle.value,
    cockpitToolCount: cockpitToolCount.value,
    cockpitToolRows: cockpitToolRows.value,
    sessionPinPending: sessionPanelState.sessionPinPending,
    selectSession: sessionPanelState.selectSession,
    createSession: sessionPanelState.createSession,
    startRename: sessionPanelState.startRename,
    commitRename: sessionPanelState.commitRename,
    cancelRename: sessionPanelState.cancelRename,
    toggleSessionPinned: sessionPanelState.toggleSessionPinned,
    exportSession,
    openDeleteSessionDialog: modalsState.openDeleteSessionDialog,
    checkedSessionIds: sessionPanelState.checkedSessionIds.value,
    sessionDropdownOpen: sessionPanelState.sessionDropdownOpen.value,
    toggleSessionDropdown: sessionPanelState.toggleSessionDropdown,
    closeSessionDropdown: sessionPanelState.closeSessionDropdown,
    isSessionChecked: sessionPanelState.isSessionChecked,
    checkedSessionCount: sessionPanelState.checkedSessionCount.value,
    allSessionsChecked: sessionPanelState.allSessionsChecked.value,
    toggleSelectAll: sessionPanelState.toggleSelectAll,
    openBulkDeleteSessionDialog: modalsState.openBulkDeleteSessionDialog,
    messageCountFor: sessionPanelState.messageCountFor,
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
    loadOlderMessages: scroll.preserveMessagesScrollWhileLoadingOlder,
    setMessagesPaneRef: scroll.setMessagesPaneRef,
    handleMessagesScroll: scroll.handleMessagesScroll,
    handleMarkdownClick,
    visibleParticipantActivityItemsForMessage,
    hasDelegatedActivityForMessage,
    isActivityCollapsed,
    expandActivity,
    collapseActivity,
    registerThreadBody: scroll.registerThreadBody,
    handleThreadBodyScroll: scroll.handleThreadBodyScroll,
    drawerBeforeEnter,
    drawerEnter,
    drawerAfterEnter,
    drawerBeforeLeave,
    drawerLeave,
    renderMarkdownOrHtml,
    shouldShowDirectActivity,
    shouldShowDirectThought,
    agentNameFor: (message: any) =>
      transcriptHelpers.agentNameFor(message, sessionAgentDefaults.value),
    hasMemoryContext,
    isMemoryContextExpanded,
    expandMemoryContext,
    collapseMemoryContext,
    memoryContextPillMeta,
    inputRequestCardClasses: inputRequests.inputRequestCardClasses,
    inputRequestStatusLabel: inputRequests.inputRequestStatusLabel,
    submitInputRequest: inputRequests.submitInputRequest,
    inputRequestChoiceSelected: inputRequests.inputRequestChoiceSelected,
    toggleInputRequestChoice: inputRequests.toggleInputRequestChoice,
    isInputRequestSubmitting: inputRequests.isInputRequestSubmitting,
    inputRequestLocalError: inputRequests.inputRequestLocalError,
    inputRequestKey: inputRequests.inputRequestKey,
    inputRequestFieldName: inputRequests.inputRequestFieldName,
    inputRequestDraft: inputRequests.inputRequestDraft,
    setInputRequestDraft: inputRequests.setInputRequestDraft,
    isInputRequestRespondable: inputRequests.isInputRequestRespondable,
    canSubmitInputRequest: inputRequests.canSubmitInputRequest,
    inputRequestAnswerSummary: inputRequests.inputRequestAnswerSummary,
    openImageModal: modalsState.openImageModal,
    canResumeDurableRun: composerActions.canResumeDurableRun,
    resumeDurableRun: composerActions.resumeDurableRun,
    copiedMessageId: composerActions.copiedMessageId.value,
    copyMessage: composerActions.copyMessage,
    regenerateAssistant: composerActions.regenerateAssistant,
    deleteChatMessage: composerActions.deleteChatMessage,
    isStreaming: isStreaming.value,
    labelForRole: transcriptHelpers.labelForRole,
    shouldShowResponseTimer: responseTimers.shouldShowResponseTimer,
    responseElapsedMs: responseTimers.responseElapsedMs,
    formatDuration: responseTimers.formatDuration,
    showScrollToBottom: showScrollToBottom.value,
    handleScrollToLatest: scroll.handleScrollToLatest,
  }));

  const composerPanel = computed(() => ({
    requiresProjectSelection: requiresProjectSelection.value,
    projectSelected: projectSelected.value,
    hasPendingInputRequest: hasPendingInputRequest.value,
    mentionMenuOpen: targeting.mentionMenuOpen.value,
    mentionCandidates: targeting.mentionCandidates.value,
    mentionActiveIndex: targeting.mentionActiveIndex.value,
    selectMentionCandidate: targeting.selectMentionCandidate,
    draft: draft.value,
    setDraftValue: composerActions.setDraftValue,
    composerPlaceholder: composerPlaceholder.value,
    setComposerRef: composerActions.setComposerRef,
    handleComposerKeydown: composerActions.handleComposerKeydown,
    handleComposerInput: composerActions.handleComposerInput,
    handleComposerKeyup: composerActions.handleComposerKeyup,
    updateMentionState: targeting.updateMentionState,
    setFileInputRef: composerActions.setFileInputRef,
    triggerFilePicker: composerActions.triggerFilePicker,
    handleFileInputChange: composerActions.handleFileInputChange,
    handleDrop: composerActions.handleDrop,
    pendingAttachments: composerActions.pendingAttachments.value,
    imageAttachments: composerActions.imageAttachments.value,
    textAttachments: composerActions.textAttachments.value,
    removeAttachment: composerActions.removeAttachment,
    isRecording: voiceRecording.isRecording.value,
    canUseMic: voiceRecording.canUseMic,
    startRecording: voiceRecording.startRecording,
    stopRecording: voiceRecording.stopRecording,
    imagePrompt: imagePrompt.value,
    setImagePromptValue: composerActions.setImagePromptValue,
    sendCurrentPrompt: composerActions.sendCurrentPrompt,
    stopStreaming: composerActions.stopStreaming,
    isStreaming: isStreaming.value,
  }));

  const participantsPanel = computed(() => ({
    selectedTeam: targeting.selectedTeam.value,
    setSelectedTeamValue: targeting.setSelectedTeamValue,
    teamOptions: targeting.teamOptions.value,
    participantList: targeting.participantList.value,
    participantRowClasses,
    participantDotClasses,
    participantStatusLabel,
    participantActivityGroupsFor,
    participantActivityGroupKey,
    participantActivityGroupPrimaryItem,
    toggleParticipantActivityGroup,
    openParticipantActivity,
    isActivityCollapsed,
    expandActivity,
    collapseActivity,
    registerThreadBody: scroll.registerThreadBody,
    handleThreadBodyScroll: scroll.handleThreadBodyScroll,
    drawerBeforeEnter,
    drawerEnter,
    drawerAfterEnter,
    drawerBeforeLeave,
    drawerLeave,
    renderMarkdownOrHtml,
  }));

  const modals = computed(() => ({
    showImageModal: modalsState.showImageModal.value,
    modalImage: modalsState.modalImage.value,
    modalImageSrc: modalsState.modalImageSrc.value,
    closeImageModal: modalsState.closeImageModal,
    showDeleteSessionDialog: modalsState.showDeleteSessionDialog.value,
    deleteSessionTarget: modalsState.deleteSessionTarget.value,
    deleteSessionPending: modalsState.deleteSessionPending.value,
    confirmDeleteSession: () =>
      modalsState.confirmDeleteSession((id) => chat.deleteSession(id)),
    showBulkDeleteSessionDialog: modalsState.showBulkDeleteSessionDialog.value,
    bulkDeleteSessionPending: modalsState.bulkDeleteSessionPending.value,
    bulkDeleteSessionError: modalsState.bulkDeleteSessionError.value,
    bulkDeleteSessionCount: modalsState.bulkDeleteSessionCount.value,
    canConfirmBulkDeleteSession: modalsState.canConfirmBulkDeleteSession.value,
    closeBulkDeleteSessionDialog: modalsState.closeBulkDeleteSessionDialog,
    confirmBulkDeleteSession: () =>
      modalsState.confirmBulkDeleteSession((id) => chat.deleteSession(id)),
    selectedParticipantActivityName: selectedParticipantActivityName.value,
    selectedParticipantActivityItems: selectedParticipantActivityItems.value,
    setParticipantActivityPaneRef: scroll.setParticipantActivityPaneRef,
    handleActivityPaneScroll: scroll.handleActivityPaneScroll,
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
