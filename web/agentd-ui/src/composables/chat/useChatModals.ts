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
  };
}
