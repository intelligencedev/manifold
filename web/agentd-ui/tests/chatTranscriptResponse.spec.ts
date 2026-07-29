import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import ChatTranscript from "@/components/chat/ChatTranscript.vue";

describe("ChatTranscript response activity", () => {
  it("renders streamed text and tool calls in event order with sticky activity", () => {
    const message = {
      id: "assistant-1",
      role: "assistant" as const,
      content: "Before tool.After tool.",
      createdAt: "2026-07-20T00:00:00.000Z",
      activityThoughtSummary: "Inspecting the relevant implementation.",
      responseParts: [
        { id: "text-0", type: "text" as const, content: "Before tool." },
        {
          id: "tool-1",
          type: "tool" as const,
          title: "Search repository",
          status: "done" as const,
          args: '{"query":"chat"}',
          result: "3 matches",
        },
        { id: "text-2", type: "text" as const, content: "After tool." },
      ],
    };
    const wrapper = mount(ChatTranscript, {
      props: {
        model: {
          displayUsername: "User",
          hasOlderMessages: false,
          olderMessagesLoading: false,
          olderMessagesError: "",
          chatMessages: [message],
          loadOlderMessages: vi.fn(),
          setMessagesPaneRef: vi.fn(),
          handleMessagesScroll: vi.fn(),
          handleMarkdownClick: vi.fn(),
          hasDelegatedActivityForMessage: () => false,
          shouldShowDirectActivity: () => true,
          shouldShowDirectThought: () => true,
          isActivityCollapsed: () => false,
          expandActivity: vi.fn(),
          collapseActivity: vi.fn(),
          agentNameFor: () => "Orchestrator",
          hasMemoryContext: () => false,
          shouldShowResponseTimer: () => false,
          renderMarkdownOrHtml: (content: string) => `<p>${content}</p>`,
          canResumeDurableRun: () => false,
          copiedMessageId: null,
          copyMessage: vi.fn(),
          regenerateAssistant: vi.fn(),
          deleteChatMessage: vi.fn(),
          isStreaming: false,
          showScrollToBottom: false,
          handleScrollToLatest: vi.fn(),
        } as never,
      },
    });

    const parts = wrapper.findAll(".response-text-part, .inline-tool-call");
    expect(parts).toHaveLength(3);
    expect(parts[0].text()).toBe("Before tool.");
    expect(parts[1].attributes("aria-label")).toBe(
      "Tool call: Search repository",
    );
    expect(parts[1].text()).toBe("Search repository");
    expect(parts[1].find("details").exists()).toBe(false);
    expect(parts[2].text()).toBe("After tool.");
    expect(wrapper.text().match(/Search repository/g)).toHaveLength(1);
    expect(wrapper.find(".direct-activity-wrapper--sticky").exists()).toBe(
      true,
    );
  });

  it("leaves pending approval requests for the focused approval surface", () => {
    const message = {
      id: "assistant-approval",
      role: "assistant" as const,
      content: "Before approval.After approval.",
      createdAt: "2026-07-20T00:00:00.000Z",
      inputRequests: [
        {
          id: "approval-1",
          question: "Allow the command?",
          choices: [{ id: "approve", label: "Approve" }],
          allowFreeText: false,
          multiple: false,
          status: "pending" as const,
          createdAt: "2026-07-20T00:00:01.000Z",
        },
      ],
      responseParts: [
        { id: "text-0", type: "text" as const, content: "Before approval." },
        {
          id: "input-request-approval-1",
          type: "input_request" as const,
          requestId: "approval-1",
        },
        { id: "text-2", type: "text" as const, content: "After approval." },
      ],
    };
    const wrapper = mount(ChatTranscript, {
      props: {
        model: {
          displayUsername: "User",
          hasOlderMessages: false,
          olderMessagesLoading: false,
          olderMessagesError: "",
          chatMessages: [message],
          loadOlderMessages: vi.fn(),
          setMessagesPaneRef: vi.fn(),
          handleMessagesScroll: vi.fn(),
          handleMarkdownClick: vi.fn(),
          hasDelegatedActivityForMessage: () => false,
          shouldShowDirectActivity: () => false,
          hasMemoryContext: () => false,
          shouldShowResponseTimer: () => false,
          agentNameFor: () => "Orchestrator",
          renderMarkdownOrHtml: (content: string) => `<p>${content}</p>`,
          inputRequestCardClasses: () => ({}),
          inputRequestStatusLabel: () => "Response required",
          submitInputRequest: vi.fn(),
          inputRequestChoiceSelected: () => false,
          toggleInputRequestChoice: vi.fn(),
          isInputRequestSubmitting: () => false,
          inputRequestLocalError: () => "",
          inputRequestFieldName: () => "approval-1",
          inputRequestDraft: () => "",
          setInputRequestDraft: vi.fn(),
          isInputRequestRespondable: () => true,
          canSubmitInputRequest: () => false,
          inputRequestAnswerSummary: () => "",
          canResumeDurableRun: () => false,
          copiedMessageId: null,
          copyMessage: vi.fn(),
          regenerateAssistant: vi.fn(),
          deleteChatMessage: vi.fn(),
          isStreaming: true,
          showScrollToBottom: false,
          handleScrollToLatest: vi.fn(),
        } as never,
      },
    });

    const parts = wrapper.findAll(
      ".response-text-part, .input-request-card, .inline-tool-call",
    );
    expect(parts).toHaveLength(2);
    expect(parts[0].text()).toBe("Before approval.");
    expect(parts[1].text()).toBe("After approval.");
    expect(wrapper.findAll(".input-request-card")).toHaveLength(0);
  });
});
