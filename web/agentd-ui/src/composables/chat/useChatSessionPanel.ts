import { computed, nextTick, ref, watch } from "vue";
import type { ComponentPublicInstance } from "vue";
import type { Ref } from "vue";
import type {
  ChatInputRequest,
  ChatMessage,
  ChatSessionMeta,
  ChatRole,
} from "@/types/chat";
import type { DropdownOption } from "@/types/dropdown";

type ChatStoreActions = {
  selectSession: (sessionId: string) => void;
  createSession: (name?: string) => Promise<void>;
  renameSession: (sessionId: string, name: string) => Promise<void>;
  updateSessionPinned: (sessionId: string, pinned: boolean) => Promise<unknown>;
  deleteSession: (sessionId: string) => Promise<void>;
  isSessionStreaming: (sessionId: string) => boolean;
};

type InputRequestChecker = {
  isInputRequestRespondable: (request: ChatInputRequest) => boolean;
};

type ScrollActions = {
  scrollMessagesToBottom: (options?: {
    force?: boolean;
    behavior?: ScrollBehavior;
  }) => void;
  autoScrollEnabled: Ref<boolean>;
};

type PinningSessionIds = Ref<Record<string, boolean>>;
type SelectedProjectBySession = Ref<Record<string, string>>;

export function useChatSessionPanel({
  chat,
  activeSessionId,
  sessions,
  sessionsLoading,
  messagesBySession,
  inputRequests,
  scroll,
  pinningSessionIds,
  selectedProjectBySession,
}: {
  chat: ChatStoreActions;
  activeSessionId: Ref<string>;
  sessions: Ref<ChatSessionMeta[]>;
  sessionsLoading: Ref<boolean>;
  messagesBySession: Ref<Record<string, ChatMessage[]>>;
  inputRequests: InputRequestChecker;
  scroll: ScrollActions;
  pinningSessionIds: PinningSessionIds;
  selectedProjectBySession: SelectedProjectBySession;
}) {
  const renamingSessionId = ref<string | null>(null);
  const renamingName = ref("");
  const renameInput = ref<HTMLInputElement | null>(null);

  function setRenameInput(el: Element | ComponentPublicInstance | null) {
    renameInput.value = el instanceof HTMLInputElement ? el : null;
  }

  function setRenamingName(value: string) {
    renamingName.value = value;
  }

  function startRename(session: ChatSessionMeta) {
    renamingSessionId.value = session.id;
    renamingName.value = session.name;
  }

  function cancelRename() {
    renamingSessionId.value = null;
    renamingName.value = "";
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
    } catch {
      // ignore
    }
    cancelRename();
  }

  function selectSession(sessionId: string) {
    if (!sessionId) return;
    chat.selectSession(sessionId);
    scroll.autoScrollEnabled.value = true;
    nextTick(() =>
      scroll.scrollMessagesToBottom({ force: true, behavior: "auto" }),
    );
  }

  async function createSession(name = "New Chat") {
    try {
      await chat.createSession(name);
      scroll.autoScrollEnabled.value = true;
      nextTick(() =>
        scroll.scrollMessagesToBottom({ force: true, behavior: "auto" }),
      );
    } catch {
      // readonly
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

  // Session message counts and awaiting-input state
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
            inputRequests.isInputRequestRespondable(request),
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

  // Watch renaming to focus input
  watch(renamingSessionId, (value) => {
    if (!value) return;
    nextTick(() => {
      const input = renameInput.value;
      if (!(input instanceof HTMLInputElement)) return;
      input.focus();
      input.select();
    });
  });

  // Prune pinning state when sessions change
  watch(
    sessions,
    (next) => {
      const keep = new Set(next.map((s) => s.id));
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

      // Also prune selectedProjectBySession
      const projectCurrent = selectedProjectBySession.value;
      let projectChanged = false;
      const projectPruned: Record<string, string> = {};
      for (const session of next) {
        if (!Object.prototype.hasOwnProperty.call(projectCurrent, session.id))
          continue;
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
    },
    { immediate: true },
  );

  return {
    renamingSessionId,
    renamingName,
    renameInput,
    setRenameInput,
    setRenamingName,
    startRename,
    cancelRename,
    commitRename,
    selectSession,
    createSession,
    sessionPinPending,
    toggleSessionPinned,
    sessionMessageCounts,
    sessionsAwaitingInput,
    messageCountFor,
    sessionAwaitingInput,
    sessionIsStreaming,
    conversationOptions,
  };
}
