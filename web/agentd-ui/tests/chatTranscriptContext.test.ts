import { fireEvent, render, screen } from "@testing-library/vue";
import { describe, expect, it, vi } from "vitest";
import ChatTranscript from "@/components/chat/ChatTranscript.vue";
import type { ChatTranscriptModel } from "@/composables/chat/useChatViewController";

function transcriptModel(
  overrides: Partial<ChatTranscriptModel> = {},
): ChatTranscriptModel {
  return {
    displayUsername: "there",
    hasOlderMessages: false,
    olderMessagesLoading: false,
    olderMessagesError: "",
    chatMessages: [
      {
        id: "assistant-1",
        role: "assistant",
        content: "Done.",
        createdAt: "2026-07-07T12:00:00.000Z",
      },
    ],
    loadOlderMessages: vi.fn(),
    setMessagesPaneRef: vi.fn(),
    handleMessagesScroll: vi.fn(),
    handleMarkdownClick: vi.fn(),
    visibleParticipantActivityItemsForMessage: vi.fn(() => []),
    hasDelegatedActivityForMessage: vi.fn(() => false),
    isActivityCollapsed: vi.fn(() => true),
    expandActivity: vi.fn(),
    collapseActivity: vi.fn(),
    registerThreadBody: vi.fn(),
    handleThreadBodyScroll: vi.fn(),
    drawerBeforeEnter: vi.fn(),
    drawerEnter: vi.fn(),
    drawerAfterEnter: vi.fn(),
    drawerBeforeLeave: vi.fn(),
    drawerLeave: vi.fn(),
    renderMarkdownOrHtml: vi.fn((value: string) => value),
    inspectContext: vi.fn(),
    shouldShowDirectActivity: vi.fn(() => false),
    shouldShowDirectThought: vi.fn(() => false),
    agentNameFor: vi.fn(() => "orchestrator"),
    hasMemoryContext: vi.fn(() => false),
    isMemoryContextExpanded: vi.fn(() => false),
    expandMemoryContext: vi.fn(),
    collapseMemoryContext: vi.fn(),
    memoryContextPillMeta: vi.fn(() => ""),
    inputRequestCardClasses: vi.fn(() => ""),
    inputRequestStatusLabel: vi.fn(() => ""),
    submitInputRequest: vi.fn(),
    inputRequestChoiceSelected: vi.fn(() => false),
    toggleInputRequestChoice: vi.fn(),
    isInputRequestSubmitting: vi.fn(() => false),
    inputRequestLocalError: vi.fn(() => ""),
    inputRequestKey: vi.fn(() => ""),
    inputRequestFieldName: vi.fn(() => ""),
    inputRequestDraft: vi.fn(() => ""),
    setInputRequestDraft: vi.fn(),
    isInputRequestRespondable: vi.fn(() => false),
    canSubmitInputRequest: vi.fn(() => false),
    inputRequestAnswerSummary: vi.fn(() => ""),
    openImageModal: vi.fn(),
    canResumeDurableRun: vi.fn(() => false),
    resumeDurableRun: vi.fn(),
    copiedMessageId: "",
    copyMessage: vi.fn(),
    regenerateAssistant: vi.fn(),
    deleteChatMessage: vi.fn(),
    isStreaming: false,
    labelForRole: vi.fn((role: string) => role),
    shouldShowResponseTimer: vi.fn(() => false),
    responseElapsedMs: vi.fn(() => 0),
    formatDuration: vi.fn(() => "0s"),
    showScrollToBottom: false,
    handleScrollToLatest: vi.fn(),
    ...overrides,
  } as unknown as ChatTranscriptModel;
}

describe("ChatTranscript Context Inspector affordance", () => {
  it("shows a visible context button for completed assistant messages", async () => {
    const inspectContext = vi.fn();
    render(ChatTranscript, {
      props: {
        model: transcriptModel({ inspectContext }),
      },
    });

    const button = screen.getByRole("button", { name: /inspect context/i });
    expect(button).toBeEnabled();

    await fireEvent.click(button);

    expect(inspectContext).toHaveBeenCalledWith("assistant-1");
  });
});
