import { computed, ref } from "vue";
import type { Ref } from "vue";
import type { ChatAttachment, ChatSessionMeta } from "@/types/chat";

export function useChatModals({
  activeSessionId,
  scrollMessagesToBottom,
  autoScrollEnabled,
}: {
  activeSessionId: Ref<string>;
  scrollMessagesToBottom: (options?: {
    force?: boolean;
    behavior?: ScrollBehavior;
  }) => void;
  autoScrollEnabled: Ref<boolean>;
}) {
  // Image modal state
  const showImageModal = ref(false);
  const modalImage = ref<ChatAttachment | null>(null);
  const modalImageSrc = computed(() => {
    const img = modalImage.value;
    if (!img) return "";
    return img.previewUrl || img.path || "";
  });

  function openImageModal(img: ChatAttachment) {
    modalImage.value = img;
    showImageModal.value = true;
  }

  function closeImageModal() {
    showImageModal.value = false;
    modalImage.value = null;
  }

  // Delete session dialog state
  const showDeleteSessionDialog = ref(false);
  const deleteSessionTarget = ref<ChatSessionMeta | null>(null);
  const deleteSessionPending = ref(false);
  const deleteSessionError = ref("");
  const canConfirmDeleteSession = computed(
    () => !!deleteSessionTarget.value?.id && !deleteSessionPending.value,
  );

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

  async function confirmDeleteSession(
    deleteSession: (sessionId: string) => Promise<void>,
  ) {
    const sessionId = deleteSessionTarget.value?.id;
    if (!sessionId || !canConfirmDeleteSession.value) return;
    deleteSessionPending.value = true;
    deleteSessionError.value = "";
    try {
      await deleteSession(sessionId);
      showDeleteSessionDialog.value = false;
      resetDeleteSessionDialogState();
      autoScrollEnabled.value = true;
      scrollMessagesToBottom({ force: true, behavior: "auto" });
    } catch {
      deleteSessionError.value = "Failed to delete conversation.";
    }
    deleteSessionPending.value = false;
  }

  // Allow-all-commands confirm dialog state
  const showAllowAllDialog = ref(false);
  const allowAllPending = ref(false);
  const allowAllError = ref("");
  const canConfirmAllowAll = computed(
    () => !!activeSessionId.value && !allowAllPending.value,
  );

  function openAllowAllDialog() {
    if (!activeSessionId.value) return;
    allowAllPending.value = false;
    allowAllError.value = "";
    showAllowAllDialog.value = true;
  }

  function closeAllowAllDialog() {
    if (allowAllPending.value) return;
    showAllowAllDialog.value = false;
    allowAllError.value = "";
  }

  async function confirmAllowAll(
    enableAllowAll: (sessionId: string) => Promise<void>,
  ) {
    const sessionId = activeSessionId.value;
    if (!sessionId || !canConfirmAllowAll.value) return;
    allowAllPending.value = true;
    allowAllError.value = "";
    try {
      await enableAllowAll(sessionId);
      showAllowAllDialog.value = false;
    } catch {
      allowAllError.value = "Could not allow all commands for this session.";
    }
    allowAllPending.value = false;
  }

  // Bulk delete session dialog state
  const showBulkDeleteSessionDialog = ref(false);
  const bulkDeleteSessionIds = ref<string[]>([]);
  const bulkDeleteSessionPending = ref(false);
  const bulkDeleteSessionError = ref("");
  const bulkDeleteSessionCount = computed(
    () => bulkDeleteSessionIds.value.length,
  );
  const canConfirmBulkDeleteSession = computed(
    () =>
      bulkDeleteSessionIds.value.length > 0 && !bulkDeleteSessionPending.value,
  );

  function openBulkDeleteSessionDialog(ids: string[]) {
    if (!ids.length) return;
    bulkDeleteSessionIds.value = ids;
    bulkDeleteSessionPending.value = false;
    bulkDeleteSessionError.value = "";
    showBulkDeleteSessionDialog.value = true;
  }

  function closeBulkDeleteSessionDialog() {
    if (bulkDeleteSessionPending.value) return;
    showBulkDeleteSessionDialog.value = false;
    bulkDeleteSessionIds.value = [];
    bulkDeleteSessionPending.value = false;
    bulkDeleteSessionError.value = "";
  }

  async function confirmBulkDeleteSession(
    deleteSession: (sessionId: string) => Promise<void>,
  ): Promise<boolean> {
    const ids = bulkDeleteSessionIds.value;
    if (!ids.length || !canConfirmBulkDeleteSession.value) return false;
    bulkDeleteSessionPending.value = true;
    bulkDeleteSessionError.value = "";
    let failedCount = 0;
    for (const id of ids) {
      try {
        await deleteSession(id);
      } catch {
        failedCount++;
      }
    }
    if (failedCount > 0) {
      bulkDeleteSessionError.value = `Failed to delete ${failedCount} of ${ids.length} conversations.`;
    } else {
      showBulkDeleteSessionDialog.value = false;
      bulkDeleteSessionIds.value = [];
      autoScrollEnabled.value = true;
      scrollMessagesToBottom({ force: true, behavior: "auto" });
    }
    bulkDeleteSessionPending.value = false;
    return failedCount === 0;
  }

  return {
    showImageModal,
    modalImage,
    modalImageSrc,
    openImageModal,
    closeImageModal,
    showDeleteSessionDialog,
    deleteSessionTarget,
    deleteSessionPending,
    deleteSessionError,
    canConfirmDeleteSession,
    openDeleteSessionDialog,
    closeDeleteSessionDialog,
    confirmDeleteSession,
    showAllowAllDialog,
    allowAllPending,
    allowAllError,
    canConfirmAllowAll,
    openAllowAllDialog,
    closeAllowAllDialog,
    confirmAllowAll,
    showBulkDeleteSessionDialog,
    bulkDeleteSessionIds,
    bulkDeleteSessionPending,
    bulkDeleteSessionError,
    bulkDeleteSessionCount,
    canConfirmBulkDeleteSession,
    openBulkDeleteSessionDialog,
    closeBulkDeleteSessionDialog,
    confirmBulkDeleteSession,
  };
}
