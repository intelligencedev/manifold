import { ref } from "vue";
import type { ChatMessage } from "@/types/chat";

export function useChatResponseTimers(isBrowser: boolean) {
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

  return {
    responseElapsedMsByMessageId,
    responseElapsedMs,
    formatDuration,
    ensureResponseTimer,
    pauseResponseTimer,
    finalizeResponseTimer,
    stopAllResponseTimers,
    shouldShowResponseTimer,
    persistedResponseDurationMs,
  };
}
