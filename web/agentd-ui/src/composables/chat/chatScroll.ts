import { nextTick, ref } from "vue";
import type { ComponentPublicInstance } from "vue";

type RefValue<T> = { value: T };

type ScrollToBottomOptions = {
  force?: boolean;
  behavior?: ScrollBehavior;
};

export function useChatScroll({
  activeSessionId,
  hasOlderMessages,
  olderMessagesLoading,
  loadOlderMessages,
  scrollLockThreshold = 80,
  loadOlderScrollThreshold = 96,
}: {
  activeSessionId: RefValue<string>;
  hasOlderMessages: RefValue<boolean>;
  olderMessagesLoading: RefValue<boolean>;
  loadOlderMessages: (sessionId: string) => Promise<unknown>;
  scrollLockThreshold?: number;
  loadOlderScrollThreshold?: number;
}) {
  const messagesPane = ref<HTMLDivElement | null>(null);
  const participantActivityPane = ref<HTMLElement | null>(null);
  const autoScrollEnabled = ref(true);
  const lastScrollTop = ref(0);
  const activityAutoScrollEnabled = ref(true);
  const activityLastScrollTop = ref(0);
  const threadBodyEls = new Map<string, HTMLElement>();
  const threadScrollEnabled = new Map<string, boolean>();
  const threadScrollLastTop = new Map<string, number>();

  function setMessagesPaneRef(el: Element | ComponentPublicInstance | null) {
    messagesPane.value = el as HTMLDivElement | null;
  }

  function setParticipantActivityPaneRef(
    el: Element | ComponentPublicInstance | null,
  ) {
    participantActivityPane.value = el as HTMLElement | null;
  }

  function scrollPaneToBottom(
    container: HTMLElement | null,
    enabledRef: RefValue<boolean>,
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

  async function preserveMessagesScrollWhileLoadingOlder() {
    const sessionId = activeSessionId.value;
    if (!sessionId || !hasOlderMessages.value || olderMessagesLoading.value) {
      return;
    }
    const container = messagesPane.value;
    const previousScrollHeight = container?.scrollHeight ?? 0;
    const previousScrollTop = container?.scrollTop ?? 0;
    await loadOlderMessages(sessionId);
    await nextTick();
    if (!container) return;
    const delta = container.scrollHeight - previousScrollHeight;
    container.scrollTop = previousScrollTop + Math.max(delta, 0);
    lastScrollTop.value = container.scrollTop;
  }

  function scrollActivityPaneToBottom(options: ScrollToBottomOptions = {}) {
    nextTick(() => {
      scrollPaneToBottom(
        participantActivityPane.value,
        activityAutoScrollEnabled,
        options,
      );
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

  function scrollThreadBodyToBottom(
    threadId: string,
    options: ScrollToBottomOptions = {},
  ) {
    nextTick(() => {
      const el = threadBodyEls.get(threadId);
      if (!el) return;
      const enabledRef = {
        get value() {
          return threadScrollEnabled.get(threadId) ?? true;
        },
        set value(v: boolean) {
          threadScrollEnabled.set(threadId, v);
        },
      };
      scrollPaneToBottom(el, enabledRef, options);
    });
  }

  function handleThreadBodyScroll(event: Event, threadId: string) {
    const enabledRef = {
      get value() {
        return threadScrollEnabled.get(threadId) ?? true;
      },
      set value(v: boolean) {
        threadScrollEnabled.set(threadId, v);
      },
    };
    const lastTopRef = {
      get value() {
        return threadScrollLastTop.get(threadId) ?? 0;
      },
      set value(v: number) {
        threadScrollLastTop.set(threadId, v);
      },
    };
    handlePaneScroll(event, enabledRef, lastTopRef);
  }

  function isNearBottom(container: HTMLElement) {
    const distance =
      container.scrollHeight - (container.scrollTop + container.clientHeight);
    return distance <= scrollLockThreshold;
  }

  function handleMessagesScroll(event: Event) {
    const container = event.target as HTMLElement | null;
    const nearTop =
      !!container && container.scrollTop <= loadOlderScrollThreshold;
    handlePaneScroll(event, autoScrollEnabled, lastScrollTop);
    if (nearTop) void preserveMessagesScrollWhileLoadingOlder();
  }

  function handleActivityPaneScroll(event: Event) {
    handlePaneScroll(event, activityAutoScrollEnabled, activityLastScrollTop);
  }

  function handlePaneScroll(
    event: Event,
    enabledRef: RefValue<boolean>,
    lastTopRef: RefValue<number>,
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

  return {
    autoScrollEnabled,
    lastScrollTop,
    activityAutoScrollEnabled,
    activityLastScrollTop,
    setMessagesPaneRef,
    setParticipantActivityPaneRef,
    scrollMessagesToBottom,
    preserveMessagesScrollWhileLoadingOlder,
    scrollActivityPaneToBottom,
    registerThreadBody,
    scrollThreadBodyToBottom,
    handleThreadBodyScroll,
    handleMessagesScroll,
    handleActivityPaneScroll,
    handleScrollToLatest,
  };
}
