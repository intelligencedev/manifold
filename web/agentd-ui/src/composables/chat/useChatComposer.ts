import { computed, nextTick, ref } from "vue";
import type { ComponentPublicInstance } from "vue";
import type { Ref } from "vue";
import type { ChatAttachment, ChatMessage } from "@/types/chat";
import { resolveLeadingChatMention } from "@/utils/chatMentions";
import type { useChatResponseTimers } from "./useChatResponseTimers";
import type { useChatTargeting } from "./chatTargeting";
import type { useChatInputRequests } from "./chatInputRequests";

type ChatStoreActions = {
  sendPrompt: (
    content: string,
    attachments: ChatAttachment[],
    files: Map<string, File>,
    options: Record<string, unknown>,
  ) => Promise<void>;
  updateSessionProject: (
    sessionId: string,
    projectId: string,
  ) => Promise<unknown>;
  stopStreaming: () => void;
  regenerateAssistant: (options: Record<string, unknown>) => Promise<void>;
  resumeDurableRun: (
    sessionId: string,
    messageId: string,
    runId: string,
  ) => Promise<void>;
  deleteMessage: (sessionId: string, messageId: string) => Promise<void>;
};

type TargetingReturn = ReturnType<typeof useChatTargeting>;
type InputRequestsReturn = ReturnType<typeof useChatInputRequests>;
type ResponseTimersReturn = ReturnType<typeof useChatResponseTimers>;

type AgentContext = { agentName: string; agentModel: string };

export function useChatComposer({
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
  resolveAgentContext,
  selectedTeam,
  selectedSpecialist,
  chatMentionTargets,
  autoScrollEnabled,
}: {
  chat: ChatStoreActions;
  activeSessionId: Ref<string>;
  draft: Ref<string>;
  composer: Ref<HTMLTextAreaElement | null>;
  selectedProjectId: Ref<string>;
  projectSelected: Ref<boolean>;
  hasPendingInputRequest: Ref<boolean>;
  memoryEnabled: Ref<boolean>;
  imagePrompt: Ref<boolean>;
  renderMode: Ref<"markdown" | "html">;
  isStreaming: Ref<boolean>;
  activeMessages: Ref<ChatMessage[]>;
  targeting: TargetingReturn;
  inputRequests: InputRequestsReturn;
  responseTimers: ResponseTimersReturn;
  resolveAgentContext: () => AgentContext;
  selectedTeam: Ref<string>;
  selectedSpecialist: Ref<string>;
  chatMentionTargets: ReturnType<typeof useChatTargeting>["chatMentionTargets"];
  autoScrollEnabled: Ref<boolean>;
}) {
  const fileInput = ref<HTMLInputElement | null>(null);
  const pendingAttachments = ref<ChatAttachment[]>([]);
  const filesByAttachment: Map<string, File> = new Map();
  const copiedMessageId = ref<string | null>(null);
  const copiedThoughtSummaries = ref(false);

  const imageAttachments = computed(() =>
    pendingAttachments.value.filter((a) => a.kind === "image"),
  );
  const textAttachments = computed(() =>
    pendingAttachments.value.filter((a) => a.kind === "text"),
  );

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

  function validateFile(f: File): "image" | "text" | null {
    const type = (f.type || "").toLowerCase();
    if (type === "image/png" || type === "image/jpeg") return "image";
    if (type.startsWith("text/")) return "text";
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

  function autoSizeComposer() {
    const el = composer.value;
    if (!el) return;
    if (!draft.value || !draft.value.trim()) {
      el.style.height = "";
      return;
    }
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }

  function handleComposerKeydown(event: KeyboardEvent) {
    if (targeting.mentionMenuOpen.value) {
      if (event.key === "Escape") {
        event.preventDefault();
        targeting.closeMentionMenu();
        return;
      }
      if (event.key === "ArrowDown") {
        event.preventDefault();
        if (targeting.mentionCandidates.value.length) {
          targeting.mentionActiveIndex.value =
            (targeting.mentionActiveIndex.value + 1) %
            targeting.mentionCandidates.value.length;
        }
        return;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        if (targeting.mentionCandidates.value.length) {
          targeting.mentionActiveIndex.value =
            (targeting.mentionActiveIndex.value -
              1 +
              targeting.mentionCandidates.value.length) %
            targeting.mentionCandidates.value.length;
        }
        return;
      }
      if (event.key === "Enter" && !event.shiftKey && !event.isComposing) {
        event.preventDefault();
        const cand =
          targeting.mentionCandidates.value[targeting.mentionActiveIndex.value];
        if (cand) targeting.selectMentionCandidate(cand);
        return;
      }
      if (event.key === "Tab") {
        const cand =
          targeting.mentionCandidates.value[targeting.mentionActiveIndex.value];
        if (cand) {
          event.preventDefault();
          targeting.selectMentionCandidate(cand);
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
    targeting.updateMentionState();
  }

  function handleComposerKeyup() {
    targeting.updateMentionState();
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

    responseTimers.stopAllResponseTimers();

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

  function copyThoughtSummaries(
    selectedActivityThoughtSummaries: Ref<string[]>,
  ) {
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

  return {
    fileInput,
    pendingAttachments,
    imageAttachments,
    textAttachments,
    copiedMessageId,
    copiedThoughtSummaries,
    composerPlaceholder,
    autoSizeComposer,
    validateFile,
    addFiles,
    handleFileInputChange,
    handleDrop,
    removeAttachment,
    handleComposerKeydown,
    handleComposerInput,
    handleComposerKeyup,
    sendCurrentPrompt,
    sendPrompt,
    stopStreaming,
    canResumeDurableRun,
    regenerateAssistant,
    resumeDurableRun,
    copyMessage,
    deleteChatMessage,
    copyThoughtSummaries,
    setDraftValue,
    setImagePromptValue,
    setComposerRef,
    setFileInputRef,
    triggerFilePicker,
  };
}
