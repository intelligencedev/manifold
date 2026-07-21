import { computed, nextTick, ref, watch } from "vue";
import type { ComputedRef, Ref } from "vue";
import type { SpecialistTeam } from "@/api/client";
import type { AgentThread, ChatMessage } from "@/types/chat";
import type {
  ChatContextMetricSegmentKind,
  ChatContextMetrics,
} from "@/types/chat";
import type { Participant } from "./chatTargeting";

export type ActivityStatus = "running" | "done" | "error" | "idle";
export type SpecialistActivityItem = {
  id: string;
  assistantMessageId?: string;
  callId?: string;
  parentCallId?: string;
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
export type CockpitTimelineTick = {
  id: string;
  label: string;
  position: string;
};
export type CockpitTimelineSegment = {
  id: string;
  label: string;
  status: ActivityStatus;
  left: string;
  width: string;
  durationLabel: string;
};
export type CockpitTimelineLane = {
  id: string;
  name: string;
  status: ActivityStatus;
  statusLabel: string;
  segments: CockpitTimelineSegment[];
};
export type CockpitContextSegmentKind =
  | ChatContextMetricSegmentKind
  | "other"
  | "remaining";
export type CockpitContextSegment = {
  id: CockpitContextSegmentKind;
  kind: CockpitContextSegmentKind;
  label: string;
  tokens: number;
  tokenLabel: string;
  percent: number;
  percentLabel: string;
  color: string;
};

export type ParticipantActivityGroup = {
  participant: Participant;
  item: SpecialistActivityItem;
  children: ParticipantActivityGroup[];
  running: boolean;
  collapsed: boolean;
};

export type ParticipantToolCall = {
  id: string;
  title: string;
  args: string;
  output: string;
  createdAt: string;
  createdAtMs: number;
  agentName: string;
  status: ActivityStatus;
};

type AgentContext = { agentName: string; agentModel: string };
type TeamConfig = SpecialistTeam;

export function useChatActivity(params: {
  isBrowser: boolean;
  activeMessages: ComputedRef<ChatMessage[]>;
  agentThreads: ComputedRef<AgentThread[]>;
  isStreaming: ComputedRef<boolean>;
  activeThoughtSummaries: ComputedRef<string[]>;
  selectedTeam: Ref<string>;
  selectedSpecialist: Ref<string>;
  selectedTeamConfig: ComputedRef<TeamConfig | null | undefined>;
  teamsByName: ComputedRef<Map<string, TeamConfig>>;
  participantList: ComputedRef<Participant[]>;
  sessionContextMetrics: ComputedRef<ChatContextMetrics | undefined>;
  resolveAgentContext: () => AgentContext;
  teamOrchestratorDisplayName: (team: TeamConfig) => string;
  responseElapsedMs: (message: ChatMessage) => number;
  scrollThreadBodyToBottom: (id: string) => void;
  scrollActivityPaneToBottom: (options?: {
    force?: boolean;
    behavior?: ScrollBehavior;
  }) => void;
  activityAutoScrollEnabled: Ref<boolean>;
  activityLastScrollTop: Ref<number>;
}) {
  const {
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
    sessionContextMetrics,
    resolveAgentContext,
    teamOrchestratorDisplayName,
    responseElapsedMs,
    scrollThreadBodyToBottom,
    scrollActivityPaneToBottom,
    activityAutoScrollEnabled,
    activityLastScrollTop,
  } = params;

  const COCKPIT_TIMELINE_TICK_MS = 5_000;
  const COCKPIT_TIMELINE_MIN_WINDOW_MS = 30_000;
  const COCKPIT_TIMELINE_LIVE_UPDATE_MS = 100;
  const CONTEXT_SEGMENT_ORDER: CockpitContextSegmentKind[] = [
    "system",
    "summary",
    "history",
    "user",
    "memory",
    "tools",
    "assistant",
    "other",
  ];
  const CONTEXT_SEGMENT_LABELS: Record<CockpitContextSegmentKind, string> = {
    system: "System",
    history: "History",
    user: "User",
    memory: "Memory",
    tools: "Tools",
    summary: "Summary",
    assistant: "Response",
    other: "Other",
    remaining: "Remaining",
  };
  const CONTEXT_SEGMENT_COLORS: Record<CockpitContextSegmentKind, string> = {
    system: "rgb(var(--color-subtle-foreground) / 0.72)",
    history: "rgb(88 166 255 / 0.82)",
    user: "rgb(71 199 132 / 0.92)",
    memory: "rgb(245 177 66 / 0.94)",
    tools: "rgb(168 130 255 / 0.92)",
    summary: "rgb(176 185 202 / 0.92)",
    assistant: "rgb(75 214 230 / 0.9)",
    other: "rgb(var(--color-accent) / 0.86)",
    remaining: "rgb(var(--color-border) / 0.54)",
  };
  const selectedActivityId = ref<string | null>(null);
  const selectedParticipantActivityName = ref<string | null>(null);
  const cockpitTimelineNowMs = ref(Date.now());
  let cockpitTimelineLiveInterval: number | null = null;

  const lastAssistant = computed(() =>
    findLast(activeMessages.value, (msg) => msg.role === "assistant"),
  );
  const lastAssistantId = computed(() => lastAssistant.value?.id || "");

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
      callId: thread.callId,
      parentCallId: thread.parentCallId,
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
      toolEntries: (assistant?.activityToolEntries || []).filter(
        (entry) => entry.type === "tool",
      ),
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
        (lastAssistant.value?.activityToolEntries?.length ?? 0) > 0 ||
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
    return message.role === "assistant" && shouldShowDirectThought(message);
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
  const manuallyExpandedParticipantActivityKeys = ref<Set<string>>(new Set());

  function setCollapsedActivityIds(ids: Set<string>) {
    collapsedActivityIds.value = ids;
  }

  function isActivityCollapsed(id: string): boolean {
    return collapsedActivityIds.value.has(id);
  }

  function collapseActivity(id: string) {
    setCollapsedActivityIds(new Set([...collapsedActivityIds.value, id]));
  }

  function expandActivity(id: string) {
    const next = new Set(collapsedActivityIds.value);
    next.delete(id);
    setCollapsedActivityIds(next);
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

  // Active participant activity expands while running, then collapses when complete.
  watch(
    () =>
      visibleParticipantActivityItems.value.map(
        (item) => `${item.id}:${item.status}`,
      ),
    () => {
      const nextCollapsed = new Set(collapsedActivityIds.value);
      const nextManual = new Set(manuallyExpandedParticipantActivityKeys.value);
      const liveIds = new Set(
        visibleParticipantActivityItems.value.map((item) => item.id),
      );

      for (const item of visibleParticipantActivityItems.value) {
        if (item.status === "running") {
          nextCollapsed.delete(item.id);
          continue;
        }
        if (nextManual.has(item.id)) continue;
        nextCollapsed.add(item.id);
      }
      for (const key of nextManual) {
        if (!liveIds.has(key)) nextManual.delete(key);
      }
      setCollapsedActivityIds(nextCollapsed);
      manuallyExpandedParticipantActivityKeys.value = nextManual;
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

  function isChildActivityOf(
    parent: SpecialistActivityItem,
    child: SpecialistActivityItem,
  ) {
    return Boolean(
      parent.callId &&
      child.parentCallId &&
      child.parentCallId.trim() === parent.callId.trim(),
    );
  }

  function directChildActivityItems(parent: SpecialistActivityItem) {
    return sortActivityItems(
      runActivityItems.value.filter((candidate) =>
        isChildActivityOf(parent, candidate),
      ),
    );
  }

  function participantActivityDescendantItems(participant: Participant) {
    const roots = participantActivityItems(participant);
    if (!roots.length) return [];

    const seen = new Set(roots.map((item) => item.id));
    const items = [...roots];
    let added = true;
    while (added) {
      added = false;
      for (const candidate of runActivityItems.value) {
        if (seen.has(candidate.id)) continue;
        if (items.some((item) => isChildActivityOf(item, candidate))) {
          seen.add(candidate.id);
          items.push(candidate);
          added = true;
        }
      }
    }

    return sortActivityItems(items);
  }

  function participantToolHistory(
    participant: Participant,
  ): ParticipantToolCall[] {
    const items = participantActivityDescendantItems(participant);
    const calls: ParticipantToolCall[] = [];
    for (const item of items) {
      for (const entry of item.toolEntries) {
        calls.push({
          id: `${item.id}:${entry.id}`,
          title: entry.title || "Tool call",
          args: entry.args || "",
          output: entry.content || entry.data || "",
          createdAt: entry.createdAt,
          createdAtMs: safeTimestampMs(entry.createdAt),
          agentName: item.name,
          status: item.status,
        });
      }
    }
    return calls.sort((a, b) => {
      if (a.createdAtMs !== b.createdAtMs) return a.createdAtMs - b.createdAtMs;
      return a.id.localeCompare(b.id);
    });
  }

  function participantLastToolCall(
    participant: Participant,
  ): ParticipantToolCall | null {
    const history = participantToolHistory(participant);
    return history.length ? history[history.length - 1] : null;
  }

  function hasDelegatedActivityForMessage(messageId: string) {
    return visibleParticipantActivityItemsForMessage(messageId).length > 0;
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
    return participant ? participantActivityDescendantItems(participant) : [];
  });

  function participantActivityGroupForItem(
    participant: Participant,
    item: SpecialistActivityItem,
    seen = new Set<string>(),
  ): ParticipantActivityGroup {
    seen.add(item.id);
    const children = directChildActivityItems(item)
      .filter((child) => !seen.has(child.id))
      .map((child) =>
        participantActivityGroupForItem(participant, child, new Set(seen)),
      );
    return {
      participant,
      item,
      children,
      running:
        item.status === "running" || children.some((child) => child.running),
      collapsed: isActivityCollapsed(item.id),
    };
  }

  function participantActivityGroups(participant: Participant) {
    const directItems = participantActivityItems(participant);
    const childCallIds = new Set(
      directItems
        .map((item) => item.parentCallId?.trim())
        .filter((parentCallId): parentCallId is string =>
          Boolean(parentCallId),
        ),
    );
    return directItems
      .filter((item) => !item.callId || !childCallIds.has(item.callId.trim()))
      .map((item) => participantActivityGroupForItem(participant, item));
  }

  const participantActivityGroupsByKey = computed(() => {
    const groups: Record<string, ParticipantActivityGroup[]> = {};
    for (const participant of participantList.value) {
      const activityGroups = participantActivityGroups(participant);
      if (activityGroups.length) {
        groups[participantActivityKey(participant)] = activityGroups;
      }
    }
    return groups;
  });

  function participantActivityGroupsFor(participant: Participant) {
    return (
      participantActivityGroupsByKey.value[
        participantActivityKey(participant)
      ] || []
    );
  }

  function participantActivityGroupKey(group: ParticipantActivityGroup) {
    return group.item.id;
  }

  function participantActivityGroupPrimaryItem(
    group: ParticipantActivityGroup,
  ) {
    return group.item;
  }

  function toggleParticipantActivityGroup(group: ParticipantActivityGroup) {
    const item = participantActivityGroupPrimaryItem(group);
    const key = participantActivityGroupKey(group);
    if (isActivityCollapsed(item.id)) {
      expandActivity(item.id);
      manuallyExpandedParticipantActivityKeys.value = new Set([
        ...manuallyExpandedParticipantActivityKeys.value,
        key,
      ]);
    } else {
      collapseActivity(item.id);
      const next = new Set(manuallyExpandedParticipantActivityKeys.value);
      next.delete(key);
      manuallyExpandedParticipantActivityKeys.value = next;
    }
  }

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
    if (!metrics?.contextWindow) return 0;
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
    if (!metrics?.contextWindow) return "Unknown";
    return `${metrics.inputTokens.toLocaleString()} / ${metrics.contextWindow.toLocaleString()}`;
  });
  const cockpitContextLegend = computed<CockpitContextSegment[]>(() => {
    const metrics = sessionContextMetrics.value;
    if (!metrics?.contextWindow) return [];
    const totals = new Map<CockpitContextSegmentKind, number>();
    for (const segment of metrics.segments) {
      if (segment.tokens <= 0) continue;
      totals.set(
        segment.kind,
        (totals.get(segment.kind) ?? 0) + segment.tokens,
      );
    }
    const segmentedTokens = Array.from(totals.values()).reduce(
      (sum, tokens) => sum + tokens,
      0,
    );
    const unsegmentedTokens = Math.max(
      0,
      metrics.inputTokens - segmentedTokens,
    );
    if (unsegmentedTokens > 0) totals.set("other", unsegmentedTokens);

    const legend = CONTEXT_SEGMENT_ORDER.flatMap((kind) => {
      const tokens = totals.get(kind) ?? 0;
      if (tokens <= 0) return [];
      return [contextLegendSegment(kind, tokens, metrics.contextWindow)];
    });
    const remainingTokens = Math.max(
      0,
      metrics.contextWindow - metrics.inputTokens,
    );
    if (remainingTokens > 0) {
      legend.push(
        contextLegendSegment(
          "remaining",
          remainingTokens,
          metrics.contextWindow,
        ),
      );
    }
    return legend;
  });
  const cockpitContextGradient = computed(() => {
    const metrics = sessionContextMetrics.value;
    if (!metrics?.contextWindow) {
      return "conic-gradient(rgb(var(--color-border) / 0.54) 0deg 360deg)";
    }
    let cursor = 0;
    const stops: string[] = [];
    for (const segment of cockpitContextLegend.value) {
      const end = Math.min(
        360,
        cursor + (segment.tokens / metrics.contextWindow) * 360,
      );
      if (end <= cursor) continue;
      stops.push(
        `${segment.color} ${cursor.toFixed(2)}deg ${end.toFixed(2)}deg`,
      );
      cursor = end;
      if (cursor >= 360) break;
    }
    if (!stops.length) {
      return "conic-gradient(rgb(var(--color-border) / 0.54) 0deg 360deg)";
    }
    return `conic-gradient(${stops.join(", ")})`;
  });
  const cockpitContextTitle = computed(() => {
    const metrics = sessionContextMetrics.value;
    if (!metrics) return "No context metrics yet";
    const lines = [
      `Context: ${metrics.inputTokens.toLocaleString()} / ${metrics.contextWindow.toLocaleString()} tokens`,
    ];
    for (const segment of cockpitContextLegend.value) {
      lines.push(
        `${segment.label}: ${segment.tokenLabel} (${segment.percentLabel})`,
      );
    }
    return lines.join("\n");
  });

  function contextLegendSegment(
    kind: CockpitContextSegmentKind,
    tokens: number,
    contextWindow: number,
  ): CockpitContextSegment {
    const percent = contextWindow > 0 ? (tokens / contextWindow) * 100 : 0;
    return {
      id: kind,
      kind,
      label: CONTEXT_SEGMENT_LABELS[kind],
      tokens,
      tokenLabel: tokens.toLocaleString(),
      percent,
      percentLabel: `${percent >= 10 ? percent.toFixed(0) : percent.toFixed(1)}%`,
      color: CONTEXT_SEGMENT_COLORS[kind],
    };
  }

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

  return {
    cockpitContextDegrees,
    cockpitContextPercent,
    cockpitContextLabel,
    cockpitContextLegend,
    cockpitContextGradient,
    cockpitContextTitle,
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
    stopAllActivityTimers: stopCockpitTimelineLiveUpdates,
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
    participantToolHistory,
    participantLastToolCall,
    closeParticipantActivity,
    activityStatusClasses,
    activityMonitorRowClasses,
    selectActivity,
  };
}

function safeTimestampMs(value?: string) {
  if (!value) return 0;
  const ms = Date.parse(value);
  return Number.isFinite(ms) ? ms : 0;
}

export function formatDuration(ms: number) {
  const clamped = Math.max(0, ms);
  const seconds = clamped / 1000;
  if (seconds < 60) return `${seconds.toFixed(1)}s`;
  const minutes = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${minutes}:${String(secs).padStart(2, "0")}`;
}

export function snippet(content: string, maxLength = 80) {
  if (!content) return "";
  const trimmed = content.replace(/\s+/g, " ").trim();
  const safeLength = Math.max(4, maxLength);
  return trimmed.length > safeLength
    ? `${trimmed.slice(0, safeLength - 3)}...`
    : trimmed;
}

function findLast<T>(items: T[], predicate: (item: T) => boolean): T | null {
  for (let i = items.length - 1; i >= 0; i -= 1) {
    if (predicate(items[i])) {
      return items[i];
    }
  }
  return null;
}
