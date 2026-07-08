import { ref } from "vue";
import {
  fetchChatLLMRequests,
  fetchLLMRequestContext,
} from "@/api/chat";
import type {
  ChatLLMRequestContext,
  ChatLLMRequestSummary,
} from "@/types/chat";

export function useContextInspector() {
  const open = ref(false);
  const sessionId = ref("");
  const messageId = ref("");
  const requests = ref<ChatLLMRequestSummary[]>([]);
  const selectedRequestId = ref("");
  const selectedContext = ref<ChatLLMRequestContext | null>(null);
  const loading = ref(false);
  const contextLoading = ref(false);
  const error = ref("");

  async function inspect(nextSessionId: string, nextMessageId: string) {
    sessionId.value = nextSessionId;
    messageId.value = nextMessageId;
    open.value = true;
    loading.value = true;
    error.value = "";
    requests.value = [];
    selectedRequestId.value = "";
    selectedContext.value = null;
    try {
      const list = await fetchChatLLMRequests(nextSessionId, nextMessageId);
      requests.value = list;
      if (list.length) {
        await selectRequest(list[0].id);
      }
    } catch (err) {
      error.value = errorMessage(err, "Failed to load LLM requests.");
    } finally {
      loading.value = false;
    }
  }

  async function selectRequest(requestId: string) {
    if (!requestId || selectedRequestId.value === requestId) return;
    selectedRequestId.value = requestId;
    selectedContext.value = null;
    contextLoading.value = true;
    error.value = "";
    try {
      selectedContext.value = await fetchLLMRequestContext(requestId);
    } catch (err) {
      error.value = errorMessage(err, "Failed to load request context.");
    } finally {
      contextLoading.value = false;
    }
  }

  function close() {
    open.value = false;
  }

  return {
    open,
    sessionId,
    messageId,
    requests,
    selectedRequestId,
    selectedContext,
    loading,
    contextLoading,
    error,
    inspect,
    selectRequest,
    close,
  };
}

function errorMessage(err: unknown, fallback: string) {
  if (err && typeof err === "object" && "message" in err) {
    const msg = String((err as { message?: unknown }).message || "").trim();
    if (msg) return msg;
  }
  return fallback;
}
